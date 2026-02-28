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
	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/engine"
	"github.com/fgrehm/crib/internal/ui"
	"github.com/fgrehm/crib/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	debugFlag     bool
	verboseFlag   bool
	configDirFlag string
	dirFlag       string
	logger        *slog.Logger
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
		if !cmd.Root().PersistentFlags().Changed("config") {
			if rc, err := loadCribRC(); err != nil {
				logger.Debug("could not load .cribrc", "error", err)
			} else if rc != nil && rc.Config != "" {
				configDirFlag = rc.Config
				logger.Debug("loaded config dir from .cribrc", "dir", rc.Config)
			}
		}

		return nil
	},
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "V", false, "show detailed output from compose and build commands")
	rootCmd.PersistentFlags().StringVarP(&configDirFlag, "config", "C", "", "devcontainer config directory (e.g. .devcontainer-custom)")
	rootCmd.PersistentFlags().StringVarP(&dirFlag, "dir", "d", "", "project directory to operate on (defaults to current directory)")
	rootCmd.MarkFlagsMutuallyExclusive("config", "dir")
	rootCmd.SetVersionTemplate(fmt.Sprintf("crib version %s\n", Version))
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(rebuildCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(versionCmd)
}

// Execute runs the root command with signal handling.
func Execute() {
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
		os.Exit(1)
	}
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

// shortID returns the first 12 characters of a container ID.
func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

// formatPortSpecs formats publish specs (e.g. "8080:8080") for display.
// Returns empty string if no ports.
func formatPortSpecs(ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	return strings.Join(ports, ", ")
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
