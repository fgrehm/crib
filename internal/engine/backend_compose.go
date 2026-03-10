package engine

import (
	"context"
	"fmt"
	"path/filepath"

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
	// Determine the override image: use stored feature image or snapshot.
	var overrideImage string
	if stored, err := b.e.store.LoadResult(b.ws.ID); err == nil && stored != nil {
		overrideImage = stored.ImageName
	}
	if img, ok := b.e.validSnapshot(ctx, b.ws, b.cfg); ok {
		overrideImage = img
	}

	if _, err := b.e.generateComposeOverride(b.ws, b.cfg, b.workspaceFolder, b.inv.files, overrideImage, pluginResp); err != nil {
		b.e.logger.Warn("failed to regenerate compose override", "error", err)
	}

	overridePath := filepath.Join(b.e.store.WorkspaceDir(b.ws.ID), "compose-override.yml")
	allFiles := append(b.inv.files[:len(b.inv.files):len(b.inv.files)], overridePath)

	b.e.reportProgress("Starting services...")
	if err := b.e.compose.Start(ctx, b.inv.projectName, allFiles, b.e.composeStdout(), b.e.composeStderr(), b.inv.env); err != nil {
		return "", fmt.Errorf("starting compose services: %w", err)
	}

	container, err := b.e.findComposeContainer(ctx, b.ws.ID, b.inv, "after start")
	if err != nil {
		return "", err
	}
	if err := b.e.ensureContainerRunning(ctx, b.ws.ID, container); err != nil {
		return "", err
	}

	return container.ID, nil
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
				b.e.reportProgress("Building services...")
				if err := b.e.compose.Build(ctx, b.inv.projectName, allFiles, others, b.e.stdout, b.e.stderr, b.inv.env); err != nil {
					return "", fmt.Errorf("building compose services: %w", err)
				}
			}
		} else {
			b.e.reportProgress("Building services...")
			if err := b.e.compose.Build(ctx, b.inv.projectName, allFiles, nil, b.e.stdout, b.e.stderr, b.inv.env); err != nil {
				return "", fmt.Errorf("building compose services: %w", err)
			}
		}
	}

	b.e.reportProgress("Starting services...")
	if err := b.e.compose.Up(ctx, b.inv.projectName, allFiles, services, b.e.composeStdout(), b.e.composeStderr(), b.inv.env); err != nil {
		return "", fmt.Errorf("starting compose services: %w", err)
	}

	container, err := b.e.findComposeContainer(ctx, b.ws.ID, b.inv, "after up")
	if err != nil {
		return "", err
	}
	if err := b.e.ensureContainerRunning(ctx, b.ws.ID, container); err != nil {
		return "", err
	}

	return container.ID, nil
}

func (b *composeBackend) deleteExisting(ctx context.Context) error {
	return b.e.composeDown(ctx, b.inv, false)
}

func (b *composeBackend) restart(ctx context.Context, containerID string, pluginResp *plugin.PreContainerRunResponse) (string, error) {
	// Regenerate the override so it stays current.
	overrideImage := ""
	if stored, err := b.e.store.LoadResult(b.ws.ID); err == nil && stored != nil {
		overrideImage = stored.ImageName
	}
	if img, ok := b.e.validSnapshot(ctx, b.ws, b.cfg); ok {
		overrideImage = img
	}

	if _, err := b.e.generateComposeOverride(b.ws, b.cfg, b.workspaceFolder, b.inv.files, overrideImage, pluginResp); err != nil {
		b.e.logger.Warn("failed to regenerate compose override", "error", err)
	}

	overridePath := filepath.Join(b.e.store.WorkspaceDir(b.ws.ID), "compose-override.yml")
	allFiles := append(b.inv.files[:len(b.inv.files):len(b.inv.files)], overridePath)

	b.e.reportProgress("Stopping services...")
	if err := b.e.compose.Stop(ctx, b.inv.projectName, allFiles, b.e.composeStdout(), b.e.composeStderr(), b.inv.env); err != nil {
		b.e.logger.Warn("failed to stop services, proceeding with start", "error", err)
	}

	b.e.reportProgress("Starting services...")
	if err := b.e.compose.Start(ctx, b.inv.projectName, allFiles, b.e.composeStdout(), b.e.composeStderr(), b.inv.env); err != nil {
		return "", fmt.Errorf("starting compose services: %w", err)
	}

	container, err := b.e.findComposeContainer(ctx, b.ws.ID, b.inv, "after restart")
	if err != nil {
		return "", err
	}

	return container.ID, nil
}

func (b *composeBackend) canResumeFromStored() bool {
	return true
}
