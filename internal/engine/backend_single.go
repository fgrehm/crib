package engine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	ocidriver "github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// applyGlobalEnv prepends KEY=VALUE entries from env into runOpts.Env so
// project-level ContainerEnv (added before this call) overrides globals on
// duplicate keys. The runtime resolves `-e` flags with last-wins semantics,
// so "first" means "lower priority".
func applyGlobalEnv(runOpts *driver.RunOptions, env map[string]string) {
	if len(env) == 0 {
		return
	}
	existing := runOpts.Env
	runOpts.Env = make([]string, 0, len(env)+len(existing))
	// Map iteration order is non-deterministic; global entries may appear in
	// any order relative to each other. Correctness is unaffected: the runtime
	// resolves duplicate keys with last-wins, and all project entries (appended
	// below) always come after all global entries.
	for k, v := range env {
		runOpts.Env = append(runOpts.Env, k+"="+v)
	}
	runOpts.Env = append(runOpts.Env, existing...)
}

// applyGlobalMounts parses each global workspace mount spec and appends it to
// runOpts.Mounts, skipping any spec whose target is already in claimed.
// claimed is updated in place as targets are added, so the caller can reuse it
// for subsequent mount sources (features, plugins) to achieve end-to-end dedup.
func applyGlobalMounts(runOpts *driver.RunOptions, specs []string, claimed map[string]bool, logger *slog.Logger) error {
	for _, spec := range specs {
		m, err := config.ParseMount(spec)
		if err != nil {
			return fmt.Errorf("global workspace mount %q from [workspace].mount: %w", spec, err)
		}
		if claimed[m.Target] {
			logger.Warn("skipping mount: target already claimed by an earlier mount source", "kind", "global", "source", m.Source, "target", m.Target)
			continue
		}
		runOpts.Mounts = append(runOpts.Mounts, m)
		claimed[m.Target] = true
	}
	return nil
}

// filterMountsAfter removes mounts appended from index start onward whose
// targets are already in claimed, logging a warning for each skip. New targets
// are added to claimed so subsequent sources can check against them.
func filterMountsAfter(mounts []config.Mount, start int, claimed map[string]bool, kind string, logger *slog.Logger) []config.Mount {
	out := mounts[:start:start]
	for _, m := range mounts[start:] {
		if claimed[m.Target] {
			logger.Warn("skipping mount: target already claimed by an earlier mount source", "kind", kind, "source", m.Source, "target", m.Target)
			continue
		}
		out = append(out, m)
		claimed[m.Target] = true
	}
	return out
}

// singleBackend implements containerBackend for single-container workspaces.
type singleBackend struct {
	e               *Engine
	ws              *workspace.Workspace
	cfg             *config.DevContainerConfig
	workspaceFolder string
}

func (b *singleBackend) pluginUser(_ context.Context, fallbacks ...string) string {
	// Config always wins over fallbacks.
	if b.cfg != nil {
		if user := configRemoteUser(b.cfg); user != "" {
			return user
		}
	}
	for _, f := range fallbacks {
		if f != "" {
			return f
		}
	}
	return ""
}

func (b *singleBackend) start(ctx context.Context, containerID string, _ *plugin.PreContainerRunResponse) (string, error) {
	if err := b.e.driver.StartContainer(ctx, b.ws.ID, containerID); err != nil {
		return "", fmt.Errorf("starting container: %w", err)
	}
	return containerID, nil
}

func (b *singleBackend) buildImage(ctx context.Context) (*buildResult, error) {
	return b.e.buildImage(ctx, b.ws, b.cfg)
}

func (b *singleBackend) createContainer(ctx context.Context, opts createOpts) (createContainerResult, error) {
	runOpts, err := b.e.buildRunOptions(b.cfg, opts.imageName, b.ws.Source, b.workspaceFolder, opts.hasEntrypoints)
	if err != nil {
		return createContainerResult{}, err
	}
	if b.e.store.IsExplicitHome() {
		runOpts.Labels[ocidriver.LabelHome] = b.e.store.BaseDir()
	}

	// claimed tracks mount targets already added so later sources (global,
	// feature, plugin) skip duplicates rather than causing docker/podman to
	// fail with "duplicate mount destination". Seeded from project mounts
	// and the workspace mount, which is tracked separately in RunOptions.
	claimed := make(map[string]bool, len(runOpts.Mounts)+1)
	for _, m := range runOpts.Mounts {
		claimed[m.Target] = true
	}
	if runOpts.WorkspaceMount.Target != "" {
		claimed[runOpts.WorkspaceMount.Target] = true
	}

	globalWS := b.e.expandedGlobalWorkspace(b.ws, b.workspaceFolder)

	// Prepend global env so project-level ContainerEnv (already present in
	// runOpts.Env via buildRunOptions) wins on duplicate keys when the
	// runtime resolves the final environment (later -e flags override).
	applyGlobalEnv(runOpts, globalWS.Env)
	if err := applyGlobalMounts(runOpts, globalWS.Mounts, claimed, b.e.logger); err != nil {
		return createContainerResult{}, err
	}

	subCtx := &config.SubstitutionContext{
		DevContainerID:           b.ws.ID,
		LocalWorkspaceFolder:     b.ws.Source,
		ContainerWorkspaceFolder: b.workspaceFolder,
		Env:                      envMap(),
	}
	preFeat := len(runOpts.Mounts)
	applyFeatureMetadata(runOpts, opts.metadata, subCtx)
	runOpts.Mounts = filterMountsAfter(runOpts.Mounts, preFeat, claimed, "feature", b.e.logger)

	// Prepend global runArgs so project values (already in runOpts.ExtraArgs)
	// win on conflict under the runtime's last-flag-wins semantics. Plugin
	// runArgs are appended below and win over both.
	if len(globalWS.RunArgs) > 0 {
		args := make([]string, 0, len(globalWS.RunArgs)+len(runOpts.ExtraArgs))
		args = append(args, globalWS.RunArgs...)
		runOpts.ExtraArgs = append(args, runOpts.ExtraArgs...)
	}

	// Merge plugin response into run options (mounts, env, runArgs).
	if opts.pluginResp != nil {
		for _, m := range opts.pluginResp.Mounts {
			if claimed[m.Target] {
				b.e.logger.Warn("skipping mount: target already claimed by an earlier mount source", "kind", "plugin", "source", m.Source, "target", m.Target)
				continue
			}
			runOpts.Mounts = append(runOpts.Mounts, m)
			claimed[m.Target] = true
		}
		for k, v := range opts.pluginResp.Env {
			runOpts.Env = append(runOpts.Env, k+"="+v)
		}
		runOpts.ExtraArgs = append(runOpts.ExtraArgs, opts.pluginResp.RunArgs...)
	}

	b.e.reportProgress(PhaseCreate, "Creating container...")
	name, err := b.e.driver.RunContainer(ctx, b.ws.ID, runOpts)
	if err != nil {
		return createContainerResult{}, fmt.Errorf("creating container: %w", err)
	}

	container, err := b.e.driver.FindContainer(ctx, b.ws.ID)
	if err != nil {
		return createContainerResult{}, fmt.Errorf("finding new container: %w", err)
	}
	if container == nil {
		return createContainerResult{}, fmt.Errorf("container not found after creation")
	}

	return createContainerResult{ContainerID: container.ID, ContainerName: name}, nil
}

func (b *singleBackend) deleteExisting(ctx context.Context) error {
	container, err := b.e.driver.FindContainer(ctx, b.ws.ID)
	if err != nil {
		return fmt.Errorf("finding container: %w", err)
	}
	if container == nil {
		return nil
	}
	return b.e.driver.DeleteContainer(ctx, b.ws.ID, container.ID)
}

func (b *singleBackend) restart(ctx context.Context, containerID string, _ *plugin.PreContainerRunResponse) (string, error) {
	if err := b.e.driver.RestartContainer(ctx, b.ws.ID, containerID); err != nil {
		return "", fmt.Errorf("restarting container: %w", err)
	}
	return containerID, nil
}

func (b *singleBackend) canResumeFromStored() bool {
	return false
}
