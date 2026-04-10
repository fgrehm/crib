package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/plugin/shellhistory"
	"github.com/fgrehm/crib/internal/workspace"
)

const testWorkspacePrefix = "test-"

// TestMain warns if non-test crib containers are running, which can lead to
// interference (stale compose state, network conflicts, etc.).
func TestMain(m *testing.M) {
	// Check for active workspaces before running integration tests. We can't
	// call testing.Short() before flag.Parse(), so check the flag directly.
	short := false
	for _, arg := range os.Args[1:] {
		if arg == "-test.short" || arg == "-test.short=true" {
			short = true
			break
		}
	}
	if !short {
		d, err := oci.NewOCIDriver(slog.Default())
		if err == nil {
			containers, _ := d.ListContainers(context.Background())
			for _, c := range containers {
				wsID := c.Config.Labels[oci.LabelWorkspace]
				if wsID != "" && !strings.HasPrefix(wsID, testWorkspacePrefix) {
					fmt.Fprintf(os.Stderr, "\n⚠ WARNING: active crib workspace %q (container %s) detected.\n", wsID, c.ID[:12])
					fmt.Fprintf(os.Stderr, "  Integration tests may interfere with running workspaces.\n")
					fmt.Fprintf(os.Stderr, "  Consider running 'crib down' first.\n\n")
					break
				}
			}
		}
	}
	os.Exit(m.Run())
}

// requireTestWorkspace fatals if the workspace ID does not start with the test
// prefix. Prevents test cleanup from accidentally removing real workspaces.
func requireTestWorkspace(t *testing.T, wsID string) {
	t.Helper()
	if !strings.HasPrefix(wsID, testWorkspacePrefix) {
		t.Fatalf("refusing to operate on non-test workspace %q (must start with %q)", wsID, testWorkspacePrefix)
	}
}

func newTestEngine(t *testing.T) (*Engine, *oci.OCIDriver, *workspace.Store) {
	t.Helper()
	d, err := oci.NewOCIDriver(slog.Default())
	if err != nil {
		t.Skipf("skipping: no container runtime available: %v", err)
	}

	store := workspace.NewStoreAt(t.TempDir())

	// compose helper is optional for these tests.
	return New(d, nil, store, slog.Default()), d, store
}

// cleanupWorkspaceImages removes all labeled images for a workspace during
// test teardown. Prevents crib-test-* images from accumulating on disk.
func cleanupWorkspaceImages(t *testing.T, d driver.Driver, wsID string) {
	t.Helper()
	requireTestWorkspace(t, wsID)
	ctx := context.Background()
	images, err := d.ListImages(ctx, oci.WorkspaceLabel(wsID))
	if err != nil {
		return
	}
	for _, img := range images {
		_ = d.RemoveImage(ctx, img.Reference)
	}
}

func TestIntegrationUpImageBased(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d, _ := newTestEngine(t)

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
		cleanupWorkspaceImages(t, d, wsID)
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

	// Down (stops and removes container, keeps workspace state).
	if err := e.Down(ctx, ws); err != nil {
		t.Fatalf("Down: %v", err)
	}

	container, err = d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer after down: %v", err)
	}
	if container != nil {
		t.Error("container still exists after down")
	}

	// Up again (should create a new container since down removed it).
	result2, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up (second): %v", err)
	}
	if result2.ContainerID == "" {
		t.Error("second Up returned empty ContainerID")
	}

	// Remove (removes container and workspace state).
	if err := e.Remove(ctx, ws); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	container, err = d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer after remove: %v", err)
	}
	if container != nil {
		t.Error("container still exists after remove")
	}
}

func TestIntegrationUpWithLifecycleHooks(t *testing.T) {
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
		cleanupWorkspaceImages(t, d, wsID)
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
	e, d, _ := newTestEngine(t)

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
		cleanupWorkspaceImages(t, d, wsID)
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
	e, d, _ := newTestEngine(t)

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
		cleanupWorkspaceImages(t, d, wsID)
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
	e, d, _ := newTestEngine(t)

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
		cleanupWorkspaceImages(t, d, wsID)
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

func TestIntegrationUpWithPlugins(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d, _ := newTestEngine(t)

	// Wire in shell-history plugin.
	mgr := plugin.NewManager(slog.Default())
	mgr.Register(shellhistory.New())
	e.SetPlugins(mgr)
	e.SetRuntime(d.Runtime().String())

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

	wsID := "test-engine-plugins"
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

	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Verify HISTFILE is set inside the container.
	var stdout bytes.Buffer
	err = d.ExecContainer(ctx, wsID, result.ContainerID, []string{"printenv", "HISTFILE"}, nil, &stdout, nil, nil, "")
	if err != nil {
		t.Fatalf("printenv HISTFILE: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	if got == "" {
		t.Error("HISTFILE is empty, want non-empty")
	}
	if !strings.Contains(got, ".crib_history/.shell_history") {
		t.Errorf("HISTFILE = %q, want to contain '.crib_history/.shell_history'", got)
	}
}

func TestIntegrationLogs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d, _ := newTestEngine(t)
	e.SetOutput(os.Stdout, os.Stderr)

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"postCreateCommand": "echo CRIB_LOG_TEST_MARKER"
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-logs"
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

	if _, err := e.Up(ctx, ws, UpOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Capture logs.
	var logBuf bytes.Buffer
	e.SetOutput(&logBuf, os.Stderr)
	if err := e.Logs(ctx, ws, LogsOptions{}); err != nil {
		t.Fatalf("Logs: %v", err)
	}

	// Verify that the log output is not empty.
	if logBuf.Len() == 0 {
		t.Error("Logs returned empty output")
	}

	// Test --tail option.
	logBuf.Reset()
	if err := e.Logs(ctx, ws, LogsOptions{Tail: "5"}); err != nil {
		t.Fatalf("Logs with tail: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
	if len(lines) > 5 {
		t.Errorf("Logs --tail 5 returned %d lines, want <= 5", len(lines))
	}
}

func TestIntegrationSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d, _ := newTestEngine(t)
	e.SetOutput(os.Stdout, os.Stderr)

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"onCreateCommand": "touch /tmp/snapshot-marker"
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-snapshot"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:           projectDir,
		DevContainerPath: ".devcontainer/devcontainer.json",
		CreatedAt:        time.Now(),
		LastUsedAt:       time.Now(),
	}

	snapshotName := snapshotImageName(wsID)
	_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	_ = d.RemoveImage(ctx, snapshotName)
	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
		cleanupWorkspaceImages(t, d, wsID)
	})

	// Up should create the container and commit a snapshot.
	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Verify onCreate hook ran.
	var stdout bytes.Buffer
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"test", "-f", "/tmp/snapshot-marker"}, nil, &stdout, nil, nil, ""); err != nil {
		t.Errorf("onCreate hook did not run: %v", err)
	}

	// Verify snapshot was committed.
	storedResult, err := e.store.LoadResult(wsID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if storedResult.SnapshotImage == "" {
		t.Error("SnapshotImage should be set after Up with create-time hooks")
	}
	if storedResult.SnapshotHookHash == "" {
		t.Error("SnapshotHookHash should be set")
	}

	// Verify snapshot image actually exists.
	if _, err := d.InspectImage(ctx, snapshotName); err != nil {
		t.Errorf("snapshot image %s not found: %v", snapshotName, err)
	}

	// ClearSnapshot should remove the image and metadata.
	e.ClearSnapshot(ctx, ws)
	if _, err := d.InspectImage(ctx, snapshotName); err == nil {
		t.Error("snapshot image should have been removed after ClearSnapshot")
	}

	storedResult, _ = e.store.LoadResult(wsID)
	if storedResult.SnapshotImage != "" {
		t.Errorf("SnapshotImage = %q, want empty after ClearSnapshot", storedResult.SnapshotImage)
	}
}

func TestIntegrationFeatureLifecycleHooks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d, _ := newTestEngine(t)
	e.SetOutput(os.Stdout, os.Stderr)

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	featureDir := filepath.Join(devcontainerDir, "test-feature")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Local feature with lifecycle hooks. The feature's onCreateCommand should
	// run before the user's onCreateCommand, and postStartCommand should run
	// on every start.
	featureJSON := `{
		"id": "test-feature",
		"version": "1.0.0",
		"name": "Test Feature",
		"onCreateCommand": "echo feature-oncreate > /tmp/feature-oncreate-ran",
		"postStartCommand": "echo feature-poststart > /tmp/feature-poststart-ran"
	}`
	if err := os.WriteFile(filepath.Join(featureDir, "devcontainer-feature.json"), []byte(featureJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Minimal install.sh (required by feature spec).
	if err := os.WriteFile(filepath.Join(featureDir, "install.sh"), []byte("#!/bin/sh\necho 'test-feature installed'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// devcontainer.json references the local feature and has its own hooks.
	// The user's onCreateCommand writes a timestamp so we can verify ordering:
	// feature hook should have written its marker before the user hook runs.
	configContent := `{
		"image": "alpine:3.20",
		"overrideCommand": true,
		"features": {
			"./test-feature": {}
		},
		"onCreateCommand": "test -f /tmp/feature-oncreate-ran && echo user-after-feature > /tmp/user-oncreate-ran",
		"postStartCommand": "echo user-poststart > /tmp/user-poststart-ran"
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-feature-hooks"
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

	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Verify feature onCreateCommand ran.
	var stdout bytes.Buffer
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"cat", "/tmp/feature-oncreate-ran"}, nil, &stdout, nil, nil, ""); err != nil {
		t.Errorf("feature onCreateCommand did not run: %v", err)
	}

	// Verify user onCreateCommand ran AND that it ran after the feature hook
	// (it checks for /tmp/feature-oncreate-ran before writing its own marker).
	stdout.Reset()
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"cat", "/tmp/user-oncreate-ran"}, nil, &stdout, nil, nil, ""); err != nil {
		t.Errorf("user onCreateCommand did not run (or ran before feature hook): %v", err)
	} else if got := strings.TrimSpace(stdout.String()); got != "user-after-feature" {
		t.Errorf("user onCreateCommand output = %q, want 'user-after-feature'", got)
	}

	// Verify feature postStartCommand ran.
	stdout.Reset()
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"test", "-f", "/tmp/feature-poststart-ran"}, nil, &stdout, nil, nil, ""); err != nil {
		t.Errorf("feature postStartCommand did not run: %v", err)
	}

	// Verify user postStartCommand ran.
	stdout.Reset()
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"test", "-f", "/tmp/user-poststart-ran"}, nil, &stdout, nil, nil, ""); err != nil {
		t.Errorf("user postStartCommand did not run: %v", err)
	}

	// Verify feature hooks are stored in result.json for the resume path.
	storedResult, err := e.store.LoadResult(wsID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if len(storedResult.FeatureOnCreateCommands) == 0 {
		t.Error("FeatureOnCreateCommands should be stored in result")
	}
	if len(storedResult.FeaturePostStartCommands) == 0 {
		t.Error("FeaturePostStartCommands should be stored in result")
	}

	// Verify snapshot was committed (feature has create-time hooks).
	if storedResult.SnapshotImage == "" {
		t.Error("SnapshotImage should be set (feature has onCreateCommand)")
	}

	// --- Resume path: Down + Up should run feature postStartCommand again ---

	// Remove the postStart markers so we can verify they re-run.
	_ = d.ExecContainer(ctx, wsID, result.ContainerID, []string{"rm", "-f", "/tmp/feature-poststart-ran", "/tmp/user-poststart-ran"}, nil, nil, nil, nil, "")

	if err := e.Down(ctx, ws); err != nil {
		t.Fatalf("Down: %v", err)
	}

	result2, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up (resume): %v", err)
	}

	// Feature postStartCommand should have run again on resume.
	stdout.Reset()
	if err := d.ExecContainer(ctx, wsID, result2.ContainerID, []string{"test", "-f", "/tmp/feature-poststart-ran"}, nil, &stdout, nil, nil, ""); err != nil {
		t.Errorf("feature postStartCommand did not run on resume: %v", err)
	}

	// User postStartCommand should have run again on resume.
	stdout.Reset()
	if err := d.ExecContainer(ctx, wsID, result2.ContainerID, []string{"test", "-f", "/tmp/user-poststart-ran"}, nil, &stdout, nil, nil, ""); err != nil {
		t.Errorf("user postStartCommand did not run on resume: %v", err)
	}

}

func TestIntegrationDoctor(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, d, _ := newTestEngine(t)
	e.SetOutput(os.Stdout, os.Stderr)

	// Clean state: doctor should find no issues.
	result, err := e.Doctor(ctx, false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if !result.RuntimeOK {
		t.Error("RuntimeOK should be true")
	}

	// Create an orphaned workspace (source directory doesn't exist).
	orphanWS := &workspace.Workspace{
		ID:     "test-doctor-orphan",
		Source: "/tmp/nonexistent-dir-for-doctor-test",
	}
	if err := e.store.Save(orphanWS); err != nil {
		t.Fatal(err)
	}

	result, err = e.Doctor(ctx, false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Check == "orphaned-workspace" && issue.WorkspaceID == "test-doctor-orphan" {
			found = true
		}
	}
	if !found {
		t.Error("expected orphaned-workspace issue")
	}

	// Fix the orphan.
	_, err = e.Doctor(ctx, true)
	if err != nil {
		t.Fatalf("Doctor --fix: %v", err)
	}

	// Verify it's gone.
	if e.store.Exists("test-doctor-orphan") {
		t.Error("orphaned workspace should have been removed by --fix")
	}
	_ = d // keep linter happy
}

func TestIntegrationImageMetadataLabel(t *testing.T) {
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

	// Dockerfile with a devcontainer.metadata label declaring remoteUser.
	// The label is the spec mechanism for pre-built images to carry user config
	// (e.g. mcr.microsoft.com/devcontainers/* images use this pattern).
	dockerfile := "FROM alpine:3.20\n" +
		"RUN adduser -D testlabeluser\n" +
		`LABEL devcontainer.metadata='[{"remoteUser":"testlabeluser"}]'` + "\n"
	if err := os.WriteFile(filepath.Join(devcontainerDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		t.Fatal(err)
	}

	// devcontainer.json intentionally omits remoteUser -- it should be inferred
	// from the devcontainer.metadata label baked into the image.
	configContent := `{
		"build": {"dockerfile": "Dockerfile"},
		"overrideCommand": true
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-metadata-label"
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

	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Restore workspace ownership so t.TempDir cleanup can remove the dir.
	// chownWorkspace transfers ownership to the non-root remoteUser; we must
	// chown it back to root before the container is deleted.
	t.Cleanup(func() {
		_ = d.ExecContainer(ctx, wsID, result.ContainerID,
			[]string{"chown", "-R", "root:root", result.WorkspaceFolder},
			nil, io.Discard, io.Discard, nil, "root")
	})

	if result.RemoteUser != "testlabeluser" {
		t.Errorf("RemoteUser = %q, want %q (from devcontainer.metadata label)", result.RemoteUser, "testlabeluser")
	}
}

func TestIntegrationDockerfileUser(t *testing.T) {
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

	// Dockerfile with a USER instruction but no remoteUser in devcontainer.json.
	// crib should infer remoteUser from the image's Config.User after the build.
	dockerfile := `FROM alpine:3.20
RUN adduser -D nonroot
USER nonroot
`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"build": {"dockerfile": "Dockerfile"},
		"overrideCommand": true
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-dockerfile-user"
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

	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Restore workspace ownership so t.TempDir cleanup can remove the dir.
	// chownWorkspace transfers ownership to the non-root remoteUser; we must
	// chown it back to root before the container is deleted.
	t.Cleanup(func() {
		_ = d.ExecContainer(ctx, wsID, result.ContainerID,
			[]string{"chown", "-R", "root:root", result.WorkspaceFolder},
			nil, io.Discard, io.Discard, nil, "root")
	})

	if result.RemoteUser != "nonroot" {
		t.Errorf("RemoteUser = %q, want %q (from Dockerfile USER instruction)", result.RemoteUser, "nonroot")
	}
}
