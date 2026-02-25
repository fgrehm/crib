package engine

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/workspace"
)

func newTestEngine(t *testing.T) (*Engine, *oci.OCIDriver) {
	t.Helper()
	d, err := oci.NewOCIDriver(slog.Default())
	if err != nil {
		t.Skipf("skipping: no container runtime available: %v", err)
	}

	store := workspace.NewStoreAt(t.TempDir())

	// compose helper is optional for these tests.
	return New(d, nil, store, slog.Default()), d
}

func TestIntegrationUpImageBased(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d := newTestEngine(t)

	// Create a temp project directory with a devcontainer.json.
	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"containerEnv": {
			"CRIB_TEST": "true"
		}
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-up"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:           projectDir,
		DevContainerPath: ".devcontainer/devcontainer.json",
		CreatedAt:        time.Now(),
		LastUsedAt:       time.Now(),
	}

	// Clean up.
	_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	})

	// Up.
	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	if result.ContainerID == "" {
		t.Error("ContainerID is empty")
	}

	if result.WorkspaceFolder == "" {
		t.Error("WorkspaceFolder is empty")
	}

	// Verify the container is running.
	container, err := d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer: %v", err)
	}
	if container == nil {
		t.Fatal("container not found")
	}
	if status := strings.ToLower(container.State.Status); status != "running" {
		t.Errorf("container status = %q, want running", status)
	}

	// Verify the container env was set.
	var stdout bytes.Buffer
	err = d.ExecContainer(ctx, wsID, container.ID, []string{"printenv", "CRIB_TEST"}, nil, &stdout, nil, nil, "")
	if err != nil {
		t.Fatalf("ExecContainer: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "true" {
		t.Errorf("CRIB_TEST = %q, want %q", got, "true")
	}

	// Stop.
	if err := e.Stop(ctx, ws); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	container, err = d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer after stop: %v", err)
	}
	if status := strings.ToLower(container.State.Status); status != "exited" {
		t.Errorf("container status after stop = %q, want exited", status)
	}

	// Up again (should start existing container).
	result2, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up (second): %v", err)
	}
	if result2.ContainerID == "" {
		t.Error("second Up returned empty ContainerID")
	}

	// Delete.
	if err := e.Delete(ctx, ws); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	container, err = d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer after delete: %v", err)
	}
	if container != nil {
		t.Error("container still exists after delete")
	}
}

func TestIntegrationUpWithLifecycleHooks(t *testing.T) {
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

	wsID := "test-engine-hooks"
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
	})

	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Verify onCreate hook ran.
	var stdout bytes.Buffer
	err = d.ExecContainer(ctx, wsID, result.ContainerID, []string{"test", "-f", "/tmp/on-create-ran"}, nil, &stdout, nil, nil, "")
	if err != nil {
		t.Errorf("onCreate hook did not run: %v", err)
	}

	// Verify postStart hook ran.
	stdout.Reset()
	err = d.ExecContainer(ctx, wsID, result.ContainerID, []string{"test", "-f", "/tmp/post-start-ran"}, nil, &stdout, nil, nil, "")
	if err != nil {
		t.Errorf("postStart hook did not run: %v", err)
	}
}

func TestIntegrationUpWithInitializeCommand(t *testing.T) {
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

	// The initializeCommand creates a file on the host (not in the container).
	hostMarker := filepath.Join(projectDir, "init-ran")
	configContent := fmt.Sprintf(`{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"initializeCommand": "touch %s"
	}`, hostMarker)
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-init-cmd"
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
	})

	_, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Verify the file was created on the host by initializeCommand.
	if _, err := os.Stat(hostMarker); err != nil {
		t.Errorf("initializeCommand did not create host file %s: %v", hostMarker, err)
	}
}

func TestIntegrationUpWithRecreate(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-recreate"
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
	})

	// First up.
	result1, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Second up with recreate.
	result2, err := e.Up(ctx, ws, UpOptions{Recreate: true})
	if err != nil {
		t.Fatalf("Up with recreate: %v", err)
	}

	// Container IDs should differ after recreation.
	if result1.ContainerID == result2.ContainerID {
		t.Error("container ID should be different after recreation")
	}
}

// TestIntegrationUpWithRemoteUserUID verifies that Up correctly synchronizes the
// container user's UID with the host UID when remoteUser is set.
//
// This exercises two important behaviors introduced to fix rootless Podman chown failures:
//
//  1. When the target UID is already in use by another user in the image (e.g., ubuntu:24.04
//     ships an "ubuntu" user at UID 1000), crib must move that user aside before calling
//     usermod, so the sync does not silently fail and fall back to chownWorkspace.
//
//  2. After a successful UID sync, chownWorkspace is skipped entirely. On rootless Podman
//     the bind-mount files are already accessible once UIDs match, and chown would fail
//     with "operation not permitted".
func TestIntegrationUpWithRemoteUserUID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getuid() == 0 {
		t.Skip("skipping: UID sync is a no-op for root; test requires a non-root host user")
	}

	ctx := context.Background()
	e, d := newTestEngine(t)

	hostUID := os.Getuid()
	hostGID := os.Getgid()

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a file that the container user should be able to read via the bind mount.
	if err := os.WriteFile(filepath.Join(projectDir, "probe.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build a Dockerfile that mirrors a realistic devcontainer setup:
	// - Uses ubuntu:24.04 (which ships an "ubuntu" user at UID 1000)
	// - Creates a "dev" user without an explicit UID (gets next available, typically 1001)
	// This means the dev UID will usually differ from the host UID, requiring crib to run
	// the full sync path including the UID-conflict resolution.
	dockerfile := `FROM ubuntu:24.04
RUN useradd -m -s /bin/bash dev
USER dev
`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"build": {"dockerfile": "Dockerfile"},
		"remoteUser": "dev",
		"overrideCommand": true
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-remote-uid"
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
	})

	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	container, err := d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer: %v", err)
	}
	if container == nil {
		t.Fatal("container not found after Up")
	}
	if status := strings.ToLower(container.State.Status); status != "running" {
		t.Errorf("container status = %q, want running", status)
	}

	// Verify the dev user's UID was synced to match the host UID.
	// This confirms syncRemoteUserUID resolved any UID conflicts (e.g., the ubuntu user
	// at UID 1000) and successfully called usermod to reassign dev.
	var stdout bytes.Buffer
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"id", "-u", "dev"}, nil, &stdout, nil, nil, ""); err != nil {
		t.Fatalf("id -u dev: %v", err)
	}
	gotUID, err := strconv.Atoi(strings.TrimSpace(stdout.String()))
	if err != nil {
		t.Fatalf("parsing container UID output %q: %v", stdout.String(), err)
	}
	if gotUID != hostUID {
		t.Errorf("container dev UID = %d, want %d (host UID)", gotUID, hostUID)
	}

	// Verify the dev group GID matches the host GID.
	stdout.Reset()
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"id", "-g", "dev"}, nil, &stdout, nil, nil, ""); err != nil {
		t.Fatalf("id -g dev: %v", err)
	}
	gotGID, err := strconv.Atoi(strings.TrimSpace(stdout.String()))
	if err != nil {
		t.Fatalf("parsing container GID output %q: %v", stdout.String(), err)
	}
	if gotGID != hostGID {
		t.Errorf("container dev GID = %d, want %d (host GID)", gotGID, hostGID)
	}

	// Verify the dev user can read a bind-mounted workspace file.
	// This confirms the workspace is accessible after UID sync, which is the
	// invariant we care about regardless of how the runtime maps UIDs.
	stdout.Reset()
	probeFile := fmt.Sprintf("%s/probe.txt", result.WorkspaceFolder)
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"cat", probeFile}, nil, &stdout, nil, nil, "dev"); err != nil {
		t.Fatalf("dev user cannot read bind-mounted file %s: %v", probeFile, err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "ok" {
		t.Errorf("probe.txt content = %q, want %q", got, "ok")
	}
}
