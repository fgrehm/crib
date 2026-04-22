package cmd

import (
	"slices"

	"github.com/spf13/cobra"
)

// knownPlugins is the canonical set of bundled plugin names. Used for
// warning on unrecognized disable entries.
var knownPlugins = []string{
	"coding-agents",
	"shell-history",
	"ssh",
	"dotfiles",
	"package-cache",
}

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

// isKnownPlugin reports whether name matches a bundled plugin.
func isKnownPlugin(name string) bool {
	return slices.Contains(knownPlugins, name)
}
