package shellhistory

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
	if p.Name() != "shell-history" {
		t.Errorf("expected name shell-history, got %s", p.Name())
	}
}

func TestPreContainerRun_CreatesHistoryDir(t *testing.T) {
	wsDir := t.TempDir()
	p := New()
	resp, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// The plugin should create the history directory.
	histDir := filepath.Join(wsDir, "plugins", "shell-history")
	info, err := os.Stat(histDir)
	if err != nil {
		t.Fatalf("expected history dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected history path to be a directory")
	}
}

func TestPreContainerRun_TouchesHistoryFile(t *testing.T) {
	wsDir := t.TempDir()
	p := New()
	if _, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode")); err != nil {
		t.Fatal(err)
	}

	histFile := filepath.Join(wsDir, "plugins", "shell-history", ".shell_history")
	if _, err := os.Stat(histFile); err != nil {
		t.Fatalf("expected history file to be created: %v", err)
	}
}

func TestPreContainerRun_PreservesExistingHistory(t *testing.T) {
	wsDir := t.TempDir()
	histDir := filepath.Join(wsDir, "plugins", "shell-history")
	if err := os.MkdirAll(histDir, 0o755); err != nil {
		t.Fatal(err)
	}
	histFile := filepath.Join(histDir, ".shell_history")
	if err := os.WriteFile(histFile, []byte("echo hello\nls -la\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := New()
	if _, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode")); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(histFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "echo hello\nls -la\n" {
		t.Errorf("expected existing history to be preserved, got: %q", string(data))
	}
}

func TestPreContainerRun_MountsDirectory(t *testing.T) {
	wsDir := t.TempDir()
	p := New()
	resp, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode"))
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(resp.Mounts))
	}

	m := resp.Mounts[0]
	if m.Type != "bind" {
		t.Errorf("expected bind mount, got %s", m.Type)
	}

	// Mount source is the directory, not the file.
	expectedSource := filepath.Join(wsDir, "plugins", "shell-history")
	if m.Source != expectedSource {
		t.Errorf("expected source %s, got %s", expectedSource, m.Source)
	}
	// Mount target is a hidden directory inside the user's home.
	if m.Target != "/home/vscode/.crib_history" {
		t.Errorf("expected target /home/vscode/.crib_history, got %s", m.Target)
	}
}

func TestPreContainerRun_SetsHistfileEnv(t *testing.T) {
	wsDir := t.TempDir()
	p := New()
	resp, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode"))
	if err != nil {
		t.Fatal(err)
	}

	if resp.Env == nil {
		t.Fatal("expected non-nil env map")
	}
	// HISTFILE points to the file inside the mounted directory.
	if resp.Env["HISTFILE"] != "/home/vscode/.crib_history/.shell_history" {
		t.Errorf("expected HISTFILE=/home/vscode/.crib_history/.shell_history, got %s", resp.Env["HISTFILE"])
	}
}

func TestPreContainerRun_RemoteUserRoot(t *testing.T) {
	wsDir := t.TempDir()
	p := New()
	resp, err := p.PreContainerRun(context.Background(), testReq(wsDir, "root"))
	if err != nil {
		t.Fatal(err)
	}

	if resp.Mounts[0].Target != "/root/.crib_history" {
		t.Errorf("expected target /root/.crib_history, got %s", resp.Mounts[0].Target)
	}
	if resp.Env["HISTFILE"] != "/root/.crib_history/.shell_history" {
		t.Errorf("expected HISTFILE=/root/.crib_history/.shell_history, got %s", resp.Env["HISTFILE"])
	}
}

func TestPreContainerRun_RemoteUserEmpty(t *testing.T) {
	wsDir := t.TempDir()
	p := New()
	resp, err := p.PreContainerRun(context.Background(), testReq(wsDir, ""))
	if err != nil {
		t.Fatal(err)
	}

	if resp.Mounts[0].Target != "/root/.crib_history" {
		t.Errorf("expected target /root/.crib_history, got %s", resp.Mounts[0].Target)
	}
	if resp.Env["HISTFILE"] != "/root/.crib_history/.shell_history" {
		t.Errorf("expected HISTFILE=/root/.crib_history/.shell_history, got %s", resp.Env["HISTFILE"])
	}
}

func TestPreContainerRun_NoCopiesOrRunArgs(t *testing.T) {
	wsDir := t.TempDir()
	p := New()
	resp, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode"))
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Copies) != 0 {
		t.Errorf("expected 0 copies, got %d", len(resp.Copies))
	}
	if len(resp.RunArgs) != 0 {
		t.Errorf("expected 0 runArgs, got %d", len(resp.RunArgs))
	}
}
