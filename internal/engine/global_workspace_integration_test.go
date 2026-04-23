package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/workspace"
)

// TestIntegrationGlobalWorkspaceEnv verifies global env is visible in the
// container and that project remoteEnv overrides global on key conflict.
func TestIntegrationGlobalWorkspaceEnv(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d, _ := newTestEngine(t)

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Project sets GLOBAL_ONE via containerEnv; leaves GLOBAL_TWO unset so
	// the global value should be injected; overrides CONFLICT to prove
	// project wins on key conflict.
	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"containerEnv": {
			"GLOBAL_ONE": "project-one",
			"CONFLICT": "project-wins"
		}
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-global-env"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:           projectDir,
		DevContainerPath: ".devcontainer/devcontainer.json",
		CreatedAt:        time.Now(),
		LastUsedAt:       time.Now(),
	}

	_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
		cleanupWorkspaceImages(t, d, wsID)
	})

	e.SetGlobalWorkspace(GlobalWorkspaceOptions{
		Env: map[string]string{
			"GLOBAL_ONE": "global-loser",
			"GLOBAL_TWO": "global-value",
			"CONFLICT":   "global-loser",
		},
	})
	if _, err := e.Up(ctx, ws, UpOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	container, err := d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer: %v", err)
	}

	check := func(name, want string) {
		t.Helper()
		var stdout bytes.Buffer
		if err := d.ExecContainer(ctx, wsID, container.ID, []string{"printenv", name}, nil, &stdout, nil, nil, ""); err != nil {
			t.Fatalf("printenv %s: %v", name, err)
		}
		got := strings.TrimSpace(stdout.String())
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	check("GLOBAL_TWO", "global-value") // only global defines it
	check("GLOBAL_ONE", "project-one")  // project ContainerEnv wins
	check("CONFLICT", "project-wins")   // project ContainerEnv wins

	if err := e.Remove(ctx, ws); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

// TestIntegrationGlobalWorkspaceMountsAndRunArgs verifies global mounts are
// present alongside project mounts and that global run_args reach the runtime.
func TestIntegrationGlobalWorkspaceMountsAndRunArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d, _ := newTestEngine(t)

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create two source dirs that will be bind-mounted separately: one via
	// project mounts, one via global mounts.
	projectMountSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectMountSrc, "marker"), []byte("project"), 0o644); err != nil {
		t.Fatal(err)
	}
	globalMountSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(globalMountSrc, "marker"), []byte("global"), 0o644); err != nil {
		t.Fatal(err)
	}

	// runArgs: add a hostname label we can verify via printenv HOSTNAME.
	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"mounts": [
			"type=bind,source=` + projectMountSrc + `,target=/project-mount,readonly"
		]
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-global-mounts"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:           projectDir,
		DevContainerPath: ".devcontainer/devcontainer.json",
		CreatedAt:        time.Now(),
		LastUsedAt:       time.Now(),
	}

	_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
		cleanupWorkspaceImages(t, d, wsID)
	})

	e.SetGlobalWorkspace(GlobalWorkspaceOptions{
		Mounts: []string{
			"type=bind,source=" + globalMountSrc + ",target=/global-mount,readonly",
		},
		RunArgs: []string{"--hostname", "crib-global-test"},
	})
	if _, err := e.Up(ctx, ws, UpOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	container, err := d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer: %v", err)
	}

	readMarker := func(path string) string {
		t.Helper()
		var stdout bytes.Buffer
		if err := d.ExecContainer(ctx, wsID, container.ID, []string{"cat", path}, nil, &stdout, nil, nil, ""); err != nil {
			t.Fatalf("cat %s: %v", path, err)
		}
		return strings.TrimSpace(stdout.String())
	}

	if got := readMarker("/project-mount/marker"); got != "project" {
		t.Errorf("/project-mount/marker = %q, want %q", got, "project")
	}
	if got := readMarker("/global-mount/marker"); got != "global" {
		t.Errorf("/global-mount/marker = %q, want %q", got, "global")
	}

	// Verify global runArgs reached the runtime by checking hostname.
	var stdout bytes.Buffer
	if err := d.ExecContainer(ctx, wsID, container.ID, []string{"hostname"}, nil, &stdout, nil, nil, ""); err != nil {
		t.Fatalf("hostname: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "crib-global-test" {
		t.Errorf("hostname = %q, want %q (global run_args not applied)", got, "crib-global-test")
	}

	if err := e.Remove(ctx, ws); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}
