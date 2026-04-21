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
// Claude Code and pi by staging credentials and injecting them into the
// container.
//
// Claude Code: ~/.claude/.credentials.json and ~/.claude.json
// pi: ~/.pi/agent/auth.json
//
// When configured with "credentials": "workspace" in devcontainer.json
// customizations, the plugin instead bind-mounts persistent directories
// so credentials created inside the container survive rebuilds.
type Plugin struct {
	plugin.BasePlugin
	homeDir string // overridable for testing; defaults to os.UserHomeDir()
}

// New creates a coding-agents plugin that uses the real user home directory.
func New() *Plugin {
	return &Plugin{}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string { return "coding-agents" }

// PreContainerRun dispatches credential handling for Claude Code and pi.
// Both agents share the single `credentials` customization ("host" default,
// or "workspace"). pi is only active when `~/.pi/agent/auth.json` exists on
// the host; otherwise crib produces no pi artifacts regardless of mode.
func (p *Plugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	mode := getCredentialsMode(req.Customizations)

	var resp *plugin.PreContainerRunResponse
	var err error
	if mode == "workspace" {
		resp, err = p.workspaceMode(req)
	} else {
		resp, err = p.hostMode(req)
	}
	if err != nil {
		return nil, err
	}

	piResp, err := p.handlePi(req, mode)
	if err != nil {
		return nil, err
	}
	if piResp != nil {
		if resp == nil {
			resp = &plugin.PreContainerRunResponse{}
		}
		resp.Mounts = append(resp.Mounts, piResp.Mounts...)
		resp.Copies = append(resp.Copies, piResp.Copies...)
	}

	return resp, nil
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

// handlePi routes pi credential handling based on the shared credentials mode.
// pi is only active when `~/.pi/agent/auth.json` exists on the host, which is
// the user-visible opt-in gesture. In host mode the auth file is copied into
// the container; in workspace mode a persistent state directory is
// bind-mounted over `~/.pi/agent/` so credentials created inside the
// container survive rebuilds.
func (p *Plugin) handlePi(req *plugin.PreContainerRunRequest, mode string) (*plugin.PreContainerRunResponse, error) {
	home := p.homeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
	}

	authSrc := filepath.Join(home, ".pi", "agent", "auth.json")
	info, err := os.Stat(authSrc)
	if os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking pi auth: %w", err)
	}
	// Treat anything other than a regular file (directory, socket, etc.) as
	// "not enabled" so we never try to CopyFile something we can't read.
	if !info.Mode().IsRegular() {
		return nil, nil
	}

	if mode == "workspace" {
		return p.piWorkspaceMode(req)
	}
	return p.piHostMode(req, authSrc)
}

// piHostMode stages pi credentials from authSrc and copies them into the container.
func (p *Plugin) piHostMode(req *plugin.PreContainerRunRequest, authSrc string) (*plugin.PreContainerRunResponse, error) {
	pluginDir := filepath.Join(req.WorkspaceDir, "plugins", "coding-agents", "pi")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plugin dir: %w", err)
	}

	// Stage auth file.
	authDst := filepath.Join(pluginDir, "auth.json")
	if err := plugin.CopyFile(authSrc, authDst, 0o600); err != nil {
		return nil, fmt.Errorf("staging pi auth: %w", err)
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)
	owner := plugin.InferOwner(req.RemoteUser)

	return &plugin.PreContainerRunResponse{
		Copies: []plugin.FileCopy{
			{
				Source: authDst,
				Target: filepath.Join(remoteHome, ".pi", "agent", "auth.json"),
				Mode:   "0600",
				User:   owner,
			},
		},
	}, nil
}

// piWorkspaceMode bind-mounts a persistent directory for pi so credentials
// created inside the container survive rebuilds. User can authenticate inside
// the container and credentials persist across rebuilds.
func (p *Plugin) piWorkspaceMode(req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	stateDir := filepath.Join(req.WorkspaceDir, "plugins", "coding-agents", "pi-state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating pi state dir: %w", err)
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)

	return &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{
			{
				Type:   "bind",
				Source: stateDir,
				Target: filepath.Join(remoteHome, ".pi", "agent"),
			},
		},
	}, nil
}
