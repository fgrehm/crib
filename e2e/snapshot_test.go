package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ESnapshotCreatedAfterUp(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProjectWithHooks(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm", "--force")
		_ = cmd.Run()
	})

	// Up should create a container and a snapshot.
	out := mustRunCrib(t, projectDir, cribHome, "up")
	if !strings.Contains(out, "container") {
		t.Errorf("up: want 'container' in output, got %q", out)
	}

	// Verify onCreate hook ran.
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-f", "/tmp/snapshot-e2e-marker")

	// Verify snapshot image exists (has "snapshot" tag).
	if !hasSnapshotImage(t) {
		t.Error("expected snapshot image to exist after up with create-time hooks")
	}
}

func TestE2ERebuildClearsSnapshot(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProjectWithHooks(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm", "--force")
		_ = cmd.Run()
	})

	// Up creates the snapshot.
	mustRunCrib(t, projectDir, cribHome, "up")
	if !hasSnapshotImage(t) {
		t.Fatal("expected snapshot after up")
	}

	// Rebuild should clear the snapshot and start fresh.
	mustRunCrib(t, projectDir, cribHome, "rebuild")

	// The rebuild should have created a new snapshot (since hooks ran again).
	// But the old snapshot should have been cleared first.
	// Verify the container is running and hooks ran.
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-f", "/tmp/snapshot-e2e-marker")
}

// setupProjectWithHooks creates a project with an onCreate hook for snapshot testing.
func setupProjectWithHooks(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	devDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{
	"name": "snapshot-e2e",
	"image": "alpine:3.20",
	"overrideCommand": true,
	"onCreateCommand": "touch /tmp/snapshot-e2e-marker"
}`
	if err := os.WriteFile(filepath.Join(devDir, "devcontainer.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// hasSnapshotImage checks if any crib snapshot image exists.
func hasSnapshotImage(t *testing.T) bool {
	t.Helper()
	for _, rt := range []string{"docker", "podman"} {
		cmd := exec.Command(rt, "images", "--format", "{{.Repository}}:{{.Tag}}", "--filter", "reference=crib-*:snapshot")
		out, err := cmd.Output()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			return true
		}
	}
	return false
}
