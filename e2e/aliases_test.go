package e2e

import (
	"strings"
	"testing"
)

// TestE2EAliases verifies that command aliases work correctly.
func TestE2EAliases(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}
	t.Parallel()

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm", "--force")
		_ = cmd.Run()
	})

	// up the workspace.
	mustRunCrib(t, projectDir, cribHome, "up")

	// "ps" alias for "status".
	out := mustRunCrib(t, projectDir, cribHome, "ps")
	if !strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("ps: want 'running', got %q", out)
	}

	// "stop" is its own command (non-destructive), not an alias for "down".
	mustRunCrib(t, projectDir, cribHome, "stop")
	out = mustRunCrib(t, projectDir, cribHome, "ps")
	if strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("ps after stop: want not-running, got %q", out)
	}
	// Container should still exist after stop (not removed).
	out = mustRunCrib(t, projectDir, cribHome, "ls")
	if strings.Contains(strings.ToLower(out), "no workspaces") {
		t.Errorf("ls after stop: workspace should still exist, got %q", out)
	}
	// Resume with up.
	mustRunCrib(t, projectDir, cribHome, "up")
	out = mustRunCrib(t, projectDir, cribHome, "ps")
	if !strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("ps after stop+up: want 'running', got %q", out)
	}

	// "ls" alias for "list".
	out = mustRunCrib(t, projectDir, cribHome, "ls")
	if strings.Contains(strings.ToLower(out), "no workspaces") {
		t.Errorf("ls: want workspace listed, got %q", out)
	}

	// "rm" alias for "remove".
	mustRunCrib(t, projectDir, cribHome, "rm", "--force")
	out = mustRunCrib(t, projectDir, cribHome, "ls")
	if !strings.Contains(strings.ToLower(out), "no workspaces") {
		t.Errorf("ls after rm: want 'no workspaces', got %q", out)
	}
}
