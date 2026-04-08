package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EShellHistory(t *testing.T) {
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

	// Up with default alpine config.
	mustRunCrib(t, projectDir, cribHome, "up")

	// Verify HISTFILE is set and points to the .crib_history directory.
	out := mustRunCrib(t, projectDir, cribHome, "exec", "--", "printenv", "HISTFILE")
	histfile := strings.TrimSpace(lastLine(out))
	if !strings.Contains(histfile, ".crib_history/.shell_history") {
		t.Errorf("HISTFILE = %q, want to contain '.crib_history/.shell_history'", histfile)
	}

	// Verify the history file was created on the host by the plugin.
	matches, err := filepath.Glob(filepath.Join(cribHome, "workspaces", "*", "plugins", "shell-history", ".shell_history"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("shell history file not found on host")
	}
}

// TestE2EComposeShellHistory verifies that the shell-history plugin works
// correctly in compose workspaces with a non-root container user. This
// exercises the fix where the container user is resolved from the compose
// service before plugin dispatch (so HISTFILE targets /home/node/, not /root/).
func TestE2EComposeShellHistory(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}
	if !hasCompose() {
		t.Fatal("docker compose or podman compose not available")
	}
	t.Parallel()

	// Set up a compose project with a non-root user (node:22-alpine with
	// USER node). No remoteUser in devcontainer.json, so the plugin must
	// resolve the user from the Dockerfile's USER directive.
	//
	// Use os.MkdirTemp instead of t.TempDir() for the project directory.
	// chownWorkspace runs "chown -R node:" on the bind-mounted workspace,
	// which changes ownership of all files including .devcontainer/*.
	// On rootful Docker, this makes the files unremovable by the test user,
	// causing t.TempDir() cleanup to fail the test.
	parent, err := os.MkdirTemp("", "TestE2EComposeShellHistory-*") //nolint:usetesting // rootful Docker chowns bind mounts, breaking t.TempDir() cleanup
	if err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(parent, "compose-plugin-e2e")
	devDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}

	dockerfile := "FROM node:22-alpine\nUSER node\n"
	composeYML := `services:
  app:
    build:
      context: .
      dockerfile: Dockerfile
`
	devcontainerJSON := `{
	"dockerComposeFile": "compose.yml",
	"service": "app",
	"overrideCommand": true
}`

	for name, content := range map[string]string{
		"Dockerfile":        dockerfile,
		"compose.yml":       composeYML,
		"devcontainer.json": devcontainerJSON,
	} {
		if err := os.WriteFile(filepath.Join(devDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cribHome := t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(projectDir, cribHome, "remove", "--force")
		_ = cmd.Run()
		// Best-effort removal. On rootful Docker, chownWorkspace changes file
		// ownership to the container user, preventing the test user from
		// deleting the directory. CI runners are ephemeral so leftover temp
		// dirs are harmless.
		_ = os.RemoveAll(parent)
	})

	mustRunCrib(t, projectDir, cribHome, "up")

	// Verify HISTFILE is set and points to the non-root user's home.
	out := mustRunCrib(t, projectDir, cribHome, "exec", "--", "printenv", "HISTFILE")
	histfile := strings.TrimSpace(lastLine(out))

	if !strings.Contains(histfile, ".crib_history/.shell_history") {
		t.Errorf("HISTFILE = %q, want to contain '.crib_history/.shell_history'", histfile)
	}
	if strings.HasPrefix(histfile, "/root/") {
		t.Errorf("HISTFILE = %q, must not start with /root/ (plugin should target non-root user)", histfile)
	}

	// Verify the shell-history plugin directory was created on the host.
	matches, err := filepath.Glob(filepath.Join(cribHome, "workspaces", "*", "plugins", "shell-history"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("shell-history plugin directory not found on host")
	}
}
