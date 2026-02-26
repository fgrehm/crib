package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/workspace"
)

// defaultEntrypoint is used when overrideCommand is not explicitly false.
const defaultEntrypoint = "/bin/sh"

// defaultCmd keeps the container alive when overrideCommand is not false.
var defaultCmd = []string{"-c", "echo Container started; trap \"exit 0\" 15; exec \"$@\"; sleep infinity"}

// upSingle handles the single container path (image or Dockerfile based).
func (e *Engine) upSingle(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, opts UpOptions) (*UpResult, error) {
	// Check for an existing container.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding container: %w", err)
	}

	if container != nil && !opts.Recreate {
		// Container exists and we're not forcing recreation.
		if !container.State.IsRunning() {
			e.reportProgress("Starting container...")
			if err := e.driver.StartContainer(ctx, ws.ID, container.ID); err != nil {
				return nil, fmt.Errorf("starting container: %w", err)
			}
		} else {
			e.reportProgress("Container already running")
		}

		return e.setupAndReturn(ctx, ws, cfg, container.ID, workspaceFolder)
	}

	// Remove existing container if recreating.
	if container != nil && opts.Recreate {
		e.reportProgress("Removing container...")
		if err := e.store.ClearHookMarkers(ws.ID); err != nil {
			e.logger.Warn("failed to clear hook markers", "error", err)
		}
		if err := e.driver.DeleteContainer(ctx, ws.ID, container.ID); err != nil {
			return nil, fmt.Errorf("deleting container for recreation: %w", err)
		}
	}

	// Build the image.
	buildRes, err := e.buildImage(ctx, ws, cfg)
	if err != nil {
		return nil, err
	}

	// Build run options.
	runOpts := e.buildRunOptions(cfg, buildRes.imageName, ws.Source, workspaceFolder)

	e.reportProgress("Creating container...")
	if err := e.driver.RunContainer(ctx, ws.ID, runOpts); err != nil {
		return nil, fmt.Errorf("creating container: %w", err)
	}

	// Find the newly created container.
	container, err = e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding new container: %w", err)
	}
	if container == nil {
		return nil, fmt.Errorf("container not found after creation")
	}

	return e.setupAndReturn(ctx, ws, cfg, container.ID, workspaceFolder)
}

// buildRunOptions constructs RunOptions from the devcontainer config.
func (e *Engine) buildRunOptions(cfg *config.DevContainerConfig, imageName, projectRoot, workspaceFolder string) *driver.RunOptions {
	opts := &driver.RunOptions{
		Image:  imageName,
		Labels: make(map[string]string),
	}

	// User.
	if cfg.ContainerUser != "" {
		opts.User = cfg.ContainerUser
	}

	// Entrypoint and command.
	overrideCommand := cfg.OverrideCommand == nil || *cfg.OverrideCommand
	if overrideCommand {
		opts.Entrypoint = defaultEntrypoint
		opts.Cmd = defaultCmd
	}

	// Environment variables.
	for k, v := range cfg.ContainerEnv {
		opts.Env = append(opts.Env, k+"="+v)
	}

	// Init process.
	if cfg.Init != nil && *cfg.Init {
		opts.Init = true
	}

	// Privileged mode.
	if cfg.Privileged != nil && *cfg.Privileged {
		opts.Privileged = true
	}

	// Capabilities.
	opts.CapAdd = cfg.CapAdd

	// Security options.
	opts.SecurityOpt = cfg.SecurityOpt

	// Workspace mount.
	if cfg.WorkspaceMount != "" {
		opts.WorkspaceMount = config.ParseMount(cfg.WorkspaceMount)
	} else {
		// Default workspace mount: bind the project root to the workspace folder.
		opts.WorkspaceMount = config.Mount{
			Type:   "bind",
			Source: projectRoot,
			Target: workspaceFolder,
		}
	}

	// Additional mounts.
	opts.Mounts = cfg.Mounts

	// Passthrough CLI args from runArgs.
	opts.ExtraArgs = cfg.RunArgs

	return opts
}

// setupAndReturn runs container setup and returns the result.
// On lifecycle hook failure, both the result and error are returned so
// callers can persist the result (container is still usable).
func (e *Engine) setupAndReturn(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, containerID, workspaceFolder string) (*UpResult, error) {
	remoteUser := e.resolveRemoteUser(ctx, ws.ID, cfg, containerID)

	result := &UpResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	}

	// Save an early result so crib exec/shell can find the container,
	// workspace folder, and user while setup (UID sync, env probe,
	// lifecycle hooks) is still running.
	e.saveResult(ws, cfg, result)

	// Run container setup (UID sync, env probe, lifecycle hooks).
	if err := e.setupContainer(ctx, ws, cfg, containerID, workspaceFolder, remoteUser); err != nil {
		return result, fmt.Errorf("setting up container: %w", err)
	}

	return result, nil
}

// detectContainerUser runs whoami inside the container to detect the default
// user. Returns empty string on failure or if the user is root.
func (e *Engine) detectContainerUser(ctx context.Context, workspaceID, containerID string) string {
	var stdout bytes.Buffer
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, []string{"whoami"}, nil, &stdout, io.Discard, nil, ""); err != nil {
		return ""
	}
	user := strings.TrimSpace(stdout.String())
	if user == "root" {
		return ""
	}
	return user
}

// resolveWorkspaceFolder determines the workspace folder path inside the container.
func resolveWorkspaceFolder(cfg *config.DevContainerConfig, projectRoot string) string {
	if cfg.WorkspaceFolder != "" {
		return cfg.WorkspaceFolder
	}
	return "/workspaces/" + filepath.Base(projectRoot)
}
