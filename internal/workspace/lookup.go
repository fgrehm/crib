package workspace

import (
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// LookupOptions controls how Lookup resolves and (optionally) creates a workspace.
type LookupOptions struct {
	ConfigDir string // explicit devcontainer config directory (from --config or .cribrc)
	Dir       string // explicit project directory (from --dir)
	Cwd       string // working directory fallback when ConfigDir and Dir are both empty
	Version   string // crib binary version recorded in the workspace CribVersion field
	Create    bool   // create the workspace if it does not exist in the store
}

// Lookup resolves a workspace from the given options. It checks ConfigDir, Dir,
// and Cwd (in that order) to locate the devcontainer config, then loads the
// workspace from the store. If the workspace does not exist and Create is true,
// it creates a new one. If Create is false, it returns ErrWorkspaceNotFound.
func Lookup(store *Store, opts LookupOptions, logger *slog.Logger) (*Workspace, error) {
	var (
		rr  *ResolveResult
		err error
	)

	switch {
	case opts.ConfigDir != "":
		rr, err = ResolveConfigDir(opts.ConfigDir)
	case opts.Dir != "":
		rr, err = Resolve(opts.Dir)
	default:
		rr, err = Resolve(opts.Cwd)
	}
	if err != nil {
		return nil, err
	}

	ws, err := store.Load(rr.WorkspaceID)
	if err != nil && !errors.Is(err, ErrWorkspaceNotFound) {
		return nil, err
	}

	if ws == nil {
		if !opts.Create {
			return nil, fmt.Errorf("no workspace for this directory (run 'crib up' first): %w", ErrWorkspaceNotFound)
		}
		now := time.Now()
		ws = &Workspace{
			ID:               rr.WorkspaceID,
			Source:           rr.ProjectRoot,
			DevContainerPath: rr.RelativeConfigPath,
			CribVersion:      opts.Version,
			CreatedAt:        now,
			LastUsedAt:       now,
		}
		if err := store.Save(ws); err != nil {
			return nil, fmt.Errorf("saving workspace: %w", err)
		}
	} else {
		// Refresh fields that may have drifted from stored state.
		var changed bool
		if ws.DevContainerPath != rr.RelativeConfigPath {
			logger.Debug("devcontainer config path changed",
				"old", ws.DevContainerPath, "new", rr.RelativeConfigPath)
			ws.DevContainerPath = rr.RelativeConfigPath
			changed = true
		}
		if ws.CribVersion != opts.Version {
			ws.CribVersion = opts.Version
			changed = true
		}
		if changed {
			ws.LastUsedAt = time.Now()
			if err := store.Save(ws); err != nil {
				logger.Warn("failed to save refreshed workspace", "error", err)
			}
		}
	}

	return ws, nil
}
