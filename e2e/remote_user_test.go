package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2ERemoteUserLiveOverride verifies the liveRemoteUser behavior end-to-end:
//  1. crib up with a metadata-labeled image (no remoteUser in config) infers the user
//  2. crib exec whoami returns the metadata-inferred user
//  3. Editing devcontainer.json to set explicit remoteUser takes effect immediately
//  4. crib exec whoami returns the overridden user without rebuild
func TestE2ERemoteUserLiveOverride(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}
	t.Parallel()

	projectDir := t.TempDir()
	cribHome := t.TempDir()
	devDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Dockerfile with a devcontainer.metadata label declaring remoteUser.
	dockerfile := "FROM alpine:3.20\n" +
		"RUN adduser -D metauser\n" +
		`LABEL devcontainer.metadata='[{"remoteUser":"metauser"}]'` + "\n"
	if err := os.WriteFile(filepath.Join(devDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		t.Fatal(err)
	}

	// Initial config: no remoteUser, should be inferred from metadata.
	configPath := filepath.Join(devDir, "devcontainer.json")
	initialConfig := `{
		"build": {"dockerfile": "Dockerfile"},
		"overrideCommand": true
	}`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// Cleanup: restore workspace permissions (chownWorkspace transfers to non-root
	// remoteUser) and remove the workspace. Uses runCrib (not mustRunCrib) so
	// cleanup doesn't mask the real test failure.
	wsFolder := "/workspaces/" + filepath.Base(projectDir)
	t.Cleanup(func() {
		_, _ = runCrib(t, projectDir, cribHome, "exec", "--user", "root", "--", "chmod", "-R", "a+rwX", wsFolder)
		_, _ = runCrib(t, projectDir, cribHome, "remove", "--force")
	})

	// 1. Up: remoteUser inferred from metadata label.
	mustRunCrib(t, projectDir, cribHome, "up")

	// 2. exec whoami: should run as the metadata-inferred user.
	out := mustRunCrib(t, projectDir, cribHome, "exec", "--", "whoami")
	if got := strings.TrimSpace(out); got != "metauser" {
		t.Errorf("whoami after up: got %q, want %q (from metadata label)", got, "metauser")
	}

	// 3. Restore write access to the workspace. chownWorkspace transferred
	// ownership to metauser; on Docker the chown takes real effect so the
	// test runner can't write to .devcontainer/ without this.
	mustRunCrib(t, projectDir, cribHome, "exec", "--user", "root", "--", "chmod", "-R", "a+rwX", wsFolder)

	// Edit devcontainer.json to override remoteUser (container still running).
	overrideConfig := `{
		"build": {"dockerfile": "Dockerfile"},
		"remoteUser": "root",
		"overrideCommand": true
	}`
	if err := os.WriteFile(configPath, []byte(overrideConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. exec whoami: liveRemoteUser picks up the config change without rebuild.
	out = mustRunCrib(t, projectDir, cribHome, "exec", "--", "whoami")
	if got := strings.TrimSpace(out); got != "root" {
		t.Errorf("whoami after config override: got %q, want %q (live config wins)", got, "root")
	}
}
