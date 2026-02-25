package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		u := newUI()
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "crib "+Version)
		u.Keyval("commit", Commit)
		u.Keyval("built", Built)
		u.Keyval("go", runtime.Version())
	},
}
