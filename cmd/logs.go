package cmd

import (
	"github.com/fgrehm/crib/internal/engine"
	"github.com/spf13/cobra"
)

var (
	logsFollowFlag bool
	logsTailFlag   string
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show container logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		eng, _, store, err := newEngine()
		if err != nil {
			return err
		}

		ws, err := currentWorkspace(store, false)
		if err != nil {
			return err
		}

		return eng.Logs(cmd.Context(), ws, engine.LogsOptions{
			Follow: logsFollowFlag,
			Tail:   logsTailFlag,
		})
	},
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollowFlag, "follow", "f", false, "follow log output")
	logsCmd.Flags().StringVar(&logsTailFlag, "tail", "", "number of lines to show from the end of the logs")
}
