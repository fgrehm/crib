package cmd

import (
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete the current workspace container and state",
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

		if err := eng.Delete(cmd.Context(), ws); err != nil {
			return err
		}

		u.Success("Deleted " + ws.ID)
		return nil
	},
}
