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
// These are arguments to defaultEntrypoint ("/bin/sh").
var defaultCmd = []string{"-c", "echo Container started; trap \"exit 0\" 15; exec \"$@\"; sleep infinity"}

// featureCmd is used when features set an ENTRYPOINT in the image.
// The feature entrypoint chains via exec "$@", so CMD must be a full command.
var featureCmd = []string{"/bin/sh", "-c", "echo Container started; trap \"exit 0\" 15; exec \"$@\"; sleep infinity"}

// buildRunOptions constructs RunOptions from the devcontainer config.
// hasFeatureEntrypoints indicates the image has feature-declared entrypoints
// baked in via ENTRYPOINT; when true, overrideCommand only sets CMD.
func (e *Engine) buildRunOptions(cfg *config.DevContainerConfig, imageName, projectRoot, workspaceFolder string, hasFeatureEntrypoints bool) (*driver.RunOptions, error) {
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
		if hasFeatureEntrypoints {
			// Feature entrypoints are baked into the image as ENTRYPOINT.
			// They chain via exec "$@", so we only set CMD to keep the
			// container alive. The entrypoint starts its daemons, then
			// execs into the sleep loop.
			opts.Cmd = featureCmd
		} else {
			opts.Entrypoint = defaultEntrypoint
			opts.Cmd = defaultCmd
		}
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

// applyFeatureMetadata merges feature-declared runtime capabilities into the
// run options. These are capabilities like privileged, init, capAdd that
// features declare in devcontainer-feature.json but can only be applied at
// container creation time (not in the Dockerfile).
// subCtx is used to substitute variables (e.g. ${devcontainerId}) in mount
// sources and containerEnv values. If nil, no substitution is performed.
func applyFeatureMetadata(opts *driver.RunOptions, metadata []*config.ImageMetadata, subCtx *config.SubstitutionContext) {
	sub := func(s string) string {
		if subCtx == nil {
			return s
		}
		return config.SubstituteString(subCtx, s)
	}
	for _, m := range metadata {
		if m.Privileged != nil && *m.Privileged {
			opts.Privileged = true
		}
		if m.Init != nil && *m.Init {
			opts.Init = true
		}
		opts.CapAdd = append(opts.CapAdd, m.CapAdd...)
		opts.SecurityOpt = append(opts.SecurityOpt, m.SecurityOpt...)
		for _, mount := range m.Mounts {
			mount.Source = sub(mount.Source)
			mount.Target = sub(mount.Target)
			opts.Mounts = append(opts.Mounts, mount)
		}
		for k, v := range m.ContainerEnv {
			opts.Env = append(opts.Env, k+"="+sub(v))
		}
	}
}

// chownPluginVolumes changes ownership of plugin volume mounts to the
// remote user. Docker/Podman create volumes with root ownership, so
// non-root users get permission errors when writing to them.
func (e *Engine) chownPluginVolumes(ctx context.Context, cc containerContext, mounts []config.Mount) {
	for _, m := range mounts {
		if m.Type != "volume" {
			continue
		}
		cmd := []string{"chown", cc.remoteUser + ":", m.Target}
		if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, io.Discard, io.Discard, nil, "root"); err != nil {
			e.logger.Debug("chown plugin volume failed", "target", m.Target, "error", err)
		}
	}
}

// detectContainerUser runs whoami inside the container to detect the default
// user. Returns empty string on failure or if the user is root.
func (e *Engine) detectContainerUser(ctx context.Context, cc containerContext) string {
	var stdout bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, []string{"whoami"}, nil, &stdout, io.Discard, nil, ""); err != nil {
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
// Error handling policy varies by caller:
//   - Fresh creation paths (upCreate, upFromImage, restartRecreate) treat errors
//     as fatal because the container hasn't been wired with plugin mounts/env yet.
//   - Resume paths (upExisting, restartSimple) treat errors as non-fatal (warn +
//     nil pluginResp) because the container already has its mounts from creation.
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

	req := &plugin.PreContainerRunRequest{
		WorkspaceID:     ws.ID,
		WorkspaceDir:    e.store.WorkspaceDir(ws.ID),
		SourceDir:       ws.Source,
		Runtime:         e.runtimeName,
		ImageName:       imageName,
		RemoteUser:      remoteUser,
		WorkspaceFolder: workspaceFolder,
		ContainerName:   "crib-" + ws.ID,
		Customizations:  extractCribCustomizations(cfg),
	}

	resp, err := e.plugins.RunPreContainerRun(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("running pre-container-run plugins: %w", err)
	}

	return resp, nil
}

// dispatchPostContainerCreate runs the post-container-create event for all
// plugins that implement PostContainerCreator. Called from finalize after
// file copies and volume chown.
func (e *Engine) dispatchPostContainerCreate(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, cc containerContext) {
	// Use configRemoteUser (from devcontainer.json) rather than cc.remoteUser,
	// which hasn't been resolved yet at this point in the finalize flow.
	remoteUser := cc.remoteUser
	if remoteUser == "" {
		remoteUser = configRemoteUser(cfg)
	}

	req := &plugin.PostContainerCreateRequest{
		WorkspaceID:     ws.ID,
		WorkspaceDir:    e.store.WorkspaceDir(ws.ID),
		ContainerID:     cc.containerID,
		RemoteUser:      remoteUser,
		WorkspaceFolder: cc.workspaceFolder,
		Customizations:  extractCribCustomizations(cfg),
		Runtime:         e.runtimeName,
		ExecFunc: func(ctx context.Context, cmd []string, user string) error {
			return e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID,
				cmd, nil, io.Discard, io.Discard, nil, user)
		},
		ExecOutputFunc: func(ctx context.Context, cmd []string, user string) (string, error) {
			var buf bytes.Buffer
			err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID,
				cmd, nil, &buf, io.Discard, nil, user)
			return buf.String(), err
		},
		CopyFileFunc: func(ctx context.Context, content []byte, destPath, mode, user string) error {
			dir := plugin.ShellQuote(filepath.Dir(destPath))
			dest := plugin.ShellQuote(destPath)
			writeCmd := fmt.Sprintf("mkdir -p '%s' && cat > '%s'", dir, dest)
			if mode != "" {
				writeCmd += fmt.Sprintf(" && chmod '%s' '%s'", plugin.ShellQuote(mode), dest)
			}
			if user != "" {
				writeCmd += fmt.Sprintf(" && chown '%s' '%s'", plugin.ShellQuote(user), dest)
			}
			return e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID,
				[]string{"sh", "-c", writeCmd}, bytes.NewReader(content),
				io.Discard, io.Discard, nil, "root")
		},
	}

	e.plugins.RunPostContainerCreate(ctx, req)
}

// execPluginCopies copies staged files into the container via exec.
// All values are shell-escaped before embedding in single-quoted arguments.
func (e *Engine) execPluginCopies(ctx context.Context, cc containerContext, copies []plugin.FileCopy) {
	for _, cp := range copies {
		data, err := os.ReadFile(cp.Source)
		if err != nil {
			e.logger.Warn("plugin copy: failed to read source", "source", cp.Source, "error", err)
			continue
		}

		// Build a shell command that creates the parent dir and writes the file.
		// Values are shell-escaped and single-quoted to handle paths with
		// spaces, special chars, or single quotes.
		dir := plugin.ShellQuote(filepath.Dir(cp.Target))
		target := plugin.ShellQuote(cp.Target)
		writeCmd := fmt.Sprintf("mkdir -p '%s' && cat > '%s'", dir, target)
		if cp.Mode != "" {
			writeCmd += fmt.Sprintf(" && chmod '%s' '%s'", plugin.ShellQuote(cp.Mode), target)
		}
		if cp.User != "" {
			writeCmd += fmt.Sprintf(" && chown '%s' '%s' '%s'", plugin.ShellQuote(cp.User), dir, target)
		}

		var shellCmd string
		if cp.IfNotExists {
			shellCmd = fmt.Sprintf("[ -f '%s' ] || { %s; }", target, writeCmd)
		} else {
			shellCmd = writeCmd
		}

		err = e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID,
			[]string{"sh", "-c", shellCmd},
			bytes.NewReader(data), io.Discard, io.Discard, nil, "root")
		if err != nil {
			e.logger.Warn("plugin copy: exec failed, skipping remaining copies", "target", cp.Target, "error", err)
			return
		}
	}
}

// extractCribCustomizations returns the customizations.crib map from a
// devcontainer config, or nil if not present.
func extractCribCustomizations(cfg *config.DevContainerConfig) map[string]any {
	if cfg.Customizations == nil {
		return nil
	}
	crib, ok := cfg.Customizations["crib"]
	if !ok {
		return nil
	}
	m, ok := crib.(map[string]any)
	if !ok {
		return nil
	}
	return m
}
