package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	composetypes "github.com/compose-spec/compose-go/v2/types"

	composehelper "github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/dockerfile"
	"github.com/fgrehm/crib/internal/driver"
	ocidriver "github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// getuid returns the current user's UID. It is a variable so tests can override it.
var getuid = os.Getuid

// buildComposeFeatures resolves features, determines the base image, and builds
// a feature image on top.
func (e *Engine) buildComposeFeatures(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, inv composeInvocation) (*buildResult, error) {
	serviceName := cfg.Service
	svcInfo, err := composehelper.GetServiceInfo(ctx, inv.files, serviceName, inv.env)
	if err != nil {
		return nil, fmt.Errorf("loading service info: %w", err)
	}

	var baseImage string
	if svcInfo.HasBuild {
		// Build-based service: run compose build first to produce the base image.
		e.reportProgress(PhaseBuild, "Building service...")
		if err := e.compose.Build(ctx, inv.projectName, inv.files, []string{serviceName}, e.stdout, e.stderr, inv.env); err != nil {
			return nil, fmt.Errorf("building compose service: %w", err)
		}
		if svcInfo.Image != "" {
			// Service has both build and image: the image field is the tag.
			baseImage = svcInfo.Image
		} else {
			baseImage = e.compose.BuiltImageName(inv.projectName, serviceName)
		}
	} else {
		baseImage = svcInfo.Image
	}

	if baseImage == "" {
		return nil, fmt.Errorf("cannot determine base image for service %q", serviceName)
	}

	containerUser := e.resolveComposeContainerUser(ctx, cfg, svcInfo.User, baseImage)
	return e.buildComposeFeatureImage(ctx, ws, cfg, baseImage, containerUser)
}

// resolveComposeUser determines the container user for the compose service by
// querying the compose config and delegating to resolveComposeContainerUser.
// This is used before plugin dispatch so plugins get the correct remote user
// even when devcontainer.json doesn't set remoteUser/containerUser.
func (e *Engine) resolveComposeUser(ctx context.Context, cfg *config.DevContainerConfig, composeFiles []string) string {
	// If devcontainer.json already specifies a user, no need to inspect.
	if cfg.RemoteUser != "" || cfg.ContainerUser != "" {
		return ""
	}

	serviceName := cfg.Service
	svcInfo, err := composehelper.GetServiceInfo(ctx, composeFiles, serviceName, nil)
	if err != nil {
		e.logger.Debug("failed to get service info for user resolution", "error", err)
		return ""
	}

	// Determine the base image. For build-based services without an explicit
	// image tag, parse the Dockerfile to find the base image and any USER
	// instruction.
	baseImage := svcInfo.Image
	var dockerfileUser string
	if baseImage == "" && svcInfo.HasBuild {
		baseImage, dockerfileUser = e.resolveComposeDockerfileInfo(svcInfo)
	}

	// If the Dockerfile has an explicit USER instruction, prefer that.
	if dockerfileUser != "" {
		if dockerfileUser == "root" {
			return ""
		}
		return dockerfileUser
	}

	user := e.resolveComposeContainerUser(ctx, cfg, svcInfo.User, baseImage)
	if user == "root" {
		return ""
	}
	return user
}

// resolveComposeDockerfileInfo reads and parses the Dockerfile referenced by a
// build-based compose service. Returns (baseImage, user) where user is the last
// USER instruction in the Dockerfile (empty if none).
func (e *Engine) resolveComposeDockerfileInfo(svcInfo *composehelper.ServiceInfo) (string, string) {
	dockerfilePath := svcInfo.Dockerfile
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	if !filepath.IsAbs(dockerfilePath) {
		dockerfilePath = filepath.Join(svcInfo.BuildCtx, dockerfilePath)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		e.logger.Debug("failed to read Dockerfile for user resolution", "path", dockerfilePath, "error", err)
		return "", ""
	}

	df, err := dockerfile.Parse(string(content))
	if err != nil {
		e.logger.Debug("failed to parse Dockerfile for user resolution", "path", dockerfilePath, "error", err)
		return "", ""
	}

	baseImage := df.FindBaseImage(nil, "")
	user := df.FindUserStatement(nil, nil, "")
	return baseImage, user
}

// featureOverrides holds capabilities and runtime configuration declared by
// DevContainer Features, collected from image metadata.
type featureOverrides struct {
	Privileged  bool
	Init        bool
	CapAdd      []string
	SecurityOpt []string
	Env         map[string]string
	Mounts      []config.Mount
}

// collectFeatureOverrides gathers feature-declared capabilities, env, and
// mounts from image metadata, applying variable substitution to values.
func collectFeatureOverrides(metadata []*config.ImageMetadata, subCtx *config.SubstitutionContext) featureOverrides {
	sub := func(s string) string { return config.SubstituteString(subCtx, s) }

	ov := featureOverrides{Env: make(map[string]string)}
	for _, m := range metadata {
		if !ov.Privileged && m.Privileged != nil && *m.Privileged {
			ov.Privileged = true
		}
		if !ov.Init && m.Init != nil && *m.Init {
			ov.Init = true
		}
		ov.CapAdd = append(ov.CapAdd, m.CapAdd...)
		ov.SecurityOpt = append(ov.SecurityOpt, m.SecurityOpt...)
		for k, v := range m.ContainerEnv {
			ov.Env[k] = sub(v)
		}
		for _, mount := range m.Mounts {
			mount.Source = sub(mount.Source)
			mount.Target = sub(mount.Target)
			ov.Mounts = append(ov.Mounts, mount)
		}
	}
	return ov
}

// generateComposeOverride creates a compose override file with crib-specific
// configuration (labels, entrypoint, env, mounts, etc.) using compose-go types.
// featureMetadata is optional; when non-nil, feature-declared capabilities
// (privileged, init, capAdd, entrypoints) are included in the override.
func (e *Engine) generateComposeOverride(ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, composeFiles []string, featureImage string, pluginResp *plugin.PreContainerRunResponse, featureMetadata ...*config.ImageMetadata) (string, error) {
	serviceName := cfg.Service

	svc := composetypes.ServiceConfig{
		Labels: composetypes.Labels{
			ocidriver.LabelWorkspace: ws.ID,
			ocidriver.LabelHome:      e.store.BaseDir(),
		},
	}

	if featureImage != "" {
		svc.Image = featureImage
	}

	// Check if features declare entrypoints (baked into image ENTRYPOINT).
	hasFeatureEntrypoints := false
	for _, m := range featureMetadata {
		if m.Entrypoint != "" {
			hasFeatureEntrypoints = true
			break
		}
	}

	// Override entrypoint/command to keep the container alive.
	overrideCommand := cfg.OverrideCommand == nil || *cfg.OverrideCommand
	sleepCmd := sleepScript
	if overrideCommand {
		if hasFeatureEntrypoints {
			// Feature entrypoints are baked into the image. Don't
			// override entrypoint; only set command as a full command
			// so the feature entrypoint can exec into it.
			svc.Command = composetypes.ShellCommand{"/bin/sh", "-c", sleepCmd}
		} else {
			svc.Entrypoint = composetypes.ShellCommand{"/bin/sh"}
			svc.Command = composetypes.ShellCommand{"-c", sleepCmd}
		}
	}

	// Collect feature capabilities, env, and mounts.
	subCtx := &config.SubstitutionContext{
		DevContainerID:           ws.ID,
		LocalWorkspaceFolder:     ws.Source,
		ContainerWorkspaceFolder: workspaceFolder,
		Env:                      envMap(),
	}
	featOv := collectFeatureOverrides(featureMetadata, subCtx)

	svc.Privileged = featOv.Privileged
	if featOv.Init {
		initTrue := true
		svc.Init = &initTrue
	}
	svc.CapAdd = featOv.CapAdd
	svc.SecurityOpt = featOv.SecurityOpt

	svc.Environment = buildOverrideEnv(cfg, featOv, pluginResp)
	svc.Volumes = buildOverrideVolumes(ws, cfg, workspaceFolder, featOv, pluginResp)

	// Auto-inject userns_mode for rootless Podman.
	isPodman := e.isRootlessPodman() && !composeFilesContainUserns(composeFiles)
	if isPodman {
		svc.UserNSMode = "keep-id"
	}

	project := &composetypes.Project{
		Services: composetypes.Services{serviceName: svc},
	}

	project.Volumes = collectNamedVolumes(svc.Volumes)

	// Disable podman-compose pod creation (incompatible with --userns).
	if isPodman {
		project.Extensions = composetypes.Extensions{
			"x-podman": map[string]any{"in_pod": false},
		}
	}

	// Marshal and persist.
	yamlBytes, err := project.MarshalYAML()
	if err != nil {
		return "", fmt.Errorf("marshalling compose override: %w", err)
	}

	wsDir := e.store.WorkspaceDir(ws.ID)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		return "", fmt.Errorf("creating workspace directory: %w", err)
	}
	overridePath := filepath.Join(wsDir, "compose-override.yml")
	if err := os.WriteFile(overridePath, yamlBytes, 0o644); err != nil {
		return "", fmt.Errorf("writing compose override: %w", err)
	}

	return overridePath, nil
}

// buildOverrideEnv merges environment variables from config, features, and
// plugins into a single MappingWithEquals for the compose override.
func buildOverrideEnv(cfg *config.DevContainerConfig, featOv featureOverrides, pluginResp *plugin.PreContainerRunResponse) composetypes.MappingWithEquals {
	env := composetypes.MappingWithEquals{}
	for k, v := range cfg.ContainerEnv {
		env[k] = &v
	}
	for k, v := range featOv.Env {
		env[k] = &v
	}
	if pluginResp != nil {
		for k, v := range pluginResp.Env {
			env[k] = &v
		}
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

// buildOverrideVolumes assembles the service volume list from the workspace
// bind mount, feature mounts, and plugin mounts.
func buildOverrideVolumes(ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, featOv featureOverrides, pluginResp *plugin.PreContainerRunResponse) []composetypes.ServiceVolumeConfig {
	var vols []composetypes.ServiceVolumeConfig
	if cfg.WorkspaceMount == "" {
		vols = append(vols, composetypes.ServiceVolumeConfig{
			Type: "bind", Source: ws.Source, Target: workspaceFolder,
		})
	}
	for _, m := range featOv.Mounts {
		vols = append(vols, toComposeVolume(m))
	}
	if pluginResp != nil {
		for _, m := range pluginResp.Mounts {
			vols = append(vols, toComposeVolume(m))
		}
	}
	return vols
}

// collectNamedVolumes returns top-level volume declarations for any "volume"
// type mounts. Compose rejects unknown volume references without these.
func collectNamedVolumes(vols []composetypes.ServiceVolumeConfig) composetypes.Volumes {
	named := composetypes.Volumes{}
	for _, v := range vols {
		if v.Type == "volume" {
			named[v.Source] = composetypes.VolumeConfig{Name: v.Source}
		}
	}
	if len(named) == 0 {
		return nil
	}
	return named
}

// toComposeVolume converts a crib config.Mount to a compose ServiceVolumeConfig.
func toComposeVolume(m config.Mount) composetypes.ServiceVolumeConfig {
	typ := m.Type
	if typ == "" {
		typ = "bind"
	}
	return composetypes.ServiceVolumeConfig{
		Type: typ, Source: m.Source, Target: m.Target,
	}
}

// composeDown wraps compose.Down, including a temporary x-podman override when
// running rootless Podman. Without this override, podman-compose tries to
// remove a pod that was never created (because Up used in_pod: false).
func (e *Engine) composeDown(ctx context.Context, inv composeInvocation, removeVolumes bool) error {
	files := inv.files
	if overridePath, ok := e.writePodmanDownOverride(inv.files); ok {
		defer func() { _ = os.Remove(overridePath) }()
		files = append(inv.files[:len(inv.files):len(inv.files)], overridePath)
	}
	return e.compose.Down(ctx, inv.projectName, files, e.composeStdout(), e.composeStderr(), inv.env, removeVolumes)
}

// writePodmanDownOverride creates a temporary override file for podman-compose
// down that disables pod creation. Returns the file path and true if written,
// or empty string and false if not needed.
func (e *Engine) writePodmanDownOverride(composeFiles []string) (string, bool) {
	if !e.isRootlessPodman() || composeFilesContainUserns(composeFiles) {
		return "", false
	}
	f, err := os.CreateTemp("", "crib-podman-down-override-*.yml")
	if err != nil {
		return "", false
	}
	path := f.Name()
	if _, err := f.WriteString("x-podman:\n  in_pod: false\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", false
	}
	_ = f.Close()
	return path, true
}

// isRootlessPodman returns true when the compose runtime is Podman and the
// current process is not running as root.
func (e *Engine) isRootlessPodman() bool {
	if e.compose == nil {
		return false
	}
	return strings.Contains(e.compose.RuntimeCommand(), "podman") && getuid() != 0
}

// composeFilesContainUserns checks whether any of the given compose files
// already contain a userns_mode directive. This is a simple text search to
// avoid pulling in a full YAML parser.
func composeFilesContainUserns(files []string) bool {
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "userns_mode") {
			return true
		}
	}
	return false
}

// ensureContainerRunning verifies that the container is in a running state.
// When the container's State is empty (fallback path from compose ps), it
// queries the driver for full container details. Returns an error with
// container log output if the container is not running.
func (e *Engine) ensureContainerRunning(ctx context.Context, workspaceID string, container *driver.ContainerDetails) error {
	state := container.State

	// When State is empty (compose ps fallback returned only an ID),
	// inspect the container to get the actual state.
	if state.Status == "" {
		inspected, err := e.driver.FindContainer(ctx, workspaceID)
		if err == nil && inspected != nil {
			state = inspected.State
		}
	}

	if state.IsRunning() {
		return nil
	}

	status := state.Status
	if status == "" {
		status = "unknown"
	}

	// Collect recent logs for a useful error message.
	var logBuf bytes.Buffer
	_ = e.driver.ContainerLogs(ctx, workspaceID, container.ID, &logBuf, &logBuf, nil)

	logSnippet := logBuf.String()
	const maxLogLen = 500
	if len(logSnippet) > maxLogLen {
		logSnippet = logSnippet[len(logSnippet)-maxLogLen:]
	}

	if logSnippet != "" {
		return fmt.Errorf("dev container is not running (status: %s). Last logs:\n%s", status, logSnippet)
	}
	return fmt.Errorf("dev container is not running (status: %s)", status)
}

// findComposeContainer finds a compose container by trying FindContainer first,
// then falling back to compose ps to handle podman/docker delegation.
// stage is used in error messages (e.g. "after up", "after restart").
func (e *Engine) findComposeContainer(ctx context.Context, workspaceID string, inv composeInvocation, stage string) (*driver.ContainerDetails, error) {
	// Try FindContainer first (using labels).
	container, err := e.driver.FindContainer(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("finding compose container %s: %w", stage, err)
	}
	if container != nil {
		return container, nil
	}

	// Fallback to compose ps --format json to find the service container.
	// This handles podman compose -> docker-compose delegation where crib
	// labels might not be visible to podman. We filter by service name to
	// avoid returning the wrong container (e.g. postgres instead of the
	// primary dev container). podman-compose doesn't support
	// `compose ps -q <service>`, so we use JSON output and match the
	// compose service label instead.
	e.logger.Debug("FindContainer returned nil, trying compose ps", "stage", stage)
	containerID, err := e.compose.FindServiceContainerID(ctx, inv.projectName, inv.files, inv.service, inv.env)
	if err != nil {
		return nil, fmt.Errorf("compose container not found %s and ps failed: %w", stage, err)
	}
	if containerID == "" {
		return nil, fmt.Errorf("compose container not found %s", stage)
	}

	return &driver.ContainerDetails{
		ID: containerID,
	}, nil
}

// ensureServiceIncluded returns services with name added if not already present.
// When runServices is empty, returns [name] so the primary service always starts.
func ensureServiceIncluded(runServices []string, name string) []string {
	if len(runServices) == 0 {
		return []string{name}
	}
	if slices.Contains(runServices, name) {
		return runServices
	}
	return append([]string{name}, runServices...)
}

// removeService returns a copy of services with name removed.
func removeService(services []string, name string) []string {
	var result []string
	for _, s := range services {
		if s != name {
			result = append(result, s)
		}
	}
	return result
}
