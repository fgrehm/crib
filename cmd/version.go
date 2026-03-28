package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Args:  noArgs,
	Run: func(cmd *cobra.Command, args []string) {
		u := newUI()
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "crib "+version)
		u.Keyval("commit", commit)
		u.Keyval("date", date)
		u.Keyval("go", runtime.Version())
	},
}
