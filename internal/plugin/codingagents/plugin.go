package codingagents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
)

// Plugin provides coding-agent credential sharing. Currently supports
// Claude Code by copying ~/.claude/.credentials.json and generating a
// minimal ~/.claude.json for the container.
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
// present, it copies the credentials file into the workspace state dir,
// generates a minimal ~/.claude.json with hasCompletedOnboarding, and returns
// bind mounts for both.
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

	// Copy credentials file.
	credsDst := filepath.Join(pluginDir, "credentials.json")
	if err := copyFile(credsSrc, credsDst, 0o600); err != nil {
		return nil, fmt.Errorf("copying credentials: %w", err)
	}

	// Generate minimal config so Claude Code skips onboarding.
	configDst := filepath.Join(pluginDir, "claude.json")
	if err := os.WriteFile(configDst, []byte(`{"hasCompletedOnboarding":true}`), 0o644); err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}

	remoteHome := inferRemoteHome(req.RemoteUser)

	return &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{
			{
				Type:   "bind",
				Source: credsDst,
				Target: filepath.Join(remoteHome, ".claude", ".credentials.json"),
			},
			{
				Type:   "bind",
				Source: configDst,
				Target: filepath.Join(remoteHome, ".claude.json"),
			},
		},
	}, nil
}

// inferRemoteHome returns the home directory path for the given user inside
// the container. Root or empty user maps to /root, others to /home/{user}.
func inferRemoteHome(user string) string {
	if user == "" || user == "root" {
		return "/root"
	}
	return "/home/" + user
}

// copyFile copies a single file from src to dst with the given permissions.
func copyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}
