package cmd

import (
	"path/filepath"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/workspace"
)

// liveRemoteUser reads remoteUser/containerUser from the current
// devcontainer.json for the given workspace. Returns "" on parse failure or
// missing config. Used by shell, exec, and run to ensure edits to remoteUser
// take effect without requiring a rebuild.
func liveRemoteUser(ws *workspace.Workspace) string {
	cfgPath := filepath.Join(ws.Source, ws.DevContainerPath)
	cfg, err := config.Parse(cfgPath)
	if err != nil {
		return ""
	}
	if cfg.RemoteUser != "" {
		return cfg.RemoteUser
	}
	return cfg.ContainerUser
}
