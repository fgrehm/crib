package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPolicy_Defaults(t *testing.T) {
	wsDir := t.TempDir()
	cfg := &sandboxConfig{}
	pol := buildPolicy(cfg, wsDir, "vscode", "/workspaces/project")

	if pol.WorkspaceFolder != "/workspaces/project" {
		t.Errorf("unexpected workspace folder: %s", pol.WorkspaceFolder)
	}
	if pol.RemoteHome != "/home/vscode" {
		t.Errorf("unexpected remote home: %s", pol.RemoteHome)
	}
	if len(pol.DenyPaths) != 0 {
		t.Errorf("expected 0 deny paths without plugins, got %d", len(pol.DenyPaths))
	}
}

func TestBuildPolicy_WithDiscovery(t *testing.T) {
	wsDir := t.TempDir()
	for _, dir := range []string{"coding-agents", "ssh"} {
		if err := os.MkdirAll(filepath.Join(wsDir, "plugins", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &sandboxConfig{}
	pol := buildPolicy(cfg, wsDir, "vscode", "/workspaces/project")

	if len(pol.DenyPaths) != 2 {
		t.Fatalf("expected 2 deny paths, got %d", len(pol.DenyPaths))
	}
}

func TestBuildPolicy_MergesUserConfig(t *testing.T) {
	wsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wsDir, "plugins", "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &sandboxConfig{
		DenyRead:   []string{"~/.config/secrets"},
		DenyWrite:  []string{"~/.local"},
		AllowWrite: []string{"/var/log"},
	}
	pol := buildPolicy(cfg, wsDir, "vscode", "/workspaces/project")

	// 1 from ssh discovery + 1 denyRead + 1 denyWrite = 3
	if len(pol.DenyPaths) != 3 {
		t.Fatalf("expected 3 deny paths, got %d", len(pol.DenyPaths))
	}

	// DenyRead user path should be expanded.
	found := false
	for _, r := range pol.DenyPaths {
		if r.Path == "/home/vscode/.config/secrets" && r.DenyRead {
			found = true
		}
	}
	if !found {
		t.Error("expected expanded denyRead path /home/vscode/.config/secrets")
	}

	// DenyWrite user path should not have DenyRead set.
	for _, r := range pol.DenyPaths {
		if r.Path == "/home/vscode/.local" && r.DenyRead {
			t.Error("denyWrite paths should not set DenyRead")
		}
	}

	if len(pol.AllowWritePaths) != 1 || pol.AllowWritePaths[0] != "/var/log" {
		t.Errorf("unexpected allowWrite: %v", pol.AllowWritePaths)
	}
}

func TestBuildPolicy_DeduplicatesDenyPaths(t *testing.T) {
	wsDir := t.TempDir()
	// ssh discovery will add ~/.ssh as deny-read.
	if err := os.MkdirAll(filepath.Join(wsDir, "plugins", "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &sandboxConfig{
		// User also configures ~/.ssh as denyRead (duplicate).
		DenyRead:  []string{"~/.ssh"},
		DenyWrite: []string{"~/.ssh"}, // deny-write for same path, should not add a second rule
	}
	pol := buildPolicy(cfg, wsDir, "vscode", "/workspaces/project")

	// Should be 1 entry (deduplicated), with DenyRead=true (deny-read wins).
	if len(pol.DenyPaths) != 1 {
		t.Fatalf("expected 1 deduplicated deny path, got %d: %+v", len(pol.DenyPaths), pol.DenyPaths)
	}
	if !pol.DenyPaths[0].DenyRead {
		t.Error("deny-read should win over deny-write for the same path")
	}
}

func TestExpandHome(t *testing.T) {
	if got := expandHome("~/.ssh", "/home/vscode"); got != "/home/vscode/.ssh" {
		t.Errorf("expected /home/vscode/.ssh, got %s", got)
	}
	if got := expandHome("/etc/shadow", "/home/vscode"); got != "/etc/shadow" {
		t.Errorf("expected /etc/shadow, got %s", got)
	}
	if got := expandHome("~/", "/root"); got != "/root" {
		t.Errorf("expected /root, got %s", got)
	}
}

func TestGenerateWrapperScript_Basic(t *testing.T) {
	pol := &policy{
		WorkspaceFolder: "/workspaces/project",
		RemoteHome:      "/home/vscode",
	}
	script := generateWrapperScript(pol)

	if !strings.Contains(script, "#!/bin/sh") {
		t.Error("missing shebang")
	}
	if !strings.Contains(script, "WARNING: sandbox plugin is experimental") {
		t.Error("missing experimental warning")
	}
	if !strings.Contains(script, "--ro-bind / /") {
		t.Error("missing ro-bind root")
	}
	if !strings.Contains(script, "--bind '/workspaces/project' '/workspaces/project'") {
		t.Error("missing workspace bind")
	}
	if !strings.Contains(script, "--bind /tmp /tmp") {
		t.Error("missing /tmp bind")
	}
	if !strings.Contains(script, "-- \"$@\"") {
		t.Error("missing passthrough args")
	}
}

func TestGenerateWrapperScript_WithDenyPaths(t *testing.T) {
	pol := &policy{
		WorkspaceFolder: "/workspaces/project",
		RemoteHome:      "/home/vscode",
		DenyPaths: []denyRule{
			{Path: "/home/vscode/.ssh", DenyRead: true},
			{Path: "/home/vscode/.crib_history", DenyRead: true},
			{Path: "/home/vscode/.config", DenyRead: false}, // deny-write
		},
	}
	script := generateWrapperScript(pol)

	if !strings.Contains(script, "--tmpfs '/home/vscode/.ssh'") {
		t.Error("missing tmpfs for .ssh (deny-read)")
	}
	if !strings.Contains(script, "--tmpfs '/home/vscode/.crib_history'") {
		t.Error("missing tmpfs for .crib_history (deny-read)")
	}
	if !strings.Contains(script, "--ro-bind-try '/home/vscode/.config' '/home/vscode/.config'") {
		t.Error("missing ro-bind-try for .config (deny-write)")
	}
}

func TestGenerateWrapperScript_QuotedPaths(t *testing.T) {
	pol := &policy{
		WorkspaceFolder: "/workspaces/it's a project",
		RemoteHome:      "/home/vscode",
		DenyPaths: []denyRule{
			{Path: "/home/vscode/o'reilly", DenyRead: true},
		},
		AllowWritePaths: []string{"/data/it's"},
	}
	script := generateWrapperScript(pol)

	if !strings.Contains(script, "--bind '/workspaces/it'\\''s a project' '/workspaces/it'\\''s a project'") {
		t.Error("workspace folder not properly escaped")
	}
	if !strings.Contains(script, "--tmpfs '/home/vscode/o'\\''reilly'") {
		t.Error("deny path not properly escaped")
	}
	if !strings.Contains(script, "--bind-try '/data/it'\\''s' '/data/it'\\''s'") {
		t.Error("allow-write path not properly escaped")
	}
}

func TestGenerateWrapperScript_WithHiddenFiles(t *testing.T) {
	pol := &policy{
		WorkspaceFolder: "/workspaces/project",
		RemoteHome:      "/home/vscode",
		HiddenFiles: []string{
			"/workspaces/project/.env",
			"/workspaces/project/config/master.key",
		},
	}
	script := generateWrapperScript(pol)

	if !strings.Contains(script, "--ro-bind-try /dev/null '/workspaces/project/.env'") {
		t.Error("missing /dev/null bind for .env")
	}
	if !strings.Contains(script, "--ro-bind-try /dev/null '/workspaces/project/config/master.key'") {
		t.Error("missing /dev/null bind for master.key")
	}
	// Hidden file entries should appear before the passthrough args.
	idxHidden := strings.Index(script, "--ro-bind-try /dev/null")
	idxArgs := strings.Index(script, "-- \"$@\"")
	if idxHidden > idxArgs {
		t.Error("hidden file entries should appear before passthrough args")
	}
}
