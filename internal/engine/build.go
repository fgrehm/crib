package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/dockerfile"
	"github.com/fgrehm/crib/internal/driver"
	ocidriver "github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/feature"
	"github.com/fgrehm/crib/internal/workspace"
)

// buildResult holds the outcome of an image build.
type buildResult struct {
	imageName      string
	imageMetadata  []*config.ImageMetadata
	hasEntrypoints bool // true if any feature declared an entrypoint
}

// buildImage handles image building for the single container path.
// It resolves features, generates the final Dockerfile, and builds.
func (e *Engine) buildImage(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig) (*buildResult, error) {
	configDir := filepath.Dir(cfg.Origin)

	// Determine image user for feature generation.
	containerUser := resolveContainerUser(cfg)

	// Resolve and order features.
	features, err := e.resolveFeatures(cfg, configDir)
	if err != nil {
		return nil, err
	}

	// Determine the build approach.
	if cfg.Image != "" {
		return e.buildFromImage(ctx, ws, cfg, features, containerUser)
	}
	return e.buildFromDockerfile(ctx, ws, cfg, features, containerUser)
}

// buildFromImage handles the image-based devcontainer path.
// If features are specified, generates a Dockerfile that extends the base image.
func (e *Engine) buildFromImage(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, features []*feature.FeatureSet, containerUser string) (*buildResult, error) {
	if len(features) == 0 {
		// No features, no build needed. Just use the image directly.
		return &buildResult{imageName: cfg.Image}, nil
	}

	// Generate a Dockerfile that installs features on top of the base image.
	remoteUser := cfg.RemoteUser
	if remoteUser == "" {
		remoteUser = containerUser
	}

	featureContent, featurePrefix := feature.GenerateDockerfile(features, containerUser, remoteUser, e.buildCacheMounts)
	// Replace the placeholder so FROM $_DEV_CONTAINERS_BASE_IMAGE resolves to
	// the actual image instead of the literal string "placeholder".
	featurePrefix = strings.ReplaceAll(featurePrefix, "=placeholder", "="+cfg.Image)
	dockerfileContent := featurePrefix + "\n" + featureContent

	return e.doBuild(ctx, ws, cfg, dockerfileContent, features, containerUser, remoteUser)
}

// buildFromDockerfile handles the Dockerfile-based devcontainer path.
func (e *Engine) buildFromDockerfile(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, features []*feature.FeatureSet, containerUser string) (*buildResult, error) {
	dockerfilePath := config.GetDockerfilePath(cfg)
	if dockerfilePath == "" {
		return nil, fmt.Errorf("no image or Dockerfile specified in devcontainer.json")
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return nil, fmt.Errorf("reading Dockerfile %s: %w", dockerfilePath, err)
	}

	dockerfileContent := string(content)
	remoteUser := cfg.RemoteUser
	if remoteUser == "" {
		remoteUser = containerUser
	}

	if len(features) > 0 {
		// Parse Dockerfile to find the base image and user.
		df, err := dockerfile.Parse(dockerfileContent)
		if err != nil {
			return nil, fmt.Errorf("parsing Dockerfile: %w", err)
		}

		buildTarget := ""
		if cfg.Build != nil {
			buildTarget = cfg.Build.Target
		}

		// Determine the container user from the Dockerfile if not set.
		if containerUser == "" {
			containerUser = df.FindUserStatement(nil, nil, buildTarget)
		}
		if remoteUser == "" {
			remoteUser = containerUser
		}

		// Ensure the final stage has a name for feature overlay.
		stageName, modifiedContent, err := dockerfile.EnsureFinalStageName(dockerfileContent, "crib_feature_base")
		if err != nil {
			return nil, fmt.Errorf("ensuring stage name: %w", err)
		}
		if modifiedContent != "" {
			dockerfileContent = modifiedContent
		}

		// Generate feature Dockerfile layers.
		featureContent, featurePrefix := feature.GenerateDockerfile(features, containerUser, remoteUser, e.buildCacheMounts)

		// Replace the placeholder so FROM $_DEV_CONTAINERS_BASE_IMAGE resolves
		// to the user's final Dockerfile stage.
		featurePrefix = strings.ReplaceAll(featurePrefix, "=placeholder", "="+stageName)

		// Strip existing syntax directives and prepend the feature prefix.
		dockerfileContent = dockerfile.RemoveSyntaxVersion(dockerfileContent)
		dockerfileContent = featurePrefix + "\n" + dockerfileContent + "\n" + featureContent
	}

	return e.doBuild(ctx, ws, cfg, dockerfileContent, features, containerUser, remoteUser)
}

// doBuild writes the final Dockerfile and invokes the driver to build.
func (e *Engine) doBuild(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, dockerfileContent string, features []*feature.FeatureSet, containerUser, remoteUser string) (*buildResult, error) {
	contextPath := config.GetContextPath(cfg)

	// Prepare feature build context if features exist.
	if len(features) > 0 {
		featuresDir, err := feature.PrepareContext(contextPath, features, containerUser, remoteUser)
		if err != nil {
			return nil, fmt.Errorf("preparing feature context: %w", err)
		}
		defer func() { _ = os.RemoveAll(featuresDir) }()
	}

	// Write the generated Dockerfile to the context.
	tmpDockerfile := filepath.Join(contextPath, ".crib-Dockerfile")
	if err := os.WriteFile(tmpDockerfile, []byte(dockerfileContent), 0o644); err != nil {
		return nil, fmt.Errorf("writing generated Dockerfile: %w", err)
	}
	defer func() { _ = os.Remove(tmpDockerfile) }()

	// Persist a copy in workspace state for troubleshooting.
	wsDir := e.store.WorkspaceDir(ws.ID)
	if err := os.MkdirAll(wsDir, 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(wsDir, "Dockerfile"), []byte(dockerfileContent), 0o644)
	}

	// Calculate prebuild hash for cache tag.
	arch, _ := e.driver.TargetArchitecture(ctx)
	hash, err := config.CalculatePrebuildHash(config.PrebuildHashParams{
		Config:            cfg,
		Platform:          arch,
		ContextPath:       contextPath,
		DockerfileContent: dockerfileContent,
	})
	if err != nil {
		e.logger.Warn("failed to calculate prebuild hash, using latest", "error", err)
		hash = "latest"
	}

	imageName := ocidriver.ImageName(ws.ID, hash)

	// Collect feature metadata regardless of cache hit. Runtime capabilities
	// (privileged, mounts, entrypoints) must be applied even when the image
	// is already built.
	var metadata []*config.ImageMetadata
	hasEntrypoints := false
	for _, f := range features {
		metadata = append(metadata, featureToMetadata(f))
		if f.Config.Entrypoint != "" {
			hasEntrypoints = true
		}
	}

	// Check if image already exists.
	if _, inspErr := e.driver.InspectImage(ctx, imageName); inspErr == nil {
		e.reportProgress("Image cached, skipping build")
		return &buildResult{
			imageName:      imageName,
			imageMetadata:  metadata,
			hasEntrypoints: hasEntrypoints,
		}, nil
	}

	// Build args from config.
	buildArgs := make(map[string]string)
	if cfg.Build != nil && cfg.Build.Args != nil {
		for k, v := range cfg.Build.Args {
			if v != nil {
				buildArgs[k] = *v
			}
		}
	}

	buildTarget := ""
	if cfg.Build != nil {
		buildTarget = cfg.Build.Target
	}

	var cacheFrom []string
	if cfg.Build != nil {
		cacheFrom = cfg.Build.CacheFrom
	}

	var buildOptions []string
	if cfg.Build != nil {
		buildOptions = cfg.Build.Options
	}

	// Clean up previous build image if hash changed.
	e.cleanupPreviousBuildImage(ctx, ws.ID, imageName)

	e.reportProgress("Building image...")
	err = e.driver.BuildImage(ctx, ws.ID, &driver.BuildOptions{
		PrebuildHash: hash,
		Image:        imageName,
		Dockerfile:   tmpDockerfile,
		Context:      contextPath,
		Args:         buildArgs,
		Target:       buildTarget,
		CacheFrom:    cacheFrom,
		Labels:       map[string]string{ocidriver.LabelWorkspace: ws.ID},
		Options:      buildOptions,
		Stdout:       e.stdout,
		Stderr:       e.stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("building image: %w", err)
	}

	return &buildResult{
		imageName:      imageName,
		imageMetadata:  metadata,
		hasEntrypoints: hasEntrypoints,
	}, nil
}

// buildComposeFeatureImage builds a feature image on top of a compose service's
// base image. Returns the base image name directly if no features are configured.
func (e *Engine) buildComposeFeatureImage(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, baseImage, containerUser string) (*buildResult, error) {
	configDir := filepath.Dir(cfg.Origin)

	features, err := e.resolveFeatures(cfg, configDir)
	if err != nil {
		return nil, err
	}
	if len(features) == 0 {
		return &buildResult{imageName: baseImage}, nil
	}

	remoteUser := cfg.RemoteUser
	if remoteUser == "" {
		remoteUser = containerUser
	}

	featureContent, featurePrefix := feature.GenerateDockerfile(features, containerUser, remoteUser, e.buildCacheMounts)
	featurePrefix = strings.ReplaceAll(featurePrefix, "=placeholder", "="+baseImage)
	dockerfileContent := featurePrefix + "\n" + featureContent

	return e.doBuild(ctx, ws, cfg, dockerfileContent, features, containerUser, remoteUser)
}

// resolveComposeContainerUser determines the container user for a compose
// service. Checks devcontainer.json fields first, then the compose service
// user, then inspects the base image.
func (e *Engine) resolveComposeContainerUser(ctx context.Context, cfg *config.DevContainerConfig, serviceUser, baseImage string) string {
	if cfg.ContainerUser != "" {
		return cfg.ContainerUser
	}
	if cfg.RemoteUser != "" {
		return cfg.RemoteUser
	}
	if serviceUser != "" {
		return serviceUser
	}
	if baseImage != "" {
		if details, err := e.driver.InspectImage(ctx, baseImage); err == nil && details.Config.User != "" {
			return details.Config.User
		}
	}
	return "root"
}

// resolveFeatures resolves and orders features from the config.
func (e *Engine) resolveFeatures(cfg *config.DevContainerConfig, configDir string) ([]*feature.FeatureSet, error) {
	if len(cfg.Features) == 0 {
		return nil, nil
	}

	cache, err := feature.NewFeatureCache()
	if err != nil {
		return nil, fmt.Errorf("initializing feature cache: %w", err)
	}
	resolver := feature.NewCompositeResolver(cache)
	var features []*feature.FeatureSet

	for id, opts := range cfg.Features {
		folder, err := resolver.Resolve(id, configDir)
		if err != nil {
			return nil, fmt.Errorf("resolving feature %q: %w", id, err)
		}

		fc, err := feature.ParseFeatureConfig(folder)
		if err != nil {
			return nil, fmt.Errorf("parsing feature config for %q: %w", id, err)
		}

		features = append(features, &feature.FeatureSet{
			ConfigID: id,
			Folder:   folder,
			Config:   fc,
			Options:  opts,
		})
	}

	ordered, err := feature.OrderFeatures(features, cfg.OverrideFeatureInstallOrder)
	if err != nil {
		return nil, fmt.Errorf("ordering features: %w", err)
	}

	return ordered, nil
}

// resolveContainerUser determines the container user from config.
func resolveContainerUser(cfg *config.DevContainerConfig) string {
	if cfg.ContainerUser != "" {
		return cfg.ContainerUser
	}
	if cfg.RemoteUser != "" {
		return cfg.RemoteUser
	}
	return "root"
}

// resolveFeatureMetadata resolves features from the config and returns their
// metadata without building. Used by restart/recreate paths that need feature
// capabilities (privileged, init, entrypoints) without a full image build.
func (e *Engine) resolveFeatureMetadata(cfg *config.DevContainerConfig) []*config.ImageMetadata {
	if len(cfg.Features) == 0 {
		return nil
	}
	configDir := filepath.Dir(cfg.Origin)
	features, err := e.resolveFeatures(cfg, configDir)
	if err != nil {
		e.logger.Debug("failed to resolve features for metadata", "error", err)
		return nil
	}
	var metadata []*config.ImageMetadata
	for _, f := range features {
		metadata = append(metadata, featureToMetadata(f))
	}
	return metadata
}

// cleanupPreviousBuildImage removes the old build image when the hash changes.
// Best-effort: logs on failure but does not return an error.
func (e *Engine) cleanupPreviousBuildImage(ctx context.Context, wsID, newImageName string) {
	stored, err := e.store.LoadResult(wsID)
	if err != nil || stored == nil {
		return
	}
	oldImage := stored.ImageName
	if oldImage == "" || oldImage == newImageName {
		return
	}
	if !strings.HasPrefix(oldImage, "crib-") {
		return
	}
	if err := e.driver.RemoveImage(ctx, oldImage); err != nil {
		e.logger.Debug("failed to remove previous build image", "image", oldImage, "error", err)
	}
}

// featureToMetadata converts a FeatureSet to an ImageMetadata entry.
func featureToMetadata(f *feature.FeatureSet) *config.ImageMetadata {
	m := &config.ImageMetadata{
		ID:         f.Config.ID,
		Entrypoint: f.Config.Entrypoint,
	}
	m.CapAdd = f.Config.CapAdd
	m.SecurityOpt = f.Config.SecurityOpt
	m.Init = f.Config.Init
	m.Privileged = f.Config.Privileged
	m.Mounts = f.Config.Mounts
	// ContainerEnv is intentionally excluded. Feature containerEnv values
	// are baked into the image as ENV instructions during the Dockerfile
	// build (see feature.GenerateDockerfile). Including them here would
	// cause them to also be passed as runtime -e flags / compose
	// environment, overriding the image's correctly-expanded values with
	// unexpanded literals (e.g. ${PATH} would resolve against the host
	// instead of the container).
	m.OnCreateCommand = f.Config.OnCreateCommand
	m.UpdateContentCommand = f.Config.UpdateContentCommand
	m.PostCreateCommand = f.Config.PostCreateCommand
	m.PostStartCommand = f.Config.PostStartCommand
	m.PostAttachCommand = f.Config.PostAttachCommand
	return m
}
