package engine

import "fmt"

// ErrNoContainer is returned when a workspace operation requires a container
// but none exists.
type ErrNoContainer struct {
	WorkspaceID string
}

func (e *ErrNoContainer) Error() string {
	return fmt.Sprintf("no container found for workspace %s (run 'crib up' first)", e.WorkspaceID)
}

// ErrContainerStopped is returned when a workspace operation requires a running
// container but the container exists in a stopped state.
type ErrContainerStopped struct {
	WorkspaceID string
	ContainerID string
}

func (e *ErrContainerStopped) Error() string {
	return "container is stopped (run 'crib up' to start it)"
}

// ErrComposeNotAvailable is returned when an operation requires docker compose
// or podman compose but neither is installed.
type ErrComposeNotAvailable struct{}

func (e *ErrComposeNotAvailable) Error() string {
	return "compose is not available (install docker compose or podman compose)"
}
