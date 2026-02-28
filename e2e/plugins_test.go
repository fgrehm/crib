package e2e

import (
	"os"
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

func TestE2EForwardPorts(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	// Create a project with forwardPorts.
	dir := t.TempDir()
	devDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configContent := `{
	"name": "forward-ports-e2e",
	"image": "alpine:3.20",
	"overrideCommand": true,
	"forwardPorts": [8080]
}`
	if err := os.WriteFile(filepath.Join(devDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(dir, cribHome, "rm")
		_ = cmd.Run()
	})

	// Up and verify port appears in output.
	out := mustRunCrib(t, dir, cribHome, "up")
	if !strings.Contains(out, "8080") {
		t.Errorf("up output should contain '8080', got %q", out)
	}

	// Verify ps also shows the port.
	out = mustRunCrib(t, dir, cribHome, "ps")
	if !strings.Contains(out, "8080") {
		t.Errorf("ps output should contain '8080', got %q", out)
	}
}
