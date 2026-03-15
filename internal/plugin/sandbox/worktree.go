package sandbox

import (
	"path/filepath"
	"strings"
)

// parseWorktreePaths extracts worktree paths from `git worktree list --porcelain`
// output. Each block starts with "worktree <path>".
func parseWorktreePaths(porcelain string) []string {
	var paths []string
	for line := range strings.SplitSeq(porcelain, "\n") {
		if after, ok := strings.CutPrefix(line, "worktree "); ok {
			p := strings.TrimSpace(after)
			if p != "" {
				paths = append(paths, p)
			}
		}
	}
	return paths
}

// worktreeBaseDirs computes the unique parent directories of worktrees that
// live outside the workspace folder. These directories need write access so
// the sandboxed agent can operate on worktree checkouts.
//
// When one parent is a prefix of another, only the shorter (broader) path is
// kept to avoid redundant bwrap --bind-try entries.
func worktreeBaseDirs(worktreePaths []string, workspaceFolder string) []string {
	wsClean := filepath.Clean(workspaceFolder)

	// Collect parent dirs of worktrees that are outside the workspace folder.
	parentSet := make(map[string]struct{})
	for _, wt := range worktreePaths {
		wt = filepath.Clean(wt)
		if wt == wsClean {
			continue // main worktree, already writable
		}
		if strings.HasPrefix(wt, wsClean+"/") {
			continue // inside workspace folder, already writable
		}
		parentSet[filepath.Dir(wt)] = struct{}{}
	}

	if len(parentSet) == 0 {
		return nil
	}

	// Collect unique parents, removing any that are subdirectories of another.
	parents := make([]string, 0, len(parentSet))
	for p := range parentSet {
		parents = append(parents, p)
	}

	var result []string
	for _, p := range parents {
		covered := false
		for _, other := range parents {
			if other != p && strings.HasPrefix(p, other+"/") {
				covered = true
				break
			}
		}
		if !covered {
			result = append(result, p)
		}
	}

	return result
}
