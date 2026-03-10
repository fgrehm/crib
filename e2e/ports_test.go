package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
		cmd := cribCmd(dir, cribHome, "rm", "--force")
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
