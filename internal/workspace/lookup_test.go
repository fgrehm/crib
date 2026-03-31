package workspace

import (
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

func TestLookup_CwdResolvesWorkspace(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"alpine"}`)

	store := NewStoreAt(t.TempDir())
	ws, err := Lookup(store, LookupOptions{Cwd: dir, Version: "v1.0.0", Create: true}, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.Source != dir {
		t.Errorf("Source = %q, want %q", ws.Source, dir)
	}
	if ws.CribVersion != "v1.0.0" {
		t.Errorf("CribVersion = %q, want %q", ws.CribVersion, "v1.0.0")
	}
}

func TestLookup_ConfigDir(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".devcontainer")
	mkdirAll(t, cfgDir)
	writeFile(t, filepath.Join(cfgDir, "devcontainer.json"), `{"image":"alpine"}`)

	store := NewStoreAt(t.TempDir())
	ws, err := Lookup(store, LookupOptions{ConfigDir: cfgDir, Version: "v1.0.0", Create: true}, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.Source != dir {
		t.Errorf("Source = %q, want %q", ws.Source, dir)
	}
}

func TestLookup_Dir(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"alpine"}`)

	store := NewStoreAt(t.TempDir())
	ws, err := Lookup(store, LookupOptions{Dir: dir, Version: "v1.0.0", Create: true}, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.Source != dir {
		t.Errorf("Source = %q, want %q", ws.Source, dir)
	}
}

func TestLookup_CreateFalse_NotFound(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"alpine"}`)

	store := NewStoreAt(t.TempDir())
	_, err := Lookup(store, LookupOptions{Cwd: dir, Version: "v1.0.0", Create: false}, slog.Default())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got: %v", err)
	}
}

func TestLookup_CreateTrue_CreatesWithVersion(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"alpine"}`)

	store := NewStoreAt(t.TempDir())
	ws, err := Lookup(store, LookupOptions{Cwd: dir, Version: "v2.0.0", Create: true}, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.CribVersion != "v2.0.0" {
		t.Errorf("CribVersion = %q, want %q", ws.CribVersion, "v2.0.0")
	}
	if ws.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestLookup_RefreshesDevContainerPath(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"alpine"}`)

	store := NewStoreAt(t.TempDir())
	// Seed with a stale DevContainerPath.
	existing := &Workspace{
		ID:               GenerateID(dir),
		Source:           dir,
		DevContainerPath: "old/path/devcontainer.json",
		CribVersion:      "v1.0.0",
		CreatedAt:        time.Now(),
		LastUsedAt:       time.Now(),
	}
	if err := store.Save(existing); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ws, err := Lookup(store, LookupOptions{Cwd: dir, Version: "v1.0.0", Create: false}, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(".devcontainer", "devcontainer.json")
	if ws.DevContainerPath != want {
		t.Errorf("DevContainerPath = %q, want %q", ws.DevContainerPath, want)
	}
}

func TestLookup_RefreshesCribVersion(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"alpine"}`)

	store := NewStoreAt(t.TempDir())
	existing := &Workspace{
		ID:          GenerateID(dir),
		Source:      dir,
		CribVersion: "v0.9.0",
		CreatedAt:   time.Now(),
		LastUsedAt:  time.Now(),
	}
	if err := store.Save(existing); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ws, err := Lookup(store, LookupOptions{Cwd: dir, Version: "v1.0.0", Create: false}, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.CribVersion != "v1.0.0" {
		t.Errorf("CribVersion = %q, want %q", ws.CribVersion, "v1.0.0")
	}
}
