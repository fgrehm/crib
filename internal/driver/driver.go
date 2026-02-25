package driver

import (
	"context"
	"io"
)

// Driver abstracts the container runtime (Docker or Podman).
type Driver interface {
	// FindContainer locates a container by workspace ID.
	// Returns nil if no container is found.
	FindContainer(ctx context.Context, workspaceID string) (*ContainerDetails, error)

	// RunContainer creates and starts a container with the given options.
	RunContainer(ctx context.Context, workspaceID string, options *RunOptions) error

	// StartContainer starts a stopped container.
	StartContainer(ctx context.Context, workspaceID, containerID string) error

	// StopContainer stops a running container.
	StopContainer(ctx context.Context, workspaceID, containerID string) error

	// DeleteContainer removes a container.
	DeleteContainer(ctx context.Context, workspaceID, containerID string) error

	// ExecContainer runs a command inside a container with attached I/O.
	// env is a list of KEY=VALUE pairs injected via -e flags.
	// user overrides the exec user (e.g. "root"); pass "" to use the container default.
	ExecContainer(ctx context.Context, workspaceID, containerID string, cmd []string, stdin io.Reader, stdout, stderr io.Writer, env []string, user string) error

	// ContainerLogs returns the logs from a container.
	ContainerLogs(ctx context.Context, workspaceID, containerID string, stdout, stderr io.Writer) error

	// BuildImage builds a container image.
	BuildImage(ctx context.Context, workspaceID string, options *BuildOptions) error

	// InspectImage returns details about a container image.
	InspectImage(ctx context.Context, imageName string) (*ImageDetails, error)

	// TargetArchitecture returns the architecture of the container runtime (e.g. "amd64", "arm64").
	TargetArchitecture(ctx context.Context) (string, error)
}
