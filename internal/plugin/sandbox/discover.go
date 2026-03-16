package sandbox

import (
	"os"
	"path/filepath"

	"github.com/fgrehm/crib/internal/plugin"
)

// denyRule pairs a container path with whether reads or writes are denied.
type denyRule struct {
	Path     string
	DenyRead bool // true: --tmpfs (hide contents), false: --ro-bind (read-only)
}

// discoveryResult holds deny rules and extra writable paths discovered from
// other plugins' staged artifacts.
type discoveryResult struct {
	DenyRules       []denyRule
	AllowWritePaths []string
}

// discoverPluginArtifacts scans {workspaceDir}/plugins/*/ to find sensitive
// files staged by other plugins. Returns deny rules and allow-write paths
// for the sandbox wrapper.
func discoverPluginArtifacts(workspaceDir, remoteUser string) discoveryResult {
	remoteHome := plugin.InferRemoteHome(remoteUser)
	var result discoveryResult

	pluginsDir := filepath.Join(workspaceDir, "plugins")

	// coding-agents: ~/.claude/ must be writable.
	// Claude Code needs write access to refresh expired OAuth tokens and
	// update local config. The root bind (--ro-bind / /) makes everything
	// read-only by default, so we explicitly grant write access here.
	if dirExists(filepath.Join(pluginsDir, "coding-agents")) {
		result.AllowWritePaths = append(result.AllowWritePaths,
			filepath.Join(remoteHome, ".claude"))
	}

	// ssh: ~/.ssh/config and *.pub
	if dirExists(filepath.Join(pluginsDir, "ssh")) {
		result.DenyRules = append(result.DenyRules, denyRule{
			Path:     filepath.Join(remoteHome, ".ssh"),
			DenyRead: true,
		})
	}

	// shell-history: ~/.crib_history/ (deny-read, agents shouldn't see command history)
	if dirExists(filepath.Join(pluginsDir, "shell-history")) {
		result.DenyRules = append(result.DenyRules, denyRule{
			Path:     filepath.Join(remoteHome, ".crib_history"),
			DenyRead: true,
		})
	}

	return result
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
