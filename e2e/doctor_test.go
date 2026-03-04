package e2e

import (
	"strings"
	"testing"
)

func TestE2EDoctor(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	// Doctor on a clean state should succeed with no issues.
	out := mustRunCrib(t, projectDir, cribHome, "doctor")
	if strings.Contains(strings.ToLower(out), "error") {
		t.Errorf("doctor: unexpected error in output: %s", out)
	}

	// Bring up a workspace so there's something to check.
	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm")
		_ = cmd.Run()
	})

	mustRunCrib(t, projectDir, cribHome, "up")

	// Doctor after up should still find no issues.
	out = mustRunCrib(t, projectDir, cribHome, "doctor")
	if strings.Contains(strings.ToLower(out), "warning") {
		t.Errorf("doctor after up: unexpected warning: %s", out)
	}
}

func TestE2EDoctorFix(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	// Doctor --fix on clean state should succeed.
	out := mustRunCrib(t, projectDir, cribHome, "doctor", "--fix")
	_ = out // no-op fix is fine
}
