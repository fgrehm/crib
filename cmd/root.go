package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/engine"
	"github.com/fgrehm/crib/internal/globalconfig"
	"github.com/fgrehm/crib/internal/pluginsetup"
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

// runtimeConfig holds configuration derived from the global config file and
// the project's .cribrc, populated by PersistentPreRunE. Bundled into one
// struct so the reset-before-load sequence is a single assignment and so
// additions don't multiply package-level globals.
type runtimeConfig struct {
	// Global is the fully loaded ~/.config/crib/config.toml.
	Global globalconfig.Config

	// CacheProviders is .cribrc's `cache` list.
	CacheProviders []string

	// ProjectDotfiles is .cribrc's [dotfiles] section (including the kill switch).
	ProjectDotfiles globalconfig.DotfilesRC

	// ProjectPluginsDisable is .cribrc's plugins.disable list.
	ProjectPluginsDisable []string

	// ProjectPluginsOff is true when .cribrc says `plugins = "false"`.
	ProjectPluginsOff bool

	// ProjectWorkspace is .cribrc's [workspace] section. Merged on top of
	// Global.Workspace when configuring the engine: project env wins on key
	// conflicts, mounts concatenate, runArgs are ordered so project values
	// win under the runtime's last-flag-wins semantics.
	ProjectWorkspace globalconfig.WorkspaceConfig
}

var (
	debugFlag     bool
	verboseFlag   bool
	configDirFlag string
	dirFlag       string
	logger        *slog.Logger
	runtimeCfg    runtimeConfig
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

		// Reset runtime config so stale values from a previous rootCmd
		// execution in the same process do not leak into this run (matters
		// for tests that reuse the command tree).
		runtimeCfg = runtimeConfig{}

		// Load global config ~/.config/crib/config.toml once per command.
		if gcfg, err := globalconfig.Load(); err != nil {
			logger.Warn("failed to load global config", "error", err)
		} else if gcfg != nil {
			runtimeCfg.Global = *gcfg
		}

		// Apply .cribrc defaults for flags not explicitly set by the user.
		rc, rcErr := loadProjectCribRC()
		if rcErr != nil {
			logger.Debug("could not load .cribrc", "error", rcErr)
		}
		if rc != nil {
			if !cmd.Root().PersistentFlags().Changed("config") && rc.Config != "" {
				configDirFlag = rc.Config
				logger.Debug("loaded config dir from .cribrc", "dir", rc.Config)
			}
			if len(rc.Cache) > 0 {
				runtimeCfg.CacheProviders = rc.Cache
				logger.Debug("loaded cache providers from .cribrc", "providers", rc.Cache)
			}
			runtimeCfg.ProjectDotfiles = rc.Dotfiles
			runtimeCfg.ProjectPluginsDisable = rc.Plugins.Disable
			runtimeCfg.ProjectPluginsOff = rc.Plugins.DisableAll
			runtimeCfg.ProjectWorkspace = rc.Workspace
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

// loadProjectCribRC loads .cribrc from the project directory. When --dir is
// set, that directory wins; otherwise the current working directory is used.
// --config is not consulted: it points at a devcontainer config directory,
// not a project root, and .cribrc sits at the project root.
func loadProjectCribRC() (*globalconfig.CribRC, error) {
	dir := dirFlag
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = cwd
	}
	return globalconfig.LoadCribRC(filepath.Join(dir, ".cribrc"))
}

// mergeWorkspaceOptions overlays .cribrc's [workspace] section on top of the
// global config's [workspace] section and returns the effective options to
// pass to the engine. The backends treat this entire result as "lower
// priority than devcontainer.json", so within the result we place project
// values where they will beat global ones:
//   - Env: project entries overwrite global entries on key conflict.
//   - Mounts: global first, project second; compose dedupes by target and
//     the single-backend driver will refuse truly duplicate targets, which
//     is the right failure mode.
//   - RunArgs: global first, project second, so when the backend prepends
//     this list to cfg.RunArgs the project .cribrc values sit later in the
//     final -flag list and win on key conflicts under last-flag-wins.
func mergeWorkspaceOptions(global, project globalconfig.WorkspaceConfig) engine.GlobalWorkspaceOptions {
	out := engine.GlobalWorkspaceOptions{}

	if len(global.Env) > 0 || len(project.Env) > 0 {
		out.Env = make(map[string]string, len(global.Env)+len(project.Env))
		maps.Copy(out.Env, global.Env)
		maps.Copy(out.Env, project.Env)
	}

	if len(global.Mounts) > 0 || len(project.Mounts) > 0 {
		out.Mounts = make([]string, 0, len(global.Mounts)+len(project.Mounts))
		out.Mounts = append(out.Mounts, global.Mounts...)
		out.Mounts = append(out.Mounts, project.Mounts...)
	}

	if len(global.RunArgs) > 0 || len(project.RunArgs) > 0 {
		out.RunArgs = make([]string, 0, len(global.RunArgs)+len(project.RunArgs))
		out.RunArgs = append(out.RunArgs, global.RunArgs...)
		out.RunArgs = append(out.RunArgs, project.RunArgs...)
	}

	return out
}

// setupPlugins builds pluginsetup.Opts from the loaded global + project
// config and CLI flags, then wires the resulting manager and cache mounts
// into the engine. Called from commands that create containers (up, rebuild,
// restart).
func setupPlugins(cmd *cobra.Command, eng *engine.Engine, d *oci.OCIDriver) {
	eng.SetRuntime(d.Runtime().String())
	eng.SetGlobalWorkspace(mergeWorkspaceOptions(runtimeCfg.Global.Workspace, runtimeCfg.ProjectWorkspace))

	result := pluginsetup.Configure(pluginsetup.Opts{
		GlobalDisable:     runtimeCfg.Global.Plugins.Disable,
		GlobalDisableAll:  runtimeCfg.Global.Plugins.DisableAll,
		ProjectDisable:    runtimeCfg.ProjectPluginsDisable,
		ProjectDisableAll: runtimeCfg.ProjectPluginsOff,
		CLIDisable:        disabledPluginsForCommand(cmd),
		GlobalDotfiles:    runtimeCfg.Global.Dotfiles,
		ProjectDotfiles:   runtimeCfg.ProjectDotfiles,
		CacheProviders:    runtimeCfg.CacheProviders,
	}, logger)

	eng.SetPlugins(result.Manager)
	if len(result.BuildCacheMounts) > 0 {
		eng.SetBuildCacheMounts(result.BuildCacheMounts)
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
