package e2e

import (
	"strings"
	"testing"
)

func TestE2EDoctor(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}
	t.Parallel()

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	// Doctor on a clean state should report no issues.
	// mustRunCrib already asserts exit code 0. We only check the summary
	// line rather than scanning for "error" because parallel tests can cause
	// transient WARN logs (e.g., containers vanishing between list and inspect).
	out := mustRunCrib(t, projectDir, cribHome, "doctor")
	if !strings.Contains(out, "No issues found") {
		t.Errorf("doctor: want 'No issues found' in output, got %q", out)
	}

	// Bring up a workspace so there's something to check.
	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm", "--force")
		_ = cmd.Run()
	})
	mustRunCrib(t, projectDir, cribHome, "up")

	// Doctor after up should find no issues for our workspace.
	out = mustRunCrib(t, projectDir, cribHome, "doctor")
	if !strings.Contains(out, "No issues found") {
		t.Errorf("doctor after up: want 'No issues found', got %q", out)
	}

	// Doctor --fix should also succeed with no issues.
	out = mustRunCrib(t, projectDir, cribHome, "doctor", "--fix")
	if !strings.Contains(out, "No issues found") {
		t.Errorf("doctor --fix: want 'No issues found', got %q", out)
	}
}
