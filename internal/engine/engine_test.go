package engine

import (
	"bytes"
	"context"
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
