package cmd

import (
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the current workspace container",
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

		container, err := eng.Status(cmd.Context(), ws)
		if err != nil {
			return err
		}

		u.Keyval("workspace", ws.ID)
		u.Keyval("source", ws.Source)

		if container == nil {
			u.Keyval("status", u.StatusColor("no container"))
			return nil
		}

		u.Keyval("container", shortID(container.ID))
		u.Keyval("status", u.StatusColor(container.State.Status))
		return nil
	},
}
