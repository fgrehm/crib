package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/fgrehm/crib/internal/plugin"
)

// validAliasName restricts alias names to safe characters for shell commands
// and file paths. Must start with alphanumeric (rejects ".", "..", "-flag").
var validAliasName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

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
		iptablesInstall := "command -v iptables >/dev/null 2>&1 || { " +
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

	// 3b. Auto-detect git worktrees and add their base dirs as writable.
	// Non-fatal: git may not be installed or workspace may not be a repo.
	wtDirs := detectWorktreeWritePaths(ctx, req)
	for _, d := range wtDirs {
		pol.AllowWritePaths = append(pol.AllowWritePaths, d)
		slog.Info("sandbox: auto-detected git worktree directory", "path", d)
	}

	// 4. Generate and write the sandbox wrapper script.
	sandboxPath := localBin + "/sandbox"
	wrapperContent := generateWrapperScript(pol)
	if err := req.CopyFileFunc(ctx, []byte(wrapperContent), sandboxPath, "0755", owner); err != nil {
		return fmt.Errorf("writing sandbox wrapper: %w", err)
	}

	// 5. Generate alias wrappers.
	for _, alias := range cfg.Aliases {
		if !validAliasName.MatchString(alias) {
			continue
		}
		aliasPath := localBin + "/" + alias
		realPath, err := resolveRealBinary(ctx, req, alias, localBin, req.RemoteUser)
		if err != nil || realPath == "" {
			continue
		}
		aliasContent := generateAliasScript(alias, realPath, sandboxPath)
		if err := req.CopyFileFunc(ctx, []byte(aliasContent), aliasPath, "0755", owner); err != nil {
			return fmt.Errorf("writing alias %s: %w", alias, err)
		}
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

// resolveRealBinary finds the real path of a binary inside the container,
// excluding ~/.local/bin to avoid self-reference from our generated aliases.
// Runs as the specified user so the lookup sees the user's PATH.
func resolveRealBinary(ctx context.Context, req *plugin.PostContainerCreateRequest, name, excludeDir, user string) (string, error) {
	resolveCmd := fmt.Sprintf(
		"PATH=$(echo \"$PATH\" | tr ':' '\\n' | grep -v -x -F '%s' | paste -sd ':') "+
			"command -v '%s' 2>/dev/null || true",
		plugin.ShellQuote(excludeDir), name)
	result, err := req.ExecOutputFunc(ctx, []string{"sh", "-c", resolveCmd}, user)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result), nil
}
