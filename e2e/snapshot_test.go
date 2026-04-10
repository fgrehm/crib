package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ESnapshot(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}
	t.Parallel()

	projectDir := setupProjectWithHooks(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm", "--force")
		_ = cmd.Run()
	})

	// Up should create a container and a snapshot.
	out := mustRunCrib(t, projectDir, cribHome, "up")
	containerName := extractContainerName(out)
	if containerName == "" {
		t.Fatalf("could not extract container name from up output: %q", out)
	}
	// Container name is "crib-{wsID}", derive wsID.
	if !strings.HasPrefix(containerName, "crib-") {
		t.Fatalf("unexpected container name format %q, want crib-* prefix", containerName)
	}
	wsID := strings.TrimPrefix(containerName, "crib-")

	// Verify onCreate hook ran.
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-f", "/tmp/snapshot-e2e-marker")

	// Verify snapshot image exists for this workspace.
	if !hasSnapshotImage(t, wsID) {
		t.Fatal("expected snapshot image to exist after up with create-time hooks")
	}

	// Rebuild should clear the old snapshot and start fresh.
	mustRunCrib(t, projectDir, cribHome, "rebuild")

	// Verify hooks ran again after rebuild.
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

// hasSnapshotImage checks if a snapshot image exists for the given workspace.
func hasSnapshotImage(t *testing.T, wsID string) bool {
	t.Helper()
	ref := "crib-" + wsID + ":snapshot"
	for _, rt := range []string{"docker", "podman"} {
		cmd := exec.Command(rt, "image", "inspect", ref)
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}
