package sandbox

import (
	"os"
	"path/filepath"

	"github.com/fgrehm/crib/internal/plugin"
)

// denyRule pairs a container path with whether reads or writes are denied.
type denyRule struct {
	Path       string
	DenyRead   bool
	AllowWrite bool
}

// discoverPluginArtifacts scans {workspaceDir}/plugins/*/ to find sensitive
// files staged by other plugins. Returns deny rules for the sandbox wrapper.
func discoverPluginArtifacts(workspaceDir, remoteUser string) []denyRule {
	remoteHome := plugin.InferRemoteHome(remoteUser)
	var rules []denyRule

	pluginsDir := filepath.Join(workspaceDir, "plugins")

	// codingagents: ~/.claude/.credentials.json
	if dirExists(filepath.Join(pluginsDir, "coding-agents")) {
		rules = append(rules, denyRule{
			Path:     filepath.Join(remoteHome, ".claude"),
			DenyRead: true,
		})
	}

	// ssh: ~/.ssh/config and *.pub
	if dirExists(filepath.Join(pluginsDir, "ssh")) {
		rules = append(rules, denyRule{
			Path:     filepath.Join(remoteHome, ".ssh"),
			DenyRead: true,
		})
	}

	// shellhistory: ~/.crib_history/ (deny-read, allow-write)
	if dirExists(filepath.Join(pluginsDir, "shell-history")) {
		rules = append(rules, denyRule{
			Path:       filepath.Join(remoteHome, ".crib_history"),
			DenyRead:   true,
			AllowWrite: true,
		})
	}

	return rules
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
