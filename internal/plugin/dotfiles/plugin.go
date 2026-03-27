package dotfiles

import (
	"context"
	"log/slog"
	"strings"

	"github.com/fgrehm/crib/internal/globalconfig"
	"github.com/fgrehm/crib/internal/plugin"
)

// installScripts is the list of scripts to auto-detect in the dotfiles repo
// root, checked in order. The first one found is executed.
var installScripts = []string{"install.sh", "bootstrap.sh", "setup.sh"}

// Plugin clones a dotfiles repository into the container and optionally
// runs an install script. Configured via ~/.config/crib/config.toml.
type Plugin struct {
	plugin.BasePlugin
	cfg globalconfig.DotfilesConfig
}

// New creates a dotfiles plugin from global config.
func New(cfg globalconfig.DotfilesConfig) *Plugin {
	return &Plugin{cfg: cfg}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string { return "dotfiles" }

// PostContainerCreate clones the dotfiles repository and runs the install
// script inside the container. No-op when no repository is configured.
func (p *Plugin) PostContainerCreate(ctx context.Context, req *plugin.PostContainerCreateRequest) (*plugin.PostContainerCreateResponse, error) {
	if p.cfg.Repository == "" {
		return nil, nil
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)
	targetPath := p.resolveTargetPath(remoteHome)

	// Clone the repository.
	cloneCmd := []string{"git", "clone", p.cfg.Repository, targetPath}
	if _, err := req.Exec(ctx, cloneCmd, req.RemoteUser, ""); err != nil {
		slog.Warn("dotfiles: clone failed", "repo", p.cfg.Repository, "error", err)
		return nil, nil
	}

	// Run install command.
	if p.cfg.InstallCommand != "" {
		// Explicit install command from config.
		installCmd := []string{"sh", "-c", p.cfg.InstallCommand}
		if _, err := req.Exec(ctx, installCmd, req.RemoteUser, targetPath); err != nil {
			slog.Warn("dotfiles: install command failed", "cmd", p.cfg.InstallCommand, "error", err)
		}
		return nil, nil
	}

	// Auto-detect install script.
	for _, script := range installScripts {
		scriptPath := targetPath + "/" + script
		checkCmd := []string{"test", "-f", scriptPath}
		if _, err := req.Exec(ctx, checkCmd, req.RemoteUser, ""); err != nil {
			continue
		}
		// Found a script, execute it.
		runCmd := []string{"sh", "-c", scriptPath}
		if _, err := req.Exec(ctx, runCmd, req.RemoteUser, targetPath); err != nil {
			slog.Warn("dotfiles: install script failed", "script", script, "error", err)
		}
		break
	}

	return nil, nil
}

// resolveTargetPath expands ~ to the remote user's home directory.
func (p *Plugin) resolveTargetPath(remoteHome string) string {
	target := p.cfg.TargetPath
	if target == "" {
		target = "~/dotfiles"
	}
	if strings.HasPrefix(target, "~/") {
		return remoteHome + target[1:]
	}
	return target
}
