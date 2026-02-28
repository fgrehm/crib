package oci

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fgrehm/crib/internal/driver"
)

// getuid returns the current user's UID. It is a variable so tests can override it.
var getuid = os.Getuid

// FindContainer locates a container by workspace ID using label filtering.
// Returns nil if no container is found. Skips containers in "removing" state.
func (d *OCIDriver) FindContainer(ctx context.Context, workspaceID string) (*driver.ContainerDetails, error) {
	// List container IDs matching the workspace label.
	out, err := d.helper.Output(ctx,
		"ps", "-q", "-a",
		"--filter", "label="+WorkspaceLabel(workspaceID),
	)
	if err != nil {
		return nil, fmt.Errorf("finding container for workspace %s: %w", workspaceID, err)
	}

	ids := parseLines(string(out))
	if len(ids) == 0 {
		return nil, nil
	}

	// Inspect the matched containers. We unmarshal into an intermediate type
	// to capture NetworkSettings.Ports from the docker/podman inspect JSON.
	var raw []inspectContainer
	if err := d.helper.Inspect(ctx, ids, "container", &raw); err != nil {
		return nil, fmt.Errorf("inspecting container for workspace %s: %w", workspaceID, err)
	}

	// Return the first container that isn't being removed.
	for i := range raw {
		details := raw[i].toContainerDetails()
		if !details.State.IsRemoving() {
			return &details, nil
		}
	}
	return nil, nil
}

// RunContainer creates and starts a new container for the workspace.
// The workspace label is injected automatically.
func (d *OCIDriver) RunContainer(ctx context.Context, workspaceID string, options *driver.RunOptions) error {
	args := d.buildRunArgs(workspaceID, options)
	_, err := d.helper.Output(ctx, args...)
	if err != nil {
		return fmt.Errorf("running container for workspace %s: %w", workspaceID, err)
	}
	return nil
}

// buildRunArgs constructs the `docker run` argument list.
func (d *OCIDriver) buildRunArgs(workspaceID string, opts *driver.RunOptions) []string {
	name := ContainerName(workspaceID)

	args := []string{"run", "-d", "--name", name}

	// Workspace label (always added).
	args = append(args, "--label", WorkspaceLabel(workspaceID))

	// User-specified labels.
	for _, k := range sortedKeys(opts.Labels) {
		args = append(args, "--label", k+"="+opts.Labels[k])
	}

	// User.
	if opts.User != "" {
		args = append(args, "--user", opts.User)
	}

	// Environment variables.
	args = appendFlags(args, "-e", opts.Env)

	// Init process.
	if opts.Init {
		args = append(args, "--init")
	}

	// Privileged mode.
	if opts.Privileged {
		args = append(args, "--privileged")
	}

	// Capabilities.
	args = appendFlags(args, "--cap-add", opts.CapAdd)

	// Security options.
	args = appendFlags(args, "--security-opt", opts.SecurityOpt)

	// Workspace mount.
	if opts.WorkspaceMount.Target != "" {
		args = append(args, "--mount", opts.WorkspaceMount.String())
	}

	// Additional mounts.
	for _, m := range opts.Mounts {
		args = append(args, "--mount", m.String())
	}

	// Published ports.
	args = appendFlags(args, "--publish", opts.Ports)

	// Entrypoint.
	if opts.Entrypoint != "" {
		args = append(args, "--entrypoint", opts.Entrypoint)
	}

	// Auto-inject --userns=keep-id for rootless Podman to fix bind mount
	// permissions. This maps the host UID to the same UID inside the
	// container, so workspace files have correct ownership for non-root users.
	if d.runtime == RuntimePodman && getuid() != 0 && !hasUsernsArg(opts.ExtraArgs) {
		args = append(args, "--userns=keep-id")
	}

	// Passthrough CLI args from runArgs.
	args = append(args, opts.ExtraArgs...)

	// Image (required).
	args = append(args, opts.Image)

	// Command.
	args = append(args, opts.Cmd...)

	return args
}

// StartContainer starts a stopped container.
func (d *OCIDriver) StartContainer(ctx context.Context, _, containerID string) error {
	_, err := d.helper.Output(ctx, "start", containerID)
	return err
}

// StopContainer stops a running container.
func (d *OCIDriver) StopContainer(ctx context.Context, _, containerID string) error {
	_, err := d.helper.Output(ctx, "stop", containerID)
	return err
}

// RestartContainer restarts a running or stopped container.
func (d *OCIDriver) RestartContainer(ctx context.Context, _, containerID string) error {
	_, err := d.helper.Output(ctx, "restart", containerID)
	return err
}

// DeleteContainer removes a container forcefully.
func (d *OCIDriver) DeleteContainer(ctx context.Context, _, containerID string) error {
	_, err := d.helper.Output(ctx, "rm", "-f", containerID)
	return err
}

// ExecContainer runs a command inside a container with attached I/O.
// env is injected as -e KEY=VALUE flags.
// user overrides the exec user (e.g. "root"); empty string uses the container default.
func (d *OCIDriver) ExecContainer(ctx context.Context, _, containerID string, cmd []string, stdin io.Reader, stdout, stderr io.Writer, env []string, user string) error {
	args := []string{"exec"}
	if stdin != nil {
		args = append(args, "-i")
	}
	if user != "" {
		args = append(args, "--user", user)
	}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, containerID)
	args = append(args, cmd...)
	return d.helper.Run(ctx, args, stdin, stdout, stderr)
}

// ContainerLogs returns the logs from a container.
func (d *OCIDriver) ContainerLogs(ctx context.Context, _, containerID string, stdout, stderr io.Writer) error {
	return d.helper.Run(ctx, []string{"logs", containerID}, nil, stdout, stderr)
}

// inspectContainer is an intermediate struct for unmarshaling docker/podman
// inspect JSON. It mirrors the fields we need from ContainerDetails plus the
// nested NetworkSettings.Ports structure.
type inspectContainer struct {
	ID      string `json:"Id"`
	Created string `json:"Created"`
	State   struct {
		Status    string `json:"Status"`
		StartedAt string `json:"StartedAt"`
	} `json:"State"`
	Config struct {
		Labels map[string]string `json:"Labels"`
		User   string            `json:"User"`
	} `json:"Config"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIp   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

// toContainerDetails converts the intermediate inspect result to a driver.ContainerDetails.
func (ic *inspectContainer) toContainerDetails() driver.ContainerDetails {
	d := driver.ContainerDetails{
		ID:      ic.ID,
		Created: ic.Created,
		State: driver.ContainerState{
			Status:    ic.State.Status,
			StartedAt: ic.State.StartedAt,
		},
		Config: driver.ContainerConfig{
			Labels: ic.Config.Labels,
			User:   ic.Config.User,
		},
	}
	for containerPort, bindings := range ic.NetworkSettings.Ports {
		port, proto := parseContainerPort(containerPort)
		for _, b := range bindings {
			hostPort, _ := strconv.Atoi(b.HostPort)
			d.Ports = append(d.Ports, driver.PortBinding{
				ContainerPort: port,
				HostPort:      hostPort,
				HostIP:        b.HostIp,
				Protocol:      proto,
			})
		}
	}
	return d
}

// parseContainerPort splits "8080/tcp" into port number and protocol.
func parseContainerPort(s string) (int, string) {
	proto := "tcp"
	portStr := s
	if i := strings.Index(s, "/"); i >= 0 {
		portStr = s[:i]
		proto = s[i+1:]
	}
	port, _ := strconv.Atoi(portStr)
	return port, proto
}

// parseLines splits output by newlines and removes empty strings.
func parseLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// hasUsernsArg checks whether any argument in args starts with "--userns".
func hasUsernsArg(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "--userns") {
			return true
		}
	}
	return false
}

// appendFlags appends "--flag value" pairs to args for each value in values.
func appendFlags(args []string, flag string, values []string) []string {
	for _, v := range values {
		args = append(args, flag, v)
	}
	return args
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
