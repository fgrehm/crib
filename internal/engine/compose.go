package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	composehelper "github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	ocidriver "github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/workspace"
)

// getuid returns the current user's UID. It is a variable so tests can override it.
var getuid = os.Getuid

// upCompose handles the Docker Compose devcontainer path.
func (e *Engine) upCompose(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, opts UpOptions) (*UpResult, error) {
	if e.compose == nil {
		return nil, fmt.Errorf("compose is not available (install docker compose or podman compose)")
	}

	configDir := filepath.Dir(cfg.Origin)
	projectName := composehelper.ProjectName(ws.ID)

	// Resolve compose file paths.
	composeFiles := resolveComposeFiles(configDir, cfg.DockerComposeFile)

	serviceName := cfg.Service
	if serviceName == "" {
		return nil, fmt.Errorf("dockerComposeFile is set but service is not specified")
	}

	// Devcontainer variables for ${VAR} substitution in compose files.
	dcEnv := devcontainerEnv(ws.ID, ws.Source, workspaceFolder)

	// Check for existing container.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding container: %w", err)
	}

	if container != nil && !opts.Recreate {
		if !container.State.IsRunning() {
			// Generate the same override file used during initial up so that
			// crib labels, userns_mode, and x-podman settings are applied on
			// restart. Without these, rootless Podman creates a pod that
			// conflicts with --userns=keep-id.
			overridePath, err := e.generateComposeOverride(ws, cfg, workspaceFolder, configDir, composeFiles, "" /* featureImage already baked in */)
			if err != nil {
				return nil, fmt.Errorf("generating compose override: %w", err)
			}
			defer func() { _ = os.Remove(overridePath) }()

			allFiles := append(composeFiles[:len(composeFiles):len(composeFiles)], overridePath)
			services := ensureServiceIncluded(cfg.RunServices, serviceName)

			e.reportProgress("Starting services...")
			if err := e.compose.Up(ctx, projectName, allFiles, services, e.composeStdout(), e.stderr, dcEnv); err != nil {
				return nil, fmt.Errorf("starting compose services: %w", err)
			}
			container, err = e.findComposeContainer(ctx, ws.ID, projectName, allFiles, dcEnv, "after restart")
			if err != nil {
				return nil, err
			}
		} else {
			e.reportProgress("Services already running")
		}

		return e.setupAndReturn(ctx, ws, cfg, container.ID, workspaceFolder)
	}

	// No container but a previous result exists (e.g. after "crib down").
	// Images are already built, so skip the build and just bring services up.
	if container == nil && !opts.Recreate {
		if storedResult, err := e.store.LoadResult(ws.ID); err == nil && storedResult != nil {
			return e.upComposeFromStored(ctx, ws, cfg, workspaceFolder, configDir, composeFiles, projectName, dcEnv, serviceName, storedResult)
		}
	}

	// Remove existing containers if recreating.
	if container != nil && opts.Recreate {
		e.reportProgress("Removing services...")
		if err := e.store.ClearHookMarkers(ws.ID); err != nil {
			e.logger.Warn("failed to clear hook markers", "error", err)
		}
		if err := e.composeDown(ctx, projectName, composeFiles, dcEnv); err != nil {
			e.logger.Warn("failed to bring down existing services", "error", err)
		}
	}

	// Build feature image if features are configured.
	var featureImage string
	if len(cfg.Features) > 0 {
		img, err := e.buildComposeFeatures(ctx, ws, cfg, projectName, composeFiles, dcEnv, serviceName)
		if err != nil {
			return nil, fmt.Errorf("building compose features: %w", err)
		}
		featureImage = img
	}

	// Generate override compose file for crib-specific configuration.
	overridePath, err := e.generateComposeOverride(ws, cfg, workspaceFolder, configDir, composeFiles, featureImage)
	if err != nil {
		return nil, fmt.Errorf("generating compose override: %w", err)
	}
	defer func() { _ = os.Remove(overridePath) }()

	allFiles := append(composeFiles[:len(composeFiles):len(composeFiles)], overridePath)

	// Build services. When features were built for the primary service,
	// only build the remaining services (the primary was already built).
	services := ensureServiceIncluded(cfg.RunServices, serviceName)
	if featureImage != "" {
		others := removeService(services, serviceName)
		if len(others) > 0 {
			e.reportProgress("Building services...")
			if err := e.compose.Build(ctx, projectName, allFiles, others, e.stdout, e.stderr, dcEnv); err != nil {
				return nil, fmt.Errorf("building compose services: %w", err)
			}
		}
	} else {
		e.reportProgress("Building services...")
		if err := e.compose.Build(ctx, projectName, allFiles, nil, e.stdout, e.stderr, dcEnv); err != nil {
			return nil, fmt.Errorf("building compose services: %w", err)
		}
	}

	// Bring up services. Always include the primary service regardless of runServices.
	e.reportProgress("Starting services...")
	if err := e.compose.Up(ctx, projectName, allFiles, services, e.composeStdout(), e.stderr, dcEnv); err != nil {
		return nil, fmt.Errorf("starting compose services: %w", err)
	}

	// Find the primary service container.
	container, err = e.findComposeContainer(ctx, ws.ID, projectName, allFiles, dcEnv, "after up")
	if err != nil {
		return nil, err
	}

	result, setupErr := e.setupAndReturn(ctx, ws, cfg, container.ID, workspaceFolder)
	if result != nil && featureImage != "" {
		result.ImageName = featureImage
		e.saveResult(ws, cfg, result)
	}
	return result, setupErr
}

// upComposeFromStored handles "crib up" after "crib down" when images are
// already built. It generates the compose override using the stored feature
// image (if any) and brings services up without rebuilding.
func (e *Engine) upComposeFromStored(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder, configDir string, composeFiles []string, projectName string, dcEnv []string, serviceName string, storedResult *workspace.Result) (*UpResult, error) {
	e.logger.Debug("compose up from stored result (skipping build)")

	// Use the stored feature image name in the override so compose uses
	// the previously built image instead of the base service image.
	featureImage := storedResult.ImageName

	overridePath, err := e.generateComposeOverride(ws, cfg, workspaceFolder, configDir, composeFiles, featureImage)
	if err != nil {
		return nil, fmt.Errorf("generating compose override: %w", err)
	}
	defer func() { _ = os.Remove(overridePath) }()

	allFiles := append(composeFiles[:len(composeFiles):len(composeFiles)], overridePath)
	services := ensureServiceIncluded(cfg.RunServices, serviceName)

	e.reportProgress("Starting services...")
	if err := e.compose.Up(ctx, projectName, allFiles, services, e.composeStdout(), e.stderr, dcEnv); err != nil {
		return nil, fmt.Errorf("starting compose services: %w", err)
	}

	container, err := e.findComposeContainer(ctx, ws.ID, projectName, allFiles, dcEnv, "after up")
	if err != nil {
		return nil, err
	}

	result, setupErr := e.setupAndReturn(ctx, ws, cfg, container.ID, workspaceFolder)
	if result != nil && featureImage != "" {
		result.ImageName = featureImage
		e.saveResult(ws, cfg, result)
	}
	return result, setupErr
}

// buildComposeFeatures resolves features, determines the base image, and builds
// a feature image on top. Returns the feature image name.
func (e *Engine) buildComposeFeatures(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, projectName string, composeFiles []string, dcEnv []string, serviceName string) (string, error) {
	svcInfo, err := composehelper.GetServiceInfo(ctx, composeFiles, serviceName, dcEnv)
	if err != nil {
		return "", fmt.Errorf("loading service info: %w", err)
	}

	var baseImage string
	if svcInfo.HasBuild {
		// Build-based service: run compose build first to produce the base image.
		e.reportProgress("Building service...")
		if err := e.compose.Build(ctx, projectName, composeFiles, []string{serviceName}, e.stdout, e.stderr, dcEnv); err != nil {
			return "", fmt.Errorf("building compose service: %w", err)
		}
		if svcInfo.Image != "" {
			// Service has both build and image: the image field is the tag.
			baseImage = svcInfo.Image
		} else {
			baseImage = e.compose.BuiltImageName(projectName, serviceName)
		}
	} else {
		baseImage = svcInfo.Image
	}

	if baseImage == "" {
		return "", fmt.Errorf("cannot determine base image for service %q", serviceName)
	}

	containerUser := e.resolveComposeContainerUser(ctx, cfg, svcInfo.User, baseImage)
	result, err := e.buildComposeFeatureImage(ctx, ws, cfg, baseImage, containerUser)
	if err != nil {
		return "", err
	}

	return result.imageName, nil
}

// generateComposeOverride creates a temporary compose override file that adds
// crib-specific labels and configuration to the target service.
func (e *Engine) generateComposeOverride(ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder, configDir string, composeFiles []string, featureImage string) (string, error) {
	serviceName := cfg.Service

	// Build the override YAML.
	var b strings.Builder
	b.WriteString("services:\n")
	fmt.Fprintf(&b, "  %s:\n", serviceName)

	// Add workspace label for container discovery.
	b.WriteString("    labels:\n")
	fmt.Fprintf(&b, "      %s: %q\n", ocidriver.LabelWorkspace, ws.ID)

	// Override image if a feature image was built.
	if featureImage != "" {
		fmt.Fprintf(&b, "    image: %s\n", featureImage)
	}

	// Override entrypoint/command to keep the container alive.
	overrideCommand := cfg.OverrideCommand == nil || *cfg.OverrideCommand
	if overrideCommand {
		b.WriteString("    entrypoint: /bin/sh\n")
		b.WriteString("    command:\n")
		b.WriteString("      - -c\n")
		b.WriteString("      - 'echo Container started; trap \"exit 0\" 15; exec \"$@\"; sleep infinity'\n")
	}

	// Container environment.
	if len(cfg.ContainerEnv) > 0 {
		b.WriteString("    environment:\n")
		for k, v := range cfg.ContainerEnv {
			fmt.Fprintf(&b, "      %s: %q\n", k, v)
		}
	}

	// Workspace mount (volumes).
	if cfg.WorkspaceMount == "" {
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:%s\n", ws.Source, workspaceFolder)
	}

	// Auto-inject userns_mode: "keep-id" for rootless Podman to fix bind mount
	// permissions. Skip if the user already set userns_mode in their compose files.
	// Also disable podman-compose pod creation (x-podman.in_pod) because
	// --userns and --pod are incompatible in Podman.
	if e.isRootlessPodman() && !composeFilesContainUserns(composeFiles) {
		b.WriteString("    userns_mode: \"keep-id\"\n")
		b.WriteString("x-podman:\n")
		b.WriteString("  in_pod: false\n")
	}

	// Write to temp file.
	overridePath := filepath.Join(configDir, ".crib-compose-override.yml")
	if err := os.WriteFile(overridePath, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("writing compose override: %w", err)
	}

	return overridePath, nil
}

// composeDown wraps compose.Down, including a temporary x-podman override when
// running rootless Podman. Without this override, podman-compose tries to
// remove a pod that was never created (because Up used in_pod: false).
func (e *Engine) composeDown(ctx context.Context, projectName string, composeFiles []string, env []string) error {
	files := composeFiles
	if overridePath, ok := e.writePodmanDownOverride(composeFiles); ok {
		defer func() { _ = os.Remove(overridePath) }()
		files = append(composeFiles[:len(composeFiles):len(composeFiles)], overridePath)
	}
	return e.compose.Down(ctx, projectName, files, e.composeStdout(), e.stderr, env)
}

// writePodmanDownOverride creates a temporary override file for podman-compose
// down that disables pod creation. Returns the file path and true if written,
// or empty string and false if not needed.
func (e *Engine) writePodmanDownOverride(composeFiles []string) (string, bool) {
	if !e.isRootlessPodman() || composeFilesContainUserns(composeFiles) {
		return "", false
	}
	dir := filepath.Dir(composeFiles[0])
	path := filepath.Join(dir, ".crib-podman-down-override.yml")
	if err := os.WriteFile(path, []byte("x-podman:\n  in_pod: false\n"), 0o644); err != nil {
		return "", false
	}
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

// findComposeContainer finds a compose container by trying FindContainer first,
// then falling back to compose ps to handle podman/docker delegation.
// stage is used in error messages (e.g. "after up", "after restart").
func (e *Engine) findComposeContainer(ctx context.Context, workspaceID, projectName string, files []string, env []string, stage string) (*driver.ContainerDetails, error) {
	// Try FindContainer first (using labels).
	container, err := e.driver.FindContainer(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("finding compose container %s: %w", stage, err)
	}
	if container != nil {
		return container, nil
	}

	// Fallback to compose ps to handle podman compose -> docker-compose delegation
	// where labels might not be visible to podman.
	e.logger.Debug("FindContainer returned nil, trying compose ps", "stage", stage)
	containerIDs, err := e.compose.ListContainers(ctx, projectName, files, env)
	if err != nil {
		return nil, fmt.Errorf("compose container not found %s and ps failed: %w", stage, err)
	}
	if len(containerIDs) == 0 {
		return nil, fmt.Errorf("compose container not found %s", stage)
	}

	// Return container details with ID from compose.
	return &driver.ContainerDetails{
		ID: containerIDs[0],
	}, nil
}

// ensureServiceIncluded returns services with name added if not already present.
// When runServices is empty, returns [name] so the primary service always starts.
func ensureServiceIncluded(runServices []string, name string) []string {
	if len(runServices) == 0 {
		return []string{name}
	}
	for _, s := range runServices {
		if s == name {
			return runServices
		}
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
