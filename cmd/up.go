package cmd

import (
	"os"

	"github.com/fgrehm/crib/internal/engine"
	"github.com/spf13/cobra"
)

var recreateFlag bool

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Create or start the workspace container",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		eng, _, store, err := newEngine()
		if err != nil {
			return err
		}
		eng.SetOutput(os.Stdout, os.Stderr)
		eng.SetVerbose(verboseFlag)
		eng.SetProgress(func(msg string) { u.Dim("  " + msg) })

		ws, err := currentWorkspace(store, true)
		if err != nil {
			return err
		}

		u.Dim(versionString())
		u.Header("Starting workspace")

		result, err := eng.Up(cmd.Context(), ws, engine.UpOptions{Recreate: recreateFlag})
		if err != nil {
			return err
		}

		u.Success("Workspace ready")
		u.Keyval("container", shortID(result.ContainerID))
		u.Keyval("workspace", result.WorkspaceFolder)
		if result.RemoteUser != "" {
			u.Keyval("user", result.RemoteUser)
		}
		if ports := formatPortSpecs(result.Ports); ports != "" {
			u.Keyval("ports", ports)
		}

		return nil
	},
}

func init() {
	upCmd.Flags().BoolVar(&recreateFlag, "recreate", false, "recreate container even if one already exists")
}
