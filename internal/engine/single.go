package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// defaultEntrypoint is used when overrideCommand is not explicitly false.
const defaultEntrypoint = "/bin/sh"

// defaultCmd keeps the container alive when overrideCommand is not false.
var defaultCmd = []string{"-c", "echo Container started; trap \"exit 0\" 15; exec \"$@\"; sleep infinity"}

// upSingle handles the single container path (image or Dockerfile based).
func (e *Engine) upSingle(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, opts UpOptions) (*UpResult, error) {
	// Check for an existing container.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding container: %w", err)
	}

	if container != nil && !opts.Recreate {
		// Container exists and we're not forcing recreation.
		if !container.State.IsRunning() {
			e.reportProgress("Starting container...")
			if err := e.driver.StartContainer(ctx, ws.ID, container.ID); err != nil {
				return nil, fmt.Errorf("starting container: %w", err)
			}
		} else {
			e.reportProgress("Container already running")
		}

		return e.setupAndReturn(ctx, ws, cfg, container.ID, workspaceFolder)
	}

	// Remove existing container if recreating.
	if container != nil && opts.Recreate {
		e.reportProgress("Removing container...")
		if err := e.store.ClearHookMarkers(ws.ID); err != nil {
			e.logger.Warn("failed to clear hook markers", "error", err)
		}
		if err := e.driver.DeleteContainer(ctx, ws.ID, container.ID); err != nil {
			return nil, fmt.Errorf("deleting container for recreation: %w", err)
		}
	}

	// Build the image.
	buildRes, err := e.buildImage(ctx, ws, cfg)
	if err != nil {
		return nil, err
	}

	// Build run options.
	runOpts := e.buildRunOptions(cfg, buildRes.imageName, ws.Source, workspaceFolder)

	// Run pre-container-run plugins to inject mounts, env, and extra args.
	pluginResp, err := e.runPreContainerRunPlugins(ctx, ws, cfg, runOpts, buildRes.imageName, workspaceFolder)
	if err != nil {
		return nil, err
	}

	e.reportProgress("Creating container...")
	if err := e.driver.RunContainer(ctx, ws.ID, runOpts); err != nil {
		return nil, fmt.Errorf("creating container: %w", err)
	}

	// Find the newly created container.
	container, err = e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding new container: %w", err)
	}
	if container == nil {
		return nil, fmt.Errorf("container not found after creation")
	}

	// Copy plugin files into the container.
	if pluginResp != nil {
		e.execPluginCopies(ctx, ws.ID, container.ID, pluginResp.Copies)
	}

	result, setupErr := e.setupAndReturn(ctx, ws, cfg, container.ID, workspaceFolder)
	if result != nil {
		result.ImageName = buildRes.imageName
		e.saveResult(ws, cfg, result)
	}
	return result, setupErr
}

// buildRunOptions constructs RunOptions from the devcontainer config.
func (e *Engine) buildRunOptions(cfg *config.DevContainerConfig, imageName, projectRoot, workspaceFolder string) *driver.RunOptions {
	opts := &driver.RunOptions{
		Image:  imageName,
		Labels: make(map[string]string),
	}

	// User.
	if cfg.ContainerUser != "" {
		opts.User = cfg.ContainerUser
	}

	// Entrypoint and command.
	overrideCommand := cfg.OverrideCommand == nil || *cfg.OverrideCommand
	if overrideCommand {
		opts.Entrypoint = defaultEntrypoint
		opts.Cmd = defaultCmd
	}

	// Environment variables.
	for k, v := range cfg.ContainerEnv {
		opts.Env = append(opts.Env, k+"="+v)
	}

	// Init process.
	if cfg.Init != nil && *cfg.Init {
		opts.Init = true
	}

	// Privileged mode.
	if cfg.Privileged != nil && *cfg.Privileged {
		opts.Privileged = true
	}

	// Capabilities.
	opts.CapAdd = cfg.CapAdd

	// Security options.
	opts.SecurityOpt = cfg.SecurityOpt

	// Workspace mount.
	if cfg.WorkspaceMount != "" {
		opts.WorkspaceMount = config.ParseMount(cfg.WorkspaceMount)
	} else {
		// Default workspace mount: bind the project root to the workspace folder.
		opts.WorkspaceMount = config.Mount{
			Type:   "bind",
			Source: projectRoot,
			Target: workspaceFolder,
		}
	}

	// Additional mounts.
	opts.Mounts = cfg.Mounts

	// Published ports from forwardPorts and appPort.
	opts.Ports = collectPorts(cfg.ForwardPorts, cfg.AppPort)

	// Passthrough CLI args from runArgs.
	opts.ExtraArgs = cfg.RunArgs

	return opts
}

// setupAndReturn runs container setup and returns the result.
// On lifecycle hook failure, both the result and error are returned so
// callers can persist the result (container is still usable).
func (e *Engine) setupAndReturn(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, containerID, workspaceFolder string) (*UpResult, error) {
	remoteUser := e.resolveRemoteUser(ctx, ws.ID, cfg, containerID)

	result := &UpResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
		Ports:           portSpecToBindings(collectPorts(cfg.ForwardPorts, cfg.AppPort)),
	}

	// Save an early result so crib exec/shell can find the container,
	// workspace folder, and user while setup (UID sync, env probe,
	// lifecycle hooks) is still running.
	e.saveResult(ws, cfg, result)

	// Run container setup (UID sync, env probe, lifecycle hooks).
	if err := e.setupContainer(ctx, ws, cfg, containerID, workspaceFolder, remoteUser); err != nil {
		return result, fmt.Errorf("setting up container: %w", err)
	}

	return result, nil
}

// detectContainerUser runs whoami inside the container to detect the default
// user. Returns empty string on failure or if the user is root.
func (e *Engine) detectContainerUser(ctx context.Context, workspaceID, containerID string) string {
	var stdout bytes.Buffer
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, []string{"whoami"}, nil, &stdout, io.Discard, nil, ""); err != nil {
		return ""
	}
	user := strings.TrimSpace(stdout.String())
	if user == "root" {
		return ""
	}
	return user
}

// collectPorts combines forwardPorts and appPort into publish specs.
// Bare numbers become "port:port"; entries with ":" pass through as-is.
// Duplicates are removed (first occurrence wins).
func collectPorts(forwardPorts, appPort config.StrIntArray) []string {
	seen := make(map[string]bool)
	var result []string
	for _, list := range []config.StrIntArray{forwardPorts, appPort} {
		for _, p := range list {
			spec := p
			if !strings.Contains(p, ":") {
				spec = p + ":" + p
			}
			if !seen[spec] {
				seen[spec] = true
				result = append(result, spec)
			}
		}
	}
	return result
}

// portSpecToBindings converts publish spec strings (e.g. "8080:3000") into
// driver.PortBinding values for display purposes.
func portSpecToBindings(specs []string) []driver.PortBinding {
	var result []driver.PortBinding
	for _, spec := range specs {
		host, container, _ := strings.Cut(spec, ":")
		hostPort, _ := strconv.Atoi(host)
		containerPort, _ := strconv.Atoi(container)
		result = append(result, driver.PortBinding{
			HostPort:      hostPort,
			ContainerPort: containerPort,
			Protocol:      "tcp",
		})
	}
	return result
}

// resolveWorkspaceFolder determines the workspace folder path inside the container.
func resolveWorkspaceFolder(cfg *config.DevContainerConfig, projectRoot string) string {
	if cfg.WorkspaceFolder != "" {
		return cfg.WorkspaceFolder
	}
	return "/workspaces/" + filepath.Base(projectRoot)
}

// runPreContainerRunPlugins dispatches the pre-container-run event to the
// plugin manager and merges the response into the run options. Returns the
// merged response so the caller can process file copies after container creation.
func (e *Engine) runPreContainerRunPlugins(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, runOpts *driver.RunOptions, imageName, workspaceFolder string) (*plugin.PreContainerRunResponse, error) {
	if e.plugins == nil {
		return nil, nil
	}

	remoteUser := cfg.RemoteUser
	if remoteUser == "" {
		remoteUser = cfg.ContainerUser
	}

	req := &plugin.PreContainerRunRequest{
		WorkspaceID:     ws.ID,
		WorkspaceDir:    e.store.WorkspaceDir(ws.ID),
		SourceDir:       ws.Source,
		Runtime:         e.runtimeName,
		ImageName:       imageName,
		RemoteUser:      remoteUser,
		WorkspaceFolder: workspaceFolder,
		ContainerName:   "crib-" + ws.ID,
	}

	resp, err := e.plugins.RunPreContainerRun(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("running pre-container-run plugins: %w", err)
	}

	runOpts.Mounts = append(runOpts.Mounts, resp.Mounts...)
	for k, v := range resp.Env {
		runOpts.Env = append(runOpts.Env, k+"="+v)
	}
	runOpts.ExtraArgs = append(runOpts.ExtraArgs, resp.RunArgs...)

	return resp, nil
}

// execPluginCopies copies staged files into the container via exec.
func (e *Engine) execPluginCopies(ctx context.Context, workspaceID, containerID string, copies []plugin.FileCopy) {
	for _, cp := range copies {
		data, err := os.ReadFile(cp.Source)
		if err != nil {
			e.logger.Warn("plugin copy: failed to read source", "source", cp.Source, "error", err)
			continue
		}

		// Build a shell command that creates the parent dir and writes the file.
		dir := filepath.Dir(cp.Target)
		shellCmd := fmt.Sprintf("mkdir -p %s && cat > %s", dir, cp.Target)
		if cp.Mode != "" {
			shellCmd += fmt.Sprintf(" && chmod %s %s", cp.Mode, cp.Target)
		}
		if cp.User != "" {
			shellCmd += fmt.Sprintf(" && chown %s %s", cp.User, cp.Target)
		}

		err = e.driver.ExecContainer(ctx, workspaceID, containerID,
			[]string{"sh", "-c", shellCmd},
			bytes.NewReader(data), io.Discard, io.Discard, nil, "root")
		if err != nil {
			e.logger.Warn("plugin copy: exec failed", "target", cp.Target, "error", err)
		}
	}
}
