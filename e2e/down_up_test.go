package e2e

import (
	"strings"
	"testing"
)

// TestE2EDownUpCycle verifies that down + up works correctly:
// - down removes the container but keeps workspace state
// - up after down creates a new container without a full rebuild
// - lifecycle hooks re-run after down (markers cleared)
func TestE2EDownUpCycle(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm")
		_ = cmd.Run()
	})

	// First up.
	out1 := mustRunCrib(t, projectDir, cribHome, "up")
	name1 := extractContainerName(out1)
	if name1 == "" {
		t.Fatalf("could not extract container name from first up: %q", out1)
	}
	id1 := containerRealID(t, name1)

	// Verify postCreateCommand ran.
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-f", "/tmp/post-create-ran")

	// Down.
	mustRunCrib(t, projectDir, cribHome, "down")

	// Workspace should still be listed (down keeps state).
	out := mustRunCrib(t, projectDir, cribHome, "ls")
	if strings.Contains(strings.ToLower(out), "no workspaces") {
		t.Error("workspace should still be listed after down")
	}

	// Up again.
	out2 := mustRunCrib(t, projectDir, cribHome, "up")
	name2 := extractContainerName(out2)
	if name2 == "" {
		t.Fatalf("could not extract container name from second up: %q", out2)
	}
	id2 := containerRealID(t, name2)

	// Container ID should differ (down removed the old one, up created a new one).
	if id1 == id2 {
		t.Error("expected different container ID after down + up")
	}

	// postCreateCommand should have run again (markers cleared by down).
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-f", "/tmp/post-create-ran")

	// Clean up.
	mustRunCrib(t, projectDir, cribHome, "rm")
}

// TestE2EDownUpComposeSkipsBuild verifies that down + up for compose workspaces
// doesn't trigger a full image rebuild.
func TestE2EDownUpComposeSkipsBuild(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}
	if !hasCompose() {
		t.Fatal("docker compose or podman compose not available")
	}

	projectDir := setupComposeProject(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm")
		_ = cmd.Run()
	})

	// First up (full creation).
	mustRunCrib(t, projectDir, cribHome, "up")

	// Down.
	mustRunCrib(t, projectDir, cribHome, "down")

	// Up again. Should not contain "Building" in output (images already exist).
	out := mustRunCrib(t, projectDir, cribHome, "up")
	if strings.Contains(out, "Building image") || strings.Contains(out, "Building service") {
		t.Errorf("second up after down should skip build, got:\n%s", out)
	}

	// postCreateCommand should still run (markers were cleared).
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-f", "/tmp/post-create-ran")
}
