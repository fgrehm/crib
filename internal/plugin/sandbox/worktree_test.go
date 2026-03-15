package sandbox

import (
	"testing"
)

func TestParseWorktreePaths_MultipleWorktrees(t *testing.T) {
	porcelain := `worktree /workspaces/web
HEAD b9d421b1d
branch refs/heads/main

worktree /workspaces/web-worktrees/fr-pdf-kickoff
HEAD aa308ae94
branch refs/heads/fr-pdf-kickoff

worktree /workspaces/web-worktrees/jt-3/9-bootstrap-optimizations
HEAD d17ce1e0c
branch refs/heads/jt-3/9-bootstrap-optimizations

`
	paths := parseWorktreePaths(porcelain)
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %d: %v", len(paths), paths)
	}
	if paths[0] != "/workspaces/web" {
		t.Errorf("expected /workspaces/web, got %s", paths[0])
	}
	if paths[1] != "/workspaces/web-worktrees/fr-pdf-kickoff" {
		t.Errorf("expected fr-pdf-kickoff worktree, got %s", paths[1])
	}
	if paths[2] != "/workspaces/web-worktrees/jt-3/9-bootstrap-optimizations" {
		t.Errorf("expected nested worktree, got %s", paths[2])
	}
}

func TestParseWorktreePaths_SingleWorktree(t *testing.T) {
	porcelain := `worktree /workspaces/project
HEAD abc123
branch refs/heads/main

`
	paths := parseWorktreePaths(porcelain)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d: %v", len(paths), paths)
	}
}

func TestParseWorktreePaths_Empty(t *testing.T) {
	paths := parseWorktreePaths("")
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
	}
}

func TestWorktreeBaseDirs_SiblingWorktrees(t *testing.T) {
	paths := []string{
		"/workspaces/web",
		"/workspaces/web-worktrees/fr-pdf-kickoff",
		"/workspaces/web-worktrees/visual-regression-pt-2",
		"/workspaces/web-worktrees/jt-3/9-bootstrap-optimizations",
	}
	dirs := worktreeBaseDirs(paths, "/workspaces/web")
	if len(dirs) != 1 {
		t.Fatalf("expected 1 base dir, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != "/workspaces/web-worktrees" {
		t.Errorf("expected /workspaces/web-worktrees, got %s", dirs[0])
	}
}

func TestWorktreeBaseDirs_NoExternalWorktrees(t *testing.T) {
	paths := []string{"/workspaces/project"}
	dirs := worktreeBaseDirs(paths, "/workspaces/project")
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs for single worktree, got %d: %v", len(dirs), dirs)
	}
}

func TestWorktreeBaseDirs_WorktreeInsideWorkspace(t *testing.T) {
	paths := []string{
		"/workspaces/project",
		"/workspaces/project/.worktrees/branch-a",
	}
	dirs := worktreeBaseDirs(paths, "/workspaces/project")
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs for worktree inside workspace, got %d: %v", len(dirs), dirs)
	}
}

func TestWorktreeBaseDirs_ScatteredWorktrees(t *testing.T) {
	paths := []string{
		"/workspaces/web",
		"/workspaces/web-worktrees/branch-a",
		"/other/place/branch-b",
	}
	dirs := worktreeBaseDirs(paths, "/workspaces/web")
	if len(dirs) != 2 {
		t.Fatalf("expected 2 base dirs, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != "/other/place" {
		t.Errorf("expected /other/place, got %s", dirs[0])
	}
	if dirs[1] != "/workspaces/web-worktrees" {
		t.Errorf("expected /workspaces/web-worktrees, got %s", dirs[1])
	}
}

func TestWorktreeBaseDirs_NestedParentsDeduped(t *testing.T) {
	// All worktrees under the same base, some nested deeper.
	paths := []string{
		"/workspaces/project",
		"/workspaces/worktrees/branch-a",
		"/workspaces/worktrees/sub/branch-b",
	}
	dirs := worktreeBaseDirs(paths, "/workspaces/project")
	if len(dirs) != 1 {
		t.Fatalf("expected 1 base dir after dedup, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != "/workspaces/worktrees" {
		t.Errorf("expected /workspaces/worktrees, got %s", dirs[0])
	}
}

func TestWorktreeBaseDirs_Empty(t *testing.T) {
	dirs := worktreeBaseDirs(nil, "/workspaces/project")
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs, got %d", len(dirs))
	}
}
