package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EShellHistory(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm")
		_ = cmd.Run()
	})

	// Up with default alpine config.
	mustRunCrib(t, projectDir, cribHome, "up")

	// Verify HISTFILE is set and points to the .crib_history directory.
	out := mustRunCrib(t, projectDir, cribHome, "exec", "--", "printenv", "HISTFILE")
	histfile := strings.TrimSpace(lastLine(out))
	if !strings.Contains(histfile, ".crib_history/.shell_history") {
		t.Errorf("HISTFILE = %q, want to contain '.crib_history/.shell_history'", histfile)
	}

	// Verify the history file was created on the host by the plugin.
	matches, err := filepath.Glob(filepath.Join(cribHome, "workspaces", "*", "plugins", "shell-history", ".shell_history"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("shell history file not found on host")
	}
}

