package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the workspace container",
	Long: `Restart the workspace container, picking up safe config changes.

If volumes, mounts, ports, environment variables, or other container runtime
settings changed in devcontainer.json (or docker-compose files), the container
is automatically recreated with the new configuration. Only the resume-flow
lifecycle hooks (postStartCommand, postAttachCommand) run — creation hooks
are skipped, making restart much faster than a full rebuild.

If image-affecting changes are detected (image, Dockerfile, features, build
args), restart will ask you to run 'crib rebuild' instead.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		eng, _, store, err := newEngine()
		if err != nil {
			return err
		}
		eng.SetOutput(os.Stdout, os.Stderr)
		eng.SetVerbose(verboseFlag || debugFlag)
		eng.SetProgress(func(msg string) { u.Dim("  " + msg) })

		ws, err := currentWorkspace(store, false)
		if err != nil {
			return err
		}

		u.Dim(versionString())
		u.Header("Restarting workspace")

		result, err := eng.Restart(cmd.Context(), ws)
		if err != nil {
			return err
		}

		if result.Recreated {
			u.Success("Workspace recreated")
		} else {
			u.Success("Workspace restarted")
		}
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
