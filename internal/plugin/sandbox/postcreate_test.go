package sandbox

import (
	"context"
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
	var execOutputCmds []string

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
			if len(cmd) > 2 {
				execCmds = append(execCmds, cmd[2])
			}
			return nil
		},
		ExecOutputFunc: func(_ context.Context, cmd []string, _ string) (string, error) {
			if len(cmd) > 2 {
				execOutputCmds = append(execOutputCmds, cmd[2])
			}
			return "", nil
		},
	}

	if err := p.PostContainerCreate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have at least: install bwrap, apply network rules, mkdir, write wrapper.
	if len(execCmds) < 4 {
		t.Fatalf("expected at least 4 exec calls, got %d: %v", len(execCmds), execCmds)
	}

	// First call should install tools (bwrap + iptables).
	if !strings.Contains(execCmds[0], "bwrap") || !strings.Contains(execCmds[0], "iptables") {
		t.Errorf("first exec should check for bwrap and iptables, got: %s", execCmds[0])
	}

	// Second call should apply network rules (iptables).
	if !strings.Contains(execCmds[1], "iptables") {
		t.Errorf("second exec should apply network rules, got: %s", execCmds[1])
	}

	// Third call should create ~/.local/bin.
	if !strings.Contains(execCmds[2], ".local/bin") {
		t.Errorf("third exec should create local bin, got: %s", execCmds[2])
	}

	// Fourth call should write the sandbox wrapper (base64 encoded).
	if !strings.Contains(execCmds[3], "base64 -d") {
		t.Errorf("fourth exec should write sandbox wrapper via base64, got: %s", execCmds[3])
	}
}

func TestPostContainerCreate_WithAliases(t *testing.T) {
	wsDir := t.TempDir()

	var execCmds []string
	execOutputResults := map[string]string{}

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
		ExecFunc: func(_ context.Context, cmd []string, _ string) error {
			if len(cmd) > 2 {
				execCmds = append(execCmds, cmd[2])
			}
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
			result := execOutputResults[cmdStr]
			return result, nil
		},
	}

	if err := p.PostContainerCreate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have written an alias for claude but not for missing-tool.
	foundClaudeAlias := false
	for _, cmd := range execCmds {
		if strings.Contains(cmd, "claude") && strings.Contains(cmd, "base64 -d") {
			foundClaudeAlias = true
		}
	}
	if !foundClaudeAlias {
		t.Error("expected claude alias to be written")
	}

	// missing-tool should not have an alias (ExecOutputFunc returns empty).
	for _, cmd := range execCmds {
		if strings.Contains(cmd, "missing-tool") && strings.Contains(cmd, "base64 -d") {
			t.Error("missing-tool should not have an alias written")
		}
	}
}
