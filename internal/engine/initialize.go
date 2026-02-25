package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/workspace"
)

// runInitializeCommand executes the initializeCommand lifecycle hook on the
// host before image build/pull. Per the devcontainer spec, this runs on the
// host machine (not in a container) on every "up" invocation.
func (e *Engine) runInitializeCommand(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig) error {
	if len(cfg.InitializeCommand) == 0 {
		return nil
	}

	e.reportProgress("Running initializeCommand...")

	for hookName, cmdParts := range cfg.InitializeCommand {
		if len(cmdParts) == 0 {
			continue
		}

		label := "initializeCommand"
		if hookName != "" {
			label = "initializeCommand:" + hookName
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
	}

	return nil
}
