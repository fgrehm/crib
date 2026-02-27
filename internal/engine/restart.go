package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/workspace"
)

// RestartResult holds the outcome of a Restart operation.
type RestartResult struct {
	// ContainerID is the container ID.
	ContainerID string

	// WorkspaceFolder is the path inside the container where the project is mounted.
	WorkspaceFolder string

	// RemoteUser is the user to run commands as inside the container.
	RemoteUser string

	// Recreated indicates whether the container was recreated (config changed)
	// rather than simply restarted.
	Recreated bool
}

// Restart restarts the container for the given workspace. It implements a
// "warm recreate" strategy:
//   - If the devcontainer config hasn't changed, it does a simple container restart
//     and runs only the resume-flow lifecycle hooks (postStartCommand, postAttachCommand).
//   - If only "safe" properties changed (volumes, mounts, ports, env, runArgs),
//     it recreates the container without rebuilding the image and runs the resume flow.
//   - If image-affecting properties changed (image, Dockerfile, features, build args),
//     it returns an error suggesting `crib rebuild`.
func (e *Engine) Restart(ctx context.Context, ws *workspace.Workspace) (*RestartResult, error) {
	e.logger.Debug("restart", "workspace", ws.ID)

	// Load stored result to get the previous config.
	storedResult, err := e.store.LoadResult(ws.ID)
	if err != nil {
		return nil, fmt.Errorf("loading workspace result: %w", err)
	}
	if storedResult == nil {
		return nil, fmt.Errorf("no previous result found for workspace %s (run 'crib up' first)", ws.ID)
	}

	// Parse current config.
	cfg, workspaceFolder, err := e.parseAndSubstitute(ws)
	if err != nil {
		return nil, err
	}

	// Detect what changed.
	var storedCfg config.DevContainerConfig
	if err := json.Unmarshal(storedResult.MergedConfig, &storedCfg); err != nil {
		return nil, fmt.Errorf("unmarshaling stored config: %w", err)
	}

	change := detectConfigChange(&storedCfg, cfg)

	switch change {
	case changeNeedsRebuild:
		return nil, fmt.Errorf("config changes require a full rebuild (image, Dockerfile, or features changed); run 'crib rebuild' instead")

	case changeSafe:
		e.reportProgress("Config changes detected, recreating container...")
		result, err := e.restartWithRecreate(ctx, ws, cfg, workspaceFolder)
		if err != nil {
			return nil, err
		}
		result.Recreated = true
		return result, nil

	default:
		// No changes — simple restart.
		e.reportProgress("Restarting container...")
		return e.restartSimple(ctx, ws, cfg, workspaceFolder, storedResult)
	}
}

// restartSimple performs a simple container restart without recreation.
func (e *Engine) restartSimple(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, storedResult *workspace.Result) (*RestartResult, error) {
	// For compose workspaces, use compose up instead of compose restart.
	// compose restart only restarts already-running containers and fails when
	// dependency services are stopped. compose up handles starting all
	// services (including dependencies) in the correct order.
	if len(cfg.DockerComposeFile) > 0 {
		if e.compose == nil {
			return nil, fmt.Errorf("compose is not available")
		}
		cd := configDir(ws)
		composeFiles := resolveComposeFiles(cd, cfg.DockerComposeFile)
		projectName := compose.ProjectName(ws.ID)
		env := devcontainerEnv(ws.ID, ws.Source, workspaceFolder)

		overridePath, err := e.generateComposeOverride(ws, cfg, workspaceFolder, cd, composeFiles, "")
		if err != nil {
			return nil, fmt.Errorf("generating compose override: %w", err)
		}
		defer func() { _ = os.Remove(overridePath) }()

		allFiles := append(composeFiles[:len(composeFiles):len(composeFiles)], overridePath)
		services := ensureServiceIncluded(cfg.RunServices, cfg.Service)

		e.reportProgress("Starting services...")
		if err := e.compose.Up(ctx, projectName, allFiles, services, e.composeStdout(), e.stderr, env); err != nil {
			return nil, fmt.Errorf("starting compose services: %w", err)
		}
	} else {
		// Non-compose: restart the individual container.
		container, err := e.driver.FindContainer(ctx, ws.ID)
		if err != nil {
			return nil, fmt.Errorf("finding container: %w", err)
		}
		if container == nil {
			return nil, fmt.Errorf("no container found for workspace %s", ws.ID)
		}
		if err := e.driver.RestartContainer(ctx, ws.ID, container.ID); err != nil {
			return nil, fmt.Errorf("restarting container: %w", err)
		}
	}

	// Run resume-flow hooks.
	containerID := storedResult.ContainerID
	remoteUser := storedResult.RemoteUser
	if err := e.runResumeHooks(ctx, ws, cfg, containerID, workspaceFolder, remoteUser); err != nil {
		e.logger.Warn("resume hooks failed", "error", err)
	}

	// Update timestamps.
	e.saveResult(ws, cfg, &UpResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	})

	return &RestartResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	}, nil
}

// restartWithRecreate stops the container, recreates it with the new config,
// and runs resume-flow hooks (not the full creation lifecycle).
func (e *Engine) restartWithRecreate(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string) (*RestartResult, error) {
	// Remove existing container first.
	if err := e.Down(ctx, ws); err != nil {
		e.logger.Warn("failed to remove container before recreate", "error", err)
	}

	// For compose: down + up (picks up volume/env/port changes).
	if len(cfg.DockerComposeFile) > 0 {
		return e.restartRecreateCompose(ctx, ws, cfg, workspaceFolder)
	}

	return e.restartRecreateSingle(ctx, ws, cfg, workspaceFolder)
}

// restartRecreateCompose handles the compose path for restartWithRecreate.
func (e *Engine) restartRecreateCompose(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string) (*RestartResult, error) {
	if e.compose == nil {
		return nil, fmt.Errorf("compose is not available")
	}

	containerID, err := e.recreateComposeServices(ctx, ws, cfg, workspaceFolder, "")
	if err != nil {
		return nil, err
	}

	remoteUser := e.resolveRemoteUser(ctx, ws.ID, cfg, containerID)
	if err := e.runResumeHooks(ctx, ws, cfg, containerID, workspaceFolder, remoteUser); err != nil {
		e.logger.Warn("resume hooks failed", "error", err)
	}

	e.saveResult(ws, cfg, &UpResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	})

	return &RestartResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	}, nil
}

// restartRecreateSingle handles the single-container path for restartWithRecreate.
func (e *Engine) restartRecreateSingle(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string) (*RestartResult, error) {
	// Delete old container.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding container: %w", err)
	}
	if container != nil {
		if err := e.driver.DeleteContainer(ctx, ws.ID, container.ID); err != nil {
			return nil, fmt.Errorf("deleting container: %w", err)
		}
	}

	// Determine the image name. For image-based, it's in the config.
	// For Dockerfile-based, use the stored result's image.
	storedResult, _ := e.store.LoadResult(ws.ID)
	imageName := cfg.Image
	if imageName == "" && storedResult != nil {
		imageName = storedResult.ImageName
	}
	if imageName == "" {
		return nil, fmt.Errorf("cannot determine image name; run 'crib rebuild' instead")
	}

	runOpts := e.buildRunOptions(cfg, imageName, ws.Source, workspaceFolder)
	e.reportProgress("Creating container...")
	if err := e.driver.RunContainer(ctx, ws.ID, runOpts); err != nil {
		return nil, fmt.Errorf("creating container: %w", err)
	}

	container, err = e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding new container: %w", err)
	}
	if container == nil {
		return nil, fmt.Errorf("container not found after recreation")
	}

	remoteUser := e.resolveRemoteUser(ctx, ws.ID, cfg, container.ID)
	if err := e.runResumeHooks(ctx, ws, cfg, container.ID, workspaceFolder, remoteUser); err != nil {
		e.logger.Warn("resume hooks failed", "error", err)
	}

	e.saveResult(ws, cfg, &UpResult{
		ContainerID:     container.ID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	})

	return &RestartResult{
		ContainerID:     container.ID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	}, nil
}

// runResumeHooks executes only the resume-flow lifecycle hooks
// (postStartCommand + postAttachCommand) for a container.
func (e *Engine) runResumeHooks(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, containerID, workspaceFolder, remoteUser string) error {
	runner := &lifecycleRunner{
		driver:      e.driver,
		store:       e.store,
		workspaceID: ws.ID,
		containerID: containerID,
		remoteUser:  remoteUser,
		remoteEnv:   cfg.RemoteEnv,
		logger:      e.logger,
		stdout:      e.stdout,
		stderr:      e.stderr,
		progress:    e.progress,
	}
	return runner.runResumeHooks(ctx, cfg, workspaceFolder)
}
