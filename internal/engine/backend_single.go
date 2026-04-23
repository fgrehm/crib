package engine

import (
	"context"
	"fmt"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	ocidriver "github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// prependGlobalEnv prepends KEY=VALUE entries from env into runOpts.Env so
// project-level ContainerEnv (added before this call) overrides globals on
// duplicate keys. The runtime resolves `-e` flags with last-wins semantics,
// so "first" means "lower priority".
func prependGlobalEnv(runOpts *driver.RunOptions, env map[string]string) {
	if len(env) == 0 {
		return
	}
	existing := runOpts.Env
	runOpts.Env = make([]string, 0, len(env)+len(existing))
	for k, v := range env {
		runOpts.Env = append(runOpts.Env, k+"="+v)
	}
	runOpts.Env = append(runOpts.Env, existing...)
}

// appendGlobalMounts parses each mount spec and appends it to runOpts.Mounts.
func appendGlobalMounts(runOpts *driver.RunOptions, specs []string) error {
	for _, spec := range specs {
		m, err := config.ParseMount(spec)
		if err != nil {
			return fmt.Errorf("global mount %q: %w", spec, err)
		}
		runOpts.Mounts = append(runOpts.Mounts, m)
	}
	return nil
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

	// Prepend global env so project-level ContainerEnv (already present in
	// runOpts.Env via buildRunOptions) wins on duplicate keys when the
	// runtime resolves the final environment (later -e flags override).
	prependGlobalEnv(runOpts, b.e.globalWorkspace.Env)
	if err := appendGlobalMounts(runOpts, b.e.globalWorkspace.Mounts); err != nil {
		return createContainerResult{}, err
	}

	subCtx := &config.SubstitutionContext{
		DevContainerID:           b.ws.ID,
		LocalWorkspaceFolder:     b.ws.Source,
		ContainerWorkspaceFolder: b.workspaceFolder,
		Env:                      envMap(),
	}
	applyFeatureMetadata(runOpts, opts.metadata, subCtx)

	// Prepend global runArgs so project values (already in runOpts.ExtraArgs)
	// win on conflict under the runtime's last-flag-wins semantics. Plugin
	// runArgs are appended below and win over both.
	if len(b.e.globalWorkspace.RunArgs) > 0 {
		runOpts.ExtraArgs = append(append([]string{}, b.e.globalWorkspace.RunArgs...), runOpts.ExtraArgs...)
	}

	// Merge plugin response into run options (mounts, env, runArgs).
	if opts.pluginResp != nil {
		runOpts.Mounts = append(runOpts.Mounts, opts.pluginResp.Mounts...)
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
