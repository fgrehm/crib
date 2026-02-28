package cmd

import (
	"os"

	"github.com/fgrehm/crib/internal/engine"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/plugin/codingagents"
	"github.com/spf13/cobra"
)

var recreateFlag bool

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Create or start the workspace container",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		eng, d, store, err := newEngine()
		if err != nil {
			return err
		}
		eng.SetOutput(os.Stdout, os.Stderr)
		eng.SetVerbose(verboseFlag || debugFlag)
		eng.SetProgress(func(msg string) { u.Dim("  " + msg) })
		eng.SetRuntime(d.Runtime().String())

		mgr := plugin.NewManager(logger)
		mgr.Register(codingagents.New())
		eng.SetPlugins(mgr)

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
		if ports := formatPorts(result.Ports); ports != "" {
			u.Keyval("ports", ports)
		}

		return nil
	},
}

func init() {
	upCmd.Flags().BoolVar(&recreateFlag, "recreate", false, "recreate container even if one already exists")
}
