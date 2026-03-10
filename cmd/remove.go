package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var removeForceFlag bool

var removeCmd = &cobra.Command{
	Use:     "remove",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove the current workspace container, images, and state",
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

		u.Dim(versionString())

		if !removeForceFlag {
			preview := eng.PreviewRemove(cmd.Context(), ws)

			fmt.Fprintf(os.Stderr, "Will remove workspace %q:\n", ws.ID)
			if preview.ContainerID != "" {
				cid := preview.ContainerID
				if len(cid) > 12 {
					cid = cid[:12]
				}
				fmt.Fprintf(os.Stderr, "  container: %s\n", cid)
			}
			for _, img := range preview.Images {
				fmt.Fprintf(os.Stderr, "  image: %s\n", img)
			}
			fmt.Fprintf(os.Stderr, "  state: %s\n", store.WorkspaceDir(ws.ID))

			confirmed, err := confirmPrompt("removal requires confirmation")
			if err != nil {
				return err
			}
			if !confirmed {
				u.Dim("Aborted")
				return nil
			}
		}

		if err := eng.Remove(cmd.Context(), ws); err != nil {
			return err
		}

		u.Success("Removed " + ws.ID)
		return nil
	},
}

func init() {
	removeCmd.Flags().BoolVarP(&removeForceFlag, "force", "f", false, "skip confirmation prompt")
}
