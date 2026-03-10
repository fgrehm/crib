package driver

import (
	"io"
	"strings"

	"github.com/fgrehm/crib/internal/config"
)

// ContainerDetails describes a running or stopped container.
type ContainerDetails struct {
	ID      string
	Created string
	State   ContainerState
	Config  ContainerConfig
	Ports   []PortBinding
}

// PortBinding describes a published port mapping.
type PortBinding struct {
	ContainerPort int    // Port inside the container
	HostPort      int    // Port on the host
	HostIP        string // Host bind address (e.g. "0.0.0.0")
	Protocol      string // "tcp" or "udp"
	RawSpec       string // Original spec when ports can't be represented as ints (e.g. ranges)
}

// ContainerState holds the runtime state of a container.
type ContainerState struct {
	Status    string
	StartedAt string
}

// IsRunning reports whether the container is in the running state.
func (s ContainerState) IsRunning() bool {
	return strings.EqualFold(s.Status, "running")
}

// IsRemoving reports whether the container is in the process of being removed.
func (s ContainerState) IsRemoving() bool {
	return strings.EqualFold(s.Status, "removing")
}

// ContainerConfig holds container configuration metadata.
type ContainerConfig struct {
	Labels map[string]string
	User   string
}

// ImageDetails describes a container image.
type ImageDetails struct {
	ID     string
	Config ImageConfig
}

// ImageConfig holds image configuration metadata.
type ImageConfig struct {
	User       string
	Env        []string
	Labels     map[string]string
	Entrypoint []string
	Cmd        []string
}

// RunOptions holds parameters for creating and starting a container.
type RunOptions struct {
	Image          string
	User           string
	Entrypoint     string
	Cmd            []string
	Env            []string
	CapAdd         []string
	SecurityOpt    []string
	Labels         map[string]string
	Privileged     bool
	Init           bool
	WorkspaceMount config.Mount
	Mounts         []config.Mount
	Ports          []string // Publish specs (e.g. "8080:8080")
	ExtraArgs      []string // Raw CLI args passed through from runArgs
}

// VolumeInfo describes a named Docker/Podman volume.
type VolumeInfo struct {
	Name string
	Size string // human-readable, best-effort (may be empty)
}

// ImageInfo describes a crib-managed image discovered by label.
type ImageInfo struct {
	Reference   string // repo:tag (e.g. "crib-myws:crib-abc123")
	ID          string // image ID
	Size        int64  // image size in bytes
	WorkspaceID string // value of crib.workspace label
}

// BuildOptions holds parameters for building a container image.
type BuildOptions struct {
	PrebuildHash string
	Image        string
	Dockerfile   string
	Context      string
	Args         map[string]string
	Target       string
	CacheFrom    []string
	Labels       map[string]string // Image labels (e.g. crib.workspace=wsID)
	Options      []string          // Extra CLI flags from build.options
	Stdout       io.Writer
	Stderr       io.Writer
}
