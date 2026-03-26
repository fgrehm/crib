package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	ocidriver "github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/workspace"
)

// doctorMockDriver supports configurable behavior for doctor tests.
type doctorMockDriver struct {
	mockDriver
	archErr    error
	containers []driver.ContainerDetails
	deleted    []string // container IDs that were deleted
}

func (m *doctorMockDriver) TargetArchitecture(_ context.Context) (string, error) {
	if m.archErr != nil {
		return "", m.archErr
	}
	return "amd64", nil
}

func (m *doctorMockDriver) ListContainers(_ context.Context) ([]driver.ContainerDetails, error) {
	return m.containers, nil
}

func (m *doctorMockDriver) DeleteContainer(_ context.Context, _, containerID string) error {
	m.deleted = append(m.deleted, containerID)
	return nil
}

func TestDoctor_RuntimeUnreachable(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	mockDrv := &doctorMockDriver{archErr: fmt.Errorf("connection refused")}
	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	result, err := eng.Doctor(context.Background(), false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	if result.RuntimeOK {
		t.Error("expected RuntimeOK=false")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Check == "runtime" {
			found = true
		}
	}
	if !found {
		t.Error("expected runtime issue")
	}
}

func TestDoctor_ComposeUnavailable(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	mockDrv := &doctorMockDriver{}
	eng := &Engine{
		driver:  mockDrv,
		compose: nil, // no compose
		store:   store,
		logger:  slog.Default(),
		stdout:  io.Discard,
		stderr:  io.Discard,
	}

	result, err := eng.Doctor(context.Background(), false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	if result.ComposeOK {
		t.Error("expected ComposeOK=false")
	}
}

func TestDoctor_OrphanedWorkspace(t *testing.T) {
	storeDir := t.TempDir()
	store := workspace.NewStoreAt(storeDir)

	// Create a workspace pointing to a non-existent directory.
	ws := &workspace.Workspace{ID: "ws-orphan", Source: "/nonexistent/path"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &doctorMockDriver{}
	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	result, err := eng.Doctor(context.Background(), false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Check == "orphaned-workspace" && issue.WorkspaceID == "ws-orphan" {
			found = true
		}
	}
	if !found {
		t.Error("expected orphaned-workspace issue")
	}

	// Verify workspace still exists (no fix).
	if !store.Exists("ws-orphan") {
		t.Error("workspace should still exist without --fix")
	}
}

func TestDoctor_OrphanedWorkspace_Fix(t *testing.T) {
	storeDir := t.TempDir()
	store := workspace.NewStoreAt(storeDir)

	ws := &workspace.Workspace{ID: "ws-orphan-fix", Source: "/nonexistent/path"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &doctorMockDriver{}
	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	_, err := eng.Doctor(context.Background(), true)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	if store.Exists("ws-orphan-fix") {
		t.Error("workspace should be deleted with --fix")
	}
}

func TestDoctor_DanglingContainer(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	mockDrv := &doctorMockDriver{
		containers: []driver.ContainerDetails{
			{
				ID: "container-dangling",
				Config: driver.ContainerConfig{
					Labels: map[string]string{"crib.workspace": "ws-gone"},
				},
			},
		},
	}

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	result, err := eng.Doctor(context.Background(), false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Check == "dangling-container" {
			found = true
		}
	}
	if !found {
		t.Error("expected dangling-container issue")
	}
}

func TestDoctor_DanglingContainer_Fix(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	mockDrv := &doctorMockDriver{
		containers: []driver.ContainerDetails{
			{
				ID: "container-dangling",
				Config: driver.ContainerConfig{
					Labels: map[string]string{"crib.workspace": "ws-gone"},
				},
			},
		},
	}

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	_, err := eng.Doctor(context.Background(), true)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	if len(mockDrv.deleted) != 1 || mockDrv.deleted[0] != "container-dangling" {
		t.Errorf("expected container-dangling to be deleted, got %v", mockDrv.deleted)
	}
}

func TestDoctor_StalePluginData(t *testing.T) {
	storeDir := t.TempDir()
	store := workspace.NewStoreAt(storeDir)

	// Create workspace without a result but with plugin data.
	ws := &workspace.Workspace{ID: "ws-stale", Source: t.TempDir()} // source exists
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	pluginDir := filepath.Join(storeDir, "ws-stale", "plugins", "shell-history")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, ".shell_history"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	mockDrv := &doctorMockDriver{}
	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	result, err := eng.Doctor(context.Background(), false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Check == "stale-plugins" && issue.WorkspaceID == "ws-stale" {
			found = true
		}
	}
	if !found {
		t.Error("expected stale-plugins issue")
	}
}

func TestDoctor_StalePluginData_Fix(t *testing.T) {
	storeDir := t.TempDir()
	store := workspace.NewStoreAt(storeDir)

	ws := &workspace.Workspace{ID: "ws-stale-fix", Source: t.TempDir()}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	pluginDir := filepath.Join(storeDir, "ws-stale-fix", "plugins", "shell-history")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, ".shell_history"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	mockDrv := &doctorMockDriver{}
	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	_, err := eng.Doctor(context.Background(), true)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	parentPluginDir := filepath.Join(storeDir, "ws-stale-fix", "plugins")
	if _, err := os.Stat(parentPluginDir); !os.IsNotExist(err) {
		t.Error("stale plugin directory should be removed with --fix")
	}
}

func TestDoctor_DanglingContainer_SkippedWhenDifferentCribHome(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	mockDrv := &doctorMockDriver{
		containers: []driver.ContainerDetails{
			{
				ID: "container-other-store",
				Config: driver.ContainerConfig{
					Labels: map[string]string{
						"crib.workspace":    "some-workspace",
						ocidriver.LabelHome: "/some/other/store",
					},
				},
			},
		},
	}

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	result, err := eng.Doctor(context.Background(), false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	for _, issue := range result.Issues {
		if issue.Check == "dangling-container" {
			t.Errorf("should skip container from different crib.home, got issue: %s", issue.Description)
		}
	}
}

func TestDoctor_DanglingContainer_FlaggedWhenSameCribHome(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	mockDrv := &doctorMockDriver{
		containers: []driver.ContainerDetails{
			{
				ID: "container-same-store",
				Config: driver.ContainerConfig{
					Labels: map[string]string{
						"crib.workspace":    "nonexistent-ws",
						ocidriver.LabelHome: store.BaseDir(),
					},
				},
			},
		},
	}

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	result, err := eng.Doctor(context.Background(), false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Check == "dangling-container" && issue.WorkspaceID == "nonexistent-ws" {
			found = true
		}
	}
	if !found {
		t.Error("expected dangling-container issue for container matching crib.home")
	}
}

func TestDoctor_DanglingContainer_FlaggedWhenNoHomeLabel(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	// Pre-v0.8.0 containers have no crib.home label. They should still be
	// flagged as dangling (the guard only skips when home is present AND different).
	mockDrv := &doctorMockDriver{
		containers: []driver.ContainerDetails{
			{
				ID: "container-legacy",
				Config: driver.ContainerConfig{
					Labels: map[string]string{
						"crib.workspace": "orphan-ws",
					},
				},
			},
		},
	}

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	result, err := eng.Doctor(context.Background(), false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Check == "dangling-container" && issue.WorkspaceID == "orphan-ws" {
			found = true
		}
	}
	if !found {
		t.Error("expected dangling-container for container without crib.home label")
	}
}

func TestDoctor_Fix_SkipsContainerFromDifferentStore(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	mockDrv := &doctorMockDriver{
		containers: []driver.ContainerDetails{
			{
				ID: "container-protected",
				Config: driver.ContainerConfig{
					Labels: map[string]string{
						"crib.workspace":    "other-ws",
						ocidriver.LabelHome: "/different/store",
					},
				},
			},
		},
	}

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	_, err := eng.Doctor(context.Background(), true)
	if err != nil {
		t.Fatalf("Doctor --fix: %v", err)
	}

	if len(mockDrv.deleted) != 0 {
		t.Errorf("should not delete container from different store, deleted: %v", mockDrv.deleted)
	}
}

func TestDoctor_HealthySystem(t *testing.T) {
	storeDir := t.TempDir()
	store := workspace.NewStoreAt(storeDir)

	// Create a healthy workspace.
	sourceDir := t.TempDir()
	ws := &workspace.Workspace{ID: "ws-healthy", Source: sourceDir}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	mergedJSON, _ := json.Marshal(cfg)
	if err := store.SaveResult(ws.ID, &workspace.Result{
		ContainerID:  "healthy-container",
		MergedConfig: mergedJSON,
	}); err != nil {
		t.Fatal(err)
	}

	mockDrv := &doctorMockDriver{
		containers: []driver.ContainerDetails{
			{
				ID: "healthy-container",
				Config: driver.ContainerConfig{
					Labels: map[string]string{"crib.workspace": "ws-healthy"},
				},
			},
		},
	}

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	result, err := eng.Doctor(context.Background(), false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	if !result.RuntimeOK {
		t.Error("expected RuntimeOK=true")
	}
	// Compose is nil, so that warning is expected.
	nonComposeIssues := 0
	for _, issue := range result.Issues {
		if issue.Check != "compose" {
			nonComposeIssues++
		}
	}
	if nonComposeIssues != 0 {
		t.Errorf("expected no non-compose issues, got %d: %v", nonComposeIssues, result.Issues)
	}
}
