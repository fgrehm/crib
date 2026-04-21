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

// disablePluginsFlag collects --disable-plugin values for up/rebuild/restart.
var disablePluginsFlag []string

// addPluginFlags registers the --disable-plugin flag on commands that create
// or recreate containers. The flag accepts repeated values or a
// comma-separated list.
func addPluginFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&disablePluginsFlag, "disable-plugin",
		nil, "disable a bundled plugin by name (repeatable or comma-separated)")
}

// isKnownPlugin reports whether name matches a bundled plugin.
func isKnownPlugin(name string) bool {
	return slices.Contains(knownPlugins, name)
}
