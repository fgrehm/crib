package dotfiles

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

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

	// Check that git is available in the container.
	if _, err := req.Exec(ctx, []string{"which", "git"}, req.RemoteUser, ""); err != nil {
		slog.Warn("dotfiles: git not found in container, skipping")
		return nil, nil
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)
	targetPath := p.resolveTargetPath(remoteHome)

	// Clone the repository. Use accept-new so the first connection to a host
	// (e.g. github.com) auto-accepts its key without a known_hosts entry.
	cloneCmd := []string{"git", "clone", "--", p.cfg.Repository, targetPath}
	if isSSHRepo(p.cfg.Repository) {
		cloneCmd = []string{
			"sh", "-c",
			"GIT_SSH_COMMAND='ssh -o StrictHostKeyChecking=accept-new' exec \"$@\"",
			"--",
			"git", "clone", "--", p.cfg.Repository, targetPath,
		}
	}
	if err := streamExecWithRetry(ctx, req, cloneCmd, req.RemoteUser, "", 3); err != nil {
		slog.Warn("dotfiles: clone failed", "repo", p.cfg.Repository, "error", err)
		return nil, nil
	}

	// Run install command.
	if p.cfg.InstallCommand != "" {
		// Explicit install command from config.
		installCmd := []string{"sh", "-c", p.cfg.InstallCommand}
		if err := req.StreamExec(ctx, installCmd, req.RemoteUser, targetPath, os.Stdout, os.Stderr); err != nil {
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
		runCmd := []string{"sh", scriptPath}
		if err := req.StreamExec(ctx, runCmd, req.RemoteUser, targetPath, os.Stdout, os.Stderr); err != nil {
			slog.Warn("dotfiles: install script failed", "script", script, "error", err)
		}
		break
	}

	return nil, nil
}

// streamExecWithRetry runs a command up to maxAttempts times with streaming
// output, waiting briefly between attempts. Retries help with transient DNS
// failures that are common on rootless Podman.
func streamExecWithRetry(ctx context.Context, req *plugin.PostContainerCreateRequest, cmd []string, user, workDir string, maxAttempts int) error {
	var err error
	for i := range maxAttempts {
		if err = req.StreamExec(ctx, cmd, user, workDir, os.Stdout, os.Stderr); err == nil {
			return nil
		}
		if i < maxAttempts-1 {
			slog.Debug("dotfiles: retrying after transient failure", "attempt", i+1, "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}
	return err
}

// isSSHRepo returns true if the repository URL uses SSH transport.
// Matches git@host:path (SCP syntax) and ssh:// URLs, but not HTTPS
// URLs that happen to contain a username (e.g. https://user@host/...).
func isSSHRepo(repo string) bool {
	if strings.HasPrefix(repo, "ssh://") {
		return true
	}
	// SCP syntax: user@host:path (no scheme, colon after host).
	// HTTPS with username has :// before the @, so we exclude that.
	if strings.Contains(repo, "://") {
		return false
	}
	return strings.Contains(repo, "@")
}

// resolveTargetPath expands ~ to the remote user's home directory.
func (p *Plugin) resolveTargetPath(remoteHome string) string {
	target := p.cfg.TargetPath
	if target == "" {
		target = "~/dotfiles"
	}
	if target == "~" {
		return remoteHome
	}
	if strings.HasPrefix(target, "~/") {
		return remoteHome + target[1:]
	}
	return target
}
