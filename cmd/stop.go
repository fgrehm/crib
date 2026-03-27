package cmd

import (
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the workspace container",
	Long:  "Stop the workspace container without removing it. Hook markers are preserved so the next 'up' resumes with only postStartCommand and postAttachCommand. Use 'down' to remove the container and re-run all hooks on next 'up'.",
	Args:  noArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		eng, _, store, err := newEngine()
		if err != nil {
			return err
		}

		ws, err := currentWorkspace(store, false)
		if err != nil {
			return err
		}
		lock, err := store.Lock(cmd.Context(), ws.ID)
		if err != nil {
			return err
		}
		defer lock.Unlock() //nolint:errcheck // best-effort cleanup

		u.Dim(versionString())

		if err := eng.Stop(cmd.Context(), ws); err != nil {
			return err
		}

		u.Success("Stopped " + ws.ID)
		return nil
	},
}
