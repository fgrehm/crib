package codingagents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgrehm/crib/internal/plugin"
)

// Plugin provides coding-agent credential sharing. Currently supports
// Claude Code by staging ~/.claude/.credentials.json and a minimal
// ~/.claude.json, then requesting they be copied into the container.
type Plugin struct {
	homeDir string // overridable for testing; defaults to os.UserHomeDir()
}

// New creates a coding-agents plugin that uses the real user home directory.
func New() *Plugin {
	return &Plugin{}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string { return "coding-agents" }

// PreContainerRun checks for ~/.claude/.credentials.json on the host. If
// present, it stages the credentials and a minimal config in the workspace
// state dir, then returns file copies to inject into the container.
func (p *Plugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	home := p.homeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
	}

	credsSrc := filepath.Join(home, ".claude", ".credentials.json")
	if _, err := os.Stat(credsSrc); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking credentials: %w", err)
	}

	pluginDir := filepath.Join(req.WorkspaceDir, "plugins", "coding-agents")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plugin dir: %w", err)
	}

	// Stage credentials file.
	credsDst := filepath.Join(pluginDir, "credentials.json")
	if err := copyFile(credsSrc, credsDst, 0o600); err != nil {
		return nil, fmt.Errorf("staging credentials: %w", err)
	}

	// Generate minimal config so Claude Code skips onboarding.
	configDst := filepath.Join(pluginDir, "claude.json")
	if err := os.WriteFile(configDst, []byte(`{"hasCompletedOnboarding":true}`), 0o644); err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)
	owner := req.RemoteUser
	if owner == "" {
		owner = "root"
	}

	return &plugin.PreContainerRunResponse{
		Copies: []plugin.FileCopy{
			{
				Source: credsDst,
				Target: filepath.Join(remoteHome, ".claude", ".credentials.json"),
				Mode:   "0600",
				User:   owner,
			},
			{
				Source: configDst,
				Target: filepath.Join(remoteHome, ".claude.json"),
				User:   owner,
			},
		},
	}, nil
}

// copyFile copies a single file from src to dst with the given permissions.
func copyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}
