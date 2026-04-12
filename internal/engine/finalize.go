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
	cc                      containerContext
	imageName               string                          // original (not snapshot) for result
	hasEntrypoints          bool                            // feature entrypoints baked into image
	pluginResp              *plugin.PreContainerRunResponse // may be nil
	storedResult            *workspace.Result               // non-nil for snapshot/stored resume
	fromSnapshot            bool                            // true = restore env + resume hooks
	skipVolumeChown         bool                            // true for restart (volumes exist)
	shouldMergeFeatureHooks bool                            // true when imageMetadata carries fresh feature
	// lifecycle hooks that must be merged and stored.
	// Set on first creation (build or image inspection) so
	// hooks are persisted for restart/resume. False on
	// resume paths that restore stored hooks.
	imageMetadata []*config.ImageMetadata // metadata for user inference and hook merging
	imageUser     string                  // Config.User from image inspect (Dockerfile USER fallback)
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
			if remoteUser == "" {
				// devcontainer.metadata remoteUser/containerUser takes priority
				// over raw image Config.User -- the label is the spec mechanism
				// for images to declare their intended dev user.
				remoteUser = remoteUserFromMetadata(opts.imageMetadata)
			}
			if remoteUser == "" {
				remoteUser = opts.imageUser
			}
			if remoteUser != "" && remoteUser != "root" {
				volCC := cc
				volCC.remoteUser = remoteUser
				e.chownPluginVolumes(ctx, volCC, opts.pluginResp.Mounts)
			}
		}
	}

	// 2. Resolve remote user (skip if already set, e.g. from restartSimple).
	if cc.remoteUser == "" {
		// devcontainer.metadata remoteUser/containerUser takes priority over
		// raw image Config.User: the label is the spec mechanism for images to
		// declare their intended dev user (e.g. node image sets remoteUser=node
		// even though Config.User may be root). Fall back to Config.User only
		// when metadata doesn't specify a user.
		fallbackUser := remoteUserFromMetadata(opts.imageMetadata)
		if fallbackUser == "" {
			fallbackUser = opts.imageUser
		}
		cc.remoteUser = e.resolveRemoteUser(ctx, cc, cfg, fallbackUser)
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
	// Include stored feature hooks so features' postStart/postAttach run too.
	hooks := hookSetWithStoredFeatures(cfg, opts.storedResult)
	runner := e.newLifecycleRunner(ws, cc, cfg.RemoteEnv)
	if err := runner.runResumeHooks(ctx, hooks, cc.workspaceFolder); err != nil {
		e.logger.Warn("resume hooks failed", "error", err)
	}

	return result, nil
}

// finalizeFreshPath handles the fresh setup path.
// Runs full setup (env probe, UID sync, lifecycle hooks), commits snapshot.
func (e *Engine) finalizeFreshPath(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, cc containerContext, opts finalizeOpts, result *UpResult) (*UpResult, error) {
	envb := NewEnvBuilder(cfg.RemoteEnv)
	envb.AddPluginResponse(opts.pluginResp)

	// Merge feature hooks with user hooks once (used for both storage and dispatch).
	// On first creation (shouldMergeFeatureHooks=true), merge from imageMetadata
	// and store the result so the restart/resume path can dispatch stored hooks
	// without re-resolving features. On resume (shouldMergeFeatureHooks=false),
	// use stored feature hooks to avoid overwriting them with label-only metadata
	// that lacks feature lifecycle hooks.
	var hooks *hookSet
	switch {
	case opts.shouldMergeFeatureHooks && len(opts.imageMetadata) > 0:
		merged := config.MergeConfiguration(cfg, opts.imageMetadata)
		hooks = hookSetFromMerged(merged)
		// Store feature-only hooks so the resume/restart path can dispatch them
		// without re-resolving features from OCI registries.
		e.storeFeatureHooks(ws.ID, merged, cfg)
	case opts.storedResult != nil:
		hooks = hookSetWithStoredFeatures(cfg, opts.storedResult)
	default:
		hooks = hookSetFromConfig(cfg)
	}

	// Early save so crib exec/shell work while setup runs.
	e.saveResult(ws, cfg, result)

	// Run container setup (UID sync, env probe, lifecycle hooks).
	finalEnv, err := e.setupContainer(ctx, ws, cfg, cc, envb, hooks)
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

// storeFeatureHooks extracts feature-only hooks from the pre-merged config and
// persists them to workspace.Result. Uses the already-merged config to avoid a
// redundant MergeConfiguration call.
func (e *Engine) storeFeatureHooks(wsID string, merged *config.MergedDevContainerConfig, cfg *config.DevContainerConfig) {
	result, err := e.store.LoadResult(wsID)
	if err != nil {
		e.logger.Warn("failed to load result for feature hook storage, skipping", "error", err)
		return
	}
	if result == nil {
		result = &workspace.Result{}
	}

	result.FeatureOnCreateCommands = toWorkspaceHooks(featureOnly(merged.OnCreateCommands, cfg.OnCreateCommand))
	result.FeatureUpdateContentCommands = toWorkspaceHooks(featureOnly(merged.UpdateContentCommands, cfg.UpdateContentCommand))
	result.FeaturePostCreateCommands = toWorkspaceHooks(featureOnly(merged.PostCreateCommands, cfg.PostCreateCommand))
	result.FeaturePostStartCommands = toWorkspaceHooks(featureOnly(merged.PostStartCommands, cfg.PostStartCommand))
	result.FeaturePostAttachCommands = toWorkspaceHooks(featureOnly(merged.PostAttachCommands, cfg.PostAttachCommand))

	if err := e.store.SaveResult(wsID, result); err != nil {
		e.logger.Warn("failed to store feature hooks", "error", err)
	}
}

// featureOnly returns the merged hook list without the user's hook (last entry).
// If the merged list only contains the user's hook, returns nil.
func featureOnly(merged []config.LifecycleHook, userHook config.LifecycleHook) []config.LifecycleHook {
	if len(merged) == 0 {
		return nil
	}
	// The user's hook is the last entry (appended by mergeLifecycleHooks).
	if len(userHook) > 0 {
		return merged[:len(merged)-1]
	}
	// No user hook means all entries are feature hooks.
	return merged
}

// toWorkspaceHooks converts config.LifecycleHook slices to workspace.LifecycleHook slices.
// The types are structurally identical (map[string][]string) but belong to different packages.
func toWorkspaceHooks(hooks []config.LifecycleHook) []workspace.LifecycleHook {
	if len(hooks) == 0 {
		return nil
	}
	result := make([]workspace.LifecycleHook, len(hooks))
	for i, h := range hooks {
		result[i] = workspace.LifecycleHook(h)
	}
	return result
}

// hookSetWithStoredFeatures builds a hookSet by prepending stored feature hooks
// from a workspace.Result to the user's hooks from the config.
func hookSetWithStoredFeatures(cfg *config.DevContainerConfig, stored *workspace.Result) *hookSet {
	hs := hookSetFromConfig(cfg)
	if stored == nil {
		return hs
	}
	hs.OnCreate = prependStoredHooks(stored.FeatureOnCreateCommands, hs.OnCreate)
	hs.UpdateContent = prependStoredHooks(stored.FeatureUpdateContentCommands, hs.UpdateContent)
	hs.PostCreate = prependStoredHooks(stored.FeaturePostCreateCommands, hs.PostCreate)
	hs.PostStart = prependStoredHooks(stored.FeaturePostStartCommands, hs.PostStart)
	hs.PostAttach = prependStoredHooks(stored.FeaturePostAttachCommands, hs.PostAttach)
	return hs
}

// prependStoredHooks prepends workspace.LifecycleHook entries (feature hooks)
// before the existing hook list (user hooks).
func prependStoredHooks(stored []workspace.LifecycleHook, existing []config.LifecycleHook) []config.LifecycleHook {
	if len(stored) == 0 {
		return existing
	}
	result := make([]config.LifecycleHook, 0, len(stored))
	for _, h := range stored {
		result = append(result, config.LifecycleHook(h))
	}
	return append(result, existing...)
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
