package cmd

import (
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:     "down",
	Aliases: []string{"stop"},
	Short:   "Stop and remove the current workspace container",
	Args:    noArgs,
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
		lock, err := store.Lock(ws.ID)
		if err != nil {
			return err
		}
		defer lock.Unlock() //nolint:errcheck // best-effort cleanup

		u.Dim(versionString())

		if err := eng.Down(cmd.Context(), ws); err != nil {
			return err
		}

		u.Success("Stopped " + ws.ID)
		return nil
	},
}
