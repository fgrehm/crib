package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverPluginArtifacts_Empty(t *testing.T) {
	wsDir := t.TempDir()
	disc := discoverPluginArtifacts(wsDir, "vscode")
	if len(disc.DenyRules) != 0 {
		t.Errorf("expected 0 deny rules for empty workspace, got %d", len(disc.DenyRules))
	}
	if len(disc.AllowWritePaths) != 0 {
		t.Errorf("expected 0 allow-write paths for empty workspace, got %d", len(disc.AllowWritePaths))
	}
}

func TestDiscoverPluginArtifacts_AllPlugins(t *testing.T) {
	wsDir := t.TempDir()
	for _, dir := range []string{"coding-agents", "ssh", "shell-history"} {
		if err := os.MkdirAll(filepath.Join(wsDir, "plugins", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	disc := discoverPluginArtifacts(wsDir, "vscode")

	// coding-agents -> ~/.claude (writable, agent needs to refresh OAuth tokens)
	if len(disc.AllowWritePaths) != 1 || disc.AllowWritePaths[0] != "/home/vscode/.claude" {
		t.Errorf("unexpected allow-write paths: %v", disc.AllowWritePaths)
	}

	if len(disc.DenyRules) != 2 {
		t.Fatalf("expected 2 deny rules, got %d", len(disc.DenyRules))
	}

	// ssh -> ~/.ssh
	if disc.DenyRules[0].Path != "/home/vscode/.ssh" || !disc.DenyRules[0].DenyRead {
		t.Errorf("unexpected ssh rule: %+v", disc.DenyRules[0])
	}

	// shellhistory -> ~/.crib_history (deny-read)
	if disc.DenyRules[1].Path != "/home/vscode/.crib_history" || !disc.DenyRules[1].DenyRead {
		t.Errorf("unexpected shellhistory rule: %+v", disc.DenyRules[1])
	}
}

func TestDiscoverPluginArtifacts_RootUser(t *testing.T) {
	wsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wsDir, "plugins", "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}

	disc := discoverPluginArtifacts(wsDir, "root")
	if len(disc.DenyRules) != 1 {
		t.Fatalf("expected 1 deny rule, got %d", len(disc.DenyRules))
	}
	if disc.DenyRules[0].Path != "/root/.ssh" {
		t.Errorf("expected /root/.ssh, got %s", disc.DenyRules[0].Path)
	}
}

func TestDiscoverPluginArtifacts_OnlySsh(t *testing.T) {
	wsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wsDir, "plugins", "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}

	disc := discoverPluginArtifacts(wsDir, "vscode")
	if len(disc.DenyRules) != 1 {
		t.Fatalf("expected 1 deny rule, got %d", len(disc.DenyRules))
	}
	if disc.DenyRules[0].Path != "/home/vscode/.ssh" {
		t.Errorf("expected /home/vscode/.ssh, got %s", disc.DenyRules[0].Path)
	}
}
