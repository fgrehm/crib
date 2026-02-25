package workspace

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStore_SaveAndLoad(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	ws := &Workspace{
		ID:               "myproject",
		Source:           "/home/user/myproject",
		DevContainerPath: ".devcontainer/devcontainer.json",
		CreatedAt:        time.Now().Truncate(time.Second),
		LastUsedAt:       time.Now().Truncate(time.Second),
	}

	if err := store.Save(ws); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("myproject")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.ID != ws.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, ws.ID)
	}
	if loaded.Source != ws.Source {
		t.Errorf("Source = %q, want %q", loaded.Source, ws.Source)
	}
	if loaded.DevContainerPath != ws.DevContainerPath {
		t.Errorf("DevContainerPath = %q, want %q", loaded.DevContainerPath, ws.DevContainerPath)
	}
	if !loaded.CreatedAt.Equal(ws.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", loaded.CreatedAt, ws.CreatedAt)
	}
}

func TestStore_LoadNotFound(t *testing.T) {
	store := NewStoreAt(t.TempDir())

	_, err := store.Load("nonexistent")
	if err != ErrWorkspaceNotFound {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	ws := &Workspace{ID: "todelete", Source: "/tmp"}

	if err := store.Save(ws); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !store.Exists("todelete") {
		t.Fatal("workspace should exist after save")
	}

	if err := store.Delete("todelete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.Exists("todelete") {
		t.Fatal("workspace should not exist after delete")
	}
}

func TestStore_List(t *testing.T) {
	store := NewStoreAt(t.TempDir())

	// Empty list.
	ids, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty list, got %v", ids)
	}

	// Add some workspaces.
	for _, id := range []string{"alpha", "beta", "gamma"} {
		if err := store.Save(&Workspace{ID: id, Source: "/tmp/" + id}); err != nil {
			t.Fatalf("Save(%s): %v", id, err)
		}
	}

	ids, err = store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("expected 3 workspaces, got %d: %v", len(ids), ids)
	}
}

func TestStore_Exists(t *testing.T) {
	store := NewStoreAt(t.TempDir())

	if store.Exists("nope") {
		t.Error("should not exist")
	}

	if err := store.Save(&Workspace{ID: "yep", Source: "/tmp"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if !store.Exists("yep") {
		t.Error("should exist")
	}
}

func TestStore_SaveAndLoadResult(t *testing.T) {
	store := NewStoreAt(t.TempDir())

	result := &Result{
		ContainerID:     "abc123def456",
		ImageName:       "crib-myproject:latest",
		MergedConfig:    json.RawMessage(`{"image":"ubuntu"}`),
		WorkspaceFolder: "/workspace",
	}

	if err := store.SaveResult("myproject", result); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	loaded, err := store.LoadResult("myproject")
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}

	if loaded.ContainerID != result.ContainerID {
		t.Errorf("ContainerID = %q, want %q", loaded.ContainerID, result.ContainerID)
	}
	if loaded.ImageName != result.ImageName {
		t.Errorf("ImageName = %q, want %q", loaded.ImageName, result.ImageName)
	}
	if loaded.WorkspaceFolder != result.WorkspaceFolder {
		t.Errorf("WorkspaceFolder = %q, want %q", loaded.WorkspaceFolder, result.WorkspaceFolder)
	}
	// Compare MergedConfig as parsed JSON (formatting may differ after round-trip).
	var origConfig, loadedConfig map[string]any
	if err := json.Unmarshal(result.MergedConfig, &origConfig); err != nil {
		t.Fatalf("unmarshal original MergedConfig: %v", err)
	}
	if err := json.Unmarshal(loaded.MergedConfig, &loadedConfig); err != nil {
		t.Fatalf("unmarshal loaded MergedConfig: %v", err)
	}
	if origConfig["image"] != loadedConfig["image"] {
		t.Errorf("MergedConfig image = %v, want %v", loadedConfig["image"], origConfig["image"])
	}
}

func TestStore_LoadResult_NotFound(t *testing.T) {
	store := NewStoreAt(t.TempDir())

	result, err := store.LoadResult("nonexistent")
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for nonexistent workspace")
	}
}
