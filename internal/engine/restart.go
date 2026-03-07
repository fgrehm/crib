package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/plugin"
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

	// Ports lists the published port bindings.
	Ports []driver.PortBinding
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
		// No changes -- simple restart.
		e.reportProgress("Restarting container...")
		return e.restartSimple(ctx, ws, cfg, workspaceFolder, storedResult)
	}
}

// restartSimple performs a simple container restart without recreation.
func (e *Engine) restartSimple(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, storedResult *workspace.Result) (*RestartResult, error) {
	// For compose workspaces, use compose stop + start instead of compose
	// restart. compose restart only restarts already-running containers and
	// fails when dependency services are stopped. stop + start handles all
	// services (including dependencies) without recreating containers.
	var containerID string
	var pluginResp *plugin.PreContainerRunResponse
	if len(cfg.DockerComposeFile) > 0 {
		if e.compose == nil {
			return nil, fmt.Errorf("compose is not available")
		}
		inv := newComposeInvocation(ws, cfg, workspaceFolder)

		// Dispatch plugins for file re-injection, Env, and PathPrepend.
		composeUser := e.resolveComposeUser(ctx, cfg, inv.files)
		resp, err := e.dispatchPlugins(ctx, ws, cfg, storedResult.ImageName, workspaceFolder, composeUser)
		if err != nil {
			return nil, err
		}
		pluginResp = resp

		// Regenerate the override so it stays current for the next
		// compose up (e.g. after crib down + crib up).
		overrideImage := storedResult.ImageName
		if img, ok := e.validSnapshot(ctx, ws, cfg); ok {
			overrideImage = img
		}
		if _, err := e.generateComposeOverride(ws, cfg, workspaceFolder, inv.files, overrideImage, pluginResp); err != nil {
			e.logger.Warn("failed to regenerate compose override", "error", err)
		}

		// Use compose stop + start instead of compose up. compose up
		// recreates containers (podman-compose always does this), losing
		// anything installed by lifecycle hooks. stop + start only
		// operates on existing containers without recreation.
		overridePath := filepath.Join(e.store.WorkspaceDir(ws.ID), "compose-override.yml")
		allFiles := append(inv.files[:len(inv.files):len(inv.files)], overridePath)

		e.reportProgress("Stopping services...")
		if err := e.compose.Stop(ctx, inv.projectName, allFiles, e.composeStdout(), e.composeStderr(), inv.env); err != nil {
			e.logger.Warn("failed to stop services", "error", err)
		}

		e.reportProgress("Starting services...")
		if err := e.compose.Start(ctx, inv.projectName, allFiles, e.composeStdout(), e.composeStderr(), inv.env); err != nil {
			return nil, fmt.Errorf("starting compose services: %w", err)
		}

		container, err := e.driver.FindContainer(ctx, ws.ID)
		if err != nil {
			return nil, fmt.Errorf("finding container after restart: %w", err)
		}
		if container == nil {
			return nil, fmt.Errorf("no container found for workspace %s after restart", ws.ID)
		}
		containerID = container.ID

		// Re-inject plugin files (SSH keys, credentials) which may have
		// changed on the host since the last up/restart.
		if pluginResp != nil {
			e.execPluginCopies(ctx, containerContext{workspaceID: ws.ID, containerID: containerID}, pluginResp.Copies)
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
		containerID = container.ID

		// Dispatch plugins to get Env and PathPrepend so the saved
		// RemoteEnv includes plugin env vars across restarts.
		if resp, err := e.dispatchPlugins(ctx, ws, cfg, "", workspaceFolder, storedResult.RemoteUser); err != nil {
			e.logger.Warn("plugin dispatch failed", "error", err)
		} else {
			pluginResp = resp
		}
	}

	// Build the final env using the EnvBuilder. This replaces the old
	// sequence of applyPathPrepend + mergeStoredRemoteEnv, and also
	// includes plugin Env vars (previously dropped in restart paths).
	envb := NewEnvBuilder(cfg.RemoteEnv)
	envb.AddPluginResponse(pluginResp)
	envb.RestoreFrom(storedResult.RemoteEnv)
	cfg.RemoteEnv = envb.Build()

	// Run resume-flow hooks.
	remoteUser := storedResult.RemoteUser
	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     containerID,
		remoteUser:      remoteUser,
		workspaceFolder: workspaceFolder,
	}
	if err := e.runResumeHooks(ctx, ws, cfg, cc); err != nil {
		e.logger.Warn("resume hooks failed", "error", err)
	}

	ports := portSpecToBindings(collectPorts(cfg.ForwardPorts, cfg.AppPort))

	// Update timestamps, preserving the stored feature image name.
	e.saveResult(ws, cfg, &UpResult{
		ContainerID:     containerID,
		ImageName:       storedResult.ImageName,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
		Ports:           ports,
	})

	return &RestartResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
		Ports:           ports,
	}, nil
}

// restartWithRecreate stops the container, recreates it with the new config,
// and runs resume-flow hooks (not the full creation lifecycle).
func (e *Engine) restartWithRecreate(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string) (*RestartResult, error) {
	// Capture stored result before Down clears hook markers and potentially
	// makes it harder to recover image names for Dockerfile-based workspaces.
	storedResult, _ := e.store.LoadResult(ws.ID)

	// Remove existing container first.
	if err := e.Down(ctx, ws); err != nil {
		e.logger.Warn("failed to remove container before recreate", "error", err)
	}

	// For compose: down + up (picks up volume/env/port changes).
	if len(cfg.DockerComposeFile) > 0 {
		return e.restartRecreateCompose(ctx, ws, cfg, workspaceFolder, storedResult)
	}

	return e.restartRecreateSingle(ctx, ws, cfg, workspaceFolder, storedResult)
}

// restartRecreateCompose handles the compose path for restartWithRecreate.
func (e *Engine) restartRecreateCompose(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, storedResult *workspace.Result) (*RestartResult, error) {
	if e.compose == nil {
		return nil, fmt.Errorf("compose is not available")
	}

	// Check for a valid snapshot image to use instead of the base image.
	snapshotImage, hasSnapshot := e.validSnapshot(ctx, ws, cfg)

	// Resolve the container user from the compose service so plugins get
	// the correct remote user for home directory paths.
	inv := newComposeInvocation(ws, cfg, workspaceFolder)
	composeUser := e.resolveComposeUser(ctx, cfg, inv.files)

	// Run pre-container-run plugins to get mounts, env, and file copies.
	featureImage := snapshotImage // pass snapshot as override image
	pluginResp, err := e.dispatchPlugins(ctx, ws, cfg, featureImage, workspaceFolder, composeUser)
	if err != nil {
		return nil, err
	}

	containerID, err := e.recreateComposeServices(ctx, ws, cfg, workspaceFolder, featureImage, pluginResp)
	if err != nil {
		return nil, err
	}

	// Copy plugin files into the container.
	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     containerID,
		workspaceFolder: workspaceFolder,
	}
	if pluginResp != nil {
		e.execPluginCopies(ctx, cc, pluginResp.Copies)
	}

	cc.remoteUser = e.resolveRemoteUser(ctx, cc, cfg)

	envb := NewEnvBuilder(cfg.RemoteEnv)
	envb.AddPluginResponse(pluginResp)
	// When using a snapshot, restore the stored remoteEnv so probed PATH
	// entries (mise, rbenv, nvm) survive the restart. setupContainer handles
	// this when there's no snapshot (full re-probe).
	if hasSnapshot && storedResult != nil {
		envb.RestoreFrom(storedResult.RemoteEnv)
	}

	e.runRecreateLifecycle(ctx, ws, cfg, cc, hasSnapshot, envb)

	ports := portSpecToBindings(collectPorts(cfg.ForwardPorts, cfg.AppPort))

	e.saveResult(ws, cfg, &UpResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      cc.remoteUser,
		Ports:           ports,
	})

	return &RestartResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      cc.remoteUser,
		Ports:           ports,
	}, nil
}

// restartRecreateSingle handles the single-container path for restartWithRecreate.
func (e *Engine) restartRecreateSingle(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, storedResult *workspace.Result) (*RestartResult, error) {
	// Check for a valid snapshot image.
	snapshotImage, hasSnapshot := e.validSnapshot(ctx, ws, cfg)

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

	// Determine the image name. Use snapshot if valid, otherwise fall back
	// to stored result or rebuild.
	imageName := snapshotImage
	if imageName == "" {
		imageName = cfg.Image
	}
	if imageName == "" && storedResult != nil {
		imageName = storedResult.ImageName
	}
	if imageName == "" {
		e.reportProgress("Image name not found in stored result, rebuilding...")
		buildRes, err := e.buildImage(ctx, ws, cfg)
		if err != nil {
			return nil, fmt.Errorf("rebuilding image: %w", err)
		}
		imageName = buildRes.imageName
	}

	runOpts, err := e.buildRunOptions(cfg, imageName, ws.Source, workspaceFolder)
	if err != nil {
		return nil, err
	}

	// Run pre-container-run plugins to inject mounts, env, and extra args.
	pluginResp, err := e.runPreContainerRunPlugins(ctx, ws, cfg, runOpts, imageName, workspaceFolder)
	if err != nil {
		return nil, err
	}

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

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     container.ID,
		workspaceFolder: workspaceFolder,
	}

	// Copy plugin files into the container.
	if pluginResp != nil {
		e.execPluginCopies(ctx, cc, pluginResp.Copies)
	}

	cc.remoteUser = e.resolveRemoteUser(ctx, cc, cfg)

	// Preserve the original image name for result storage (not the snapshot).
	resultImageName := ""
	if storedResult != nil {
		resultImageName = storedResult.ImageName
	}

	envb := NewEnvBuilder(cfg.RemoteEnv)
	envb.AddPluginResponse(pluginResp)
	// When using a snapshot, restore the stored remoteEnv so probed PATH
	// entries (mise, rbenv, nvm) survive the restart.
	if hasSnapshot && storedResult != nil {
		envb.RestoreFrom(storedResult.RemoteEnv)
	}

	e.runRecreateLifecycle(ctx, ws, cfg, cc, hasSnapshot, envb)

	ports := portSpecToBindings(collectPorts(cfg.ForwardPorts, cfg.AppPort))

	e.saveResult(ws, cfg, &UpResult{
		ContainerID:     container.ID,
		ImageName:       resultImageName,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      cc.remoteUser,
		Ports:           ports,
	})

	return &RestartResult{
		ContainerID:     container.ID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      cc.remoteUser,
		Ports:           ports,
	}, nil
}

// runRecreateLifecycle decides which hooks to run after a container recreate.
// When a valid snapshot exists, only resume hooks run (create-time effects are
// already baked in). Otherwise, full setup runs and a new snapshot is committed.
func (e *Engine) runRecreateLifecycle(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, cc containerContext, hasSnapshot bool, envb *EnvBuilder) {
	if hasSnapshot {
		cfg.RemoteEnv = envb.Build()
		if err := e.runResumeHooks(ctx, ws, cfg, cc); err != nil {
			e.logger.Warn("resume hooks failed", "error", err)
		}
	} else {
		e.reportProgress("No snapshot available, running full setup...")
		finalEnv, err := e.setupContainer(ctx, ws, cfg, cc, envb)
		cfg.RemoteEnv = finalEnv
		if err != nil {
			e.logger.Warn("setup failed", "error", err)
		}
		e.commitSnapshot(ctx, ws, cfg, cc.containerID)
	}
}

// runResumeHooks executes only the resume-flow lifecycle hooks
// (postStartCommand + postAttachCommand) for a container.
func (e *Engine) runResumeHooks(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, cc containerContext) error {
	runner := e.newLifecycleRunner(ws, cc, cfg.RemoteEnv)
	return runner.runResumeHooks(ctx, cfg, cc.workspaceFolder)
}
