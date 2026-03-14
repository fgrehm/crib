package engine

import (
	"context"
	"fmt"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// finalizeOpts configures the finalize method.
type finalizeOpts struct {
	cc              containerContext
	imageName       string                          // original (not snapshot) for result
	hasEntrypoints  bool                            // feature entrypoints baked into image
	pluginResp      *plugin.PreContainerRunResponse // may be nil
	storedResult    *workspace.Result               // non-nil for snapshot/stored resume
	fromSnapshot    bool                            // true = restore env + resume hooks
	skipVolumeChown bool                            // true for restart (volumes exist)
}

// finalize runs post-creation/post-restart steps: plugin file copies, volume
// chown, user resolution, env building, result saving, and lifecycle hooks.
// All flows (up, restart, recreate) converge here.
//
// On fresh setup, lifecycle hook failures return both a result and a non-nil
// error so callers can still use the container. On snapshot/stored resume,
// resume hook failures are logged but not propagated (the container is
// already usable from the snapshot).
func (e *Engine) finalize(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, opts finalizeOpts) (*UpResult, error) {
	cc := opts.cc

	// 1. Plugin post-creation: file copies, volume chown, and post-create hooks.
	if opts.pluginResp != nil {
		e.execPluginCopies(ctx, cc, opts.pluginResp.Copies)

		if !opts.skipVolumeChown {
			remoteUser := configRemoteUser(cfg)
			if remoteUser != "" && remoteUser != "root" {
				volCC := cc
				volCC.remoteUser = remoteUser
				e.chownPluginVolumes(ctx, volCC, opts.pluginResp.Mounts)
			}
		}
	}

	// 1b. Plugin post-container-create hooks (e.g. sandbox installs bubblewrap).
	if e.plugins != nil {
		e.dispatchPostContainerCreate(ctx, ws, cfg, cc)
	}

	// 2. Resolve remote user (skip if already set, e.g. from restartSimple).
	if cc.remoteUser == "" {
		cc.remoteUser = e.resolveRemoteUser(ctx, cc, cfg)
	}

	// 3. Build result (shared across both paths).
	result := &UpResult{
		ContainerID:           cc.containerID,
		ImageName:             opts.imageName,
		WorkspaceFolder:       cc.workspaceFolder,
		RemoteUser:            cc.remoteUser,
		Ports:                 portSpecToBindings(collectPorts(cfg.ForwardPorts, cfg.AppPort)),
		HasFeatureEntrypoints: opts.hasEntrypoints,
	}

	// 4. Build env and run lifecycle.
	if opts.fromSnapshot {
		return e.finalizeFromSnapshotPath(ctx, ws, cfg, cc, opts, result)
	}
	return e.finalizeFreshPath(ctx, ws, cfg, cc, opts, result)
}

// finalizeFromSnapshotPath handles the snapshot/stored resume path.
// Restores env from stored result, saves, and runs resume hooks only.
func (e *Engine) finalizeFromSnapshotPath(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, cc containerContext, opts finalizeOpts, result *UpResult) (*UpResult, error) {
	if opts.storedResult == nil {
		return nil, fmt.Errorf("internal: storedResult is nil for snapshot/stored resume path")
	}

	// Build env from stored result + plugins (no container probing needed).
	configEnv := resolveConfigEnvFromStored(cfg, opts.storedResult.RemoteEnv)
	envb := NewEnvBuilder(configEnv)
	envb.AddPluginResponse(opts.pluginResp)
	envb.RestoreFrom(opts.storedResult.RemoteEnv)
	cfg.RemoteEnv = envb.Build()

	// Early save so crib exec/shell work while resume hooks run.
	e.saveResult(ws, cfg, result)

	// Run only resume-flow hooks (create-time effects are in the snapshot).
	if err := e.runResumeHooks(ctx, ws, cfg, cc); err != nil {
		e.logger.Warn("resume hooks failed", "error", err)
	}

	return result, nil
}

// finalizeFreshPath handles the fresh setup path.
// Runs full setup (env probe, UID sync, lifecycle hooks), commits snapshot.
func (e *Engine) finalizeFreshPath(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, cc containerContext, opts finalizeOpts, result *UpResult) (*UpResult, error) {
	envb := NewEnvBuilder(cfg.RemoteEnv)
	envb.AddPluginResponse(opts.pluginResp)

	// Early save so crib exec/shell work while setup runs.
	e.saveResult(ws, cfg, result)

	// Run container setup (UID sync, env probe, lifecycle hooks).
	finalEnv, err := e.setupContainer(ctx, ws, cfg, cc, envb)
	cfg.RemoteEnv = finalEnv
	if err != nil {
		// Persist probed env even on hook failure so crib exec/shell
		// have the correct PATH (mise, rbenv, nvm entries).
		e.saveResult(ws, cfg, result)
		return result, fmt.Errorf("container setup: %w", err)
	}

	// After create-time hooks complete, commit a snapshot.
	e.commitSnapshot(ctx, ws, cfg, cc.containerID)

	// Final save with probed env.
	e.saveResult(ws, cfg, result)

	return result, nil
}

// toRestartResult converts an UpResult to a RestartResult.
func toRestartResult(up *UpResult) *RestartResult {
	return &RestartResult{
		ContainerID:     up.ContainerID,
		WorkspaceFolder: up.WorkspaceFolder,
		RemoteUser:      up.RemoteUser,
		Ports:           up.Ports,
	}
}
