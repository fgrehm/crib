package codingagents

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
)

// handlePi mirrors Claude's behavior for pi. Workspace mode always creates
// a persistent state directory and bind-mounts it over `~/.pi/agent/`. Host
// mode copies `~/.pi/agent/auth.json` into the container only if the file
// exists on the host.
func (p *Plugin) handlePi(req *plugin.PreContainerRunRequest, mode string) (*plugin.PreContainerRunResponse, error) {
	if mode == "workspace" {
		return p.piWorkspaceMode(req)
	}

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
		slog.Warn("coding-agents: pi auth path is not a regular file, skipping pi injection", "path", authSrc)
		return nil, nil
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
// created inside the container survive rebuilds.
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
