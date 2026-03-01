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
// Claude Code by staging ~/.claude/.credentials.json and a minimal
// ~/.claude.json, then requesting they be copied into the container.
//
// When configured with "credentials": "workspace" in devcontainer.json
// customizations, the plugin instead bind-mounts a persistent directory
// for ~/.claude/ so credentials created inside the container survive
// rebuilds. The user authenticates inside the container on first use.
type Plugin struct {
	homeDir string // overridable for testing; defaults to os.UserHomeDir()
}

// New creates a coding-agents plugin that uses the real user home directory.
func New() *Plugin {
	return &Plugin{}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string { return "coding-agents" }

// PreContainerRun checks the credentials mode from devcontainer.json
// customizations and either copies host credentials into the container
// ("host" mode, default) or bind-mounts a persistent directory for
// in-container authentication ("workspace" mode).
func (p *Plugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	mode := getCredentialsMode(req.Customizations)
	if mode == "workspace" {
		return p.workspaceMode(req)
	}
	return p.hostMode(req)
}

// hostMode is the default behavior: copy host ~/.claude/.credentials.json
// and a minimal ~/.claude.json into the container.
func (p *Plugin) hostMode(req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
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

// workspaceMode bind-mounts a persistent directory for ~/.claude/ so
// credentials created inside the container survive rebuilds. A minimal
// ~/.claude.json is injected via FileCopy (not bind-mount) because
// Claude Code does atomic renames on that file.
func (p *Plugin) workspaceMode(req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
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

// getCredentialsMode reads the credentials mode from customizations.crib.coding-agents.
// Returns "host" (default) or "workspace".
func getCredentialsMode(customizations map[string]any) string {
	if customizations == nil {
		return "host"
	}
	caConfig, ok := customizations["coding-agents"]
	if !ok {
		return "host"
	}
	m, ok := caConfig.(map[string]any)
	if !ok {
		return "host"
	}
	if creds, ok := m["credentials"].(string); ok && creds == "workspace" {
		return "workspace"
	}
	return "host"
}
