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

// setupComposeProject creates a temporary project directory with a known
// basename ("compose-e2e") so ${localWorkspaceFolderBasename} is predictable.
func setupComposeProject(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	dir := filepath.Join(parent, "compose-e2e")
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

func TestE2ECompose(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}
	if !hasCompose() {
		t.Fatal("docker compose or podman compose not available")
	}

	projectDir := setupComposeProject(t)
	cribHome := t.TempDir()

	// Best-effort cleanup so containers don't linger if the test fails mid-way.
	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "delete")
		_ = cmd.Run()
	})

	// 1. up - bring both services up.
	out := mustRunCrib(t, projectDir, cribHome, "up")
	if !strings.Contains(out, "container") {
		t.Errorf("up: want 'container' in output, got %q", out)
	}
	// Workspace folder must have ${localWorkspaceFolderBasename} expanded.
	if !strings.Contains(out, "/workspaces/compose-e2e") {
		t.Errorf("up: want 'workspace: /workspaces/compose-e2e' in output, got %q", out)
	}

	// 2. remoteEnv: PROJECT_NAME resolved via ${containerEnv:PROJECT}.
	out = mustRunCrib(t, projectDir, cribHome, "exec", "--", "printenv", "PROJECT_NAME")
	outLast := lastLine(out)
	if outLast != "compose-e2e" {
		t.Errorf("printenv PROJECT_NAME: want %q, got %q", "compose-e2e", outLast)
	}

	// 3. remoteEnv: static REDIS_URL passed through unchanged.
	out = mustRunCrib(t, projectDir, cribHome, "exec", "--", "printenv", "REDIS_URL")
	outLast = lastLine(out)
	if outLast != "redis://db:6379" {
		t.Errorf("printenv REDIS_URL: want %q, got %q", "redis://db:6379", outLast)
	}

	// 4. postCreateCommand ran.
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-f", "/tmp/post-create-ran")

	// 5. stop - should stop all compose services without error.
	mustRunCrib(t, projectDir, cribHome, "stop")

	out = mustRunCrib(t, projectDir, cribHome, "status")
	if strings.Contains(strings.ToLower(out), "running") {
		t.Errorf("status after stop: want not-running, got %q", out)
	}

	// 6. up again - restart existing services.
	mustRunCrib(t, projectDir, cribHome, "up")

	// 7. delete - must remove all compose services cleanly.
	mustRunCrib(t, projectDir, cribHome, "delete")

	out = mustRunCrib(t, projectDir, cribHome, "list")
	if !strings.Contains(strings.ToLower(out), "no workspaces") {
		t.Errorf("list after delete: want 'no workspaces', got %q", out)
	}
}
