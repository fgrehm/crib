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

		// Dispatch plugins to get PathPrepend so setupContainer can inject
		// plugin PATH entries into RemoteEnv before saving the result.
		var pathPrepend []string
		remoteUser := cfg.RemoteUser
		if remoteUser == "" {
			remoteUser = cfg.ContainerUser
		}
		if resp, err := e.dispatchPlugins(ctx, ws, cfg, remoteUser, workspaceFolder, ""); err != nil {
			e.logger.Warn("plugin dispatch failed for already-running container", "error", err)
		} else if resp != nil {
			pathPrepend = resp.PathPrepend
		}

		return e.setupAndReturn(ctx, ws, cfg, container.ID, workspaceFolder, pathPrepend)
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
	runOpts, err := e.buildRunOptions(cfg, buildRes.imageName, ws.Source, workspaceFolder)
	if err != nil {
		return nil, err
	}

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

	return e.finalizeSetup(ctx, ws, cfg, container.ID, workspaceFolder, buildRes.imageName, pluginResp)
}

// buildRunOptions constructs RunOptions from the devcontainer config.
func (e *Engine) buildRunOptions(cfg *config.DevContainerConfig, imageName, projectRoot, workspaceFolder string) (*driver.RunOptions, error) {
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
		mount, err := config.ParseMount(cfg.WorkspaceMount)
		if err != nil {
			return nil, fmt.Errorf("parsing workspace mount: %w", err)
		}
		opts.WorkspaceMount = mount
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

	return opts, nil
}

// finalizeSetup copies plugin files, runs container setup, and persists the
// result. Both the single-container and compose paths converge here after the
// container has been created/started.
func (e *Engine) finalizeSetup(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, containerID, workspaceFolder, imageName string, pluginResp *plugin.PreContainerRunResponse) (*UpResult, error) {
	if pluginResp != nil {
		e.execPluginCopies(ctx, ws.ID, containerID, pluginResp.Copies)

		// Chown volume mounts to the remote user. Docker volumes are
		// created with root ownership, so non-root users can't write
		// to them until we fix permissions.
		remoteUser := configRemoteUser(cfg)
		if remoteUser != "" && remoteUser != "root" {
			e.chownPluginVolumes(ctx, ws.ID, containerID, remoteUser, pluginResp.Mounts)
		}

		// Merge plugin env vars into RemoteEnv so they take precedence
		// over values from the user's shell profile (userEnvProbe).
		// Without this, image-baked profile scripts (e.g. /etc/bash.bashrc
		// setting CARGO_HOME) would override plugin-set env vars.
		for k, v := range pluginResp.Env {
			if cfg.RemoteEnv == nil {
				cfg.RemoteEnv = make(map[string]string)
			}
			if _, exists := cfg.RemoteEnv[k]; !exists {
				cfg.RemoteEnv[k] = v
			}
		}
	}

	var pathPrepend []string
	if pluginResp != nil {
		pathPrepend = pluginResp.PathPrepend
	}

	result, setupErr := e.setupAndReturn(ctx, ws, cfg, containerID, workspaceFolder, pathPrepend)
	if result != nil {
		result.ImageName = imageName
		e.saveResult(ws, cfg, result)
	}
	return result, setupErr
}

// chownPluginVolumes changes ownership of plugin volume mounts to the
// remote user. Docker/Podman create volumes with root ownership, so
// non-root users get permission errors when writing to them.
func (e *Engine) chownPluginVolumes(ctx context.Context, workspaceID, containerID, remoteUser string, mounts []config.Mount) {
	for _, m := range mounts {
		if m.Type != "volume" {
			continue
		}
		cmd := []string{"chown", remoteUser + ":", m.Target}
		if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, io.Discard, io.Discard, nil, "root"); err != nil {
			e.logger.Debug("chown plugin volume failed", "target", m.Target, "error", err)
		}
	}
}

// setupAndReturn runs container setup and returns the result.
// On lifecycle hook failure, both the result and error are returned so
// callers can persist the result (container is still usable).
func (e *Engine) setupAndReturn(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, containerID, workspaceFolder string, pathPrepend []string) (*UpResult, error) {
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
	if err := e.setupContainer(ctx, ws, cfg, containerID, workspaceFolder, remoteUser, pathPrepend); err != nil {
		return result, fmt.Errorf("setting up container: %w", err)
	}

	// After create-time hooks complete, commit a snapshot so restart can
	// use it instead of re-running hooks.
	e.commitSnapshot(ctx, ws, cfg, containerID)

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
// driver.PortBinding values for display purposes. Specs that cannot be parsed
// as simple integer ports (e.g. range specs like "8000-8010:8000-8010") are
// stored with RawSpec for display as-is.
func portSpecToBindings(specs []string) []driver.PortBinding {
	var result []driver.PortBinding
	for _, spec := range specs {
		host, container, _ := strings.Cut(spec, ":")
		hostPort, errH := strconv.Atoi(host)
		containerPort, errC := strconv.Atoi(container)
		if errH != nil || errC != nil {
			result = append(result, driver.PortBinding{
				RawSpec:  spec,
				Protocol: "tcp",
			})
			continue
		}
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

// dispatchPlugins builds a pre-container-run request and dispatches it to the
// plugin manager. Returns the plugin response (nil if no plugins configured).
// Used by both single-container and compose paths.
//
// remoteUser overrides the user from cfg when non-empty. Compose callers pass
// the user resolved from the service/image so plugins get the correct home
// directory even when devcontainer.json doesn't set remoteUser/containerUser.
func (e *Engine) dispatchPlugins(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, imageName, workspaceFolder, remoteUser string) (*plugin.PreContainerRunResponse, error) {
	if e.plugins == nil {
		return nil, nil
	}

	if remoteUser == "" {
		remoteUser = configRemoteUser(cfg)
	}

	// Extract customizations.crib from devcontainer.json for plugins.
	var cribCustomizations map[string]any
	if cfg.Customizations != nil {
		if crib, ok := cfg.Customizations["crib"]; ok {
			if m, ok := crib.(map[string]any); ok {
				cribCustomizations = m
			}
		}
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
		Customizations:  cribCustomizations,
	}

	resp, err := e.plugins.RunPreContainerRun(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("running pre-container-run plugins: %w", err)
	}

	return resp, nil
}

// runPreContainerRunPlugins dispatches the pre-container-run event to the
// plugin manager and merges the response into the run options. Returns the
// merged response so the caller can process file copies after container creation.
func (e *Engine) runPreContainerRunPlugins(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, runOpts *driver.RunOptions, imageName, workspaceFolder string) (*plugin.PreContainerRunResponse, error) {
	resp, err := e.dispatchPlugins(ctx, ws, cfg, imageName, workspaceFolder, "")
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}

	runOpts.Mounts = append(runOpts.Mounts, resp.Mounts...)
	for k, v := range resp.Env {
		runOpts.Env = append(runOpts.Env, k+"="+v)
	}
	runOpts.ExtraArgs = append(runOpts.ExtraArgs, resp.RunArgs...)

	return resp, nil
}

// execPluginCopies copies staged files into the container via exec.
//
// NOTE: Values are embedded in single-quoted shell arguments. This is safe for
// all current callers (bundled plugins with hardcoded paths like
// ~/.claude/.credentials.json). If we add external/user-defined plugins, the
// values must be shell-escaped first to prevent breakage or injection from
// paths containing single quotes.
func (e *Engine) execPluginCopies(ctx context.Context, workspaceID, containerID string, copies []plugin.FileCopy) {
	for _, cp := range copies {
		data, err := os.ReadFile(cp.Source)
		if err != nil {
			e.logger.Warn("plugin copy: failed to read source", "source", cp.Source, "error", err)
			continue
		}

		// Build a shell command that creates the parent dir and writes the file.
		// Values are single-quoted to handle paths with spaces or special chars.
		dir := filepath.Dir(cp.Target)
		writeCmd := fmt.Sprintf("mkdir -p '%s' && cat > '%s'", dir, cp.Target)
		if cp.Mode != "" {
			writeCmd += fmt.Sprintf(" && chmod '%s' '%s'", cp.Mode, cp.Target)
		}
		if cp.User != "" {
			writeCmd += fmt.Sprintf(" && chown '%s' '%s'", cp.User, cp.Target)
		}

		var shellCmd string
		if cp.IfNotExists {
			shellCmd = fmt.Sprintf("[ -f '%s' ] || { %s; }", cp.Target, writeCmd)
		} else {
			shellCmd = writeCmd
		}

		err = e.driver.ExecContainer(ctx, workspaceID, containerID,
			[]string{"sh", "-c", shellCmd},
			bytes.NewReader(data), io.Discard, io.Discard, nil, "root")
		if err != nil {
			e.logger.Warn("plugin copy: exec failed, skipping remaining copies", "target", cp.Target, "error", err)
			return
		}
	}
}
