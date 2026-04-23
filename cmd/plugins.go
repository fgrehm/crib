package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// addPluginFlags registers the --disable-plugin flag on commands that create
// or recreate containers. The flag accepts repeated values or a
// comma-separated list. Values are read per-invocation via
// disabledPluginsForCommand so state does not leak across cobra executions.
func addPluginFlags(cmd *cobra.Command) {
	cmd.Flags().StringSlice("disable-plugin",
		nil, "disable a bundled plugin by name (repeatable or comma-separated)")
}

// disabledPluginsForCommand returns the parsed --disable-plugin values for cmd.
// Returns nil if the flag is unset or not registered on the command.
func disabledPluginsForCommand(cmd *cobra.Command) []string {
	if cmd == nil {
		return nil
	}
	vals, err := cmd.Flags().GetStringSlice("disable-plugin")
	if err != nil {
		return nil
	}
	return vals
}

// resetPerExecutionFlags clears parsed state for flags that must not leak
// across Execute() calls in the same process. pflag retains the slice value
// and the Changed bit between invocations of cobra's parser, so a stray
// --disable-plugin from a previous run would otherwise affect a later run
// where the flag is absent.
func resetPerExecutionFlags(cmd *cobra.Command) {
	for _, c := range cmd.Commands() {
		if f := c.Flags().Lookup("disable-plugin"); f != nil {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			}
			f.Changed = false
		}
		resetPerExecutionFlags(c)
	}
}
