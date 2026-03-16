package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/fgrehm/crib/internal/plugin"
)

// PostContainerCreate installs bubblewrap and generates wrapper scripts
// inside the container. No-op when sandbox config is absent.
func (p *Plugin) PostContainerCreate(ctx context.Context, req *plugin.PostContainerCreateRequest) error {
	cfg := parseConfig(req.Customizations)
	if cfg == nil {
		return nil
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)
	localBin := remoteHome + "/.local/bin"
	owner := plugin.InferOwner(req.RemoteUser)

	// 1. Install bubblewrap (required for filesystem sandboxing).
	bwrapInstall := "command -v bwrap >/dev/null 2>&1 || { " +
		"command -v apt-get >/dev/null 2>&1 && " +
		"apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq bubblewrap >/dev/null 2>&1 || " +
		"{ echo 'crib sandbox: bubblewrap not found and apt-get not available; install bubblewrap manually' >&2; exit 1; }; }"
	if err := req.ExecFunc(ctx, []string{"sh", "-c", bwrapInstall}, "root"); err != nil {
		return fmt.Errorf("installing bubblewrap (image may need it pre-installed): %w", err)
	}

	// 2. Install iptables and apply network restrictions (non-fatal).
	// iptables may be unavailable or fail in rootless/restricted environments.
	// Filesystem sandboxing (bwrap) works independently.
	var netErr error
	if cfg.BlockLocalNetwork {
		iptablesInstall := "export PATH=\"/usr/sbin:/sbin:$PATH\"; " +
			"command -v iptables >/dev/null 2>&1 || { " +
			"command -v apt-get >/dev/null 2>&1 && " +
			"apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq iptables >/dev/null 2>&1; }"
		if err := req.ExecFunc(ctx, []string{"sh", "-c", iptablesInstall}, "root"); err != nil {
			netErr = fmt.Errorf("installing iptables: %w", err)
		}
	}
	if cfg.BlockLocalNetwork && netErr == nil {
		netScript := generateNetworkScript(cfg)
		netErr = execScriptViaFile(ctx, req, netScript)
	}

	// 3. Build the sandbox policy.
	pol := buildPolicy(cfg, req.WorkspaceDir, req.RemoteUser, req.WorkspaceFolder)

	// 3b. Apply user-configured hideFiles (paths relative to workspace folder).
	// Validate that resolved paths stay within the workspace.
	wsFolder := filepath.Clean(req.WorkspaceFolder)
	for _, rel := range cfg.HideFiles {
		if rel == "" || rel == "." {
			continue
		}
		if filepath.IsAbs(rel) {
			slog.Debug("sandbox: hideFiles entry must be relative, skipping", "path", rel)
			continue
		}
		abs := filepath.Clean(filepath.Join(wsFolder, rel))
		if !strings.HasPrefix(abs, wsFolder+"/") {
			slog.Debug("sandbox: hideFiles path escapes workspace, skipping", "path", rel)
			continue
		}
		pol.HiddenFiles = append(pol.HiddenFiles, abs)
		slog.Debug("sandbox: hiding file", "path", abs)
	}

	// 3c. Auto-detect git worktrees and add their base dirs as writable.
	// Non-fatal: git may not be installed or workspace may not be a repo.
	// Deduplicate against existing AllowWritePaths (user may have configured
	// the same path via allowWrite in devcontainer.json).
	wtDirs := detectWorktreeWritePaths(ctx, req)
	existing := make(map[string]struct{}, len(pol.AllowWritePaths))
	for _, p := range pol.AllowWritePaths {
		existing[filepath.Clean(p)] = struct{}{}
	}
	for _, d := range wtDirs {
		if _, dup := existing[filepath.Clean(d)]; dup {
			continue
		}
		pol.AllowWritePaths = append(pol.AllowWritePaths, d)
		slog.Debug("sandbox: auto-detected git worktree directory", "path", d)
	}

	// 4. Generate and write the sandbox wrapper script.
	sandboxPath := localBin + "/sandbox"
	wrapperContent := generateWrapperScript(pol)
	if err := req.CopyFileFunc(ctx, []byte(wrapperContent), sandboxPath, "0755", owner); err != nil {
		return fmt.Errorf("writing sandbox wrapper: %w", err)
	}

	// Surface network setup failure after wrapper generation so the plugin
	// manager logs it as a warning (fail-open).
	if netErr != nil {
		return fmt.Errorf("applying network rules (filesystem sandbox still active): %w", netErr)
	}

	return nil
}

// execScriptViaFile copies a shell script into the container and executes it.
// Uses mktemp to create a unique path (avoids symlink attacks in /tmp).
func execScriptViaFile(ctx context.Context, req *plugin.PostContainerCreateRequest, script string) error {
	// Create a temp file via mktemp to avoid symlink races on a fixed path.
	tmpPath, err := req.ExecOutputFunc(ctx, []string{"mktemp", "/tmp/crib-sandbox-XXXXXX.sh"}, "root")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpScript := strings.TrimSpace(tmpPath)
	if err := req.CopyFileFunc(ctx, []byte(script), tmpScript, "0700", "root"); err != nil {
		_ = req.ExecFunc(ctx, []string{"rm", "-f", tmpScript}, "root")
		return fmt.Errorf("writing temp script: %w", err)
	}
	execErr := req.ExecFunc(ctx, []string{"sh", tmpScript}, "root")
	_ = req.ExecFunc(ctx, []string{"rm", "-f", tmpScript}, "root")
	return execErr
}

// detectWorktreeWritePaths runs `git worktree list --porcelain` inside the
// container and returns directories that need write access for worktree
// checkouts. Returns nil when no external worktrees are found or when git
// is unavailable.
func detectWorktreeWritePaths(ctx context.Context, req *plugin.PostContainerCreateRequest) []string {
	out, err := req.ExecOutputFunc(ctx, []string{
		"git", "-C", req.WorkspaceFolder, "worktree", "list", "--porcelain",
	}, req.RemoteUser)
	if err != nil {
		slog.Debug("sandbox: git worktree detection skipped", "error", err)
		return nil
	}
	paths := parseWorktreePaths(out)
	return worktreeBaseDirs(paths, req.WorkspaceFolder)
}
