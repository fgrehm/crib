package sandbox

import (
	"context"
	"fmt"
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

	// 1. Install required tools.
	packages := "bubblewrap"
	if cfg.BlockLocalNetwork {
		packages += " iptables"
	}
	installCmd := fmt.Sprintf(
		"apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq %s >/dev/null 2>&1",
		packages)
	// Only install if any package is missing.
	checkCmd := "command -v bwrap >/dev/null 2>&1"
	if cfg.BlockLocalNetwork {
		checkCmd += " && command -v iptables >/dev/null 2>&1"
	}
	fullInstallCmd := fmt.Sprintf("%s || { %s; }", checkCmd, installCmd)
	if err := req.ExecFunc(ctx, []string{"sh", "-c", fullInstallCmd}, "root"); err != nil {
		return fmt.Errorf("installing sandbox tools: %w", err)
	}

	// 2. Apply network restrictions (once, container-wide).
	// The script is copied to a temp file and executed to avoid ARG_MAX
	// limits with large rule sets.
	if cfg.BlockLocalNetwork {
		netScript := generateNetworkScript(cfg)
		if err := execScriptViaFile(ctx, req, netScript); err != nil {
			return fmt.Errorf("applying network rules: %w", err)
		}
	}

	// 3. Build the sandbox policy.
	pol := buildPolicy(cfg, req.WorkspaceDir, req.RemoteUser, req.WorkspaceFolder)

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
		return err
	}
	execErr := req.ExecFunc(ctx, []string{"sh", tmpScript}, "root")
	_ = req.ExecFunc(ctx, []string{"rm", "-f", tmpScript}, "root")
	return execErr
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
