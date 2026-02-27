package engine

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"

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

func TestDown_ClearsHookMarkers(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())

	ws := &workspace.Workspace{
		ID:               "test-down-markers",
		Source:            t.TempDir(),
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
		driver:  &mockDriver{},
		store:   store,
		logger:  slog.Default(),
		stdout:  io.Discard,
		stderr:  io.Discard,
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
		Source:            t.TempDir(),
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
		driver:  &mockDriver{},
		store:   store,
		logger:  slog.Default(),
		stdout:  io.Discard,
		stderr:  io.Discard,
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
