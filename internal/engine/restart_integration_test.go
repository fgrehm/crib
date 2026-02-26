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

// TestIntegrationRestartSimple verifies that Restart performs a simple container
// restart when the config hasn't changed, running only resume-flow hooks.
func TestIntegrationRestartSimple(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d := newTestEngine(t)

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"onCreateCommand": "touch /tmp/on-create-ran",
		"postStartCommand": "touch /tmp/post-start-ran"
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-restart-simple"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:            projectDir,
		DevContainerPath:  ".devcontainer/devcontainer.json",
		CreatedAt:         time.Now(),
		LastUsedAt:        time.Now(),
	}

	_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	})

	// Initial Up — runs all hooks.
	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Remove the marker files so we can verify which hooks run on restart.
	for _, f := range []string{"/tmp/on-create-ran", "/tmp/post-start-ran"} {
		_ = d.ExecContainer(ctx, wsID, result.ContainerID, []string{"rm", "-f", f}, nil, nil, nil, nil, "")
	}

	// Restart with unchanged config.
	restartResult, err := e.Restart(ctx, ws)
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}

	if restartResult.Recreated {
		t.Error("expected Recreated=false for unchanged config")
	}

	// Container ID should be the same (simple restart, no recreation).
	if restartResult.ContainerID != result.ContainerID {
		t.Errorf("container ID changed: %s -> %s", result.ContainerID, restartResult.ContainerID)
	}

	// Verify container is running.
	container, err := d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer: %v", err)
	}
	if container == nil {
		t.Fatal("container not found after restart")
	}
	if status := strings.ToLower(container.State.Status); status != "running" {
		t.Errorf("container status = %q, want running", status)
	}

	// postStartCommand should have run (resume flow).
	err = d.ExecContainer(ctx, wsID, restartResult.ContainerID, []string{"test", "-f", "/tmp/post-start-ran"}, nil, nil, nil, nil, "")
	if err != nil {
		t.Error("postStartCommand did not run on restart")
	}

	// onCreateCommand should NOT have run again (creation-only hook).
	err = d.ExecContainer(ctx, wsID, restartResult.ContainerID, []string{"test", "-f", "/tmp/on-create-ran"}, nil, nil, nil, nil, "")
	if err == nil {
		t.Error("onCreateCommand ran again on restart, expected it to be skipped")
	}
}

// TestIntegrationRestartSafeChange verifies that Restart recreates the container
// when a safe config change is detected (e.g. environment variable), and runs
// only resume-flow hooks.
func TestIntegrationRestartSafeChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d := newTestEngine(t)

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"containerEnv": {
			"MY_VAR": "original"
		},
		"onCreateCommand": "touch /tmp/on-create-ran",
		"postStartCommand": "touch /tmp/post-start-ran"
	}`
	configPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-restart-safe"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:            projectDir,
		DevContainerPath:  ".devcontainer/devcontainer.json",
		CreatedAt:         time.Now(),
		LastUsedAt:        time.Now(),
	}

	_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	})

	// Initial Up.
	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	originalContainerID := result.ContainerID

	// Verify original env var.
	var stdout bytes.Buffer
	err = d.ExecContainer(ctx, wsID, originalContainerID, []string{"printenv", "MY_VAR"}, nil, &stdout, nil, nil, "")
	if err != nil {
		t.Fatalf("printenv MY_VAR: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "original" {
		t.Errorf("MY_VAR = %q, want %q", got, "original")
	}

	// Change the env var (safe change).
	updatedConfig := `{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"containerEnv": {
			"MY_VAR": "updated"
		},
		"onCreateCommand": "touch /tmp/on-create-ran",
		"postStartCommand": "touch /tmp/post-start-ran"
	}`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// Restart should detect the safe change and recreate.
	restartResult, err := e.Restart(ctx, ws)
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}

	if !restartResult.Recreated {
		t.Error("expected Recreated=true for env change")
	}

	// Container ID should be different (recreated).
	if restartResult.ContainerID == originalContainerID {
		t.Error("container ID should be different after recreation")
	}

	// Verify updated env var is present.
	stdout.Reset()
	err = d.ExecContainer(ctx, wsID, restartResult.ContainerID, []string{"printenv", "MY_VAR"}, nil, &stdout, nil, nil, "")
	if err != nil {
		t.Fatalf("printenv MY_VAR after restart: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "updated" {
		t.Errorf("MY_VAR = %q, want %q", got, "updated")
	}

	// postStartCommand should have run (resume flow).
	err = d.ExecContainer(ctx, wsID, restartResult.ContainerID, []string{"test", "-f", "/tmp/post-start-ran"}, nil, nil, nil, nil, "")
	if err != nil {
		t.Error("postStartCommand did not run after recreate")
	}

	// onCreateCommand should NOT have run (creation hooks skipped on restart).
	// The marker file from the first Up should not exist in the new container.
	// If it does exist, it means the hook ran again (which is wrong).
	err = d.ExecContainer(ctx, wsID, restartResult.ContainerID, []string{"test", "-f", "/tmp/on-create-ran"}, nil, nil, nil, nil, "")
	if err == nil {
		t.Error("onCreateCommand ran on recreate, expected it to be skipped")
	}
}

// TestIntegrationRestartNeedsRebuild verifies that Restart returns an error
// when image-affecting changes are detected.
func TestIntegrationRestartNeedsRebuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d := newTestEngine(t)

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true
	}`
	configPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-restart-rebuild"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:            projectDir,
		DevContainerPath:  ".devcontainer/devcontainer.json",
		CreatedAt:         time.Now(),
		LastUsedAt:        time.Now(),
	}

	_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	})

	// Initial Up.
	_, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Change the image (needs rebuild).
	updatedConfig := `{
		"image": "alpine:3.19",
		"overrideCommand": true
	}`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// Restart should fail with a rebuild suggestion.
	_, err = e.Restart(ctx, ws)
	if err == nil {
		t.Fatal("expected error for image change, got nil")
	}
	if !strings.Contains(err.Error(), "rebuild") {
		t.Errorf("error should mention 'rebuild', got: %v", err)
	}
}

// TestIntegrationRestartNoWorkspace verifies that Restart returns an error
// when there's no previous Up result.
func TestIntegrationRestartNoWorkspace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, _ := newTestEngine(t)

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-restart-no-ws"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:            projectDir,
		DevContainerPath:  ".devcontainer/devcontainer.json",
		CreatedAt:         time.Now(),
		LastUsedAt:        time.Now(),
	}

	_, err := e.Restart(ctx, ws)
	if err == nil {
		t.Fatal("expected error for workspace with no previous Up, got nil")
	}
	if !strings.Contains(err.Error(), "crib up") {
		t.Errorf("error should mention 'crib up', got: %v", err)
	}
}

// TestIntegrationRestartMountChange verifies that Restart recreates the container
// when mounts change and the new mount is functional.
func TestIntegrationRestartMountChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d := newTestEngine(t)

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a temp dir to mount.
	mountSource := t.TempDir()
	if err := os.WriteFile(filepath.Join(mountSource, "hello.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true
	}`
	configPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-restart-mount"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:            projectDir,
		DevContainerPath:  ".devcontainer/devcontainer.json",
		CreatedAt:         time.Now(),
		LastUsedAt:        time.Now(),
	}

	_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	})

	// Initial Up without the extra mount.
	_, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Add a mount (safe change).
	updatedConfig := `{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"mounts": [
			"type=bind,src=` + mountSource + `,dst=/extra"
		]
	}`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// Restart should detect the mount change and recreate.
	restartResult, err := e.Restart(ctx, ws)
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}

	if !restartResult.Recreated {
		t.Error("expected Recreated=true for mount change")
	}

	// Verify the mount is functional.
	var stdout bytes.Buffer
	err = d.ExecContainer(ctx, wsID, restartResult.ContainerID, []string{"cat", "/extra/hello.txt"}, nil, &stdout, nil, nil, "")
	if err != nil {
		t.Fatalf("cat /extra/hello.txt: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "world" {
		t.Errorf("/extra/hello.txt = %q, want %q", got, "world")
	}
}
