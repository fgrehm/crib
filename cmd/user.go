package cmd

import (
	"os"
	"path/filepath"
	"strings"

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

	// Mirror Engine.parseAndSubstitute: resolve workspaceFolder and substitute
	// local-path variables so the SubstitutionContext has a concrete
	// ContainerWorkspaceFolder. We skip the post-substitution re-resolve
	// since we only need remoteUser/containerUser, not workspaceFolder.
	workspaceFolder := cfg.WorkspaceFolder
	if workspaceFolder == "" {
		workspaceFolder = "/workspaces/" + filepath.Base(ws.Source)
	}
	workspaceFolder = strings.NewReplacer(
		"${localWorkspaceFolder}", ws.Source,
		"${localWorkspaceFolderBasename}", filepath.Base(ws.Source),
	).Replace(workspaceFolder)

	subCtx := &config.SubstitutionContext{
		DevContainerID:           ws.ID,
		LocalWorkspaceFolder:     ws.Source,
		ContainerWorkspaceFolder: workspaceFolder,
		Env:                      envMap(),
	}

	cfg, err = config.Substitute(subCtx, cfg)
	if err != nil {
		// Fall back to stored result in result.json if substitution fails.
		return ""
	}

	// RemoteUser/ContainerUser are already substituted by config.Substitute above.
	// No need to re-resolve workspaceFolder since we don't return it.
	if cfg.RemoteUser != "" {
		return cfg.RemoteUser
	}
	if cfg.ContainerUser != "" {
		return cfg.ContainerUser
	}
	return ""
}

// envMap returns the current process environment as a map.
func envMap() map[string]string {
	env := make(map[string]string, len(os.Environ()))
	for _, e := range os.Environ() {
		for i := range len(e) {
			if e[i] == '=' {
				env[e[:i]] = e[i+1:]
				break
			}
		}
	}
	return env
}
