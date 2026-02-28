package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"

	"golang.org/x/sync/errgroup"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/workspace"
)

// runInitializeCommand executes the initializeCommand lifecycle hook on the
// host before image build/pull. Per the devcontainer spec, this runs on the
// host machine (not in a container) on every "up" invocation.
// Object-form hooks (named entries) run in parallel per the spec.
func (e *Engine) runInitializeCommand(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig) error {
	if len(cfg.InitializeCommand) == 0 {
		return nil
	}

	e.reportProgress("Running initializeCommand...")

	// String/array form uses the "" key: single sequential entry.
	if _, sequential := cfg.InitializeCommand[""]; sequential {
		return e.execInitCmd(ctx, ws, "initializeCommand", "", cfg.InitializeCommand[""])
	}

	// Object form: all named entries run in parallel.
	g, gCtx := errgroup.WithContext(ctx)
	for hookName, cmdParts := range cfg.InitializeCommand {
		hookName, cmdParts := hookName, cmdParts
		g.Go(func() error {
			return e.execInitCmd(gCtx, ws, "initializeCommand", hookName, cmdParts)
		})
	}
	return g.Wait()
}

// execInitCmd runs a single initializeCommand entry on the host.
func (e *Engine) execInitCmd(ctx context.Context, ws *workspace.Workspace, hookStage, hookName string, cmdParts []string) error {
	if len(cmdParts) == 0 {
		return nil
	}

	label := hookStage
	if hookName != "" {
		label = hookStage + ":" + hookName
	}

	var cmd *exec.Cmd
	if len(cmdParts) == 1 {
		// Single string: run via shell.
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdParts[0])
	} else {
		// Array: run as direct exec.
		cmd = exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	}

	cmd.Dir = ws.Source
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr

	e.logger.Debug("executing host command", slog.String("hook", label), slog.String("cmd", cmd.String()))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("lifecycle hook %q failed: %w", label, err)
	}
	return nil
}
