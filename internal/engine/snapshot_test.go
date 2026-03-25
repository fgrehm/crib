package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestComputeHookHash_Stable(t *testing.T) {
	cfg := &config.DevContainerConfig{}
	cfg.OnCreateCommand = config.LifecycleHook{"": {"npm install"}}
	cfg.PostCreateCommand = config.LifecycleHook{"": {"echo done"}}

	hash1 := computeHookHash(cfg, nil)
	hash2 := computeHookHash(cfg, nil)

	if hash1 != hash2 {
		t.Errorf("hash not stable: %q != %q", hash1, hash2)
	}
	if hash1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestComputeHookHash_ChangesWhenHooksChange(t *testing.T) {
	cfg1 := &config.DevContainerConfig{}
	cfg1.OnCreateCommand = config.LifecycleHook{"": {"npm install"}}

	cfg2 := &config.DevContainerConfig{}
	cfg2.OnCreateCommand = config.LifecycleHook{"": {"yarn install"}}

	if computeHookHash(cfg1, nil) == computeHookHash(cfg2, nil) {
		t.Error("different hooks should produce different hashes")
	}
}

func TestComputeHookHash_ChangesWithFeatureHooks(t *testing.T) {
	cfg := &config.DevContainerConfig{}
	cfg.OnCreateCommand = config.LifecycleHook{"": {"echo user"}}

	hashNoFeatures := computeHookHash(cfg, nil)

	stored := &workspace.Result{
		FeatureOnCreateCommands: []workspace.LifecycleHook{
			{"": {"echo feature-oncreate"}},
		},
	}
	hashWithFeatures := computeHookHash(cfg, stored)

	if hashNoFeatures == hashWithFeatures {
		t.Error("hash should differ when feature hooks are added")
	}
}

func TestComputeHookHash_EmptyHooks(t *testing.T) {
	cfg := &config.DevContainerConfig{}
	hash := computeHookHash(cfg, nil)
	if hash == "" {
		t.Error("hash should not be empty even for empty hooks")
	}
}

func TestHasCreateTimeHooks(t *testing.T) {
	t.Run("no hooks", func(t *testing.T) {
		cfg := &config.DevContainerConfig{}
		if hasCreateTimeHooks(cfg, nil) {
			t.Error("expected false for no hooks")
		}
	})

	t.Run("onCreate", func(t *testing.T) {
		cfg := &config.DevContainerConfig{}
		cfg.OnCreateCommand = config.LifecycleHook{"": {"echo hi"}}
		if !hasCreateTimeHooks(cfg, nil) {
			t.Error("expected true for onCreate")
		}
	})

	t.Run("postCreate", func(t *testing.T) {
		cfg := &config.DevContainerConfig{}
		cfg.PostCreateCommand = config.LifecycleHook{"": {"echo hi"}}
		if !hasCreateTimeHooks(cfg, nil) {
			t.Error("expected true for postCreate")
		}
	})

	t.Run("postStart only", func(t *testing.T) {
		cfg := &config.DevContainerConfig{}
		cfg.PostStartCommand = config.LifecycleHook{"": {"echo hi"}}
		if hasCreateTimeHooks(cfg, nil) {
			t.Error("expected false for postStart only")
		}
	})
}

func TestSnapshotImageName(t *testing.T) {
	if got := snapshotImageName("ws-abc"); got != "crib-ws-abc:snapshot" {
		t.Errorf("snapshotImageName = %q, want crib-ws-abc:snapshot", got)
	}
}

// snapshotMockDriver tracks commit and remove calls.
type snapshotMockDriver struct {
	mockDriver
	committed   map[string]string // containerID -> imageName
	removed     []string          // image names removed
	imageExists map[string]bool   // images that exist
	findResult  *driver.ContainerDetails
	lastChanges []string // changes passed to last CommitContainer call
}

func newSnapshotMockDriver() *snapshotMockDriver {
	return &snapshotMockDriver{
		committed:   make(map[string]string),
		imageExists: make(map[string]bool),
	}
}

func (m *snapshotMockDriver) CommitContainer(_ context.Context, _, containerID, imageName string, changes []string) error {
	m.committed[containerID] = imageName
	m.imageExists[imageName] = true
	m.lastChanges = changes
	return nil
}

func (m *snapshotMockDriver) RemoveImage(_ context.Context, imageName string) error {
	m.removed = append(m.removed, imageName)
	delete(m.imageExists, imageName)
	return nil
}

func (m *snapshotMockDriver) InspectImage(_ context.Context, imageName string) (*driver.ImageDetails, error) {
	if m.imageExists[imageName] {
		return &driver.ImageDetails{}, nil
	}
	return nil, fmt.Errorf("image %s not found", imageName)
}

func (m *snapshotMockDriver) FindContainer(_ context.Context, _ string) (*driver.ContainerDetails, error) {
	return m.findResult, nil
}

func TestCommitSnapshot_SavesMetadata(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-snap", Source: "/tmp/test"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	cfg.OnCreateCommand = config.LifecycleHook{"": {"npm install"}}

	mergedJSON, _ := json.Marshal(cfg)
	if err := store.SaveResult(ws.ID, &workspace.Result{
		ContainerID:  "container-1",
		MergedConfig: mergedJSON,
	}); err != nil {
		t.Fatal(err)
	}

	mockDrv := newSnapshotMockDriver()
	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	eng.commitSnapshot(context.Background(), ws, cfg, "container-1")

	// Verify commit was called.
	if imageName, ok := mockDrv.committed["container-1"]; !ok {
		t.Error("CommitContainer not called")
	} else if imageName != "crib-ws-snap:snapshot" {
		t.Errorf("image name = %q, want crib-ws-snap:snapshot", imageName)
	}

	// Verify result was updated.
	result, _ := store.LoadResult(ws.ID)
	if result.SnapshotImage != "crib-ws-snap:snapshot" {
		t.Errorf("SnapshotImage = %q, want crib-ws-snap:snapshot", result.SnapshotImage)
	}
	if result.SnapshotHookHash == "" {
		t.Error("SnapshotHookHash should not be empty")
	}
}

func TestCommitSnapshot_IncludesWorkspaceLabel(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-label", Source: "/tmp/test"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	cfg.OnCreateCommand = config.LifecycleHook{"": {"echo hi"}}

	mergedJSON, _ := json.Marshal(cfg)
	if err := store.SaveResult(ws.ID, &workspace.Result{
		ContainerID:  "container-1",
		MergedConfig: mergedJSON,
	}); err != nil {
		t.Fatal(err)
	}

	mockDrv := newSnapshotMockDriver()
	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	eng.commitSnapshot(context.Background(), ws, cfg, "container-1")

	// Verify --change label was passed.
	if len(mockDrv.lastChanges) != 1 {
		t.Fatalf("expected 1 change, got %d", len(mockDrv.lastChanges))
	}
	want := "LABEL crib.workspace=ws-label"
	if mockDrv.lastChanges[0] != want {
		t.Errorf("change = %q, want %q", mockDrv.lastChanges[0], want)
	}
}

func TestCommitSnapshot_SkipsNoHooks(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-nohook", Source: "/tmp/test"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	// No create-time hooks.

	mockDrv := newSnapshotMockDriver()
	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
	}

	eng.commitSnapshot(context.Background(), ws, cfg, "container-1")

	if len(mockDrv.committed) != 0 {
		t.Error("CommitContainer should not be called when no create-time hooks exist")
	}
}

func TestValidSnapshot_Valid(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-valid", Source: "/tmp/test"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	cfg := &config.DevContainerConfig{}
	cfg.OnCreateCommand = config.LifecycleHook{"": {"npm install"}}

	hash := computeHookHash(cfg, nil)
	mergedJSON, _ := json.Marshal(cfg)
	if err := store.SaveResult(ws.ID, &workspace.Result{
		ContainerID:      "container-1",
		MergedConfig:     mergedJSON,
		SnapshotImage:    "crib-ws-valid:snapshot",
		SnapshotHookHash: hash,
	}); err != nil {
		t.Fatal(err)
	}

	mockDrv := newSnapshotMockDriver()
	mockDrv.imageExists["crib-ws-valid:snapshot"] = true

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
	}

	imageName, ok := eng.validSnapshot(context.Background(), ws, cfg)
	if !ok {
		t.Error("expected valid snapshot")
	}
	if imageName != "crib-ws-valid:snapshot" {
		t.Errorf("imageName = %q, want crib-ws-valid:snapshot", imageName)
	}
}

func TestValidSnapshot_StaleHash(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-stale-hash", Source: "/tmp/test"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	oldCfg := &config.DevContainerConfig{}
	oldCfg.OnCreateCommand = config.LifecycleHook{"": {"npm install"}}

	mergedJSON, _ := json.Marshal(oldCfg)
	if err := store.SaveResult(ws.ID, &workspace.Result{
		ContainerID:      "container-1",
		MergedConfig:     mergedJSON,
		SnapshotImage:    "crib-ws-stale-hash:snapshot",
		SnapshotHookHash: computeHookHash(oldCfg, nil),
	}); err != nil {
		t.Fatal(err)
	}

	// Change the hooks.
	newCfg := &config.DevContainerConfig{}
	newCfg.OnCreateCommand = config.LifecycleHook{"": {"yarn install"}}

	mockDrv := newSnapshotMockDriver()
	mockDrv.imageExists["crib-ws-stale-hash:snapshot"] = true

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
	}

	_, ok := eng.validSnapshot(context.Background(), ws, newCfg)
	if ok {
		t.Error("expected stale snapshot (hash mismatch)")
	}
}

func TestValidSnapshot_MissingImage(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-missing-img", Source: "/tmp/test"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	cfg := &config.DevContainerConfig{}
	cfg.OnCreateCommand = config.LifecycleHook{"": {"npm install"}}

	mergedJSON, _ := json.Marshal(cfg)
	if err := store.SaveResult(ws.ID, &workspace.Result{
		ContainerID:      "container-1",
		MergedConfig:     mergedJSON,
		SnapshotImage:    "crib-ws-missing-img:snapshot",
		SnapshotHookHash: computeHookHash(cfg, nil),
	}); err != nil {
		t.Fatal(err)
	}

	mockDrv := newSnapshotMockDriver()
	// image does not exist

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
	}

	_, ok := eng.validSnapshot(context.Background(), ws, cfg)
	if ok {
		t.Error("expected invalid snapshot (image missing)")
	}
}

func TestClearSnapshot_RemovesImageAndMetadata(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-clear", Source: "/tmp/test"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	cfg := &config.DevContainerConfig{}
	mergedJSON, _ := json.Marshal(cfg)
	if err := store.SaveResult(ws.ID, &workspace.Result{
		ContainerID:      "container-1",
		MergedConfig:     mergedJSON,
		SnapshotImage:    "crib-ws-clear:snapshot",
		SnapshotHookHash: "abc123",
	}); err != nil {
		t.Fatal(err)
	}

	mockDrv := newSnapshotMockDriver()
	mockDrv.imageExists["crib-ws-clear:snapshot"] = true

	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
	}

	eng.clearSnapshot(context.Background(), ws)

	// Verify image was removed.
	if len(mockDrv.removed) != 1 || mockDrv.removed[0] != "crib-ws-clear:snapshot" {
		t.Errorf("RemoveImage calls = %v, want [crib-ws-clear:snapshot]", mockDrv.removed)
	}

	// Verify metadata was cleared.
	result, _ := store.LoadResult(ws.ID)
	if result.SnapshotImage != "" {
		t.Errorf("SnapshotImage = %q, want empty", result.SnapshotImage)
	}
	if result.SnapshotHookHash != "" {
		t.Errorf("SnapshotHookHash = %q, want empty", result.SnapshotHookHash)
	}
}
