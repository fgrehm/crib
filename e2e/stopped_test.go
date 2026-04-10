package e2e

import (
	"strings"
	"testing"
)

// TestE2EStoppedContainerErrors verifies that exec, run, and shell return a
// clear error when the container is stopped rather than surfacing the raw
// Docker/Podman "container state improper" message.
func TestE2EStoppedContainerErrors(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}
	t.Parallel()

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm", "--force")
		_ = cmd.Run()
	})

	mustRunCrib(t, projectDir, cribHome, "up")
	mustRunCrib(t, projectDir, cribHome, "stop")

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"exec", []string{"exec", "--", "echo", "hi"}},
		{"run", []string{"run", "--", "echo", "hi"}},
		{"shell", []string{"shell"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCrib(t, projectDir, cribHome, tc.args...)
			if err == nil {
				t.Fatalf("%s on stopped container: want error, got nil\noutput: %s", tc.name, out)
			}
			if !strings.Contains(out, "container is stopped") {
				t.Errorf("%s on stopped container: want 'container is stopped' in output, got %q", tc.name, out)
			}
			if !strings.Contains(out, "crib up") {
				t.Errorf("%s on stopped container: want 'crib up' hint in output, got %q", tc.name, out)
			}
		})
	}
}
