package engine

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestPruneImages_StaleRemoved_ActiveKept(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	// Save workspace + result with active image and snapshot.
	ws := &workspace.Workspace{ID: "myws", Source: "/tmp/myws"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveResult("myws", &workspace.Result{
		ImageName:     "crib-myws:crib-abc1234",
		SnapshotImage: "crib-myws:snapshot",
	}); err != nil {
		t.Fatal(err)
	}

	md := &imageTrackingDriver{
		images: []driver.ImageInfo{
			{Reference: "crib-myws:crib-abc1234", ID: "sha256:active", Size: 100, WorkspaceID: "myws"},
			{Reference: "crib-myws:snapshot", ID: "sha256:snap", Size: 200, WorkspaceID: "myws"},
			{Reference: "crib-myws:crib-old1111", ID: "sha256:stale1", Size: 300, WorkspaceID: "myws"},
			{Reference: "crib-myws:crib-old2222", ID: "sha256:stale2", Size: 400, WorkspaceID: "myws"},
		},
	}
	eng := &Engine{driver: md, store: store, logger: slog.Default()}

	result, err := eng.PruneImages(context.Background(), PruneOptions{})
	if err != nil {
		t.Fatalf("PruneImages: %v", err)
	}

	if len(md.removedImages) != 2 {
		t.Fatalf("removed %d images, want 2: %v", len(md.removedImages), md.removedImages)
	}
	if len(result.Removed) != 2 {
		t.Errorf("result.Removed = %d, want 2", len(result.Removed))
	}

	// Verify active and snapshot were kept.
	for _, img := range md.removedImages {
		if img == "crib-myws:crib-abc1234" || img == "crib-myws:snapshot" {
			t.Errorf("should not have removed active/snapshot image %s", img)
		}
	}
}

func TestPruneImages_OrphanImagesRemoved(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	// No workspace saved for "gone-ws" -- it's orphaned.

	md := &imageTrackingDriver{
		images: []driver.ImageInfo{
			{Reference: "crib-gone-ws:crib-abc", ID: "sha256:orphan1", Size: 500, WorkspaceID: "gone-ws"},
			{Reference: "crib-gone-ws:snapshot", ID: "sha256:orphan2", Size: 600, WorkspaceID: "gone-ws"},
		},
	}
	eng := &Engine{driver: md, store: store, logger: slog.Default()}

	result, err := eng.PruneImages(context.Background(), PruneOptions{})
	if err != nil {
		t.Fatalf("PruneImages: %v", err)
	}

	if len(md.removedImages) != 2 {
		t.Fatalf("removed %d images, want 2", len(md.removedImages))
	}
	// All orphan images should be marked as orphans.
	for _, img := range result.Removed {
		if !img.Orphan {
			t.Errorf("image %s should be marked orphan", img.Reference)
		}
	}
}

func TestPruneImages_WorkspaceFilter(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	// Two workspaces, both with stale images.
	for _, id := range []string{"ws1", "ws2"} {
		if err := store.Save(&workspace.Workspace{ID: id, Source: "/tmp/" + id}); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveResult(id, &workspace.Result{ImageName: "crib-" + id + ":crib-active"}); err != nil {
			t.Fatal(err)
		}
	}

	md := &imageTrackingDriver{
		images: []driver.ImageInfo{
			{Reference: "crib-ws1:crib-active", ID: "sha256:a1", Size: 100, WorkspaceID: "ws1"},
			{Reference: "crib-ws1:crib-stale", ID: "sha256:s1", Size: 200, WorkspaceID: "ws1"},
			{Reference: "crib-ws2:crib-active", ID: "sha256:a2", Size: 100, WorkspaceID: "ws2"},
			{Reference: "crib-ws2:crib-stale", ID: "sha256:s2", Size: 200, WorkspaceID: "ws2"},
		},
	}
	eng := &Engine{driver: md, store: store, logger: slog.Default()}

	result, err := eng.PruneImages(context.Background(), PruneOptions{WorkspaceID: "ws1"})
	if err != nil {
		t.Fatalf("PruneImages: %v", err)
	}

	// Should only remove ws1's stale image, not ws2's.
	if len(md.removedImages) != 1 || md.removedImages[0] != "crib-ws1:crib-stale" {
		t.Errorf("removedImages = %v, want [crib-ws1:crib-stale]", md.removedImages)
	}
	if len(result.Removed) != 1 {
		t.Errorf("result.Removed = %d, want 1", len(result.Removed))
	}
}

func TestPruneImages_DryRun(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	if err := store.Save(&workspace.Workspace{ID: "myws", Source: "/tmp/myws"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveResult("myws", &workspace.Result{ImageName: "crib-myws:crib-active"}); err != nil {
		t.Fatal(err)
	}

	md := &imageTrackingDriver{
		images: []driver.ImageInfo{
			{Reference: "crib-myws:crib-active", ID: "sha256:a", Size: 100, WorkspaceID: "myws"},
			{Reference: "crib-myws:crib-stale", ID: "sha256:s", Size: 200, WorkspaceID: "myws"},
		},
	}
	eng := &Engine{driver: md, store: store, logger: slog.Default()}

	result, err := eng.PruneImages(context.Background(), PruneOptions{DryRun: true})
	if err != nil {
		t.Fatalf("PruneImages: %v", err)
	}

	// Dry run: no actual removal but result lists what would be removed.
	if len(md.removedImages) != 0 {
		t.Errorf("removedImages = %v, want none (dry run)", md.removedImages)
	}
	if len(result.Removed) != 1 {
		t.Errorf("result.Removed = %d, want 1 (stale image listed)", len(result.Removed))
	}
}

func TestPruneImages_RemoveFailure_Continues(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	if err := store.Save(&workspace.Workspace{ID: "myws", Source: "/tmp/myws"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveResult("myws", &workspace.Result{ImageName: "crib-myws:crib-active"}); err != nil {
		t.Fatal(err)
	}

	md := &imageTrackingDriver{
		images: []driver.ImageInfo{
			{Reference: "crib-myws:crib-active", ID: "sha256:a", Size: 100, WorkspaceID: "myws"},
			{Reference: "crib-myws:crib-stale1", ID: "sha256:s1", Size: 200, WorkspaceID: "myws"},
			{Reference: "crib-myws:crib-stale2", ID: "sha256:s2", Size: 300, WorkspaceID: "myws"},
		},
		removeErrs: map[string]error{
			"crib-myws:crib-stale1": fmt.Errorf("image in use"),
		},
	}
	eng := &Engine{driver: md, store: store, logger: slog.Default()}

	result, err := eng.PruneImages(context.Background(), PruneOptions{})
	if err != nil {
		t.Fatalf("PruneImages: %v", err)
	}

	// Both stale images attempted, one failed.
	if len(md.removedImages) != 2 {
		t.Errorf("removedImages = %v, want 2 attempts", md.removedImages)
	}
	// Result should report the one that succeeded.
	if len(result.Removed) != 1 {
		t.Errorf("result.Removed = %d, want 1 (only successful removal)", len(result.Removed))
	}
	if len(result.Errors) != 1 {
		t.Errorf("result.Errors = %d, want 1", len(result.Errors))
	}
}
