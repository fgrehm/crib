package ssh

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
)

const agentSocketTarget = "/tmp/ssh-agent.sock"

// Plugin shares SSH credentials with containers. It provides:
//   - SSH agent socket forwarding (so private keys never leave the host)
//   - SSH config file injection (~/.ssh/config)
//   - SSH public key injection (~/.ssh/*.pub, for git commit signing)
//   - Git SSH signing config extraction (only when gpg.format=ssh)
type Plugin struct {
	homeDir      string // overridable for testing
	getenvFunc   func(string) string
	gitConfigCmd func(key string) string
}

// New creates an SSH plugin that uses the real user environment.
func New() *Plugin {
	return &Plugin{}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string { return "ssh" }

func (p *Plugin) getenv(key string) string {
	if p.getenvFunc != nil {
		return p.getenvFunc(key)
	}
	return os.Getenv(key)
}

func (p *Plugin) home() (string, error) {
	if p.homeDir != "" {
		return p.homeDir, nil
	}
	return os.UserHomeDir()
}

func (p *Plugin) gitConfig(key string) string {
	if p.gitConfigCmd != nil {
		return p.gitConfigCmd(key)
	}
	out, err := exec.Command("git", "config", "--global", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// PreContainerRun composes results from agent forwarding, config/key copying,
// and git signing config. Each sub-function is fail-safe: partial results are
// returned if one component fails.
func (p *Plugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	home, err := p.home()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)
	owner := plugin.InferOwner(req.RemoteUser)

	pluginDir := filepath.Join(req.WorkspaceDir, "plugins", "ssh")

	var mounts []config.Mount
	env := map[string]string{}
	var copies []plugin.FileCopy

	// SSH agent forwarding.
	if m, e := p.agentForwarding(); m != nil {
		mounts = append(mounts, *m)
		for k, v := range e {
			env[k] = v
		}
	}

	// SSH config file.
	if c := p.sshConfig(home, pluginDir, remoteHome, owner); c != nil {
		copies = append(copies, *c)
	}

	// SSH public keys (for git signing via agent).
	copies = append(copies, p.sshPublicKeys(home, pluginDir, remoteHome, owner)...)

	// Git SSH signing config.
	if c := p.gitSigningConfig(home, pluginDir, remoteHome, owner); c != nil {
		copies = append(copies, *c)
	}

	if len(mounts) == 0 && len(env) == 0 && len(copies) == 0 {
		return nil, nil
	}

	resp := &plugin.PreContainerRunResponse{
		Mounts: mounts,
		Copies: copies,
	}
	if len(env) > 0 {
		resp.Env = env
	}
	return resp, nil
}

// agentForwarding binds the host's SSH agent socket into the container.
// Returns nil if SSH_AUTH_SOCK is unset or the socket doesn't exist.
func (p *Plugin) agentForwarding() (*config.Mount, map[string]string) {
	sock := p.getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, nil
	}

	// Verify socket exists.
	if _, err := os.Stat(sock); err != nil {
		return nil, nil
	}

	mount := &config.Mount{
		Type:   "bind",
		Source: sock,
		Target: agentSocketTarget,
	}
	env := map[string]string{
		"SSH_AUTH_SOCK": agentSocketTarget,
	}
	return mount, env
}

// sshConfig copies ~/.ssh/config into the container.
func (p *Plugin) sshConfig(home, pluginDir, remoteHome, owner string) *plugin.FileCopy {
	src := filepath.Join(home, ".ssh", "config")
	if _, err := os.Stat(src); err != nil {
		return nil
	}

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		slog.Warn("ssh plugin: failed to create config staging dir", "path", pluginDir, "error", err)
		return nil
	}

	dst := filepath.Join(pluginDir, "config")
	if err := plugin.CopyFile(src, dst, 0o644); err != nil {
		return nil
	}

	return &plugin.FileCopy{
		Source: dst,
		Target: filepath.Join(remoteHome, ".ssh", "config"),
		Mode:   "0644",
		User:   owner,
	}
}

// sshPublicKeys copies *.pub files from ~/.ssh/ into the container.
// Skips authorized_keys, known_hosts, and non-regular files.
func (p *Plugin) sshPublicKeys(home, pluginDir, remoteHome, owner string) []plugin.FileCopy {
	sshDir := filepath.Join(home, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return nil
	}

	keysDir := filepath.Join(pluginDir, "keys")
	if err := os.MkdirAll(keysDir, 0o755); err != nil {
		slog.Warn("ssh plugin: failed to create keys staging dir", "path", keysDir, "error", err)
		return nil
	}

	var copies []plugin.FileCopy

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".pub") {
			continue
		}
		if strings.HasPrefix(name, "authorized_keys") {
			continue
		}

		src := filepath.Join(sshDir, name)
		dst := filepath.Join(keysDir, name)
		if err := plugin.CopyFile(src, dst, 0o644); err != nil {
			continue
		}

		copies = append(copies, plugin.FileCopy{
			Source: dst,
			Target: filepath.Join(remoteHome, ".ssh", name),
			Mode:   "0644",
			User:   owner,
		})
	}

	return copies
}

// gitSigningConfig extracts SSH signing settings from the host's git config
// and generates a minimal .gitconfig for the container. Only active when
// gpg.format is "ssh".
func (p *Plugin) gitSigningConfig(home, pluginDir, remoteHome, owner string) *plugin.FileCopy {
	format := p.gitConfig("gpg.format")
	if format != "ssh" {
		return nil
	}

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		slog.Warn("ssh plugin: failed to create git signing staging dir", "path", pluginDir, "error", err)
		return nil
	}

	// Collect signing-related settings.
	type setting struct {
		section, key, value string
	}
	var settings []setting

	// user.name and user.email are needed for commits to have the right author.
	if v := p.gitConfig("user.name"); v != "" {
		settings = append(settings, setting{"user", "name", v})
	}
	if v := p.gitConfig("user.email"); v != "" {
		settings = append(settings, setting{"user", "email", v})
	}

	// Signing key, rewriting path if it references ~/.ssh/.
	if v := p.gitConfig("user.signingkey"); v != "" {
		v = rewriteKeyPath(v, home, remoteHome)
		settings = append(settings, setting{"user", "signingkey", v})
	}

	settings = append(settings, setting{"gpg", "format", "ssh"})

	if v := p.gitConfig("gpg.ssh.program"); v != "" {
		settings = append(settings, setting{"gpg \"ssh\"", "program", v})
	}

	if v := p.gitConfig("commit.gpgsign"); v != "" {
		settings = append(settings, setting{"commit", "gpgsign", v})
	}
	if v := p.gitConfig("tag.gpgsign"); v != "" {
		settings = append(settings, setting{"tag", "gpgsign", v})
	}

	// Generate minimal gitconfig.
	var buf strings.Builder
	currentSection := ""
	for _, s := range settings {
		if s.section != currentSection {
			if currentSection != "" {
				buf.WriteString("\n")
			}
			fmt.Fprintf(&buf, "[%s]\n", s.section)
			currentSection = s.section
		}
		fmt.Fprintf(&buf, "\t%s = %s\n", s.key, s.value)
	}

	dst := filepath.Join(pluginDir, "gitconfig")
	if err := os.WriteFile(dst, []byte(buf.String()), 0o644); err != nil {
		return nil
	}

	return &plugin.FileCopy{
		Source: dst,
		Target: filepath.Join(remoteHome, ".gitconfig"),
		User:   owner,
	}
}

// rewriteKeyPath rewrites a signing key path that references the host's
// ~/.ssh/ directory to use the container's home directory instead.
func rewriteKeyPath(keyPath, hostHome, remoteHome string) string {
	// Handle ~/... paths.
	if strings.HasPrefix(keyPath, "~/.ssh/") {
		return filepath.Join(remoteHome, ".ssh", keyPath[len("~/.ssh/"):])
	}
	// Handle absolute paths with host home.
	hostSSH := filepath.Join(hostHome, ".ssh") + "/"
	if strings.HasPrefix(keyPath, hostSSH) {
		return filepath.Join(remoteHome, ".ssh", keyPath[len(hostSSH):])
	}
	return keyPath
}
