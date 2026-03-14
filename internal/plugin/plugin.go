package plugin

import (
	"context"
	"os"

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
	WorkspaceID     string         // unique workspace identifier
	WorkspaceDir    string         // ~/.crib/workspaces/{id}/
	SourceDir       string         // project root on host
	Runtime         string         // "docker" or "podman"
	ImageName       string         // resolved image name
	RemoteUser      string         // user inside the container
	WorkspaceFolder string         // path inside container
	ContainerName   string         // crib-{workspace-id}
	Customizations  map[string]any // customizations.crib from devcontainer.json
}

// PreContainerRunResponse carries additions that the plugin wants injected
// into the container run command. Nil means no-op.
type PreContainerRunResponse struct {
	Mounts      []config.Mount
	Env         map[string]string
	RunArgs     []string
	Copies      []FileCopy
	PathPrepend []string // absolute paths to prepend to PATH in remoteEnv
}

// PostContainerCreator is an optional interface for plugins that need to run
// commands inside the container after it has been created and started.
// Plugins that don't need post-create behavior only implement Plugin.
type PostContainerCreator interface {
	PostContainerCreate(ctx context.Context, req *PostContainerCreateRequest) error
}

// PostContainerCreateEnabler is an optional companion to PostContainerCreator.
// When implemented, the manager calls IsPostContainerCreateEnabled before
// printing a progress message or dispatching PostContainerCreate. Returning
// false silently skips the plugin for this request (e.g. when unconfigured).
type PostContainerCreateEnabler interface {
	IsPostContainerCreateEnabled(req *PostContainerCreateRequest) bool
}

// PostContainerCreateRequest carries context about a running container.
// Plugins use this to install tools or generate files inside the container.
type PostContainerCreateRequest struct {
	WorkspaceID     string         // unique workspace identifier
	WorkspaceDir    string         // ~/.crib/workspaces/{id}/
	ContainerID     string         // running container ID
	RemoteUser      string         // user inside the container
	WorkspaceFolder string         // path inside container
	Customizations  map[string]any // customizations.crib from devcontainer.json
	Runtime         string         // "docker" or "podman"

	// ExecFunc runs a command inside the container as the given user.
	// Stdout/stderr are discarded.
	ExecFunc func(ctx context.Context, cmd []string, user string) error

	// ExecOutputFunc runs a command and returns its stdout as a string.
	ExecOutputFunc func(ctx context.Context, cmd []string, user string) (string, error)

	// CopyFileFunc writes content to a path inside the container.
	// Uses the same stdin-piped cat approach as the engine's execPluginCopies.
	CopyFileFunc func(ctx context.Context, content []byte, destPath, mode, user string) error
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

// InferOwner returns the container user for chown operations.
// Empty user maps to "root", otherwise returns the user as-is.
func InferOwner(user string) string {
	if user == "" {
		return "root"
	}
	return user
}

// CopyFile copies a single file from src to dst with the given permissions.
// Shared by plugins that stage host files into workspace state directories.
func CopyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}

// FileCopy describes a file to copy into the container after creation.
type FileCopy struct {
	Source      string // path on host
	Target      string // path inside container
	Mode        string // chmod mode (e.g. "0600"), empty for default
	User        string // chown user inside container (e.g. "vscode"), empty for default
	IfNotExists bool   // skip copy if the target file already exists in the container
}
