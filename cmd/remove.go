package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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
				fmt.Fprintf(os.Stderr, "  container: %s\n", preview.ContainerID[:12])
			}
			for _, img := range preview.Images {
				fmt.Fprintf(os.Stderr, "  image: %s\n", img)
			}
			fmt.Fprintf(os.Stderr, "  state: ~/.crib/workspaces/%s/\n", ws.ID)

			if !stdinIsTerminal() {
				return fmt.Errorf("removal requires confirmation; use --force to skip (stdin is not a terminal)")
			}
			fmt.Fprint(os.Stderr, "Continue? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
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
