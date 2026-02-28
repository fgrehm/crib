package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
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
}

// runLifecycleHooks executes the devcontainer lifecycle hooks in order.
// Hooks run as the remote user. Marker files provide idempotency for
// create-time hooks (onCreate, updateContent, postCreate).
// After the stage named by cfg.WaitFor (default: "updateContentCommand"),
// a "Container ready." progress message is emitted.
func (r *lifecycleRunner) runLifecycleHooks(ctx context.Context, cfg *config.DevContainerConfig, workspaceFolder string) error {
	waitFor := cfg.WaitFor
	if waitFor == "" {
		waitFor = "updateContentCommand"
	}

	// onCreate hooks: run only once (marker file prevents re-execution).
	if err := r.runHookWithMarker(ctx, "onCreateCommand", cfg.OnCreateCommand, workspaceFolder); err != nil {
		return err
	}
	r.signalReadyAt("onCreateCommand", waitFor)

	// updateContent hooks.
	if err := r.runHookWithMarker(ctx, "updateContentCommand", cfg.UpdateContentCommand, workspaceFolder); err != nil {
		return err
	}
	r.signalReadyAt("updateContentCommand", waitFor)

	// postCreate hooks: run only once.
	if err := r.runHookWithMarker(ctx, "postCreateCommand", cfg.PostCreateCommand, workspaceFolder); err != nil {
		return err
	}
	r.signalReadyAt("postCreateCommand", waitFor)

	// postStart hooks: run every time the container starts.
	if err := r.runHook(ctx, "postStartCommand", cfg.PostStartCommand, workspaceFolder); err != nil {
		return err
	}
	r.signalReadyAt("postStartCommand", waitFor)

	// postAttach hooks: run every time.
	if err := r.runHook(ctx, "postAttachCommand", cfg.PostAttachCommand, workspaceFolder); err != nil {
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
	// postStart hooks: run every time the container starts.
	if err := r.runHook(ctx, "postStartCommand", cfg.PostStartCommand, workspaceFolder); err != nil {
		return err
	}

	// postAttach hooks: run every time.
	if err := r.runHook(ctx, "postAttachCommand", cfg.PostAttachCommand, workspaceFolder); err != nil {
		return err
	}

	return nil
}

// runHookWithMarker executes a lifecycle hook, using a host-side marker
// file to ensure it only runs once. Markers are stored in the workspace
// directory (~/.crib/workspaces/{id}/hooks/) so they survive container
// recreation (e.g. docker compose up recreating stopped containers).
func (r *lifecycleRunner) runHookWithMarker(ctx context.Context, name string, hook config.LifecycleHook, workspaceFolder string) error {
	if len(hook) == 0 {
		return nil
	}

	// Check if marker exists on the host (hook already ran).
	if r.store.IsHookDone(r.workspaceID, name) {
		r.logger.Debug("skipping hook (already ran)", "hook", name)
		return nil
	}

	if err := r.runHook(ctx, name, hook, workspaceFolder); err != nil {
		return err
	}

	// Create marker file on the host.
	if err := r.store.MarkHookDone(r.workspaceID, name); err != nil {
		r.logger.Warn("failed to write hook marker", "hook", name, "error", err)
	}
	return nil
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

	// String/array form uses the "" key: single sequential entry.
	if _, sequential := hook[""]; sequential {
		return r.execHookCmd(ctx, name, "", hook[""], workspaceFolder)
	}

	// Object form: all named entries run in parallel.
	g, gCtx := errgroup.WithContext(ctx)
	for hookName, cmdParts := range hook {
		hookName, cmdParts := hookName, cmdParts
		g.Go(func() error {
			return r.execHookCmd(gCtx, name, hookName, cmdParts, workspaceFolder)
		})
	}
	return g.Wait()
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
	var cmdStr string
	if len(cmdParts) == 1 {
		cmdStr = cmdParts[0]
	} else {
		cmdStr = strings.Join(cmdParts, " ")
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
