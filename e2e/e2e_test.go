// Package e2e contains end-to-end tests that exercise the crib binary against
// a real container runtime. Tests are skipped when no runtime is available.
//
// Run with:
//
//	make test-e2e
package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// cribBin is the path to the compiled crib binary, set by TestMain.
var cribBin string

func TestMain(m *testing.M) {
	bin, cleanup, err := buildCrib()
	if err != nil {
		fmt.Fprintf(os.Stderr, "building crib: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()
	cribBin = bin
	os.Exit(m.Run())
}

// buildCrib compiles the crib binary into a temp directory and returns its
// path along with a cleanup function.
func buildCrib() (string, func(), error) {
	dir, err := os.MkdirTemp("", "crib-e2e-bin-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }

	bin := filepath.Join(dir, "crib")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	// e2e/ is one level below the repo root.
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		cleanup()
		return "", nil, err
	}

	cmd := exec.Command("go", "build", "-o", bin, repoRoot)
	cmd.Stdout = os.Stderr // build output goes to stderr so it doesn't pollute test output
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("go build: %w", err)
	}

	return bin, cleanup, nil
}

// hasRuntime returns true if a container runtime (docker or podman) is available
// and can actually run containers. This is called to verify the test environment
// is properly configured; tests fail if the runtime is not working.
func hasRuntime() bool {
	for _, rt := range []string{"docker", "podman"} {
		// Try to run a simple container to verify the daemon is working.
		cmd := exec.Command(rt, "run", "--rm", "alpine", "true")
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

// cribCmd builds an exec.Cmd for the crib binary with the given args,
// running in projectDir with workspace state isolated to cribHome (CRIB_HOME).
// Stdin is explicitly wired to /dev/null so that crib's TTY detection returns
// false, preventing "the input device is not a TTY" errors from Docker when
// crib exec replaces itself via syscall.Exec.
func cribCmd(projectDir, cribHome string, args ...string) *exec.Cmd {
	cmd := exec.Command(cribBin, args...)
	cmd.Dir = projectDir
	devNull, _ := os.Open(os.DevNull)
	cmd.Stdin = devNull
	cmd.Env = append(os.Environ(), "CRIB_HOME="+cribHome)
	return cmd
}

// runCrib runs the crib binary and returns combined stdout+stderr output.
func runCrib(t *testing.T, projectDir, cribHome string, args ...string) (string, error) {
	t.Helper()
	cmd := cribCmd(projectDir, cribHome, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// mustRunCrib runs crib and fails the test if it exits non-zero.
func mustRunCrib(t *testing.T, projectDir, cribHome string, args ...string) string {
	t.Helper()
	out, err := runCrib(t, projectDir, cribHome, args...)
	if err != nil {
		t.Fatalf("crib %v: %v\noutput:\n%s", args, err, out)
	}
	return out
}

// devcontainerJSON creates a simple devcontainer.json for testing.
const devcontainerJSON = `{
	"name": "e2e-test",
	"image": "alpine:3.20",
	"overrideCommand": true,
	"containerEnv": {
		"CRIB_E2E": "true"
	},
	"postCreateCommand": "touch /tmp/post-create-ran"
}`

// setupProject creates a temporary project directory with a devcontainer.json.
func setupProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	devDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "devcontainer.json"), []byte(devcontainerJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestE2EFullLifecycle(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	// Derive the workspace ID the same way crib does: slugify the dir basename.
	wsID := filepath.Base(projectDir)
	// TempDir names contain hyphens and digits, already slug-compatible.

	// 1. list - nothing yet.
	out := mustRunCrib(t, projectDir, cribHome, "list")
	if !strings.Contains(strings.ToLower(out), "no workspaces") {
		t.Errorf("list before up: want 'no workspaces', got %q", out)
	}

	// 2. up - brings the container up.
	out = mustRunCrib(t, projectDir, cribHome, "up")
	if !strings.Contains(out, "container") {
		t.Errorf("up: want 'container' in output, got %q", out)
	}
	if !strings.Contains(out, "workspace") {
		t.Errorf("up: want 'workspace' in output, got %q", out)
	}

	// 3. list - workspace should appear.
	out = mustRunCrib(t, projectDir, cribHome, "list")
	if !strings.Contains(out, wsID) {
		t.Errorf("list after up: want workspace ID %q in output, got %q", wsID, out)
	}

	// 4. status - container should be running.
	out = mustRunCrib(t, projectDir, cribHome, "status")
	if !strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("status after up: want 'running', got %q", out)
	}

	// 5. exec - run a command inside the container.
	out = mustRunCrib(t, projectDir, cribHome, "exec", "--", "echo", "hello-from-container")
	if !strings.Contains(out, "hello-from-container") {
		t.Errorf("exec: want 'hello-from-container' in output, got %q", out)
	}

	// 6. exec - verify containerEnv was set.
	out = mustRunCrib(t, projectDir, cribHome, "exec", "--", "printenv", "CRIB_E2E")
	if !strings.Contains(strings.TrimSpace(out), "true") {
		t.Errorf("exec printenv CRIB_E2E: want 'true', got %q", out)
	}

	// 7. exec - verify postCreate hook ran.
	out = mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-f", "/tmp/post-create-ran")
	_ = out // exit 0 means the file exists; mustRunCrib already asserts that

	// 8. down - stops and removes the container.
	mustRunCrib(t, projectDir, cribHome, "down")

	// 9. status - container should be gone (down removes it).
	out = mustRunCrib(t, projectDir, cribHome, "status")
	if strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("status after down: want not-running, got %q", out)
	}

	// 10. up again - should create a new container (down removed the old one).
	out = mustRunCrib(t, projectDir, cribHome, "up")
	if !strings.Contains(out, "container") {
		t.Errorf("second up: want 'container' in output, got %q", out)
	}

	// 11. status - container should be running again.
	out = mustRunCrib(t, projectDir, cribHome, "status")
	if !strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("status after second up: want 'running', got %q", out)
	}

	// 12. rebuild - removes and recreates the container.
	out = mustRunCrib(t, projectDir, cribHome, "rebuild")
	if !strings.Contains(out, "container") {
		t.Errorf("rebuild: want 'container' in output, got %q", out)
	}

	// 13. status - should be running after rebuild.
	out = mustRunCrib(t, projectDir, cribHome, "status")
	if !strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("status after rebuild: want 'running', got %q", out)
	}

	// 14. remove - removes container and workspace state.
	mustRunCrib(t, projectDir, cribHome, "remove")

	// 15. list - workspace should be gone.
	out = mustRunCrib(t, projectDir, cribHome, "list")
	if !strings.Contains(strings.ToLower(out), "no workspaces") {
		t.Errorf("list after remove: want 'no workspaces', got %q", out)
	}

	// 16. status - should error (no workspace).
	_, err := runCrib(t, projectDir, cribHome, "status")
	if err == nil {
		t.Error("status after remove: want error, got nil")
	}
}

func TestE2EUpRecreate(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	// First up.
	out1 := mustRunCrib(t, projectDir, cribHome, "up")

	// Second up with --recreate.
	out2 := mustRunCrib(t, projectDir, cribHome, "up", "--recreate")

	// Container IDs should differ.
	id1 := extractContainerID(out1)
	id2 := extractContainerID(out2)
	if id1 == "" || id2 == "" {
		t.Fatalf("could not extract container IDs: first=%q second=%q", out1, out2)
	}
	if id1 == id2 {
		t.Errorf("up --recreate: want new container ID, got same %q", id1)
	}

	mustRunCrib(t, projectDir, cribHome, "remove")
}

func TestE2ENoDevcontainer(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	// A directory with no devcontainer.json.
	emptyDir := t.TempDir()
	cribHome := t.TempDir()

	_, err := runCrib(t, emptyDir, cribHome, "up")
	if err == nil {
		t.Error("up in dir without devcontainer.json: want error, got nil")
	}
}

func TestE2EShellRejectsArgs(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	// No up needed: cobra's Args validator rejects before RunE.
	projectDir := setupProject(t)
	cribHome := t.TempDir()

	out, err := runCrib(t, projectDir, cribHome, "shell", "--", "foobar")
	if err == nil {
		t.Fatal("shell with args: want error, got nil")
	}
	if !strings.Contains(out, "crib exec") {
		t.Errorf("shell with args: want output to mention 'crib exec', got %q", out)
	}
}

func TestE2ERebuildNoContainer(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir := setupProject(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "rm")
		_ = cmd.Run()
	})

	// Rebuild with no prior up (no existing container).
	mustRunCrib(t, projectDir, cribHome, "rebuild")

	// Verify the container is running.
	out := mustRunCrib(t, projectDir, cribHome, "ps")
	if !strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("ps after rebuild: want 'running', got %q", out)
	}
}

// extractContainerID pulls the short container ID from `crib up` output.
// Output line looks like: "  container   abc123def456"
func extractContainerID(out string) string {
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "container") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return ""
}
