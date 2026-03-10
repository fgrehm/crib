package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2ERestartSimple verifies that "crib restart" with no config change
// restarts the container in place (same container ID) and runs resume hooks.
func TestE2ERestartSimple(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm", "--force")
		_ = cmd.Run()
	})

	// Bring the container up first.
	out := mustRunCrib(t, projectDir, cribHome, "up")
	name := extractContainerName(out)
	if name == "" {
		t.Fatalf("could not extract container name from up output: %q", out)
	}
	id1 := containerRealID(t, name)

	// Restart with no config change: should reuse the container (simple restart).
	out = mustRunCrib(t, projectDir, cribHome, "restart")
	if !strings.Contains(strings.ToLower(out), "restarted") {
		t.Errorf("restart: want 'restarted' in output, got %q", out)
	}
	if strings.Contains(strings.ToLower(out), "recreated") {
		t.Errorf("restart: want simple restart, not recreate; got %q", out)
	}

	// Container ID should be identical (simple restart does not recreate).
	id2 := containerRealID(t, name)
	if id1 != id2 {
		t.Errorf("simple restart: want same container ID %q, got %q", id1, id2)
	}

	// Container should still be running.
	out = mustRunCrib(t, projectDir, cribHome, "status")
	if !strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("status after restart: want 'running', got %q", out)
	}
}

// TestE2ERestartRecreate verifies that "crib restart" with a safe config change
// (new containerEnv entry) recreates the container without a full rebuild and
// makes the new env var available.
func TestE2ERestartRecreate(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm", "--force")
		_ = cmd.Run()
	})

	// Bring the container up first.
	out := mustRunCrib(t, projectDir, cribHome, "up")
	name := extractContainerName(out)
	if name == "" {
		t.Fatalf("could not extract container name from up output: %q", out)
	}
	id1 := containerRealID(t, name)

	// Modify devcontainer.json: add a new containerEnv entry (safe change).
	updatedConfig := `{
	"name": "e2e-test",
	"image": "alpine:3.20",
	"overrideCommand": true,
	"containerEnv": {
		"CRIB_E2E": "true",
		"CRIB_RECREATE_TEST": "1"
	},
	"postCreateCommand": "touch /tmp/post-create-ran"
}`
	devcontainerPath := filepath.Join(projectDir, ".devcontainer", "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(updatedConfig), 0o644); err != nil {
		t.Fatalf("writing updated devcontainer.json: %v", err)
	}

	// Restart should detect the env change and recreate (not rebuild).
	out = mustRunCrib(t, projectDir, cribHome, "restart")
	if !strings.Contains(strings.ToLower(out), "recreated") {
		t.Errorf("restart with env change: want 'recreated' in output, got %q", out)
	}

	// Container ID should differ after recreation.
	id2 := containerRealID(t, name)
	if id1 == id2 {
		t.Errorf("restart recreate: want new container ID, got same %q", id1)
	}

	// The new containerEnv should be available inside the container.
	out = mustRunCrib(t, projectDir, cribHome, "exec", "--", "printenv", "CRIB_RECREATE_TEST")
	if strings.TrimSpace(lastLine(out)) != "1" {
		t.Errorf("CRIB_RECREATE_TEST: want '1', got %q", out)
	}
}

// TestE2ERestartNoPreviousUp verifies that "crib restart" fails gracefully
// when no container has been brought up yet.
func TestE2ERestartNoPreviousUp(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	_, err := runCrib(t, projectDir, cribHome, "restart")
	if err == nil {
		t.Error("restart without prior up: want error, got nil")
	}
}
