// Package pluginsetup wires bundled plugins into a plugin.Manager, applying
// the disable-list / kill-switch precedence across global config, project
// .cribrc, and CLI flags.
package pluginsetup

import (
	"log/slog"
	"slices"
	"strings"

	"github.com/fgrehm/crib/internal/globalconfig"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/plugin/codingagents"
	"github.com/fgrehm/crib/internal/plugin/dotfiles"
	"github.com/fgrehm/crib/internal/plugin/packagecache"
	"github.com/fgrehm/crib/internal/plugin/shellhistory"
	pluginssh "github.com/fgrehm/crib/internal/plugin/ssh"
)

var knownPlugins = []string{
	"coding-agents",
	"shell-history",
	"ssh",
	"dotfiles",
	"package-cache",
}

func isKnown(name string) bool {
	return slices.Contains(knownPlugins, name)
}

// Opts holds the inputs for plugin setup.
type Opts struct {
	// GlobalDisable is the plugin disable list from the global config
	// ([plugins] disable = [...]).
	GlobalDisable []string

	// GlobalDisableAll is the kill switch from the global config
	// ([plugins] disable_all = true).
	GlobalDisableAll bool

	// ProjectDisable is the plugin disable list from .cribrc
	// ([plugins] disable = [...] or plugins.disable = "a, b").
	ProjectDisable []string

	// ProjectDisableAll is the kill switch from .cribrc
	// (plugins = "false").
	ProjectDisableAll bool

	// CLIDisable is the plugin disable list passed via --disable-plugin.
	CLIDisable []string

	// GlobalDotfiles is the dotfiles section of the global config.
	GlobalDotfiles globalconfig.DotfilesConfig

	// ProjectDotfiles is the dotfiles section of .cribrc.
	ProjectDotfiles globalconfig.DotfilesRC

	// CacheProviders lists package cache providers for the package-cache plugin.
	CacheProviders []string
}

// Result holds the outputs of plugin setup.
type Result struct {
	// Manager is the configured plugin manager, ready to attach to the engine.
	Manager *plugin.Manager

	// Plugins lists the names of plugins Configure registered on Manager, in
	// registration order. Exposed so callers (and tests) can introspect the
	// effective plugin set without reaching into Manager internals.
	Plugins []string

	// BuildCacheMounts lists BuildKit cache mount targets for feature builds
	// (derived from the package-cache plugin when registered).
	BuildCacheMounts []string
}

// Configure creates a plugin manager and registers bundled plugins according
// to the merged disable precedence (global + project + CLI). The kill switch
// from any layer skips every plugin; warnings for unknown plugin names are
// logged before the kill switch is honored so typos stay visible.
func Configure(opts Opts, logger *slog.Logger) *Result {
	mgr := plugin.NewManager(logger)
	result := &Result{Manager: mgr}

	register := func(p plugin.Plugin) {
		mgr.Register(p)
		result.Plugins = append(result.Plugins, p.Name())
	}

	disabled := collectDisabled(opts.GlobalDisable, opts.ProjectDisable, opts.CLIDisable)
	warnUnknown(disabled, logger)

	if opts.GlobalDisableAll || opts.ProjectDisableAll {
		return result
	}

	if !disabled["coding-agents"] {
		register(codingagents.New())
	}
	if !disabled["shell-history"] {
		register(shellhistory.New())
	}
	if !disabled["ssh"] {
		register(pluginssh.New())
	}
	if !disabled["dotfiles"] {
		if cfg, ok := ResolveDotfiles(opts.GlobalDotfiles, opts.ProjectDotfiles); ok {
			register(dotfiles.New(cfg))
		}
	}
	if !disabled["package-cache"] && len(opts.CacheProviders) > 0 {
		if unknown := packagecache.ValidateProviders(opts.CacheProviders); len(unknown) > 0 {
			logger.Warn("unknown cache providers in .cribrc", "unknown", unknown, "supported", packagecache.SupportedProviders())
		}
		register(packagecache.New(opts.CacheProviders))
		result.BuildCacheMounts = packagecache.BuildCacheMounts(opts.CacheProviders)
	}

	return result
}

func collectDisabled(layers ...[]string) map[string]bool {
	out := map[string]bool{}
	for _, layer := range layers {
		for _, name := range layer {
			name = strings.TrimSpace(name)
			if name != "" {
				out[name] = true
			}
		}
	}
	return out
}

func warnUnknown(disabled map[string]bool, logger *slog.Logger) {
	unknown := make([]string, 0, len(disabled))
	for name := range disabled {
		if !isKnown(name) {
			unknown = append(unknown, name)
		}
	}
	slices.Sort(unknown)
	for _, name := range unknown {
		logger.Warn("unknown plugin in disable list", "name", name, "known", knownPlugins)
	}
}

// ResolveDotfiles merges the global dotfiles config with per-project overrides
// and reports whether the plugin should be registered.
func ResolveDotfiles(gcfg globalconfig.DotfilesConfig, rc globalconfig.DotfilesRC) (globalconfig.DotfilesConfig, bool) {
	if rc.Disabled {
		return globalconfig.DotfilesConfig{}, false
	}

	merged := gcfg
	if rc.Repository != "" {
		merged.Repository = rc.Repository
	}
	if rc.TargetPath != "" {
		merged.TargetPath = rc.TargetPath
	}
	if rc.InstallCommand != "" {
		merged.InstallCommand = rc.InstallCommand
	}

	if merged.Repository == "" {
		return globalconfig.DotfilesConfig{}, false
	}

	// Default target path when the repo came from .cribrc (Config.applyDefaults
	// only runs on the global struct).
	if merged.TargetPath == "" {
		merged.TargetPath = "~/dotfiles"
	}

	return merged, true
}
