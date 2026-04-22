package engine

import (
	"context"

	"github.com/fgrehm/crib/internal/compose"
	ocidriver "github.com/fgrehm/crib/internal/driver/oci"
)

// PruneOptions controls which images PruneImages removes.
type PruneOptions struct {
	WorkspaceID string // empty = all workspaces
	DryRun      bool
}

// PrunedImage describes an image that was (or would be) removed.
type PrunedImage struct {
	Reference   string
	ID          string
	Size        int64
	WorkspaceID string
	Orphan      bool
}

// PruneError records a failed image removal.
type PruneError struct {
	Reference string
	Err       error
}

// PruneResult holds the outcome of a prune operation.
type PruneResult struct {
	Removed []PrunedImage
	Errors  []PruneError
}

// PruneImages removes stale and orphan crib-managed images.
func (e *Engine) PruneImages(ctx context.Context, opts PruneOptions) (*PruneResult, error) {
	label := ocidriver.LabelWorkspace
	if opts.WorkspaceID != "" {
		label = ocidriver.WorkspaceLabel(opts.WorkspaceID)
	}

	images, err := e.driver.ListImages(ctx, label)
	if err != nil {
		return nil, err
	}

	// Also scan compose-built images for the targeted workspace. These carry
	// com.docker.compose.project=<project> but not crib.workspace, so the
	// crib.workspace filter above misses them. For global prune we skip this
	// scan to avoid incorrectly targeting unrelated compose projects.
	if opts.WorkspaceID != "" {
		projectName := ""
		if r, err := e.store.LoadResult(opts.WorkspaceID); err == nil && r != nil {
			projectName = r.ComposeProjectName
		}
		if projectName == "" {
			projectName = compose.ProjectName(opts.WorkspaceID)
		}
		projectLabel := "com.docker.compose.project=" + projectName
		if composeImages, err := e.driver.ListImages(ctx, projectLabel); err == nil {
			for _, img := range composeImages {
				img.WorkspaceID = opts.WorkspaceID
				images = append(images, img)
			}
		}
	}

	// Build a set of active images per workspace.
	type activeSet struct {
		image    string
		snapshot string
		exists   bool
	}
	active := make(map[string]*activeSet)

	for _, img := range images {
		wsID := img.WorkspaceID
		if _, ok := active[wsID]; ok {
			continue
		}
		a := &activeSet{exists: e.store.Exists(wsID)}
		if a.exists {
			if r, err := e.store.LoadResult(wsID); err == nil && r != nil {
				a.image = r.ImageName
				a.snapshot = r.SnapshotImage
			}
		}
		active[wsID] = a
	}

	result := &PruneResult{}
	for _, img := range images {
		a := active[img.WorkspaceID]

		// Keep active images for existing workspaces.
		if a.exists && (img.Reference == a.image || img.Reference == a.snapshot) {
			continue
		}

		pruned := PrunedImage{
			Reference:   img.Reference,
			ID:          img.ID,
			Size:        img.Size,
			WorkspaceID: img.WorkspaceID,
			Orphan:      !a.exists,
		}

		if opts.DryRun {
			result.Removed = append(result.Removed, pruned)
			continue
		}

		if err := e.driver.RemoveImage(ctx, img.Reference); err != nil {
			e.logger.Debug("failed to remove image during prune", "image", img.Reference, "error", err)
			result.Errors = append(result.Errors, PruneError{Reference: img.Reference, Err: err})
			continue
		}
		result.Removed = append(result.Removed, pruned)
	}

	return result, nil
}
