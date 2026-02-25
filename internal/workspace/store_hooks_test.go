package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarkHookDone(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreAt(dir)

	if err := store.MarkHookDone("ws1", "onCreateCommand"); err != nil {
		t.Fatalf("MarkHookDone: %v", err)
	}

	// Marker file should exist.
	path := filepath.Join(dir, "ws1", "hooks", "onCreateCommand.done")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected marker file at %s: %v", path, err)
	}
}

func TestIsHookDone(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreAt(dir)

	if store.IsHookDone("ws1", "onCreateCommand") {
		t.Fatal("expected IsHookDone to return false before marking")
	}

	if err := store.MarkHookDone("ws1", "onCreateCommand"); err != nil {
		t.Fatalf("MarkHookDone: %v", err)
	}

	if !store.IsHookDone("ws1", "onCreateCommand") {
		t.Fatal("expected IsHookDone to return true after marking")
	}

	// Different hook name should not be marked.
	if store.IsHookDone("ws1", "postCreateCommand") {
		t.Fatal("expected IsHookDone to return false for different hook")
	}

	// Different workspace should not be marked.
	if store.IsHookDone("ws2", "onCreateCommand") {
		t.Fatal("expected IsHookDone to return false for different workspace")
	}
}

func TestClearHookMarkers(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreAt(dir)

	// Mark several hooks.
	for _, hook := range []string{"onCreateCommand", "updateContentCommand", "postCreateCommand"} {
		if err := store.MarkHookDone("ws1", hook); err != nil {
			t.Fatalf("MarkHookDone(%s): %v", hook, err)
		}
	}

	if err := store.ClearHookMarkers("ws1"); err != nil {
		t.Fatalf("ClearHookMarkers: %v", err)
	}

	for _, hook := range []string{"onCreateCommand", "updateContentCommand", "postCreateCommand"} {
		if store.IsHookDone("ws1", hook) {
			t.Fatalf("expected IsHookDone(%s) to return false after clearing", hook)
		}
	}
}

func TestClearHookMarkersNoDir(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreAt(dir)

	// Clearing markers when no hooks directory exists should not error.
	if err := store.ClearHookMarkers("ws-nonexistent"); err != nil {
		t.Fatalf("ClearHookMarkers on missing dir: %v", err)
	}
}
