package compose

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func detectTestRuntime(t *testing.T) string {
	t.Helper()
	// Try docker first (more common in CI), then podman.
	for _, cmd := range []string{"docker", "podman"} {
		if path, err := exec.LookPath(cmd); err == nil {
			if out, err := exec.Command(path, "compose", "version", "--short").CombinedOutput(); err == nil {
				t.Logf("using runtime: %s (compose %s)", cmd, strings.TrimSpace(string(out)))
				return cmd
			}
		}
	}
	t.Skip("skipping: no container runtime with compose support found")
	return ""
}

func TestIntegrationComposeUpDown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	runtimeCmd := detectTestRuntime(t)

	h, err := NewHelper(runtimeCmd, slog.Default())
	if err != nil {
		t.Fatalf("NewHelper: %v", err)
	}

	_, thisFile, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(thisFile), "testdata")
	composePath := filepath.Join(testdataDir, "simple-compose.yml")

	projectName := "crib-test-compose"

	// Clean up any leftover state.
	_ = h.Down(ctx, projectName, []string{composePath}, nil, nil, nil)

	t.Cleanup(func() {
		_ = h.Down(ctx, projectName, []string{composePath}, nil, nil, nil)
	})

	// Bring up the project.
	var stdout, stderr bytes.Buffer
	if err := h.Up(ctx, projectName, []string{composePath}, nil, &stdout, &stderr, nil); err != nil {
		t.Fatalf("Up: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Verify the container exists using docker/podman ps.
	out, err := exec.CommandContext(ctx, runtimeCmd, "ps", "--filter", "label="+ProjectLabel+"="+projectName, "--format", "{{.Names}}").CombinedOutput()
	if err != nil {
		t.Fatalf("ps: %v: %s", err, string(out))
	}

	names := strings.TrimSpace(string(out))
	if names == "" {
		t.Error("no containers found after compose up")
	}
	if !strings.Contains(names, projectName) {
		t.Errorf("container names %q do not contain project name %q", names, projectName)
	}

	// Stop the project.
	stdout.Reset()
	stderr.Reset()
	if err := h.Stop(ctx, projectName, []string{composePath}, &stdout, &stderr, nil); err != nil {
		t.Fatalf("Stop: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Bring it down.
	stdout.Reset()
	stderr.Reset()
	if err := h.Down(ctx, projectName, []string{composePath}, &stdout, &stderr, nil); err != nil {
		t.Fatalf("Down: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Verify containers are gone.
	out, err = exec.CommandContext(ctx, runtimeCmd, "ps", "-a", "--filter", "label="+ProjectLabel+"="+projectName, "--format", "{{.Names}}").CombinedOutput()
	if err != nil {
		t.Fatalf("ps after down: %v: %s", err, string(out))
	}

	if names := strings.TrimSpace(string(out)); names != "" {
		t.Errorf("containers still exist after down: %s", names)
	}
}
