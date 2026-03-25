package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// lifecycleRunner executes lifecycle hooks inside a container.
type lifecycleRunner struct {
	driver      driver.Driver
	store       *workspace.Store
	workspaceID string
	containerID string
	remoteUser  string
	remoteEnv   map[string]string
	logger      *slog.Logger
	stdout      io.Writer
	stderr      io.Writer
	progress    func(string)
	verbose     bool

	// featureHooks holds lifecycle hooks declared by DevContainer Features.
	// When non-nil, feature hooks are dispatched before user hooks at each
	// stage, in feature installation order (per the spec).
	featureHooks *config.MergedConfigProperties
}

// newLifecycleRunner creates a lifecycleRunner from the engine's dependencies,
// a container context, and the resolved remote environment.
func (e *Engine) newLifecycleRunner(ws *workspace.Workspace, cc containerContext, remoteEnv map[string]string) *lifecycleRunner {
	return &lifecycleRunner{
		driver:      e.driver,
		store:       e.store,
		workspaceID: ws.ID,
		containerID: cc.containerID,
		remoteUser:  cc.remoteUser,
		remoteEnv:   remoteEnv,
		logger:      e.logger,
		stdout:      e.stdout,
		stderr:      e.stderr,
		progress:    e.progress,
		verbose:     e.verbose,
	}
}

// runLifecycleHooks executes the devcontainer lifecycle hooks in order.
// Hooks run as the remote user. Marker files provide idempotency for
// create-time hooks (onCreate, updateContent, postCreate).
// After the stage named by cfg.WaitFor (default: "updateContentCommand"),
// a "Container ready." progress message is emitted.
//
// When featureHooks is set, feature-declared hooks are dispatched before
// user hooks at each stage, in feature installation order.
func (r *lifecycleRunner) runLifecycleHooks(ctx context.Context, cfg *config.DevContainerConfig, workspaceFolder string) error {
	waitFor := cfg.WaitFor
	if waitFor == "" {
		waitFor = "updateContentCommand"
	}

	fh := func(get func(*config.MergedConfigProperties) []config.LifecycleHook) []config.LifecycleHook {
		return r.featureHookList(get)
	}

	// onCreate hooks: feature hooks first, then user hook. Run only once.
	if err := r.runStageWithMarker(ctx, "onCreateCommand", fh(func(m *config.MergedConfigProperties) []config.LifecycleHook { return m.OnCreateCommands }), cfg.OnCreateCommand, workspaceFolder); err != nil {
		return err
	}
	r.signalReadyAt("onCreateCommand", waitFor)

	// updateContent hooks.
	if err := r.runStageWithMarker(ctx, "updateContentCommand", fh(func(m *config.MergedConfigProperties) []config.LifecycleHook { return m.UpdateContentCommands }), cfg.UpdateContentCommand, workspaceFolder); err != nil {
		return err
	}
	r.signalReadyAt("updateContentCommand", waitFor)

	// postCreate hooks: run only once.
	if err := r.runStageWithMarker(ctx, "postCreateCommand", fh(func(m *config.MergedConfigProperties) []config.LifecycleHook { return m.PostCreateCommands }), cfg.PostCreateCommand, workspaceFolder); err != nil {
		return err
	}
	r.signalReadyAt("postCreateCommand", waitFor)

	// postStart hooks: run every time the container starts.
	if err := r.runStage(ctx, "postStartCommand", fh(func(m *config.MergedConfigProperties) []config.LifecycleHook { return m.PostStartCommands }), cfg.PostStartCommand, workspaceFolder); err != nil {
		return err
	}
	r.signalReadyAt("postStartCommand", waitFor)

	// postAttach hooks: run every time.
	if err := r.runStage(ctx, "postAttachCommand", fh(func(m *config.MergedConfigProperties) []config.LifecycleHook { return m.PostAttachCommands }), cfg.PostAttachCommand, workspaceFolder); err != nil {
		return err
	}

	return nil
}

// signalReadyAt emits a "Container ready." progress message when stage matches waitFor.
func (r *lifecycleRunner) signalReadyAt(stage, waitFor string) {
	if stage == waitFor && r.progress != nil {
		r.progress("Container ready.")
	}
}

// runResumeHooks executes only the resume-flow lifecycle hooks (postStartCommand
// and postAttachCommand). Per the devcontainer spec, these are the only hooks
// that run when a container is restarted (as opposed to freshly created).
func (r *lifecycleRunner) runResumeHooks(ctx context.Context, cfg *config.DevContainerConfig, workspaceFolder string) error {
	fh := func(get func(*config.MergedConfigProperties) []config.LifecycleHook) []config.LifecycleHook {
		return r.featureHookList(get)
	}

	// postStart hooks: feature hooks first, then user hook.
	if err := r.runStage(ctx, "postStartCommand", fh(func(m *config.MergedConfigProperties) []config.LifecycleHook { return m.PostStartCommands }), cfg.PostStartCommand, workspaceFolder); err != nil {
		return err
	}

	// postAttach hooks: feature hooks first, then user hook.
	if err := r.runStage(ctx, "postAttachCommand", fh(func(m *config.MergedConfigProperties) []config.LifecycleHook { return m.PostAttachCommands }), cfg.PostAttachCommand, workspaceFolder); err != nil {
		return err
	}

	return nil
}

// featureHookList returns the feature hook list for a stage, or nil.
func (r *lifecycleRunner) featureHookList(get func(*config.MergedConfigProperties) []config.LifecycleHook) []config.LifecycleHook {
	if r.featureHooks == nil {
		return nil
	}
	return get(r.featureHooks)
}

// runStage dispatches feature hooks followed by the user hook for a stage.
func (r *lifecycleRunner) runStage(ctx context.Context, name string, featureHooks []config.LifecycleHook, userHook config.LifecycleHook, workspaceFolder string) error {
	for _, fh := range featureHooks {
		if err := r.runHook(ctx, name, fh, workspaceFolder); err != nil {
			return err
		}
	}
	return r.runHook(ctx, name, userHook, workspaceFolder)
}

// runStageWithMarker dispatches feature hooks followed by the user hook,
// using a host-side marker file to ensure the entire stage only runs once.
func (r *lifecycleRunner) runStageWithMarker(ctx context.Context, name string, featureHooks []config.LifecycleHook, userHook config.LifecycleHook, workspaceFolder string) error {
	if len(featureHooks) == 0 && len(userHook) == 0 {
		return nil
	}

	if r.store.IsHookDone(r.workspaceID, name) {
		r.logger.Debug("skipping hook (already ran)", "hook", name)
		return nil
	}

	if err := r.runStage(ctx, name, featureHooks, userHook, workspaceFolder); err != nil {
		return err
	}

	if err := r.store.MarkHookDone(r.workspaceID, name); err != nil {
		r.logger.Warn("failed to write hook marker", "hook", name, "error", err)
	}
	return nil
}

// dispatchHook runs a LifecycleHook's entries using executor.
// String/array hooks (stored under the "" key) call executor once, sequentially.
// Object hooks (named entries) call executor for each entry in parallel via errgroup;
// all must succeed for the hook to succeed.
func dispatchHook(ctx context.Context, hook config.LifecycleHook, executor func(context.Context, string, []string) error) error {
	// String/array form uses the "" key: single sequential entry.
	if _, sequential := hook[""]; sequential {
		return executor(ctx, "", hook[""])
	}

	// Object form: all named entries run in parallel.
	g, gCtx := errgroup.WithContext(ctx)
	for hookName, cmdParts := range hook {
		g.Go(func() error { return executor(gCtx, hookName, cmdParts) })
	}
	return g.Wait()
}

// runHook executes a lifecycle hook's commands inside the container.
// Object-form hooks (named entries) run in parallel per the devcontainer spec.
// String and array-form hooks (stored under the "" key) run sequentially.
func (r *lifecycleRunner) runHook(ctx context.Context, name string, hook config.LifecycleHook, workspaceFolder string) error {
	if len(hook) == 0 {
		return nil
	}

	if r.progress != nil {
		r.progress("Running " + name + "...")
	}
	r.logger.Debug("running lifecycle hook", "hook", name)

	return dispatchHook(ctx, hook, func(ctx context.Context, hookName string, cmdParts []string) error {
		return r.execHookCmd(ctx, name, hookName, cmdParts, workspaceFolder)
	})
}

// execHookCmd executes a single hook command inside the container.
func (r *lifecycleRunner) execHookCmd(ctx context.Context, hookStage, hookName string, cmdParts []string, workspaceFolder string) error {
	if len(cmdParts) == 0 {
		return nil
	}

	label := hookStage
	if hookName != "" {
		label = hookStage + ":" + hookName
	}

	// Build the command string for the shell wrapper.
	// Single-element cmdParts are shell strings (from "cmd" or ["cmd"]):
	// pass as-is so the shell can parse flags, pipes, and redirects.
	// Multi-element cmdParts are exec-style (from ["cmd", "arg1", "arg2"]):
	// shell-quote each argument to preserve spaces and metacharacters.
	var cmdStr string
	if len(cmdParts) == 1 {
		cmdStr = cmdParts[0]
	} else {
		cmdStr = plugin.ShellQuoteJoin(cmdParts)
	}

	// Wrap with user switch and working directory.
	execCmd := r.wrapCommand(cmdStr, workspaceFolder)

	r.logger.Debug("executing hook command", "hook", label, "cmd", execCmd)
	if r.verbose {
		_, _ = fmt.Fprintf(r.stderr, "  $ %s\n", cmdStr)
	}
	if err := r.driver.ExecContainer(ctx, r.workspaceID, r.containerID, execCmd, nil, r.stdout, r.stderr, envSlice(r.remoteEnv), r.remoteUser); err != nil {
		return fmt.Errorf("lifecycle hook %q failed: %w", label, err)
	}
	return nil
}

// wrapCommand wraps a command string to run in the workspace folder.
// User switching is handled at the driver level via --user.
func (r *lifecycleRunner) wrapCommand(cmdStr string, workspaceFolder string) []string {
	inner := cmdStr
	if workspaceFolder != "" {
		inner = fmt.Sprintf("cd %q 2>/dev/null; %s", workspaceFolder, inner)
	}
	return []string{"sh", "-c", inner}
}
