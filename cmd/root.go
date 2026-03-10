package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/engine"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/plugin/codingagents"
	"github.com/fgrehm/crib/internal/plugin/packagecache"
	"github.com/fgrehm/crib/internal/plugin/shellhistory"
	pluginssh "github.com/fgrehm/crib/internal/plugin/ssh"
	"github.com/fgrehm/crib/internal/ui"
	"github.com/fgrehm/crib/internal/workspace"
	"github.com/spf13/cobra"
)

// errNoContainer is returned when a command requires a running container but
// none exists for the workspace. Used by exec, run, and shell.
var errNoContainer = fmt.Errorf("no container found (run 'crib up' first)")

// noArgs rejects any positional arguments with a clear error message.
// Prefer this over cobra.NoArgs, whose error says "unknown command" which is
// misleading when the extra token is not a subcommand.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%s does not accept arguments", cmd.CommandPath())
	}
	return nil
}

var (
	debugFlag      bool
	verboseFlag    bool
	configDirFlag  string
	dirFlag        string
	logger         *slog.Logger
	cacheProviders []string // loaded from .cribrc cache key
)

// Version variables injected at build time.
var (
	Version = "dev"
	Commit  = "unknown"
	Built   = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "crib",
	Short:   "Dev containers without the ceremony",
	Version: Version,
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
	rootCmd.SetVersionTemplate(fmt.Sprintf("crib version %s\n", Version))
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(downCmd)
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
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		u := newUI()
		u.Error(err.Error())
		fmt.Fprintf(os.Stderr, "\ncrib %s (%s)\n", Version, Commit)
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
	var (
		rr  *workspace.ResolveResult
		err error
	)

	switch {
	case configDirFlag != "":
		rr, err = workspace.ResolveConfigDir(configDirFlag)
	case dirFlag != "":
		rr, err = workspace.Resolve(dirFlag)
	default:
		var cwd string
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}
		rr, err = workspace.Resolve(cwd)
	}
	if err != nil {
		return nil, err
	}

	ws, err := store.Load(rr.WorkspaceID)
	if err != nil && !errors.Is(err, workspace.ErrWorkspaceNotFound) {
		return nil, err
	}

	if ws == nil {
		if !create {
			return nil, fmt.Errorf("no workspace for this directory (run 'crib up' first)")
		}
		now := time.Now()
		ws = &workspace.Workspace{
			ID:               rr.WorkspaceID,
			Source:           rr.ProjectRoot,
			DevContainerPath: rr.RelativeConfigPath,
			CreatedAt:        now,
			LastUsedAt:       now,
		}
		if err := store.Save(ws); err != nil {
			return nil, fmt.Errorf("saving workspace: %w", err)
		}
	}

	return ws, nil
}

// versionString returns a formatted version string for display.
// For dev builds, includes commit and build timestamp.
func versionString() string {
	v := "crib " + Version
	if strings.Contains(Version, "-dev") && Commit != "unknown" {
		v += " (" + Commit
		if Built != "unknown" {
			v += ", " + Built
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

// setupPlugins creates a plugin manager with bundled plugins and attaches it
// to the engine. Called from commands that create containers (up, rebuild, restart).
func setupPlugins(eng *engine.Engine, d *oci.OCIDriver) {
	eng.SetRuntime(d.Runtime().String())
	mgr := plugin.NewManager(logger)
	mgr.Register(codingagents.New())
	mgr.Register(shellhistory.New())
	mgr.Register(pluginssh.New())
	if len(cacheProviders) > 0 {
		if unknown := packagecache.ValidateProviders(cacheProviders); len(unknown) > 0 {
			logger.Warn("unknown cache providers in .cribrc", "unknown", unknown, "supported", packagecache.SupportedProviders())
		}
		mgr.Register(packagecache.New(cacheProviders))
		eng.SetBuildCacheMounts(packagecache.BuildCacheMounts(cacheProviders))
	}
	eng.SetPlugins(mgr)
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
