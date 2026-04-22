package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/engine"
	"github.com/fgrehm/crib/internal/globalconfig"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/plugin/codingagents"
	"github.com/fgrehm/crib/internal/plugin/dotfiles"
	"github.com/fgrehm/crib/internal/plugin/packagecache"
	"github.com/fgrehm/crib/internal/plugin/shellhistory"
	pluginssh "github.com/fgrehm/crib/internal/plugin/ssh"
	"github.com/fgrehm/crib/internal/ui"
	"github.com/fgrehm/crib/internal/workspace"
	"github.com/spf13/cobra"
)

// noArgs rejects any positional arguments with a clear error message.
// Prefer this over cobra.NoArgs, whose error says "unknown command" which is
// misleading when the extra token is not a subcommand.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%s does not accept arguments", cmd.CommandPath())
	}
	return nil
}

// displayContainerName returns the container name for CLI output. Prefers the
// recorded name (which reflects runArgs --name overrides); falls back to the
// default for compose backends and workspaces created before ContainerName was
// persisted.
func displayContainerName(recorded, wsID string) string {
	if recorded != "" {
		return recorded
	}
	return "crib-" + wsID
}

var (
	debugFlag             bool
	verboseFlag           bool
	configDirFlag         string
	dirFlag               string
	logger                *slog.Logger
	cacheProviders        []string   // loaded from .cribrc cache key
	projectDotfiles       dotfilesRC // loaded from .cribrc dotfiles keys
	projectPluginsDisable []string   // loaded from .cribrc plugins.disable
	projectPluginsOff     bool       // loaded from .cribrc plugins = false
)

// version variables injected at build time via ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "crib",
	Short:   "Dev containers without the ceremony",
	Version: version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		level := slog.LevelWarn
		if debugFlag {
			level = slog.LevelDebug
		}
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					if t, ok := a.Value.Any().(time.Time); ok {
						a.Value = slog.TimeValue(t.UTC())
					}
				}
				return a
			},
		}))

		// Reset .cribrc-derived globals so stale values from a previous
		// rootCmd execution in the same process do not leak into this run
		// (matters for tests that reuse the command tree).
		cacheProviders = nil
		projectDotfiles = dotfilesRC{}
		projectPluginsDisable = nil
		projectPluginsOff = false

		// Apply .cribrc defaults for flags not explicitly set by the user.
		rc, rcErr := loadCribRC()
		if rcErr != nil {
			logger.Debug("could not load .cribrc", "error", rcErr)
		}
		if rc != nil {
			if !cmd.Root().PersistentFlags().Changed("config") && rc.Config != "" {
				configDirFlag = rc.Config
				logger.Debug("loaded config dir from .cribrc", "dir", rc.Config)
			}
			if len(rc.Cache) > 0 {
				cacheProviders = rc.Cache
				logger.Debug("loaded cache providers from .cribrc", "providers", rc.Cache)
			}
			projectDotfiles = rc.Dotfiles
			projectPluginsDisable = rc.PluginsDisable
			projectPluginsOff = rc.PluginsDisableAll
		}

		return nil
	},
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&verboseFlag, "verbose", false, "show detailed output from compose and build commands")
	rootCmd.PersistentFlags().StringVarP(&configDirFlag, "config", "C", "", "devcontainer config directory (e.g. .devcontainer-custom)")
	rootCmd.PersistentFlags().StringVarP(&dirFlag, "dir", "d", "", "project directory to operate on (defaults to current directory)")
	rootCmd.MarkFlagsMutuallyExclusive("config", "dir")
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &errUsage{err: err}
	})
	rootCmd.SetVersionTemplate(fmt.Sprintf("crib version %s\n", version))
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(rebuildCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(cacheCmd)
	rootCmd.AddCommand(pruneCmd)
	rootCmd.AddCommand(versionCmd)
}

// Exit codes.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2 // bad flags, unknown subcommand, missing required args
)

// errUsage wraps an error to signal a usage mistake (exit code 2).
type errUsage struct{ err error }

func (e *errUsage) Error() string { return e.err.Error() }
func (e *errUsage) Unwrap() error { return e.err }

// Execute runs the root command with signal handling and returns the exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.TimeValue(t.UTC())
				}
			}
			return a
		},
	}))
	resetPerExecutionFlags(rootCmd)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		u := newUI()
		u.Error(err.Error())
		fmt.Fprintf(os.Stderr, "\ncrib %s (%s)\n", version, commit)
		var ue *errUsage
		if errors.As(err, &ue) {
			return exitUsage
		}
		return exitError
	}
	return exitOK
}

// newUI creates a UI that writes to stdout and stderr.
func newUI() *ui.UI {
	return ui.New(os.Stdout, os.Stderr)
}

// newEngine creates the OCI driver, workspace store, and engine.
// The compose helper is optional; nil is passed to the engine if compose is not available.
func newEngine() (*engine.Engine, *oci.OCIDriver, *workspace.Store, error) {
	d, err := oci.NewOCIDriver(logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("initializing container runtime: %w", err)
	}

	store, err := workspace.NewStore()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("initializing workspace store: %w", err)
	}

	composeHelper, err := compose.NewHelper(d.Runtime().String(), logger)
	if err != nil {
		logger.Debug("compose not available", "error", err)
		composeHelper = nil
	}

	eng := engine.New(d, composeHelper, store, logger)
	return eng, d, store, nil
}

// currentWorkspace resolves the workspace from the current directory,
// or from the devcontainer config directory if --config / .cribrc is set,
// or from an explicit project directory if --dir is set.
// If create is true and the workspace is not yet in the store, it creates one.
func currentWorkspace(store *workspace.Store, create bool) (*workspace.Workspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	return workspace.Lookup(store, workspace.LookupOptions{
		ConfigDir: configDirFlag,
		Dir:       dirFlag,
		Cwd:       cwd,
		Version:   version,
		Create:    create,
	}, logger)
}

// versionString returns a formatted version string for display.
// For dev builds, includes commit and build timestamp.
func versionString() string {
	v := "crib " + version
	if strings.Contains(version, "-dev") && commit != "unknown" {
		v += " (" + commit
		if date != "unknown" {
			v += ", " + date
		}
		v += ")"
	}
	return v
}

// composePortsToDriver converts compose.PortBinding values to driver.PortBinding
// so the same formatPorts function can be used for both.
func composePortsToDriver(ports []compose.PortBinding) []driver.PortBinding {
	result := make([]driver.PortBinding, len(ports))
	for i, p := range ports {
		result[i] = driver.PortBinding{
			ContainerPort: p.ContainerPort,
			HostPort:      p.HostPort,
			HostIP:        p.HostIP,
			Protocol:      p.Protocol,
		}
	}
	return result
}

// resolveDotfilesPlugin merges global config with per-project overrides and
// returns the effective config and whether the plugin should be registered.
func resolveDotfilesPlugin(gcfg globalconfig.DotfilesConfig, rc dotfilesRC) (globalconfig.DotfilesConfig, bool) {
	if rc.Disabled {
		return globalconfig.DotfilesConfig{}, false
	}

	// Merge: start with global, apply per-project overrides.
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

	// Apply ~/dotfiles default: applyDefaults only ran on the global config
	// struct; the repo may have come from per-project.
	if merged.TargetPath == "" {
		merged.TargetPath = "~/dotfiles"
	}

	return merged, true
}

// setupPlugins creates a plugin manager with bundled plugins and attaches it
// to the engine. Called from commands that create containers (up, rebuild, restart).
//
// Plugins can be disabled via the global config (`[plugins]` with
// `disable = [...]` or `disable_all = true`), `.cribrc`
// (`plugins.disable = ssh, ...` or `plugins = false`), or the
// `--disable-plugin` flag on the current command.
func setupPlugins(cmd *cobra.Command, eng *engine.Engine, d *oci.OCIDriver) {
	eng.SetRuntime(d.Runtime().String())
	mgr := plugin.NewManager(logger)

	var globalCfg globalconfig.Config
	if gcfg, err := globalconfig.Load(); err != nil {
		logger.Warn("failed to load global config", "error", err)
	} else if gcfg != nil {
		globalCfg = *gcfg
	}

	// Warn about unknown names in any disable layer before honoring the kill
	// switch — a typo is a config mistake whether or not plugins are all off.
	disabled := collectDisabledPlugins(globalCfg.Plugins.Disable, projectPluginsDisable, disabledPluginsForCommand(cmd))
	warnUnknownDisabledPlugins(disabled)

	// Kill switch: skip every plugin when any layer asks for it.
	if globalCfg.Plugins.DisableAll || projectPluginsOff {
		eng.SetPlugins(mgr)
		return
	}

	if !disabled["coding-agents"] {
		mgr.Register(codingagents.New())
	}
	if !disabled["shell-history"] {
		mgr.Register(shellhistory.New())
	}
	if !disabled["ssh"] {
		mgr.Register(pluginssh.New())
	}
	if !disabled["dotfiles"] {
		if cfg, ok := resolveDotfilesPlugin(globalCfg.Dotfiles, projectDotfiles); ok {
			mgr.Register(dotfiles.New(cfg))
		}
	}
	if !disabled["package-cache"] && len(cacheProviders) > 0 {
		if unknown := packagecache.ValidateProviders(cacheProviders); len(unknown) > 0 {
			logger.Warn("unknown cache providers in .cribrc", "unknown", unknown, "supported", packagecache.SupportedProviders())
		}
		mgr.Register(packagecache.New(cacheProviders))
		eng.SetBuildCacheMounts(packagecache.BuildCacheMounts(cacheProviders))
	}
	eng.SetPlugins(mgr)
}

// collectDisabledPlugins merges disable entries from every layer into a set.
// Trimmed, empty-filtered.
func collectDisabledPlugins(layers ...[]string) map[string]bool {
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

// warnUnknownDisabledPlugins logs a warning for names that do not match any
// bundled plugin. Typos shouldn't silently disable nothing. Names are sorted
// so log output is stable across runs.
func warnUnknownDisabledPlugins(disabled map[string]bool) {
	unknown := make([]string, 0, len(disabled))
	for name := range disabled {
		if !isKnownPlugin(name) {
			unknown = append(unknown, name)
		}
	}
	slices.Sort(unknown)
	for _, name := range unknown {
		logger.Warn("unknown plugin in disable list", "name", name, "known", knownPlugins)
	}
}

// appendRemoteEnv appends -e KEY=VALUE flags for each entry in result.RemoteEnv.
// result may be nil, in which case args is returned unchanged.
func appendRemoteEnv(args []string, result *workspace.Result) []string {
	if result == nil {
		return args
	}
	for k, v := range result.RemoteEnv {
		args = append(args, "-e", k+"="+v)
	}
	return args
}
