package codingagents

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgrehm/crib/internal/plugin"
)

func testReq(workspaceDir, remoteUser string) *plugin.PreContainerRunRequest {
	return &plugin.PreContainerRunRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    workspaceDir,
		SourceDir:       "/home/user/project",
		Runtime:         "docker",
		ImageName:       "ubuntu:22.04",
		RemoteUser:      remoteUser,
		WorkspaceFolder: "/workspaces/project",
		ContainerName:   "crib-test-ws",
	}
}

func TestName(t *testing.T) {
	p := New()
	if p.Name() != "coding-agents" {
		t.Errorf("expected name coding-agents, got %s", p.Name())
	}
}

func TestPreContainerRun_ClaudeExists(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "credentials.json"), []byte(`{"token":"abc"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	wsDir := t.TempDir()
	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(resp.Mounts))
	}

	mount := resp.Mounts[0]
	if mount.Type != "bind" {
		t.Errorf("expected bind mount, got %s", mount.Type)
	}
	expectedSource := filepath.Join(wsDir, "plugins", "coding-agents", "claude")
	if mount.Source != expectedSource {
		t.Errorf("expected source %s, got %s", expectedSource, mount.Source)
	}
	if mount.Target != "/home/vscode/.claude" {
		t.Errorf("expected target /home/vscode/.claude, got %s", mount.Target)
	}
}

func TestPreContainerRun_ClaudeNotExists(t *testing.T) {
	home := t.TempDir() // no .claude/ directory

	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(t.TempDir(), "vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for missing ~/.claude, got %+v", resp)
	}
}

func TestPreContainerRun_RemoteUserVscode(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(t.TempDir(), "vscode"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Mounts[0].Target != "/home/vscode/.claude" {
		t.Errorf("expected target /home/vscode/.claude, got %s", resp.Mounts[0].Target)
	}
}

func TestPreContainerRun_RemoteUserRoot(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(t.TempDir(), "root"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Mounts[0].Target != "/root/.claude" {
		t.Errorf("expected target /root/.claude, got %s", resp.Mounts[0].Target)
	}
}

func TestPreContainerRun_RemoteUserEmpty(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(t.TempDir(), ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Mounts[0].Target != "/root/.claude" {
		t.Errorf("expected target /root/.claude (default), got %s", resp.Mounts[0].Target)
	}
}

func TestPreContainerRun_CopyContents(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a file and a subdirectory with a file.
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(claudeDir, "projects")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "project.json"), []byte(`{"name":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	wsDir := t.TempDir()
	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode"))
	if err != nil {
		t.Fatal(err)
	}

	copyDir := filepath.Join(wsDir, "plugins", "coding-agents", "claude")

	// Check top-level file.
	data, err := os.ReadFile(filepath.Join(copyDir, "settings.json"))
	if err != nil {
		t.Fatalf("expected settings.json to be copied: %v", err)
	}
	if string(data) != `{"theme":"dark"}` {
		t.Errorf("unexpected content: %s", data)
	}

	// Check subdirectory file.
	data, err = os.ReadFile(filepath.Join(copyDir, "projects", "project.json"))
	if err != nil {
		t.Fatalf("expected projects/project.json to be copied: %v", err)
	}
	if string(data) != `{"name":"test"}` {
		t.Errorf("unexpected content: %s", data)
	}

	// Verify mount source matches copy location.
	if resp.Mounts[0].Source != copyDir {
		t.Errorf("mount source %s doesn't match copy dir %s", resp.Mounts[0].Source, copyDir)
	}
}
