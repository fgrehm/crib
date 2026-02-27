package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"ps"},
	Short:   "Show the status of the current workspace container",
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

		result, err := eng.Status(cmd.Context(), ws)
		if err != nil {
			return err
		}

		u.Dim(versionString())
		u.Header(ws.ID)
		fmt.Printf("%-12s%s\n", "source", ws.Source)

		if result.Container == nil {
			fmt.Printf("%-12s%s\n", "status", u.StatusColor("no container"))
			return nil
		}

		fmt.Printf("%-12s%s\n", "container", shortID(result.Container.ID))
		fmt.Printf("%-12s%s\n", "status", u.StatusColor(result.Container.State.Status))

		if len(result.Services) > 0 {
			fmt.Println("services")
			for _, svc := range result.Services {
				u.Keyval(svc.Service, u.StatusColor(svc.State))
			}
		}

		return nil
	},
}
