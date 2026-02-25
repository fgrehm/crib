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
	ExtraArgs      []string // Raw CLI args passed through from runArgs
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
	Stdout       io.Writer
	Stderr       io.Writer
}
