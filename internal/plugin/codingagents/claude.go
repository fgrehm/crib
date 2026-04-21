package codingagents

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
)

// handleClaude dispatches Claude Code credential handling based on the
// configured mode.
func (p *Plugin) handleClaude(req *plugin.PreContainerRunRequest, mode string) (*plugin.PreContainerRunResponse, error) {
	if mode == "workspace" {
		return p.claudeWorkspaceMode(req)
	}
	return p.claudeHostMode(req)
}

// claudeHostMode is the default behavior: copy host ~/.claude/.credentials.json
// and a minimal ~/.claude.json into the container.
func (p *Plugin) claudeHostMode(req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
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

	pluginDir := filepath.Join(req.WorkspaceDir, "plugins", "coding-agents", "claude-code")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plugin dir: %w", err)
	}

	// Stage credentials file.
	credsDst := filepath.Join(pluginDir, "credentials.json")
	if err := plugin.CopyFile(credsSrc, credsDst, 0o600); err != nil {
		return nil, fmt.Errorf("staging credentials: %w", err)
	}

	// Generate minimal config so Claude Code skips onboarding.
	configDst := filepath.Join(pluginDir, "claude.json")
	if err := os.WriteFile(configDst, []byte(`{"hasCompletedOnboarding":true}`), 0o644); err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)
	owner := plugin.InferOwner(req.RemoteUser)

	return &plugin.PreContainerRunResponse{
		Copies: []plugin.FileCopy{
			{
				Source: credsDst,
				Target: filepath.Join(remoteHome, ".claude", ".credentials.json"),
				Mode:   "0600",
				User:   owner,
			},
			{
				Source:      configDst,
				Target:      filepath.Join(remoteHome, ".claude.json"),
				User:        owner,
				IfNotExists: true,
			},
		},
	}, nil
}

// claudeWorkspaceMode bind-mounts a persistent directory for ~/.claude/ so
// credentials created inside the container survive rebuilds. A minimal
// ~/.claude.json is injected via FileCopy (not bind-mount) because
// Claude Code does atomic renames on that file.
func (p *Plugin) claudeWorkspaceMode(req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	stateDir := filepath.Join(req.WorkspaceDir, "plugins", "coding-agents", "claude-state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating state dir: %w", err)
	}

	// Generate minimal config so Claude Code skips onboarding on first run.
	// This is re-injected on each rebuild via FileCopy.
	configDir := filepath.Join(req.WorkspaceDir, "plugins", "coding-agents", "claude-code")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plugin dir: %w", err)
	}
	configDst := filepath.Join(configDir, "claude.json")
	if err := os.WriteFile(configDst, []byte(`{"hasCompletedOnboarding":true}`), 0o644); err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)
	owner := plugin.InferOwner(req.RemoteUser)

	return &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{
			{
				Type:   "bind",
				Source: stateDir,
				Target: filepath.Join(remoteHome, ".claude"),
			},
		},
		Copies: []plugin.FileCopy{
			{
				Source:      configDst,
				Target:      filepath.Join(remoteHome, ".claude.json"),
				User:        owner,
				IfNotExists: true,
			},
		},
	}, nil
}
