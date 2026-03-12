package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverPluginArtifacts_Empty(t *testing.T) {
	wsDir := t.TempDir()
	rules := discoverPluginArtifacts(wsDir, "vscode")
	if len(rules) != 0 {
		t.Errorf("expected 0 rules for empty workspace, got %d", len(rules))
	}
}

func TestDiscoverPluginArtifacts_AllPlugins(t *testing.T) {
	wsDir := t.TempDir()
	for _, dir := range []string{"coding-agents", "ssh", "shell-history"} {
		if err := os.MkdirAll(filepath.Join(wsDir, "plugins", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	rules := discoverPluginArtifacts(wsDir, "vscode")
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}

	// codingagents -> ~/.claude
	if rules[0].Path != "/home/vscode/.claude" || !rules[0].DenyRead {
		t.Errorf("unexpected codingagents rule: %+v", rules[0])
	}

	// ssh -> ~/.ssh
	if rules[1].Path != "/home/vscode/.ssh" || !rules[1].DenyRead {
		t.Errorf("unexpected ssh rule: %+v", rules[1])
	}

	// shellhistory -> ~/.crib_history (deny-read, allow-write)
	if rules[2].Path != "/home/vscode/.crib_history" || !rules[2].DenyRead || !rules[2].AllowWrite {
		t.Errorf("unexpected shellhistory rule: %+v", rules[2])
	}
}

func TestDiscoverPluginArtifacts_RootUser(t *testing.T) {
	wsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wsDir, "plugins", "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}

	rules := discoverPluginArtifacts(wsDir, "root")
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Path != "/root/.ssh" {
		t.Errorf("expected /root/.ssh, got %s", rules[0].Path)
	}
}

func TestDiscoverPluginArtifacts_OnlySsh(t *testing.T) {
	wsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wsDir, "plugins", "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}

	rules := discoverPluginArtifacts(wsDir, "vscode")
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Path != "/home/vscode/.ssh" {
		t.Errorf("expected /home/vscode/.ssh, got %s", rules[0].Path)
	}
}
