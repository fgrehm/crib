package ssh

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/plugin/plugintest"
)

func TestName(t *testing.T) {
	p := New()
	if p.Name() != "ssh" {
		t.Errorf("expected name ssh, got %s", p.Name())
	}
}

func TestPreContainerRun_NoSSHDir_NoAgent(t *testing.T) {
	home := t.TempDir() // no .ssh/
	p := &Plugin{
		homeDir:      home,
		getenvFunc:   func(string) string { return "" },
		gitConfigCmd: func(string) string { return "" },
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(t.TempDir(), "vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response when nothing is available, got %+v", resp)
	}
}

// --- Agent forwarding tests ---

func TestPreContainerRun_AgentForwarding(t *testing.T) {
	// Create a real Unix socket so os.Stat sees it.
	sockDir := t.TempDir()
	sockPath := filepath.Join(sockDir, "agent.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	home := t.TempDir()
	p := &Plugin{
		homeDir: home,
		getenvFunc: func(key string) string {
			if key == "SSH_AUTH_SOCK" {
				return sockPath
			}
			return ""
		},
		gitConfigCmd: func(string) string { return "" },
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(t.TempDir(), "vscode"))
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response with agent forwarding")
	}

	if len(resp.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(resp.Mounts))
	}
	mount := resp.Mounts[0]
	if mount.Source != sockPath {
		t.Errorf("mount source: expected %s, got %s", sockPath, mount.Source)
	}
	if mount.Target != agentSocketTarget {
		t.Errorf("mount target: expected %s, got %s", agentSocketTarget, mount.Target)
	}

	if resp.Env["SSH_AUTH_SOCK"] != agentSocketTarget {
		t.Errorf("SSH_AUTH_SOCK env: expected %s, got %s", agentSocketTarget, resp.Env["SSH_AUTH_SOCK"])
	}
}

func TestPreContainerRun_AgentForwarding_NoSocket(t *testing.T) {
	home := t.TempDir()
	p := &Plugin{
		homeDir: home,
		getenvFunc: func(key string) string {
			if key == "SSH_AUTH_SOCK" {
				return "/nonexistent/agent.sock"
			}
			return ""
		},
		gitConfigCmd: func(string) string { return "" },
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(t.TempDir(), "vscode"))
	if err != nil {
		t.Fatal(err)
	}
	// No agent socket exists, so no mounts or env.
	if resp != nil {
		t.Errorf("expected nil response when socket doesn't exist, got %+v", resp)
	}
}

// --- SSH config tests ---

func TestPreContainerRun_SSHConfig(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host *\n  ForwardAgent yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	wsDir := t.TempDir()
	p := &Plugin{
		homeDir:      home,
		getenvFunc:   func(string) string { return "" },
		gitConfigCmd: func(string) string { return "" },
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(wsDir, "vscode"))
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response with SSH config")
	}

	// Find the config copy.
	var configCopy *plugin.FileCopy
	for i := range resp.Copies {
		if strings.HasSuffix(resp.Copies[i].Target, ".ssh/config") {
			configCopy = &resp.Copies[i]
			break
		}
	}
	if configCopy == nil {
		t.Fatal("expected SSH config copy in response")
	}
	if configCopy.Target != "/home/vscode/.ssh/config" {
		t.Errorf("config target: expected /home/vscode/.ssh/config, got %s", configCopy.Target)
	}
	if configCopy.Mode != "0644" {
		t.Errorf("config mode: expected 0644, got %s", configCopy.Mode)
	}
	if configCopy.User != "vscode" {
		t.Errorf("config user: expected vscode, got %s", configCopy.User)
	}

	// Verify staged content.
	staged := filepath.Join(wsDir, "plugins", "ssh", "config")
	data, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("expected staged config: %v", err)
	}
	if !strings.Contains(string(data), "ForwardAgent") {
		t.Errorf("staged config should contain original content")
	}
}

// --- SSH public keys tests ---

func TestPreContainerRun_SSHPublicKeys(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Create various files.
	files := map[string]string{
		"id_ed25519.pub":      "ssh-ed25519 AAAAC3...",
		"id_ed25519":          "-----BEGIN OPENSSH PRIVATE KEY-----",
		"id_ed25519-sign.pub": "ssh-ed25519 AAAAB4...",
		"id_ed25519-sign":     "-----BEGIN OPENSSH PRIVATE KEY-----",
		"known_hosts":         "github.com ssh-ed25519 AAAA...",
		"authorized_keys":     "ssh-rsa AAAA...",
		"authorized_keys.pub": "ssh-rsa AAAA...", // edge case: skip authorized_keys*.pub
		"config":              "Host *",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(sshDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	wsDir := t.TempDir()
	p := &Plugin{
		homeDir:      home,
		getenvFunc:   func(string) string { return "" },
		gitConfigCmd: func(string) string { return "" },
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(wsDir, "vscode"))
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response with SSH keys")
	}

	// Collect targets for key copies (exclude config copy).
	var keyCopies []string
	for _, c := range resp.Copies {
		if !strings.HasSuffix(c.Target, ".ssh/config") {
			keyCopies = append(keyCopies, filepath.Base(c.Target))
		}
	}

	// Should include the two .pub files (excluding authorized_keys.pub).
	expected := map[string]bool{
		"id_ed25519.pub":      true,
		"id_ed25519-sign.pub": true,
	}
	if len(keyCopies) != len(expected) {
		t.Fatalf("expected %d key copies, got %d: %v", len(expected), len(keyCopies), keyCopies)
	}
	for _, name := range keyCopies {
		if !expected[name] {
			t.Errorf("unexpected key copy: %s", name)
		}
	}

	// Verify no private keys were copied.
	for _, c := range resp.Copies {
		base := filepath.Base(c.Target)
		if base == "id_ed25519" || base == "id_ed25519-sign" {
			t.Errorf("private key should not be copied: %s", base)
		}
	}
}

// --- Git signing config tests ---

func TestPreContainerRun_GitSigningSSH(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}

	wsDir := t.TempDir()
	p := &Plugin{
		homeDir:    home,
		getenvFunc: func(string) string { return "" },
		gitConfigCmd: func(key string) string {
			switch key {
			case "gpg.format":
				return "ssh"
			case "user.name":
				return "Test User"
			case "user.email":
				return "test@example.com"
			case "user.signingkey":
				return "~/.ssh/id_ed25519-sign.pub"
			case "commit.gpgsign":
				return "true"
			default:
				return ""
			}
		},
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(wsDir, "vscode"))
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response with git signing config")
	}

	// Find the gitconfig copy.
	var gitconfigCopy *plugin.FileCopy
	for i := range resp.Copies {
		if strings.HasSuffix(resp.Copies[i].Target, ".gitconfig") {
			gitconfigCopy = &resp.Copies[i]
			break
		}
	}
	if gitconfigCopy == nil {
		t.Fatal("expected gitconfig copy in response")
	}
	if gitconfigCopy.Target != "/home/vscode/.gitconfig" {
		t.Errorf("gitconfig target: expected /home/vscode/.gitconfig, got %s", gitconfigCopy.Target)
	}

	// Read and verify the generated gitconfig.
	data, err := os.ReadFile(gitconfigCopy.Source)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "name = Test User") {
		t.Errorf("expected user.name in gitconfig")
	}
	if !strings.Contains(content, "email = test@example.com") {
		t.Errorf("expected user.email in gitconfig")
	}
	if !strings.Contains(content, "format = ssh") {
		t.Errorf("expected gpg.format = ssh in gitconfig")
	}
	if !strings.Contains(content, "gpgsign = true") {
		t.Errorf("expected commit.gpgsign = true in gitconfig")
	}
}

func TestPreContainerRun_GitSigningNonSSH(t *testing.T) {
	home := t.TempDir()
	p := &Plugin{
		homeDir:    home,
		getenvFunc: func(string) string { return "" },
		gitConfigCmd: func(key string) string {
			if key == "gpg.format" {
				return "openpgp"
			}
			return ""
		},
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(t.TempDir(), "vscode"))
	if err != nil {
		t.Fatal(err)
	}
	// Non-SSH signing format should not generate a gitconfig.
	if resp != nil {
		for _, c := range resp.Copies {
			if strings.HasSuffix(c.Target, ".gitconfig") {
				t.Errorf("should not generate gitconfig for non-SSH signing format")
			}
		}
	}
}

func TestPreContainerRun_GitSigningNoFormat(t *testing.T) {
	home := t.TempDir()
	p := &Plugin{
		homeDir:      home,
		getenvFunc:   func(string) string { return "" },
		gitConfigCmd: func(string) string { return "" },
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(t.TempDir(), "vscode"))
	if err != nil {
		t.Fatal(err)
	}
	// No gpg.format set should not generate a gitconfig.
	if resp != nil {
		for _, c := range resp.Copies {
			if strings.HasSuffix(c.Target, ".gitconfig") {
				t.Errorf("should not generate gitconfig when gpg.format is not set")
			}
		}
	}
}

// --- Path rewriting tests ---

func TestRewriteKeyPath(t *testing.T) {
	tests := []struct {
		name       string
		keyPath    string
		hostHome   string
		remoteHome string
		want       string
	}{
		{
			name:       "tilde path",
			keyPath:    "~/.ssh/id_ed25519-sign.pub",
			hostHome:   "/home/fabio",
			remoteHome: "/home/vscode",
			want:       "/home/vscode/.ssh/id_ed25519-sign.pub",
		},
		{
			name:       "absolute host path",
			keyPath:    "/home/fabio/.ssh/id_ed25519.pub",
			hostHome:   "/home/fabio",
			remoteHome: "/home/vscode",
			want:       "/home/vscode/.ssh/id_ed25519.pub",
		},
		{
			name:       "non-ssh path unchanged",
			keyPath:    "/opt/keys/signing.pub",
			hostHome:   "/home/fabio",
			remoteHome: "/home/vscode",
			want:       "/opt/keys/signing.pub",
		},
		{
			name:       "key literal unchanged",
			keyPath:    "ssh-ed25519 AAAAC3...",
			hostHome:   "/home/fabio",
			remoteHome: "/home/vscode",
			want:       "ssh-ed25519 AAAAC3...",
		},
		{
			name:       "root remote user",
			keyPath:    "~/.ssh/id_ed25519.pub",
			hostHome:   "/home/fabio",
			remoteHome: "/root",
			want:       "/root/.ssh/id_ed25519.pub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteKeyPath(tt.keyPath, tt.hostHome, tt.remoteHome)
			if got != tt.want {
				t.Errorf("rewriteKeyPath(%q) = %q, want %q", tt.keyPath, got, tt.want)
			}
		})
	}
}

// --- Root user tests ---

func TestPreContainerRun_RootUser(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host *"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &Plugin{
		homeDir:      home,
		getenvFunc:   func(string) string { return "" },
		gitConfigCmd: func(string) string { return "" },
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(t.TempDir(), "root"))
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range resp.Copies {
		if !strings.HasPrefix(c.Target, "/root/") {
			t.Errorf("expected /root/ prefix for root user, got %s", c.Target)
		}
		if c.User != "root" {
			t.Errorf("expected user root, got %s", c.User)
		}
	}
}

func TestPreContainerRun_EmptyUser(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host *"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &Plugin{
		homeDir:      home,
		getenvFunc:   func(string) string { return "" },
		gitConfigCmd: func(string) string { return "" },
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(t.TempDir(), ""))
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range resp.Copies {
		if !strings.HasPrefix(c.Target, "/root/") {
			t.Errorf("expected /root/ prefix for empty user, got %s", c.Target)
		}
		if c.User != "root" {
			t.Errorf("expected user root for empty user, got %s", c.User)
		}
	}
}

// --- Combined test ---

func TestPreContainerRun_AllFeatures(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host *"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519.pub"), []byte("ssh-ed25519 AAAA"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a real socket for agent forwarding.
	sockDir := t.TempDir()
	sockPath := filepath.Join(sockDir, "agent.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	wsDir := t.TempDir()
	p := &Plugin{
		homeDir: home,
		getenvFunc: func(key string) string {
			if key == "SSH_AUTH_SOCK" {
				return sockPath
			}
			return ""
		},
		gitConfigCmd: func(key string) string {
			switch key {
			case "gpg.format":
				return "ssh"
			case "user.name":
				return "Test"
			case "user.signingkey":
				return "~/.ssh/id_ed25519.pub"
			case "commit.gpgsign":
				return "true"
			default:
				return ""
			}
		},
	}

	resp, err := p.PreContainerRun(context.Background(), plugintest.TestReq(wsDir, "vscode"))
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have: 1 mount (agent), env (SSH_AUTH_SOCK), copies (config, key, gitconfig).
	if len(resp.Mounts) != 1 {
		t.Errorf("expected 1 mount, got %d", len(resp.Mounts))
	}
	if resp.Env["SSH_AUTH_SOCK"] != agentSocketTarget {
		t.Errorf("expected SSH_AUTH_SOCK=%s", agentSocketTarget)
	}
	if len(resp.Copies) != 3 {
		t.Errorf("expected 3 copies (config, pub key, gitconfig), got %d", len(resp.Copies))
	}
}
