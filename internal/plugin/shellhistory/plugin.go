package shellhistory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
)

const (
	historyFile = ".shell_history"
	mountDir    = ".crib_history"
)

// Plugin persists shell history across container recreations by bind-mounting
// the history directory from the workspace state directory and setting HISTFILE.
// We mount the directory (not the file) so that shells like zsh can do atomic
// renames when saving history (e.g. writing .shell_history.new then renaming).
type Plugin struct{}

// New creates a shell-history plugin.
func New() *Plugin {
	return &Plugin{}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string { return "shell-history" }

// PreContainerRun creates a history file in the workspace state dir (if it
// doesn't already exist) and returns a bind mount plus HISTFILE env var so
// both bash and zsh write to the persistent location.
func (p *Plugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	pluginDir := filepath.Join(req.WorkspaceDir, "plugins", "shell-history")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating shell-history plugin dir: %w", err)
	}

	histPath := filepath.Join(pluginDir, historyFile)

	// Touch the file if it doesn't exist, preserve contents if it does.
	if _, err := os.Stat(histPath); os.IsNotExist(err) {
		if err := os.WriteFile(histPath, nil, 0o644); err != nil {
			return nil, fmt.Errorf("creating history file: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("checking history file: %w", err)
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)
	mountTarget := filepath.Join(remoteHome, mountDir)

	return &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{
			{
				Type:   "bind",
				Source: pluginDir,
				Target: mountTarget,
			},
		},
		Env: map[string]string{
			"HISTFILE": filepath.Join(mountTarget, historyFile),
		},
	}, nil
}

