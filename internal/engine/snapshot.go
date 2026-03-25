package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/fgrehm/crib/internal/config"
	ocidriver "github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/workspace"
)

// snapshotImageName returns the image name for a workspace snapshot.
func snapshotImageName(workspaceID string) string {
	return "crib-" + workspaceID + ":snapshot"
}

// computeHookHash computes a stable hash of the create-time hook definitions
// (onCreate, updateContent, postCreate) including any feature hooks from the
// stored result. If hooks change, the snapshot is stale.
func computeHookHash(cfg *config.DevContainerConfig, stored *workspace.Result) string {
	data := struct {
		OnCreate        config.LifecycleHook      `json:"onCreate,omitempty"`
		UpdateContent   config.LifecycleHook      `json:"updateContent,omitempty"`
		PostCreate      config.LifecycleHook      `json:"postCreate,omitempty"`
		FeatureOnCreate []workspace.LifecycleHook `json:"featureOnCreate,omitempty"`
		FeatureUpdate   []workspace.LifecycleHook `json:"featureUpdate,omitempty"`
		FeaturePost     []workspace.LifecycleHook `json:"featurePost,omitempty"`
	}{
		OnCreate:      cfg.OnCreateCommand,
		UpdateContent: cfg.UpdateContentCommand,
		PostCreate:    cfg.PostCreateCommand,
	}
	if stored != nil {
		data.FeatureOnCreate = stored.FeatureOnCreateCommands
		data.FeatureUpdate = stored.FeatureUpdateContentCommands
		data.FeaturePost = stored.FeaturePostCreateCommands
	}

	b, _ := json.Marshal(data)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:8]) // 16 hex chars is enough
}

// hasCreateTimeHooks returns true if the config has any create-time hooks.
func hasCreateTimeHooks(cfg *config.DevContainerConfig) bool {
	return len(cfg.OnCreateCommand) > 0 ||
		len(cfg.UpdateContentCommand) > 0 ||
		len(cfg.PostCreateCommand) > 0
}

// commitSnapshot creates a snapshot image from the container and saves
// the metadata in the workspace result.
func (e *Engine) commitSnapshot(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, containerID string) {
	if !hasCreateTimeHooks(cfg) {
		return
	}

	imageName := snapshotImageName(ws.ID)
	e.logger.Debug("committing snapshot", "image", imageName, "container", containerID)

	changes := []string{
		fmt.Sprintf("LABEL %s=%s", ocidriver.LabelWorkspace, ws.ID),
	}
	if err := e.driver.CommitContainer(ctx, ws.ID, containerID, imageName, changes); err != nil {
		e.logger.Warn("failed to commit snapshot", "error", err)
		return
	}

	// Update result with snapshot metadata.
	result, err := e.store.LoadResult(ws.ID)
	if err != nil || result == nil {
		e.logger.Warn("failed to load result for snapshot metadata", "error", err)
		return
	}

	result.SnapshotImage = imageName
	result.SnapshotHookHash = computeHookHash(cfg, result)
	if err := e.store.SaveResult(ws.ID, result); err != nil {
		e.logger.Warn("failed to save snapshot metadata", "error", err)
	}
}

// ClearSnapshot removes the snapshot image and clears the metadata.
// Exported so the cmd layer can call it before rebuild.
func (e *Engine) ClearSnapshot(ctx context.Context, ws *workspace.Workspace) {
	e.clearSnapshot(ctx, ws)
}

// clearSnapshot removes the snapshot image and clears the metadata.
func (e *Engine) clearSnapshot(ctx context.Context, ws *workspace.Workspace) {
	result, err := e.store.LoadResult(ws.ID)
	if err != nil || result == nil {
		return
	}

	if result.SnapshotImage != "" {
		e.logger.Debug("removing snapshot image", "image", result.SnapshotImage)
		if err := e.driver.RemoveImage(ctx, result.SnapshotImage); err != nil {
			e.logger.Debug("failed to remove snapshot image (may not exist)", "error", err)
		}
		result.SnapshotImage = ""
		result.SnapshotHookHash = ""
		_ = e.store.SaveResult(ws.ID, result)
	}
}

// validSnapshot checks if a valid snapshot exists for the workspace.
// Returns the image name if valid, empty string if stale or missing.
func (e *Engine) validSnapshot(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig) (string, bool) {
	result, err := e.store.LoadResult(ws.ID)
	if err != nil || result == nil || result.SnapshotImage == "" {
		return "", false
	}

	// Check if hooks changed since snapshot was taken.
	currentHash := computeHookHash(cfg, result)
	if currentHash != result.SnapshotHookHash {
		e.logger.Debug("snapshot is stale (hook hash mismatch)", "stored", result.SnapshotHookHash, "current", currentHash)
		return "", false
	}

	// Verify the image still exists.
	if _, err := e.driver.InspectImage(ctx, result.SnapshotImage); err != nil {
		e.logger.Debug("snapshot image not found", "image", result.SnapshotImage, "error", err)
		return "", false
	}

	return result.SnapshotImage, true
}
