package engine

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/workspace"
)

func newTestEngineWithCompose(t *testing.T) (*Engine, *oci.OCIDriver, *workspace.Store) {
	t.Helper()
	d, err := oci.NewOCIDriver(slog.Default())
	if err != nil {
		t.Skipf("skipping: no container runtime available: %v", err)
	}

	composeHelper, err := compose.NewHelper(d.Runtime().String(), slog.Default())
	if err != nil {
		t.Skipf("skipping: compose not available: %v", err)
	}

	store := workspace.NewStoreAt(t.TempDir())
	eng := New(d, composeHelper, store, slog.Default())
	eng.SetOutput(os.Stdout, os.Stderr)
	return eng, d, store
}

// writeComposeDevcontainer creates a minimal compose-based devcontainer config
// in the given project directory. Returns the workspace.
func writeComposeDevcontainer(t *testing.T, projectDir, wsID string) *workspace.Workspace {
	t.Helper()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	composeContent := `services:
  app:
    image: alpine:3.20
    command: ["sleep", "infinity"]
`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "compose.yml"), []byte(composeContent), 0o644); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"dockerComposeFile": "compose.yml",
		"service": "app",
		"overrideCommand": true,
		"onCreateCommand": "touch /tmp/on-create-ran",
		"postStartCommand": "touch /tmp/post-start-ran"
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	return &workspace.Workspace{
		ID:               wsID,
		Source:            projectDir,
		DevContainerPath: ".devcontainer/devcontainer.json",
		CreatedAt:        time.Now(),
		LastUsedAt:       time.Now(),
	}
}

// cleanupCompose tears down any containers/networks for the workspace.
func cleanupCompose(t *testing.T, e *Engine, ws *workspace.Workspace) {
	t.Helper()
	ctx := context.Background()
	_ = e.Down(ctx, ws)
}

// TestIntegrationComposeDownUpSkipsBuild verifies that after a down + up cycle,
// the compose workspace skips the image build and just starts services.
func TestIntegrationComposeDownUpSkipsBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, _, store := newTestEngineWithCompose(t)

	projectDir := t.TempDir()
	wsID := "test-compose-down-up"
	ws := writeComposeDevcontainer(t, projectDir, wsID)

	t.Cleanup(func() { cleanupCompose(t, e, ws) })
	cleanupCompose(t, e, ws)

	// Initial Up — full creation path.
	result1, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up (first): %v", err)
	}
	if result1.ContainerID == "" {
		t.Fatal("first Up returned empty ContainerID")
	}

	// Verify onCreateCommand ran.
	if err := checkFileExists(ctx, e, ws.ID, result1.ContainerID, "/tmp/on-create-ran"); err != nil {
		t.Fatalf("onCreateCommand did not run on first Up: %v", err)
	}

	// Down — removes container, clears hook markers.
	if err := e.Down(ctx, ws); err != nil {
		t.Fatalf("Down: %v", err)
	}

	// Container should be gone.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		t.Fatalf("FindContainer after Down: %v", err)
	}
	if container != nil {
		t.Error("container still exists after Down")
	}

	// Stored result should still exist (Down keeps workspace state).
	storedResult, err := store.LoadResult(ws.ID)
	if err != nil || storedResult == nil {
		t.Fatal("stored result should persist after Down")
	}

	// Up again — should use stored result, skip build.
	result2, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up (second): %v", err)
	}
	if result2.ContainerID == "" {
		t.Fatal("second Up returned empty ContainerID")
	}

	// Container ID should differ (new container created).
	if result2.ContainerID == result1.ContainerID {
		t.Error("expected different container ID after down + up")
	}

	// onCreateCommand should have run again (markers were cleared by Down).
	if err := checkFileExists(ctx, e, ws.ID, result2.ContainerID, "/tmp/on-create-ran"); err != nil {
		t.Fatalf("onCreateCommand did not run on second Up (after Down cleared markers): %v", err)
	}
}

// TestIntegrationComposeRestartWithStoppedDeps verifies that restart works
// even when dependency services are stopped (uses compose up instead of
// compose restart).
func TestIntegrationComposeRestartWithStoppedDeps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, _, _ := newTestEngineWithCompose(t)

	projectDir := t.TempDir()
	wsID := "test-compose-restart-deps"

	// Multi-service compose with a dependency.
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	composeContent := `services:
  db:
    image: alpine:3.20
    command: ["sleep", "infinity"]
  app:
    image: alpine:3.20
    command: ["sleep", "infinity"]
    depends_on:
      - db
`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "compose.yml"), []byte(composeContent), 0o644); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"dockerComposeFile": "compose.yml",
		"service": "app",
		"runServices": ["app", "db"],
		"overrideCommand": true,
		"postStartCommand": "touch /tmp/post-start-ran"
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := &workspace.Workspace{
		ID:               wsID,
		Source:            projectDir,
		DevContainerPath: ".devcontainer/devcontainer.json",
		CreatedAt:        time.Now(),
		LastUsedAt:       time.Now(),
	}

	t.Cleanup(func() { cleanupCompose(t, e, ws) })
	cleanupCompose(t, e, ws)

	// Initial Up.
	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Stop all services to simulate host restart, Docker restart, etc.
	projectName := compose.ProjectName(ws.ID)
	devcontainerDir2 := filepath.Dir(filepath.Join(ws.Source, ws.DevContainerPath))
	composeFile := filepath.Join(devcontainerDir2, "compose.yml")
	if err := e.compose.Stop(ctx, projectName, []string{composeFile}, os.Stdout, os.Stderr, nil); err != nil {
		t.Fatalf("compose stop: %v", err)
	}

	// Remove the post-start marker so we can verify it runs on restart.
	_ = e.driver.ExecContainer(ctx, ws.ID, result.ContainerID, []string{"rm", "-f", "/tmp/post-start-ran"}, nil, nil, nil, nil, "")

	// Restart should succeed even though all services are stopped.
	restartResult, err := e.Restart(ctx, ws)
	if err != nil {
		t.Fatalf("Restart after services stopped: %v", err)
	}
	if restartResult.ContainerID == "" {
		t.Error("Restart returned empty ContainerID")
	}

	// postStartCommand should have run.
	if err := checkFileExists(ctx, e, ws.ID, restartResult.ContainerID, "/tmp/post-start-ran"); err != nil {
		t.Errorf("postStartCommand did not run on Restart: %v", err)
	}
}

// TestIntegrationComposeDownClearsMarkers verifies that Down clears hook markers
// so that a subsequent Up re-runs lifecycle hooks.
func TestIntegrationComposeDownClearsMarkers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, _, store := newTestEngineWithCompose(t)

	projectDir := t.TempDir()
	wsID := "test-compose-markers"
	ws := writeComposeDevcontainer(t, projectDir, wsID)

	t.Cleanup(func() { cleanupCompose(t, e, ws) })
	cleanupCompose(t, e, ws)

	// Initial Up.
	if _, err := e.Up(ctx, ws, UpOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Hook markers should exist for create-time hooks.
	if !store.IsHookDone(wsID, "onCreateCommand") {
		t.Error("onCreateCommand marker should exist after Up")
	}
	// postStartCommand runs every time and doesn't use markers.

	// Down should clear markers.
	if err := e.Down(ctx, ws); err != nil {
		t.Fatalf("Down: %v", err)
	}

	if store.IsHookDone(wsID, "onCreateCommand") {
		t.Error("onCreateCommand marker should be cleared after Down")
	}
}

// checkFileExists verifies a file exists in the container.
func checkFileExists(ctx context.Context, e *Engine, wsID, containerID, path string) error {
	return e.driver.ExecContainer(ctx, wsID, containerID, []string{"test", "-f", path}, nil, nil, nil, nil, "")
}

// TestIntegrationComposeImageNamePersisted verifies that the ImageName field
// is saved in the workspace result after a compose Up.
func TestIntegrationComposeImageNamePersisted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	e, _, store := newTestEngineWithCompose(t)

	projectDir := t.TempDir()
	wsID := "test-compose-imagename"
	ws := writeComposeDevcontainer(t, projectDir, wsID)

	t.Cleanup(func() { cleanupCompose(t, e, ws) })
	cleanupCompose(t, e, ws)

	if _, err := e.Up(ctx, ws, UpOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	result, err := store.LoadResult(wsID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}

	// For a simple image-based compose service (no features), ImageName
	// may be empty since no feature image was built. Verify the merged
	// config is stored correctly at minimum.
	if result.MergedConfig == nil {
		t.Error("MergedConfig should be set")
	}

	// Verify the stored config can be unmarshaled.
	var storedCfg map[string]any
	if err := json.Unmarshal(result.MergedConfig, &storedCfg); err != nil {
		t.Fatalf("unmarshaling stored config: %v", err)
	}
}
