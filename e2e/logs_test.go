package e2e

import (
	"strings"
	"testing"
)

func TestE2ELogs(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm")
		_ = cmd.Run()
	})

	mustRunCrib(t, projectDir, cribHome, "up")

	// Default logs: should produce some output.
	out := mustRunCrib(t, projectDir, cribHome, "logs")
	if len(strings.TrimSpace(out)) == 0 {
		t.Error("logs: expected non-empty output")
	}

	// Logs with --tail.
	out = mustRunCrib(t, projectDir, cribHome, "logs", "--tail", "3")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > 3 {
		t.Errorf("logs --tail 3: got %d lines, want <= 3", len(lines))
	}
}

func TestE2ELogsNoWorkspace(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	// No up: logs should fail.
	_, err := runCrib(t, projectDir, cribHome, "logs")
	if err == nil {
		t.Error("logs without up: want error, got nil")
	}
}
