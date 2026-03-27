package engine

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// composeBackend implements containerBackend for compose workspaces.
type composeBackend struct {
	e               *Engine
	ws              *workspace.Workspace
	cfg             *config.DevContainerConfig
	workspaceFolder string
	inv             composeInvocation
}

func (b *composeBackend) pluginUser(ctx context.Context) string {
	return b.e.resolveComposeUser(ctx, b.cfg, b.inv.files)
}

func (b *composeBackend) start(ctx context.Context, containerID string, pluginResp *plugin.PreContainerRunResponse) (string, error) {
	allFiles := b.prepareOverride(ctx, pluginResp)

	var stderrBuf bytes.Buffer
	b.e.reportProgress(PhaseCreate, "Starting services...")
	if err := b.e.compose.Start(ctx, b.inv.projectName, allFiles, b.e.composeStdout(), b.e.composeStderrTee(&stderrBuf), b.inv.env); err != nil {
		return "", fmt.Errorf("starting compose services: %w", err)
	}

	return b.findRunningContainer(ctx, "after start", stderrBuf.String())
}

func (b *composeBackend) buildImage(ctx context.Context) (*buildResult, error) {
	if len(b.cfg.Features) == 0 {
		return &buildResult{}, nil
	}
	return b.e.buildComposeFeatures(ctx, b.ws, b.cfg, b.inv)
}

func (b *composeBackend) createContainer(ctx context.Context, opts createOpts) (string, error) {
	// Resolve feature metadata for the override (capabilities, entrypoints).
	var fmeta []*config.ImageMetadata
	if opts.metadata != nil {
		fmeta = opts.metadata
	} else if opts.imageName != "" {
		// When creating from stored/snapshot, resolve from config.
		fmeta = b.e.resolveFeatureMetadata(b.cfg)
	}

	overridePath, err := b.e.generateComposeOverride(b.ws, b.cfg, b.workspaceFolder, b.inv.files, opts.imageName, opts.pluginResp, fmeta...)
	if err != nil {
		return "", fmt.Errorf("generating compose override: %w", err)
	}

	allFiles := append(b.inv.files[:len(b.inv.files):len(b.inv.files)], overridePath)
	services := ensureServiceIncluded(b.cfg.RunServices, b.cfg.Service)

	if !opts.skipBuild {
		// Build services. When the primary service image was already built
		// (feature image), skip building it and only build others.
		if opts.imageName != "" {
			others := removeService(services, b.cfg.Service)
			if len(others) > 0 {
				b.e.reportProgress(PhaseBuild, "Building services...")
				if err := b.e.compose.Build(ctx, b.inv.projectName, allFiles, others, b.e.stdout, b.e.stderr, b.inv.env); err != nil {
					return "", fmt.Errorf("building compose services: %w", err)
				}
			}
		} else {
			b.e.reportProgress(PhaseBuild, "Building services...")
			if err := b.e.compose.Build(ctx, b.inv.projectName, allFiles, nil, b.e.stdout, b.e.stderr, b.inv.env); err != nil {
				return "", fmt.Errorf("building compose services: %w", err)
			}
		}
	}

	var stderrBuf bytes.Buffer
	b.e.reportProgress(PhaseCreate, "Starting services...")
	if err := b.e.compose.Up(ctx, b.inv.projectName, allFiles, services, b.e.composeStdout(), b.e.composeStderrTee(&stderrBuf), b.inv.env); err != nil {
		return "", fmt.Errorf("starting compose services: %w", err)
	}

	return b.findRunningContainer(ctx, "after up", stderrBuf.String())
}

func (b *composeBackend) deleteExisting(ctx context.Context) error {
	return b.e.composeDown(ctx, b.inv, false)
}

func (b *composeBackend) restart(ctx context.Context, containerID string, pluginResp *plugin.PreContainerRunResponse) (string, error) {
	allFiles := b.prepareOverride(ctx, pluginResp)

	b.e.reportProgress(PhaseRestart, "Stopping services...")
	if err := b.e.compose.Stop(ctx, b.inv.projectName, allFiles, b.e.composeStdout(), b.e.composeStderr(), b.inv.env); err != nil {
		b.e.logger.Warn("failed to stop services, proceeding with start", "error", err)
	}

	var stderrBuf bytes.Buffer
	b.e.reportProgress(PhaseRestart, "Starting services...")
	if err := b.e.compose.Start(ctx, b.inv.projectName, allFiles, b.e.composeStdout(), b.e.composeStderrTee(&stderrBuf), b.inv.env); err != nil {
		return "", fmt.Errorf("starting compose services: %w", err)
	}

	return b.findRunningContainer(ctx, "after restart", stderrBuf.String())
}

func (b *composeBackend) canResumeFromStored() bool {
	return true
}

// prepareOverride resolves the override image, regenerates the compose override
// file, and returns the full compose file list including the override.
// Used by start() and restart() where override generation failures are
// non-fatal (the stale override file on disk is used as fallback).
func (b *composeBackend) prepareOverride(ctx context.Context, pluginResp *plugin.PreContainerRunResponse) []string {
	overrideImage := ""
	if stored, err := b.e.store.LoadResult(b.ws.ID); err == nil && stored != nil {
		overrideImage = stored.ImageName
	}
	if img, ok := b.e.validSnapshot(ctx, b.ws, b.cfg); ok {
		overrideImage = img
	}

	fmeta := b.e.resolveFeatureMetadata(b.cfg)

	if _, err := b.e.generateComposeOverride(b.ws, b.cfg, b.workspaceFolder, b.inv.files, overrideImage, pluginResp, fmeta...); err != nil {
		b.e.logger.Warn("failed to regenerate compose override", "error", err)
	}

	overridePath := filepath.Join(b.e.store.WorkspaceDir(b.ws.ID), "compose-override.yml")
	return append(b.inv.files[:len(b.inv.files):len(b.inv.files)], overridePath)
}

// findRunningContainer locates the primary service container and verifies
// it is running. composeOutput is included in the error message when the
// container cannot be found, providing diagnostics that would otherwise be
// lost when compose stderr is discarded in non-verbose mode.
func (b *composeBackend) findRunningContainer(ctx context.Context, stage, composeOutput string) (string, error) {
	container, err := b.e.findComposeContainer(ctx, b.ws.ID, b.inv, stage)
	if err != nil {
		if hint := strings.TrimSpace(composeOutput); hint != "" {
			return "", fmt.Errorf("%w\ncompose output:\n%s", err, hint)
		}
		return "", err
	}
	if err := b.e.ensureContainerRunning(ctx, b.ws.ID, container); err != nil {
		return "", err
	}
	return container.ID, nil
}
