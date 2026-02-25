package oci

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Runtime identifies the container runtime.
type Runtime int

const (
	RuntimeDocker Runtime = iota
	RuntimePodman
)

// String returns the runtime name.
func (r Runtime) String() string {
	switch r {
	case RuntimePodman:
		return "podman"
	default:
		return "docker"
	}
}

// LabelWorkspace is the container label key used for workspace discovery.
const LabelWorkspace = "crib.workspace"

// OCIDriver implements driver.Driver using docker or podman CLI commands.
type OCIDriver struct {
	helper  *Helper
	runtime Runtime
	logger  *slog.Logger
}

// NewOCIDriver creates an OCIDriver by auto-detecting the container runtime.
func NewOCIDriver(logger *slog.Logger) (*OCIDriver, error) {
	rt, cmd, err := detectRuntime()
	if err != nil {
		return nil, err
	}
	logger.Info("detected container runtime", "runtime", rt.String(), "command", cmd)
	return &OCIDriver{
		helper:  NewHelper(cmd, logger),
		runtime: rt,
		logger:  logger,
	}, nil
}

// Runtime returns the detected container runtime.
func (d *OCIDriver) Runtime() Runtime {
	return d.runtime
}

// TargetArchitecture returns the architecture of the container runtime host.
func (d *OCIDriver) TargetArchitecture(ctx context.Context) (string, error) {
	var format string
	switch d.runtime {
	case RuntimePodman:
		format = "{{.Host.Arch}}"
	default:
		format = "{{.Architecture}}"
	}

	out, err := d.helper.Output(ctx, "info", "--format", format)
	if err != nil {
		d.logger.Warn("failed to detect architecture from runtime, using GOARCH", "error", err)
		return runtime.GOARCH, nil
	}

	arch := strings.TrimSpace(string(out))
	if arch == "" {
		return runtime.GOARCH, nil
	}
	return arch, nil
}

// detectRuntime checks for an available container runtime.
// Priority: CRIB_RUNTIME env > podman > docker.
func detectRuntime() (Runtime, string, error) {
	if env := os.Getenv("CRIB_RUNTIME"); env != "" {
		switch strings.ToLower(env) {
		case "docker":
			cmd, err := findResponsiveRuntime("docker")
			if err != nil {
				return 0, "", fmt.Errorf("CRIB_RUNTIME=docker but docker is not available: %w", err)
			}
			return RuntimeDocker, cmd, nil
		case "podman":
			cmd, err := findResponsiveRuntime("podman")
			if err != nil {
				return 0, "", fmt.Errorf("CRIB_RUNTIME=podman but podman is not available: %w", err)
			}
			return RuntimePodman, cmd, nil
		default:
			return 0, "", fmt.Errorf("CRIB_RUNTIME=%q is not supported (use docker or podman)", env)
		}
	}

	// Auto-detect: try podman first, then docker.
	podmanCmd, podmanErr := findResponsiveRuntime("podman")
	if podmanErr == nil {
		return RuntimePodman, podmanCmd, nil
	}
	dockerCmd, dockerErr := findResponsiveRuntime("docker")
	if dockerErr == nil {
		return RuntimeDocker, dockerCmd, nil
	}

	return 0, "", fmt.Errorf("no container runtime found:\n  podman: %v\n  docker: %v", podmanErr, dockerErr)
}

// findResponsiveRuntime checks if a runtime command exists on PATH and responds to `version`.
func findResponsiveRuntime(name string) (string, error) {
	cmd, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found on PATH: %w", name, err)
	}

	// Verify the runtime is responsive.
	out, err := exec.Command(cmd, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s not responsive: %w: %s", name, err, string(out))
	}
	return cmd, nil
}

// ContainerName returns the container name for a workspace.
func ContainerName(workspaceID string) string {
	return "crib-" + workspaceID
}

// ImageName returns the image name for a workspace with the given tag.
func ImageName(workspaceID, tag string) string {
	return "crib-" + workspaceID + ":" + tag
}

// WorkspaceLabel returns the label filter string for finding workspace containers.
func WorkspaceLabel(workspaceID string) string {
	return LabelWorkspace + "=" + workspaceID
}
