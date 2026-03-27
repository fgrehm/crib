package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewStore_ExplicitHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CRIB_HOME", tmp)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if !store.IsExplicitHome() {
		t.Error("IsExplicitHome should be true when CRIB_HOME is set")
	}
}

func TestNewStore_DefaultHome(t *testing.T) {
	t.Setenv("CRIB_HOME", "")
	t.Setenv("HOME", t.TempDir())

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if store.IsExplicitHome() {
		t.Error("IsExplicitHome should be false when CRIB_HOME is not set")
	}
}

func TestNewStoreAt_NotExplicitHome(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	if store.IsExplicitHome() {
		t.Error("NewStoreAt should not be explicit home")
	}
}

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
	if !errors.Is(err, ErrWorkspaceNotFound) {
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

func TestStore_SaveAndLoadResult_FeatureHooks(t *testing.T) {
	store := NewStoreAt(t.TempDir())

	result := &Result{
		ContainerID:     "abc123",
		ImageName:       "crib-ws:latest",
		MergedConfig:    json.RawMessage(`{}`),
		WorkspaceFolder: "/workspace",
		FeatureOnCreateCommands: []LifecycleHook{
			{"": {"echo feature1-oncreate"}},
			{"": {"echo feature2-oncreate"}},
		},
		FeaturePostStartCommands: []LifecycleHook{
			{"setup": {"echo feature-poststart"}},
		},
	}

	if err := store.SaveResult("ws-hooks", result); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	loaded, err := store.LoadResult("ws-hooks")
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}

	if len(loaded.FeatureOnCreateCommands) != 2 {
		t.Fatalf("FeatureOnCreateCommands length = %d, want 2", len(loaded.FeatureOnCreateCommands))
	}
	if loaded.FeatureOnCreateCommands[0][""][0] != "echo feature1-oncreate" {
		t.Errorf("FeatureOnCreateCommands[0] = %v, want echo feature1-oncreate", loaded.FeatureOnCreateCommands[0])
	}
	if len(loaded.FeaturePostStartCommands) != 1 {
		t.Fatalf("FeaturePostStartCommands length = %d, want 1", len(loaded.FeaturePostStartCommands))
	}
	// Empty fields should not appear.
	if len(loaded.FeatureUpdateContentCommands) != 0 {
		t.Errorf("FeatureUpdateContentCommands should be empty, got %v", loaded.FeatureUpdateContentCommands)
	}
	if len(loaded.FeaturePostCreateCommands) != 0 {
		t.Errorf("FeaturePostCreateCommands should be empty, got %v", loaded.FeaturePostCreateCommands)
	}
	if len(loaded.FeaturePostAttachCommands) != 0 {
		t.Errorf("FeaturePostAttachCommands should be empty, got %v", loaded.FeaturePostAttachCommands)
	}
}

func TestStore_Lock(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	ctx := context.Background()

	// Lock should succeed and create the workspace directory.
	lock, err := store.Lock(ctx, "locktest")
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}

	// A second Lock with an already-cancelled context should fail immediately
	// because the lock is held.
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.Lock(cancelledCtx, "locktest")
	if err == nil {
		t.Error("Lock should fail when workspace is already locked and context is cancelled")
	}

	// Unlock, then Lock should succeed.
	if err := lock.Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	lock2, err := store.Lock(ctx, "locktest")
	if err != nil {
		t.Fatalf("Lock after unlock: %v", err)
	}
	lock2.Unlock()
}

func TestStore_Lock_DeleteCleansUp(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	ctx := context.Background()

	// Create a workspace and lock it.
	if err := store.Save(&Workspace{ID: "cleanup", Source: "/tmp"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	lock, err := store.Lock(ctx, "cleanup")
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	lock.Unlock()

	// Delete removes the entire workspace dir including the lock file.
	if err := store.Delete("cleanup"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.Exists("cleanup") {
		t.Error("workspace should not exist after delete")
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
