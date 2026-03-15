package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/plugin"
)

func TestPostContainerCreate_NoConfig_Noop(t *testing.T) {
	p := New()
	req := &plugin.PostContainerCreateRequest{
		Customizations: nil,
	}
	if err := p.PostContainerCreate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostContainerCreate_InstallsAndGeneratesWrapper(t *testing.T) {
	wsDir := t.TempDir()
	// Simulate ssh plugin artifacts.
	if err := os.MkdirAll(filepath.Join(wsDir, "plugins", "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}

	var execCmds []string
	copiedFiles := map[string]string{} // path -> content

	p := New()
	req := &plugin.PostContainerCreateRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    wsDir,
		ContainerID:     "abc123",
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspaces/project",
		Runtime:         "docker",
		Customizations: map[string]any{
			"sandbox": map[string]any{
				"blockLocalNetwork": true,
			},
		},
		ExecFunc: func(_ context.Context, cmd []string, _ string) error {
			execCmds = append(execCmds, strings.Join(cmd, " "))
			return nil
		},
		ExecOutputFunc: func(_ context.Context, cmd []string, _ string) (string, error) {
			// Simulate mktemp returning a temp path.
			if len(cmd) > 0 && cmd[0] == "mktemp" {
				return "/tmp/crib-sandbox-abc123.sh\n", nil
			}
			return "", nil
		},
		CopyFileFunc: func(_ context.Context, content []byte, dest, _, _ string) error {
			copiedFiles[dest] = string(content)
			return nil
		},
	}

	if err := p.PostContainerCreate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First exec installs bwrap, second installs iptables (independent steps).
	if len(execCmds) < 2 || !strings.Contains(execCmds[0], "bwrap") {
		t.Errorf("first exec should install bwrap, got: %v", execCmds)
	}
	if !strings.Contains(execCmds[1], "iptables") {
		t.Errorf("second exec should install iptables, got: %v", execCmds)
	}

	// Network script should be copied to the mktemp path, then executed.
	netScript, ok := copiedFiles["/tmp/crib-sandbox-abc123.sh"]
	if !ok {
		t.Fatal("expected network script to be copied to temp file")
	}
	if !strings.Contains(netScript, "CRIB_SANDBOX") {
		t.Errorf("network script should use CRIB_SANDBOX chain, got:\n%s", netScript)
	}

	// Sandbox wrapper should be copied.
	wrapper, ok := copiedFiles["/home/vscode/.local/bin/sandbox"]
	if !ok {
		t.Fatal("expected sandbox wrapper to be copied")
	}
	if !strings.Contains(wrapper, "exec bwrap") {
		t.Errorf("sandbox wrapper should contain bwrap invocation, got:\n%s", wrapper)
	}
}

func TestPostContainerCreate_WithAliases(t *testing.T) {
	wsDir := t.TempDir()

	copiedFiles := map[string]string{}

	p := New()
	req := &plugin.PostContainerCreateRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    wsDir,
		ContainerID:     "abc123",
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspaces/project",
		Runtime:         "docker",
		Customizations: map[string]any{
			"sandbox": map[string]any{
				"aliases": []any{"claude", "missing-tool"},
			},
		},
		ExecFunc: func(_ context.Context, _ []string, _ string) error {
			return nil
		},
		ExecOutputFunc: func(_ context.Context, cmd []string, _ string) (string, error) {
			cmdStr := ""
			if len(cmd) > 2 {
				cmdStr = cmd[2]
			}
			// Simulate: claude exists at /usr/local/bin/claude, missing-tool does not.
			if strings.Contains(cmdStr, "claude") {
				return "/usr/local/bin/claude\n", nil
			}
			return "", nil
		},
		CopyFileFunc: func(_ context.Context, content []byte, dest, _, _ string) error {
			copiedFiles[dest] = string(content)
			return nil
		},
	}

	if err := p.PostContainerCreate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have written an alias for claude.
	alias, ok := copiedFiles["/home/vscode/.local/bin/claude"]
	if !ok {
		t.Fatal("expected claude alias to be written")
	}
	if !strings.Contains(alias, "[crib sandbox]") {
		t.Errorf("alias should contain sandbox banner, got:\n%s", alias)
	}

	// missing-tool should not have an alias.
	if _, ok := copiedFiles["/home/vscode/.local/bin/missing-tool"]; ok {
		t.Error("missing-tool should not have an alias written")
	}
}

func TestPostContainerCreate_WorktreeAutoDetection(t *testing.T) {
	wsDir := t.TempDir()
	copiedFiles := map[string]string{}

	porcelainOutput := "worktree /workspaces/project\n" +
		"HEAD abc123\n" +
		"branch refs/heads/main\n" +
		"\n" +
		"worktree /workspaces/project-worktrees/feature-a\n" +
		"HEAD def456\n" +
		"branch refs/heads/feature-a\n" +
		"\n"

	p := New()
	req := &plugin.PostContainerCreateRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    wsDir,
		ContainerID:     "abc123",
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspaces/project",
		Runtime:         "docker",
		Customizations: map[string]any{
			"sandbox": map[string]any{},
		},
		ExecFunc: func(_ context.Context, _ []string, _ string) error {
			return nil
		},
		ExecOutputFunc: func(_ context.Context, cmd []string, _ string) (string, error) {
			if len(cmd) >= 3 && cmd[0] == "git" && cmd[2] == "/workspaces/project" {
				return porcelainOutput, nil
			}
			return "", nil
		},
		CopyFileFunc: func(_ context.Context, content []byte, dest, _, _ string) error {
			copiedFiles[dest] = string(content)
			return nil
		},
	}

	if err := p.PostContainerCreate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wrapper, ok := copiedFiles["/home/vscode/.local/bin/sandbox"]
	if !ok {
		t.Fatal("expected sandbox wrapper to be copied")
	}
	if !strings.Contains(wrapper, "--bind-try '/workspaces/project-worktrees' '/workspaces/project-worktrees'") {
		t.Errorf("sandbox wrapper should allow writes to worktree base dir, got:\n%s", wrapper)
	}
}

func TestPostContainerCreate_WorktreeDetectionFailsGracefully(t *testing.T) {
	wsDir := t.TempDir()
	copiedFiles := map[string]string{}

	p := New()
	req := &plugin.PostContainerCreateRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    wsDir,
		ContainerID:     "abc123",
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspaces/project",
		Runtime:         "docker",
		Customizations: map[string]any{
			"sandbox": map[string]any{},
		},
		ExecFunc: func(_ context.Context, _ []string, _ string) error {
			return nil
		},
		ExecOutputFunc: func(_ context.Context, cmd []string, _ string) (string, error) {
			if len(cmd) >= 3 && cmd[0] == "git" {
				return "", fmt.Errorf("git not found")
			}
			return "", nil
		},
		CopyFileFunc: func(_ context.Context, content []byte, dest, _, _ string) error {
			copiedFiles[dest] = string(content)
			return nil
		},
	}

	if err := p.PostContainerCreate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still generate wrapper without worktree paths.
	wrapper, ok := copiedFiles["/home/vscode/.local/bin/sandbox"]
	if !ok {
		t.Fatal("expected sandbox wrapper to be copied even when git fails")
	}
	if strings.Contains(wrapper, "worktree") {
		t.Errorf("wrapper should not reference worktrees when git fails, got:\n%s", wrapper)
	}
}

func TestPostContainerCreate_InvalidAliasNamesSkipped(t *testing.T) {
	wsDir := t.TempDir()
	copiedFiles := map[string]string{}

	p := New()
	req := &plugin.PostContainerCreateRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    wsDir,
		ContainerID:     "abc123",
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspaces/project",
		Runtime:         "docker",
		Customizations: map[string]any{
			"sandbox": map[string]any{
				"aliases": []any{"valid-name", "bad;name", "../escape", "also bad", ".", "..", "-flag"},
			},
		},
		ExecFunc: func(_ context.Context, _ []string, _ string) error {
			return nil
		},
		ExecOutputFunc: func(_ context.Context, cmd []string, _ string) (string, error) {
			if len(cmd) > 2 && strings.Contains(cmd[2], "valid-name") {
				return "/usr/bin/valid-name\n", nil
			}
			return "", nil
		},
		CopyFileFunc: func(_ context.Context, content []byte, dest, _, _ string) error {
			copiedFiles[dest] = string(content)
			return nil
		},
	}

	if err := p.PostContainerCreate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only valid-name should have an alias.
	if _, ok := copiedFiles["/home/vscode/.local/bin/valid-name"]; !ok {
		t.Error("expected valid-name alias to be written")
	}
	for _, bad := range []string{"bad;name", "../escape", "also bad", ".", "..", "-flag"} {
		path := "/home/vscode/.local/bin/" + bad
		if _, ok := copiedFiles[path]; ok {
			t.Errorf("invalid alias %q should have been skipped", bad)
		}
	}
}
