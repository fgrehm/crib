package cmd

import (
	"fmt"
	"os"

	"github.com/fgrehm/crib/internal/engine"
	"github.com/spf13/cobra"
)

var rebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild and restart the workspace container",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		eng, _, store, err := newEngine()
		if err != nil {
			return err
		}
		eng.SetOutput(os.Stdout, os.Stderr)
		eng.SetProgress(func(msg string) { u.Dim("  " + msg) })

		ws, err := currentWorkspace(store, true)
		if err != nil {
			return err
		}

		u.Header("Rebuilding workspace")

		if err := eng.Down(cmd.Context(), ws); err != nil {
			return fmt.Errorf("removing existing container: %w", err)
		}
		u.Success("Container removed")

		result, err := eng.Up(cmd.Context(), ws, engine.UpOptions{})
		if err != nil {
			return err
		}

		u.Success("Workspace ready")
		u.Keyval("container", shortID(result.ContainerID))
		u.Keyval("workspace", result.WorkspaceFolder)
		if result.RemoteUser != "" {
			u.Keyval("user", result.RemoteUser)
		}

		return nil
	},
}
