package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

// ServiceInfo holds extracted service configuration needed for feature builds.
type ServiceInfo struct {
	// Image is the image reference for the service (empty if build-only).
	Image string
	// HasBuild is true when the service has a build section.
	HasBuild bool
	// BuildCtx is the absolute path to the build context directory.
	BuildCtx string
	// Dockerfile is the Dockerfile path (relative to BuildCtx).
	Dockerfile string
	// User is the user directive from the compose service.
	User string
}

// GetServiceInfo loads compose files and extracts configuration for the named service.
// env provides extra environment variables (KEY=VALUE) for ${VAR} substitution in
// compose files (e.g. localWorkspaceFolder, devcontainerId).
func GetServiceInfo(ctx context.Context, paths []string, serviceName string, env []string) (*ServiceInfo, error) {
	project, err := LoadProject(ctx, paths, nil, env)
	if err != nil {
		return nil, fmt.Errorf("loading compose project: %w", err)
	}

	svc, err := project.GetService(serviceName)
	if err != nil {
		return nil, fmt.Errorf("service %q: %w", serviceName, err)
	}

	info := &ServiceInfo{
		Image: svc.Image,
		User:  svc.User,
	}
	if svc.Build != nil {
		info.HasBuild = true
		info.BuildCtx = svc.Build.Context
		info.Dockerfile = svc.Build.Dockerfile
	}
	return info, nil
}

// BuiltImageName returns the expected image name for a compose-built service.
// The separator between project and service differs by compose provider:
// Docker Compose v2 uses "-", podman-compose uses "_".
func (h *Helper) BuiltImageName(projectName, serviceName string) string {
	sep := "-"
	if strings.Contains(filepath.Base(h.baseCommand), "podman") {
		sep = "_"
	}
	return projectName + sep + serviceName
}

// LoadProject loads a Docker Compose project from the given file paths and env files.
// extraEnv provides additional KEY=VALUE variables for ${VAR} substitution; they take
// precedence over env file values but NOT over process environment variables.
func LoadProject(ctx context.Context, paths []string, envFiles []string, extraEnv []string) (*types.Project, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no compose files specified")
	}

	// Resolve absolute paths.
	configFiles := make([]types.ConfigFile, len(paths))
	var workingDir string
	for i, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("resolving path %s: %w", p, err)
		}
		configFiles[i] = types.ConfigFile{Filename: abs}
		if i == 0 {
			workingDir = filepath.Dir(abs)
		}
	}

	// Build environment: process env > extraEnv > env files.
	env := currentEnv()
	for _, pair := range extraEnv {
		if k, v, ok := strings.Cut(pair, "="); ok {
			if _, exists := env[k]; !exists {
				env[k] = v
			}
		}
	}
	for _, ef := range envFiles {
		parsed, err := dotenv.Read(ef)
		if err != nil {
			return nil, fmt.Errorf("reading env file %s: %w", ef, err)
		}
		for k, v := range parsed {
			if _, exists := env[k]; !exists {
				env[k] = v
			}
		}
	}

	details := types.ConfigDetails{
		WorkingDir:  workingDir,
		ConfigFiles: configFiles,
		Environment: env,
	}

	project, err := loader.LoadWithContext(ctx, details, func(opts *loader.Options) {
		opts.SkipConsistencyCheck = true
		opts.SetProjectName(filepath.Base(workingDir), false)
	})
	if err != nil {
		return nil, fmt.Errorf("loading compose project: %w", err)
	}
	return project, nil
}

// currentEnv returns the current process environment as a map.
func currentEnv() types.Mapping {
	env := make(types.Mapping)
	for _, e := range os.Environ() {
		k, v, ok := strings.Cut(e, "=")
		if ok {
			env[k] = v
		}
	}
	return env
}
