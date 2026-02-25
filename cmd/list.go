package cmd

import (
	"fmt"

	"github.com/fgrehm/crib/internal/workspace"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		store, err := workspace.NewStore()
		if err != nil {
			return err
		}

		ids, err := store.List()
		if err != nil {
			return err
		}

		if len(ids) == 0 {
			u.Dim("No workspaces")
			return nil
		}

		headers := []string{"WORKSPACE", "SOURCE"}
		var rows [][]string
		for _, id := range ids {
			ws, err := store.Load(id)
			if err != nil {
				rows = append(rows, []string{id, fmt.Sprintf("(error: %v)", err)})
				continue
			}
			rows = append(rows, []string{ws.ID, ws.Source})
		}
		u.Table(headers, rows)

		return nil
	},
}
