package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

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
}

// runLifecycleHooks executes the devcontainer lifecycle hooks in order.
// Hooks run as the remote user. Marker files provide idempotency for
// create-time hooks (onCreate, updateContent, postCreate).
func (r *lifecycleRunner) runLifecycleHooks(ctx context.Context, cfg *config.DevContainerConfig, workspaceFolder string) error {
	// onCreate hooks: run only once (marker file prevents re-execution).
	if err := r.runHookWithMarker(ctx, "onCreateCommand", cfg.OnCreateCommand, workspaceFolder); err != nil {
		return err
	}

	// updateContent hooks.
	if err := r.runHookWithMarker(ctx, "updateContentCommand", cfg.UpdateContentCommand, workspaceFolder); err != nil {
		return err
	}

	// postCreate hooks: run only once.
	if err := r.runHookWithMarker(ctx, "postCreateCommand", cfg.PostCreateCommand, workspaceFolder); err != nil {
		return err
	}

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
func (r *lifecycleRunner) runHook(ctx context.Context, name string, hook config.LifecycleHook, workspaceFolder string) error {
	if len(hook) == 0 {
		return nil
	}

	if r.progress != nil {
		r.progress("Running " + name + "...")
	}
	r.logger.Debug("running lifecycle hook", "hook", name)

	for hookName, cmdParts := range hook {
		if len(cmdParts) == 0 {
			continue
		}

		label := name
		if hookName != "" {
			label = name + ":" + hookName
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
		if err := r.driver.ExecContainer(ctx, r.workspaceID, r.containerID, execCmd, nil, r.stdout, r.stderr, envSlice(r.remoteEnv), r.remoteUser); err != nil {
			return fmt.Errorf("lifecycle hook %q failed: %w", label, err)
		}
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

