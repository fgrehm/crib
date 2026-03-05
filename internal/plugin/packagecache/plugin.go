package packagecache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
)

type cacheSpec struct {
	containerDir string // relative to home, or absolute if isSystem is true
	isSystem     bool   // mount at a fixed system path instead of ~/containerDir
	envVar       string // if set, inject this env var pointing to the mount target
}

var cacheMap = map[string]cacheSpec{
	"npm":       {containerDir: ".npm"},
	"yarn":      {containerDir: ".cache/yarn"},
	"pip":       {containerDir: ".cache/pip"},
	"go":        {containerDir: "go/pkg/mod", envVar: "GOMODCACHE"},
	"cargo":     {containerDir: ".cargo", envVar: "CARGO_HOME"},
	"maven":     {containerDir: ".m2/repository"},
	"gradle":    {containerDir: ".gradle/caches"},
	"bundler":   {containerDir: ".bundle/cache"},
	"apt":       {containerDir: "/var/cache/apt", isSystem: true},
	"downloads": {containerDir: ".cache/crib", envVar: "CRIB_CACHE"},
}

// VolumeName returns the Docker volume name for a workspace's cache provider.
func VolumeName(workspaceID, provider string) string {
	return "crib-cache-" + workspaceID + "-" + provider
}

// VolumePrefix returns the volume name prefix for a workspace's cache volumes.
func VolumePrefix(workspaceID string) string {
	return "crib-cache-" + workspaceID + "-"
}

// GlobalVolumePrefix is the prefix shared by all crib cache volumes.
const GlobalVolumePrefix = "crib-cache-"

// Plugin shares host package caches via named Docker volumes.
type Plugin struct {
	providers []string
}

// New creates a package cache plugin for the given providers.
// Unknown providers are skipped at runtime. Use ValidateProviders to
// check for unknown names before creating the plugin.
func New(providers []string) *Plugin {
	return &Plugin{providers: providers}
}

// ValidateProviders returns provider names that are not recognized.
func ValidateProviders(providers []string) []string {
	var unknown []string
	for _, p := range providers {
		if _, ok := cacheMap[p]; !ok {
			unknown = append(unknown, p)
		}
	}
	return unknown
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string { return "package-cache" }

// PreContainerRun returns volume mounts for each configured cache provider.
func (p *Plugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	if len(p.providers) == 0 {
		return nil, nil
	}

	remoteHome := plugin.InferRemoteHome(req.RemoteUser)

	var mounts []config.Mount
	var env map[string]string
	var hasApt bool
	for _, provider := range p.providers {
		spec, ok := cacheMap[provider]
		if !ok {
			continue
		}

		var target string
		if spec.isSystem {
			target = spec.containerDir
		} else {
			target = fmt.Sprintf("%s/%s", remoteHome, spec.containerDir)
		}

		if provider == "apt" {
			hasApt = true
		}

		mounts = append(mounts, config.Mount{
			Type:   "volume",
			Source: VolumeName(req.WorkspaceID, provider),
			Target: target,
		})

		if spec.envVar != "" {
			if env == nil {
				env = make(map[string]string)
			}
			env[spec.envVar] = target
		}
	}

	if len(mounts) == 0 {
		return nil, nil
	}

	// When apt caching is enabled, disable the docker-clean hook that
	// wipes /var/cache/apt/archives/*.deb after every install/update.
	// Without this, the cache volume is emptied by apt itself.
	var copies []plugin.FileCopy
	if hasApt {
		copy, err := stageDockerCleanOverride(req.WorkspaceDir)
		if err != nil {
			return nil, fmt.Errorf("staging docker-clean override: %w", err)
		}
		copies = append(copies, copy)
	}

	return &plugin.PreContainerRunResponse{
		Mounts: mounts,
		Env:    env,
		Copies: copies,
	}, nil
}

// stageDockerCleanOverride writes an empty file to the workspace state dir
// and returns a FileCopy that overwrites /etc/apt/apt.conf.d/docker-clean
// inside the container. This prevents apt from deleting cached .deb files.
func stageDockerCleanOverride(workspaceDir string) (plugin.FileCopy, error) {
	pluginDir := filepath.Join(workspaceDir, "plugins", "package-cache")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return plugin.FileCopy{}, fmt.Errorf("creating plugin dir: %w", err)
	}

	src := filepath.Join(pluginDir, "docker-clean")
	content := "# Disabled by crib (package-cache plugin) to preserve apt cache across rebuilds.\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		return plugin.FileCopy{}, fmt.Errorf("writing docker-clean override: %w", err)
	}

	return plugin.FileCopy{
		Source: src,
		Target: "/etc/apt/apt.conf.d/docker-clean",
	}, nil
}

// BuildCacheMounts returns cache mount target paths for use during image builds.
// Feature install scripts run as root, so home-relative paths use /root.
// For apt, both /var/cache/apt and /var/lib/apt/lists are included.
func BuildCacheMounts(providers []string) []string {
	var targets []string
	for _, provider := range providers {
		spec, ok := cacheMap[provider]
		if !ok {
			continue
		}
		if spec.isSystem {
			targets = append(targets, spec.containerDir)
		} else {
			targets = append(targets, "/root/"+spec.containerDir)
		}
		if provider == "apt" {
			targets = append(targets, "/var/lib/apt/lists")
		}
	}
	return targets
}

// SupportedProviders returns the list of known cache provider names.
func SupportedProviders() []string {
	providers := make([]string, 0, len(cacheMap))
	for k := range cacheMap {
		providers = append(providers, k)
	}
	return providers
}
