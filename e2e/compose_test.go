package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// composeDockerCompose is the docker-compose.yml for the compose e2e test.
// Uses alpine (small, fast) for the app service and redis for the db service.
// PROJECT uses ${localWorkspaceFolderBasename} to verify compose var substitution.
const composeDockerCompose = `services:
  app:
    image: alpine:3.20
    environment:
      PROJECT: ${localWorkspaceFolderBasename}
  db:
    image: redis:7-alpine
`

// composeDevcontainerJSON is the devcontainer.json for the compose e2e test.
// Exercises: workspaceFolder variable expansion, remoteEnv with ${containerEnv:VAR},
// runServices, and postCreateCommand.
const composeDevcontainerJSON = `{
	"name": "compose-e2e",
	"dockerComposeFile": "docker-compose.yml",
	"service": "app",
	"runServices": ["app", "db"],
	"workspaceFolder": "/workspaces/${localWorkspaceFolderBasename}",
	"remoteEnv": {
		"PROJECT_NAME": "${containerEnv:PROJECT}",
		"REDIS_URL": "redis://db:6379"
	},
	"postCreateCommand": "touch /tmp/post-create-ran"
}`

// setupComposeProject creates a temporary project directory with a unique
// basename derived from the test name so parallel tests don't collide on
// workspace/compose project names.
func setupComposeProject(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	// Use a sanitized test name as basename so each parallel test gets a
	// unique workspace ID and compose project name.
	basename := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dir := filepath.Join(parent, basename)
	devDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"docker-compose.yml": composeDockerCompose,
		"devcontainer.json":  composeDevcontainerJSON,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(devDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// hasCompose reports whether docker compose or podman compose is available.
func hasCompose() bool {
	for _, args := range [][]string{
		{"docker", "compose", "version"},
		{"podman", "compose", "version"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

// lastLine returns the last non-empty line of output, handling log messages on earlier lines.
func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// TestE2EComposeRestartOnFileChange verifies that "crib restart" detects
// a volume change inside a compose file and recreates the container, even
// though devcontainer.json itself hasn't changed.
func TestE2EComposeRestartOnFileChange(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}
	if !hasCompose() {
		t.Fatal("docker compose or podman compose not available")
	}
	t.Parallel()

	projectDir := setupComposeProject(t)
	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "remove", "--force")
		_ = cmd.Run()
	})

	// Initial up.
	mustRunCrib(t, projectDir, cribHome, "up")

	// Restart with no changes: should be a simple restart.
	out := mustRunCrib(t, projectDir, cribHome, "restart")
	if strings.Contains(strings.ToLower(out), "recreated") {
		t.Errorf("restart without changes: want simple restart, got recreate; output: %q", out)
	}

	// Modify compose file: add a volume.
	composeFile := filepath.Join(projectDir, ".devcontainer", "docker-compose.yml")
	updated := composeDockerCompose + `    volumes:
      - appdata:/data
volumes:
  appdata:
`
	if err := os.WriteFile(composeFile, []byte(updated), 0o644); err != nil {
		t.Fatalf("writing updated compose file: %v", err)
	}

	// Restart should detect the compose file change and recreate.
	out = mustRunCrib(t, projectDir, cribHome, "restart")
	if !strings.Contains(strings.ToLower(out), "recreated") {
		t.Errorf("restart after compose change: want 'recreated' in output, got %q", out)
	}

	// Container should still be running after recreate.
	out = mustRunCrib(t, projectDir, cribHome, "status")
	if !strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("status after restart recreate: want 'running', got %q", out)
	}
}

func TestE2ECompose(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}
	if !hasCompose() {
		t.Fatal("docker compose or podman compose not available")
	}
	t.Parallel()

	projectDir := setupComposeProject(t)
	cribHome := t.TempDir()

	// Best-effort cleanup so containers don't linger if the test fails mid-way.
	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "remove", "--force")
		_ = cmd.Run()
	})

	// 1. up - bring both services up.
	out := mustRunCrib(t, projectDir, cribHome, "up")
	if !strings.Contains(out, "container") {
		t.Errorf("up: want 'container' in output, got %q", out)
	}
	// Workspace folder must have ${localWorkspaceFolderBasename} expanded.
	baseName := filepath.Base(projectDir)
	if !strings.Contains(out, "/workspaces/"+baseName) {
		t.Errorf("up: want '/workspaces/%s' in output, got %q", baseName, out)
	}

	// 2. remoteEnv: PROJECT_NAME resolved via ${containerEnv:PROJECT}.
	out = mustRunCrib(t, projectDir, cribHome, "exec", "--", "printenv", "PROJECT_NAME")
	outLast := lastLine(out)
	if outLast != baseName {
		t.Errorf("printenv PROJECT_NAME: want %q, got %q", baseName, outLast)
	}

	// 3. remoteEnv: static REDIS_URL passed through unchanged.
	out = mustRunCrib(t, projectDir, cribHome, "exec", "--", "printenv", "REDIS_URL")
	outLast = lastLine(out)
	if outLast != "redis://db:6379" {
		t.Errorf("printenv REDIS_URL: want %q, got %q", "redis://db:6379", outLast)
	}

	// 4. postCreateCommand ran.
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-f", "/tmp/post-create-ran")

	// 5. down - should stop and remove all compose services.
	mustRunCrib(t, projectDir, cribHome, "down")

	out = mustRunCrib(t, projectDir, cribHome, "status")
	if strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("status after down: want not-running, got %q", out)
	}

	// 6. up again - should recreate services (down removed them).
	mustRunCrib(t, projectDir, cribHome, "up")

	// 7. remove - must remove all compose services and workspace state.
	mustRunCrib(t, projectDir, cribHome, "remove", "--force")

	out = mustRunCrib(t, projectDir, cribHome, "list")
	if !strings.Contains(strings.ToLower(out), "no workspaces") {
		t.Errorf("list after remove: want 'no workspaces', got %q", out)
	}
}
