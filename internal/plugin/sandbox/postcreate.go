package sandbox

import (
	"context"
	"encoding/base64"
	"fmt"
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

	// 1. Install required tools.
	packages := "bubblewrap"
	if cfg.BlockLocalNetwork || cfg.BlockCloudProviders {
		packages += " iptables"
	}
	installCmd := fmt.Sprintf(
		"apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq %s >/dev/null 2>&1",
		packages)
	// Only install if any package is missing.
	checkCmd := "command -v bwrap >/dev/null 2>&1"
	if cfg.BlockLocalNetwork || cfg.BlockCloudProviders {
		checkCmd += " && command -v iptables >/dev/null 2>&1"
	}
	fullInstallCmd := fmt.Sprintf("%s || { %s; }", checkCmd, installCmd)
	if err := req.ExecFunc(ctx, []string{"sh", "-c", fullInstallCmd}, "root"); err != nil {
		return fmt.Errorf("installing sandbox tools: %w", err)
	}

	// 2. Apply network restrictions (once, container-wide).
	// Individual rules suppress errors with 2>/dev/null for cases where
	// some work but others don't (e.g. ip6tables missing in some setups).
	if cfg.BlockLocalNetwork || cfg.BlockCloudProviders {
		netScript := generateNetworkScript(cfg) + "true\n"
		if err := req.ExecFunc(ctx, []string{"sh", "-c", netScript}, "root"); err != nil {
			return fmt.Errorf("applying network rules: %w", err)
		}
	}

	// 3. Build the sandbox policy.
	pol := buildPolicy(cfg, req.WorkspaceDir, req.RemoteUser, req.WorkspaceFolder)

	// 4. Ensure ~/.local/bin exists.
	mkdirCmd := fmt.Sprintf("mkdir -p '%s' && chown '%s' '%s'", localBin, owner, localBin)
	if err := req.ExecFunc(ctx, []string{"sh", "-c", mkdirCmd}, "root"); err != nil {
		return fmt.Errorf("creating local bin: %w", err)
	}

	// 5. Generate and write the sandbox wrapper script.
	sandboxPath := localBin + "/sandbox"
	wrapperContent := generateWrapperScript(pol)
	if err := writeFileViaExec(ctx, req, sandboxPath, wrapperContent, owner); err != nil {
		return fmt.Errorf("writing sandbox wrapper: %w", err)
	}

	// 6. Generate alias wrappers.
	for _, alias := range cfg.Aliases {
		aliasPath := localBin + "/" + alias
		realPath, err := resolveRealBinary(ctx, req, alias, localBin)
		if err != nil || realPath == "" {
			continue
		}
		aliasContent := generateAliasScript(alias, realPath, sandboxPath)
		if err := writeFileViaExec(ctx, req, aliasPath, aliasContent, owner); err != nil {
			return fmt.Errorf("writing alias %s: %w", alias, err)
		}
	}

	return nil
}

// writeFileViaExec writes content to a file inside the container using
// base64 encoding to avoid shell quoting issues.
func writeFileViaExec(ctx context.Context, req *plugin.PostContainerCreateRequest, path, content, owner string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	cmd := fmt.Sprintf("echo '%s' | base64 -d > '%s' && chmod 755 '%s' && chown '%s' '%s'",
		encoded, path, path, owner, path)
	return req.ExecFunc(ctx, []string{"sh", "-c", cmd}, "root")
}

// resolveRealBinary finds the real path of a binary inside the container,
// excluding ~/.local/bin to avoid self-reference from our generated aliases.
func resolveRealBinary(ctx context.Context, req *plugin.PostContainerCreateRequest, name, excludeDir string) (string, error) {
	resolveCmd := fmt.Sprintf(
		"PATH=$(echo \"$PATH\" | tr ':' '\\n' | grep -v -F '%s' | paste -sd ':') "+
			"command -v '%s' 2>/dev/null || true",
		excludeDir, name)
	result, err := req.ExecOutputFunc(ctx, []string{"sh", "-c", resolveCmd}, "root")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result), nil
}
