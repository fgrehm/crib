package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestComposeStdout_Default(t *testing.T) {
	e := &Engine{
		stdout: &bytes.Buffer{},
	}

	if got := e.composeStdout(); got != io.Discard {
		t.Error("composeStdout should return io.Discard when verbose is false")
	}
}

func TestComposeStdout_Verbose(t *testing.T) {
	buf := &bytes.Buffer{}
	e := &Engine{
		stdout:  buf,
		verbose: true,
	}

	if got := e.composeStdout(); got != buf {
		t.Error("composeStdout should return stdout when verbose is true")
	}
}

func TestComposeStderr_Default(t *testing.T) {
	e := &Engine{
		stderr: &bytes.Buffer{},
	}

	if got := e.composeStderr(); got != io.Discard {
		t.Error("composeStderr should return io.Discard when verbose is false")
	}
}

func TestComposeStderr_Verbose(t *testing.T) {
	buf := &bytes.Buffer{}
	e := &Engine{
		stderr:  buf,
		verbose: true,
	}

	if got := e.composeStderr(); got != buf {
		t.Error("composeStderr should return stderr when verbose is true")
	}
}

// composeWorkspaceResult returns a stored result with a compose devcontainer config.
func composeWorkspaceResult(t *testing.T, store *workspace.Store, wsID string) {
	t.Helper()
	result := &workspace.Result{
		MergedConfig: []byte(`{"dockerComposeFile":["docker-compose.yml"],"service":"app"}`),
	}
	if err := store.SaveResult(wsID, result); err != nil {
		t.Fatal(err)
	}
}

func TestDown_ComposeMissing_ReturnsError(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "test-down-compose-nil", Source: t.TempDir(), DevContainerPath: ".devcontainer/devcontainer.json"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	composeWorkspaceResult(t, store, ws.ID)

	e := &Engine{driver: &mockDriver{}, store: store, logger: slog.Default(), stdout: io.Discard, stderr: io.Discard}

	err := e.Down(context.Background(), ws)
	if err == nil {
		t.Fatal("expected error when compose is nil for compose workspace")
	}
	var target *ErrComposeNotAvailable
	if !errors.As(err, &target) {
		t.Errorf("expected ErrComposeNotAvailable, got: %v", err)
	}
}

func TestStop_ComposeMissing_ReturnsError(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "test-stop-compose-nil", Source: t.TempDir(), DevContainerPath: ".devcontainer/devcontainer.json"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	composeWorkspaceResult(t, store, ws.ID)

	e := &Engine{driver: &mockDriver{}, store: store, logger: slog.Default(), stdout: io.Discard, stderr: io.Discard}

	err := e.Stop(context.Background(), ws)
	if err == nil {
		t.Fatal("expected error when compose is nil for compose workspace")
	}
	var target *ErrComposeNotAvailable
	if !errors.As(err, &target) {
		t.Errorf("expected ErrComposeNotAvailable, got: %v", err)
	}
}

func TestRemove_ComposeMissing_ReturnsError(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "test-remove-compose-nil", Source: t.TempDir(), DevContainerPath: ".devcontainer/devcontainer.json"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	composeWorkspaceResult(t, store, ws.ID)

	e := &Engine{driver: &mockDriver{}, store: store, logger: slog.Default(), stdout: io.Discard, stderr: io.Discard}

	err := e.Remove(context.Background(), ws)
	if err == nil {
		t.Fatal("expected error when compose is nil for compose workspace")
	}
	var target *ErrComposeNotAvailable
	if !errors.As(err, &target) {
		t.Errorf("expected ErrComposeNotAvailable, got: %v", err)
	}
}

func TestDown_ClearsHookMarkers(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	ws := &workspace.Workspace{
		ID:               "test-down-markers",
		Source:           t.TempDir(),
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	// Create hook markers.
	for _, hook := range []string{"onCreateCommand", "updateContentCommand", "postCreateCommand"} {
		if err := store.MarkHookDone(ws.ID, hook); err != nil {
			t.Fatal(err)
		}
	}

	// Verify markers exist.
	for _, hook := range []string{"onCreateCommand", "updateContentCommand", "postCreateCommand"} {
		if !store.IsHookDone(ws.ID, hook) {
			t.Fatalf("expected marker for %s to exist", hook)
		}
	}

	e := &Engine{
		driver: &mockDriver{},
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	// Down will fail (no container), but should still clear markers.
	_ = e.Down(context.Background(), ws)

	// Verify markers were cleared.
	for _, hook := range []string{"onCreateCommand", "updateContentCommand", "postCreateCommand"} {
		if store.IsHookDone(ws.ID, hook) {
			t.Errorf("expected marker for %s to be cleared after Down", hook)
		}
	}
}

func TestStop_PreservesHookMarkers(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	ws := &workspace.Workspace{
		ID:               "test-stop-markers",
		Source:           t.TempDir(),
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	// Create hook markers.
	for _, hook := range []string{"onCreateCommand", "updateContentCommand", "postCreateCommand"} {
		if err := store.MarkHookDone(ws.ID, hook); err != nil {
			t.Fatal(err)
		}
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "abc123",
			State: driver.ContainerState{Status: "running"},
		},
	}

	e := &Engine{
		driver: drv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	if err := e.Stop(context.Background(), ws); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Verify markers are still present (not cleared like Down).
	for _, hook := range []string{"onCreateCommand", "updateContentCommand", "postCreateCommand"} {
		if !store.IsHookDone(ws.ID, hook) {
			t.Errorf("expected marker for %s to survive Stop", hook)
		}
	}
}

func TestStop_NoContainer(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	ws := &workspace.Workspace{
		ID:               "test-stop-nocontainer",
		Source:           t.TempDir(),
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	e := &Engine{
		driver: &mockDriver{}, // FindContainer returns nil
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	err := e.Stop(context.Background(), ws)
	if err == nil {
		t.Fatal("expected error when no container exists")
	}
	var target *ErrNoContainer
	if !errors.As(err, &target) {
		t.Errorf("expected ErrNoContainer, got: %v", err)
	}
}

func TestStop_AlreadyStopped(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	ws := &workspace.Workspace{
		ID:               "test-stop-exited",
		Source:           t.TempDir(),
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "abc123",
			State: driver.ContainerState{Status: "exited"},
		},
	}

	e := &Engine{
		driver: drv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	// Stopping an already-stopped container should not error.
	if err := e.Stop(context.Background(), ws); err != nil {
		t.Fatalf("Stop on exited container should succeed, got: %v", err)
	}
}

func TestRemove_DeletesWorkspaceState(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	ws := &workspace.Workspace{
		ID:               "test-remove-state",
		Source:           t.TempDir(),
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	// Verify workspace exists.
	if _, err := store.Load(ws.ID); err != nil {
		t.Fatalf("workspace should exist: %v", err)
	}

	e := &Engine{
		driver: &mockDriver{},
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	// Remove will warn about missing container but should delete state.
	if err := e.Remove(context.Background(), ws); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify workspace state is gone.
	if _, err := store.Load(ws.ID); err == nil {
		t.Error("workspace state should be deleted after Remove")
	}
}

func TestRequireRunningContainer_Running(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-1", Source: t.TempDir()}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{ID: "abc123", State: driver.ContainerState{Status: "running"}},
	}
	eng := &Engine{driver: drv, store: store, logger: slog.Default()}

	got, err := eng.RequireRunningContainer(context.Background(), ws)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.ID != "abc123" {
		t.Errorf("container ID = %q, want %q", got.ID, "abc123")
	}
}

func TestRequireRunningContainer_NoContainer(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-2", Source: t.TempDir()}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	eng := &Engine{driver: &mockDriver{}, store: store, logger: slog.Default()} // FindContainer returns nil

	_, err := eng.RequireRunningContainer(context.Background(), ws)
	if err == nil {
		t.Fatal("expected error for missing container")
	}
	var target *ErrNoContainer
	if !errors.As(err, &target) {
		t.Errorf("expected ErrNoContainer, got: %v", err)
	}
}

func TestRequireRunningContainer_Stopped(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-3", Source: t.TempDir()}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{ID: "abc123", State: driver.ContainerState{Status: "exited"}},
	}
	eng := &Engine{driver: drv, store: store, logger: slog.Default()}

	_, err := eng.RequireRunningContainer(context.Background(), ws)
	if err == nil {
		t.Fatal("expected error for stopped container")
	}
	var target *ErrContainerStopped
	if !errors.As(err, &target) {
		t.Errorf("expected ErrContainerStopped, got: %v", err)
	}
	if target.ContainerID != "abc123" {
		t.Errorf("ContainerID = %q, want %q", target.ContainerID, "abc123")
	}
}

func TestEnsureContainerRunning_Running(t *testing.T) {
	eng := &Engine{driver: &mockDriver{}, logger: slog.Default()}

	container := &driver.ContainerDetails{
		ID:    "abc123",
		State: driver.ContainerState{Status: "running"},
	}

	err := eng.ensureContainerRunning(context.Background(), "ws-1", container)
	if err != nil {
		t.Errorf("expected no error for running container, got: %v", err)
	}
}

func TestEnsureContainerRunning_Exited(t *testing.T) {
	eng := &Engine{driver: &mockDriver{}, logger: slog.Default()}

	container := &driver.ContainerDetails{
		ID:    "abc123",
		State: driver.ContainerState{Status: "exited"},
	}

	err := eng.ensureContainerRunning(context.Background(), "ws-1", container)
	if err == nil {
		t.Fatal("expected error for exited container")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("expected 'not running' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("expected status in error, got: %v", err)
	}
}

func TestEnsureContainerRunning_EmptyState_FindReturnsNil(t *testing.T) {
	// mockDriver.FindContainer returns nil by default, so the empty state
	// from compose ps fallback is used. Empty status is not "running".
	eng := &Engine{driver: &mockDriver{}, logger: slog.Default()}

	container := &driver.ContainerDetails{
		ID: "abc123",
		// State is zero value (empty Status)
	}

	err := eng.ensureContainerRunning(context.Background(), "ws-1", container)
	if err == nil {
		t.Fatal("expected error for container with empty state")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("expected 'not running' in error, got: %v", err)
	}
}

// fixedFindContainerDriver wraps mockDriver but returns a specific container
// from FindContainer.
type fixedFindContainerDriver struct {
	mockDriver
	container *driver.ContainerDetails
}

func (m *fixedFindContainerDriver) FindContainer(_ context.Context, _ string) (*driver.ContainerDetails, error) {
	return m.container, nil
}

func TestEnsureContainerRunning_EmptyState_FindReturnsRunning(t *testing.T) {
	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "abc123",
			State: driver.ContainerState{Status: "running"},
		},
	}
	eng := &Engine{driver: drv, logger: slog.Default()}

	// Container from compose ps fallback (empty state).
	container := &driver.ContainerDetails{ID: "abc123"}

	err := eng.ensureContainerRunning(context.Background(), "ws-1", container)
	if err != nil {
		t.Errorf("expected no error after re-inspect finds running container, got: %v", err)
	}
}

func TestRemove_CleansUpBuildAndLabeledImages(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	ws := &workspace.Workspace{
		ID:               "test-ws",
		Source:           t.TempDir(),
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveResult(ws.ID, &workspace.Result{
		ImageName:     "crib-test-ws:crib-abc",
		SnapshotImage: "crib-test-ws:snapshot",
	}); err != nil {
		t.Fatal(err)
	}

	md := &imageTrackingDriver{
		images: []driver.ImageInfo{
			{Reference: "crib-test-ws:crib-old", ID: "sha256:stale", Size: 100, WorkspaceID: "test-ws"},
		},
	}
	eng := &Engine{
		driver: md,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	if err := eng.Remove(context.Background(), ws); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Should have removed: snapshot, build image, and the stale labeled image.
	want := map[string]bool{
		"crib-test-ws:snapshot": true,
		"crib-test-ws:crib-abc": true,
		"crib-test-ws:crib-old": true,
	}
	for _, img := range md.removedImages {
		delete(want, img)
	}
	if len(want) > 0 {
		t.Errorf("expected images not removed: %v (removed: %v)", want, md.removedImages)
	}
}

// TestCleanupWorkspaceImages_ComposeBuiltImages verifies that cleanup for a
// compose workspace also removes images tagged with
// com.docker.compose.project=<proj>, not just crib.workspace-labeled images.
// Pulled images (images with no project label for this project) are left
// alone because ListImages only returns labeled ones.
func TestCleanupWorkspaceImages_ComposeBuiltImages(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{
		ID:               "compose-ws",
		Source:           t.TempDir(),
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveResult(ws.ID, &workspace.Result{
		MergedConfig: []byte(`{"dockerComposeFile":["docker-compose.yml"],"service":"app"}`),
	}); err != nil {
		t.Fatal(err)
	}

	md := &composeImageMockDriver{
		byLabel: map[string][]driver.ImageInfo{
			"crib.workspace=compose-ws": {
				{Reference: "crib-compose-ws:base", ID: "sha256:base"},
			},
			"com.docker.compose.project=crib-compose-ws": {
				{Reference: "crib-compose-ws-app", ID: "sha256:app"},
				{Reference: "crib-compose-ws-worker", ID: "sha256:worker"},
			},
		},
	}
	eng := &Engine{
		driver: md,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	eng.cleanupWorkspaceImages(context.Background(), ws.ID)

	want := map[string]bool{
		"crib-compose-ws:base":   false,
		"crib-compose-ws-app":    false,
		"crib-compose-ws-worker": false,
	}
	for _, img := range md.removedImages {
		if _, ok := want[img]; ok {
			want[img] = true
		}
	}
	for img, removed := range want {
		if !removed {
			t.Errorf("expected %q to be removed (got %v)", img, md.removedImages)
		}
	}
}

// TestCleanupWorkspaceImages_NonComposeSkipsProjectLabelLookup verifies that
// the compose project-label lookup is not performed for non-compose workspaces.
func TestCleanupWorkspaceImages_NonComposeSkipsProjectLabelLookup(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{
		ID:               "image-ws",
		Source:           t.TempDir(),
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveResult(ws.ID, &workspace.Result{
		MergedConfig: []byte(`{"image":"alpine:3.20"}`),
	}); err != nil {
		t.Fatal(err)
	}

	md := &composeImageMockDriver{
		byLabel: map[string][]driver.ImageInfo{
			"crib.workspace=image-ws": {
				{Reference: "crib-image-ws:build", ID: "sha256:build"},
			},
		},
	}
	eng := &Engine{
		driver: md,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	eng.cleanupWorkspaceImages(context.Background(), ws.ID)

	for _, label := range md.queriedLabels {
		if strings.HasPrefix(label, "com.docker.compose.project=") {
			t.Errorf("unexpected compose-project lookup for non-compose workspace: %q", label)
		}
	}
}

// composeImageMockDriver returns a different image list per label filter and
// records RemoveImage and ListImages calls. Used to verify scope-A2 compose
// image cleanup.
type composeImageMockDriver struct {
	mockDriver
	byLabel       map[string][]driver.ImageInfo
	removedImages []string
	queriedLabels []string
}

func (m *composeImageMockDriver) ListImages(_ context.Context, label string) ([]driver.ImageInfo, error) {
	m.queriedLabels = append(m.queriedLabels, label)
	return m.byLabel[label], nil
}

func (m *composeImageMockDriver) RemoveImage(_ context.Context, image string) error {
	m.removedImages = append(m.removedImages, image)
	return nil
}

func TestRemove_SkipsBaseImages(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	ws := &workspace.Workspace{
		ID:               "test-ws",
		Source:           t.TempDir(),
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveResult(ws.ID, &workspace.Result{
		ImageName: "ubuntu:22.04",
	}); err != nil {
		t.Fatal(err)
	}

	md := &imageTrackingDriver{}
	eng := &Engine{
		driver: md,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	if err := eng.Remove(context.Background(), ws); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Should not remove the base image (ubuntu:22.04).
	for _, img := range md.removedImages {
		if img == "ubuntu:22.04" {
			t.Errorf("should not have removed base image %s", img)
		}
	}
}

func TestStoredComposeConfig(t *testing.T) {
	tests := []struct {
		name   string
		result *workspace.Result
		want   bool // true == non-nil return
	}{
		{"nil result", nil, false},
		{"non-compose config", &workspace.Result{MergedConfig: []byte(`{"image":"ubuntu"}`)}, false},
		{"compose config", &workspace.Result{MergedConfig: []byte(`{"dockerComposeFile":["docker-compose.yml"]}`)}, true},
		{"invalid JSON", &workspace.Result{MergedConfig: []byte(`{bad}`)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := storedComposeConfig(tt.result)
			if (got != nil) != tt.want {
				t.Errorf("storedComposeConfig() = %v, want non-nil=%v", got, tt.want)
			}
		})
	}
}

func TestNewComposeInvocation_IncludesService(t *testing.T) {
	ws := &workspace.Workspace{
		ID:               "web",
		Source:           t.TempDir(),
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
	cfg := &config.DevContainerConfig{
		ComposeContainer: config.ComposeContainer{
			Service:           "rails-app",
			DockerComposeFile: []string{"compose.yaml"},
		},
	}

	inv := newComposeInvocation(ws, cfg, ws.Source)

	if inv.service != "rails-app" {
		t.Errorf("inv.service = %q, want %q", inv.service, "rails-app")
	}
}
