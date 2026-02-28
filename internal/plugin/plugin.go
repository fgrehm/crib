package plugin

import (
	"context"

	"github.com/fgrehm/crib/internal/config"
)

// Plugin is the interface all plugins implement.
type Plugin interface {
	Name() string
	PreContainerRun(ctx context.Context, req *PreContainerRunRequest) (*PreContainerRunResponse, error)
}

// PreContainerRunRequest carries context about the workspace and container
// that is about to be created. Plugins use this to decide what mounts,
// env vars, or extra args to inject.
type PreContainerRunRequest struct {
	WorkspaceID     string // unique workspace identifier
	WorkspaceDir    string // ~/.crib/workspaces/{id}/
	SourceDir       string // project root on host
	Runtime         string // "docker" or "podman"
	ImageName       string // resolved image name
	RemoteUser      string // user inside the container
	WorkspaceFolder string // path inside container
	ContainerName   string // crib-{workspace-id}
}

// PreContainerRunResponse carries additions that the plugin wants injected
// into the container run command. Nil means no-op.
type PreContainerRunResponse struct {
	Mounts  []config.Mount
	Env     map[string]string
	RunArgs []string
	Copies  []FileCopy
}

// InferRemoteHome returns the home directory path for the given user inside
// the container. Root or empty user maps to /root, others to /home/{user}.
// Used by plugins that need to place files in the container user's home
// before the container exists.
func InferRemoteHome(user string) string {
	if user == "" || user == "root" {
		return "/root"
	}
	return "/home/" + user
}

// FileCopy describes a file to copy into the container after creation.
type FileCopy struct {
	Source string      // path on host
	Target string      // path inside container
	Mode   string      // chmod mode (e.g. "0600"), empty for default
	User   string      // chown user inside container (e.g. "vscode"), empty for default
}
