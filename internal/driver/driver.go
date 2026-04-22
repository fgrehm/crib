package driver

import (
	"context"
	"io"
)

// LogsOptions controls container log output.
type LogsOptions struct {
	Follow bool   // stream logs as they are produced
	Tail   string // number of lines from the end ("all" or a number)
}

// Driver abstracts the container runtime (Docker or Podman).
type Driver interface {
	// FindContainer locates a container by workspace ID.
	// Returns nil if no container is found.
	FindContainer(ctx context.Context, workspaceID string) (*ContainerDetails, error)

	// RunContainer creates and starts a container with the given options.
	// Returns the container name chosen by the driver (either the default
	// crib-<ws-id> or a user override from runArgs --name).
	RunContainer(ctx context.Context, workspaceID string, options *RunOptions) (string, error)

	// StartContainer starts a stopped container.
	StartContainer(ctx context.Context, workspaceID, containerID string) error

	// StopContainer stops a running container.
	StopContainer(ctx context.Context, workspaceID, containerID string) error

	// RestartContainer restarts a running or stopped container.
	RestartContainer(ctx context.Context, workspaceID, containerID string) error

	// DeleteContainer removes a container.
	DeleteContainer(ctx context.Context, workspaceID, containerID string) error

	// ExecContainer runs a command inside a container with attached I/O.
	// env is a list of KEY=VALUE pairs injected via -e flags.
	// user overrides the exec user (e.g. "root"); pass "" to use the container default.
	ExecContainer(ctx context.Context, workspaceID, containerID string, cmd []string, stdin io.Reader, stdout, stderr io.Writer, env []string, user string) error

	// ContainerLogs returns the logs from a container.
	// opts may be nil for default behavior (all logs, no follow).
	ContainerLogs(ctx context.Context, workspaceID, containerID string, stdout, stderr io.Writer, opts *LogsOptions) error

	// BuildImage builds a container image.
	BuildImage(ctx context.Context, workspaceID string, options *BuildOptions) error

	// InspectImage returns details about a container image.
	InspectImage(ctx context.Context, imageName string) (*ImageDetails, error)

	// TargetArchitecture returns the architecture of the container runtime (e.g. "amd64", "arm64").
	TargetArchitecture(ctx context.Context) (string, error)

	// ListContainers returns all containers with the crib.workspace label.
	ListContainers(ctx context.Context) ([]ContainerDetails, error)

	// CommitContainer creates an image from a container's changes.
	// changes are passed as --change flags (e.g. "LABEL key=value").
	CommitContainer(ctx context.Context, workspaceID, containerID, imageName string, changes []string) error

	// RemoveImage removes a container image.
	RemoveImage(ctx context.Context, imageName string) error

	// ListImages returns images matching the given label filter.
	// Use "crib.workspace" for all crib images, or "crib.workspace=wsID" for a specific workspace.
	ListImages(ctx context.Context, label string) ([]ImageInfo, error)

	// ListVolumes returns volumes whose names match the given filter prefix.
	ListVolumes(ctx context.Context, nameFilter string) ([]VolumeInfo, error)

	// RemoveVolume removes a named volume.
	RemoveVolume(ctx context.Context, name string) error
}
