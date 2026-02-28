package codingagents

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
)

// Plugin provides coding-agent credential sharing. Currently supports
// Claude Code by copying ~/.claude/ into the workspace state directory
// and returning a bind mount.
type Plugin struct {
	homeDir string // overridable for testing; defaults to os.UserHomeDir()
}

// New creates a coding-agents plugin that uses the real user home directory.
func New() *Plugin {
	return &Plugin{}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string { return "coding-agents" }

// PreContainerRun checks for ~/.claude/ on the host. If present, it copies the
// directory into the workspace state dir and returns a bind mount so the
// container gets the credentials.
func (p *Plugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	home := p.homeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
	}

	claudeSrc := filepath.Join(home, ".claude")
	if _, err := os.Stat(claudeSrc); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking ~/.claude: %w", err)
	}

	copyDst := filepath.Join(req.WorkspaceDir, "plugins", "coding-agents", "claude")
	if err := copyDir(claudeSrc, copyDst); err != nil {
		return nil, fmt.Errorf("copying ~/.claude: %w", err)
	}

	remoteHome := inferRemoteHome(req.RemoteUser)

	return &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{
			{
				Type:   "bind",
				Source: copyDst,
				Target: filepath.Join(remoteHome, ".claude"),
			},
		},
	}, nil
}

// inferRemoteHome returns the home directory path for the given user inside
// the container. Root or empty user maps to /root, others to /home/{user}.
func inferRemoteHome(user string) string {
	if user == "" || user == "root" {
		return "/root"
	}
	return "/home/" + user
}

// copyDir recursively copies src to dst, creating dst if needed.
func copyDir(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}

	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		// Copy regular files only (skip symlinks for safety).
		if !d.Type().IsRegular() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		return os.WriteFile(target, data, info.Mode())
	})
}
