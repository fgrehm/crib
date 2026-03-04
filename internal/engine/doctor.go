package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// DoctorIssue describes a single problem found by Doctor.
type DoctorIssue struct {
	Level       string // "error" or "warning"
	Check       string // short name of the check
	Description string // human-readable description
	Fix         string // description of the fix action
	WorkspaceID string // workspace ID if applicable
}

// DoctorResult holds the outcome of a Doctor check.
type DoctorResult struct {
	Issues    []DoctorIssue
	RuntimeOK bool
	ComposeOK bool
}

// Doctor runs health checks on the crib installation and workspaces.
// If fix is true, it attempts to automatically resolve found issues.
func (e *Engine) Doctor(ctx context.Context, fix bool) (*DoctorResult, error) {
	result := &DoctorResult{}

	// Check 1: Runtime availability.
	_, err := e.driver.TargetArchitecture(ctx)
	if err != nil {
		result.Issues = append(result.Issues, DoctorIssue{
			Level:       "error",
			Check:       "runtime",
			Description: fmt.Sprintf("Container runtime is not reachable: %v", err),
		})
	} else {
		result.RuntimeOK = true
	}

	// Check 2: Compose availability.
	if e.compose == nil {
		result.Issues = append(result.Issues, DoctorIssue{
			Level:       "warning",
			Check:       "compose",
			Description: "Docker Compose is not available. Compose-based devcontainers will not work.",
		})
	} else {
		result.ComposeOK = true
	}

	// Check 3: Orphaned workspaces (source directory deleted).
	ids, err := e.store.List()
	if err != nil {
		e.logger.Warn("failed to list workspaces for doctor check", "error", err)
	} else {
		for _, id := range ids {
			ws, err := e.store.Load(id)
			if err != nil {
				continue
			}
			if _, err := os.Stat(ws.Source); os.IsNotExist(err) {
				issue := DoctorIssue{
					Level:       "warning",
					Check:       "orphaned-workspace",
					Description: fmt.Sprintf("Workspace %q points to missing directory: %s", id, ws.Source),
					Fix:         "Remove workspace state",
					WorkspaceID: id,
				}
				if fix {
					if err := e.store.Delete(id); err != nil {
						e.logger.Warn("failed to delete orphaned workspace", "id", id, "error", err)
					} else {
						issue.Fix = "Removed workspace state"
					}
				}
				result.Issues = append(result.Issues, issue)
			}
		}
	}

	// Check 4: Dangling containers (crib label but no workspace state).
	if result.RuntimeOK {
		containers, err := e.driver.ListContainers(ctx)
		if err != nil {
			e.logger.Warn("failed to list containers for doctor check", "error", err)
		} else {
			for _, c := range containers {
				wsID := c.Config.Labels["crib.workspace"]
				if wsID == "" {
					continue
				}
				if !e.store.Exists(wsID) {
					issue := DoctorIssue{
						Level:       "warning",
						Check:       "dangling-container",
						Description: fmt.Sprintf("Container %.12s has crib label for workspace %q but no workspace state exists", c.ID, wsID),
						Fix:         "Remove container",
						WorkspaceID: wsID,
					}
					if fix {
						if err := e.driver.DeleteContainer(ctx, wsID, c.ID); err != nil {
							e.logger.Warn("failed to delete dangling container", "id", c.ID, "error", err)
						} else {
							issue.Fix = "Removed container"
						}
					}
					result.Issues = append(result.Issues, issue)
				}
			}
		}
	}

	// Check 5: Stale plugin data (workspace has no result but has plugin dirs).
	if ids == nil {
		ids, _ = e.store.List()
	}
	for _, id := range ids {
		storedResult, _ := e.store.LoadResult(id)
		if storedResult != nil {
			continue // has result, plugin data is live
		}
		pluginDir := filepath.Join(e.store.WorkspaceDir(id), "plugins")
		entries, err := os.ReadDir(pluginDir)
		if err != nil || len(entries) == 0 {
			continue
		}
		issue := DoctorIssue{
			Level:       "warning",
			Check:       "stale-plugins",
			Description: fmt.Sprintf("Workspace %q has no container but has stale plugin data", id),
			Fix:         "Remove plugin data",
			WorkspaceID: id,
		}
		if fix {
			if err := os.RemoveAll(pluginDir); err != nil {
				e.logger.Warn("failed to remove stale plugin data", "id", id, "error", err)
			} else {
				issue.Fix = "Removed plugin data"
			}
		}
		result.Issues = append(result.Issues, issue)
	}

	return result, nil
}
