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
		cmd := cribCmd(projectDir, cribHome, "rm", "--force")
		_ = cmd.Run()
	})
	mustRunCrib(t, projectDir, cribHome, "up")

	// Doctor after up should find no issues for our workspace.
	out = mustRunCrib(t, projectDir, cribHome, "doctor")
	for line := range strings.SplitSeq(out, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "warning") && !strings.Contains(lower, "dangling-container") {
			t.Errorf("doctor after up: unexpected warning: %s", line)
		}
	}

	// Doctor --fix should also succeed with no issues.
	out = mustRunCrib(t, projectDir, cribHome, "doctor", "--fix")
	if strings.Contains(strings.ToLower(out), "error") {
		t.Errorf("doctor --fix: unexpected error in output: %s", out)
	}
}
