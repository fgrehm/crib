package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/workspace"
)

// RestartResult holds the outcome of a Restart operation.
type RestartResult struct {
	// ContainerID is the container ID.
	ContainerID string

	// WorkspaceFolder is the path inside the container where the project is mounted.
	WorkspaceFolder string

	// RemoteUser is the user to run commands as inside the container.
	RemoteUser string

	// Recreated indicates whether the container was recreated (config changed)
	// rather than simply restarted.
	Recreated bool

	// Ports lists the published port bindings.
	Ports []driver.PortBinding
}

// Restart restarts the container for the given workspace. It implements a
// "warm recreate" strategy:
//   - If the devcontainer config hasn't changed, it does a simple container restart
//     and runs only the resume-flow lifecycle hooks (postStartCommand, postAttachCommand).
//   - If only "safe" properties changed (volumes, mounts, ports, env, runArgs),
//     it recreates the container without rebuilding the image and runs the resume flow.
//   - If image-affecting properties changed (image, Dockerfile, features, build args),
//     it returns an error suggesting `crib rebuild`.
func (e *Engine) Restart(ctx context.Context, ws *workspace.Workspace) (*RestartResult, error) {
	e.logger.Debug("restart", "workspace", ws.ID)

	// Load stored result to get the previous config.
	storedResult, err := e.store.LoadResult(ws.ID)
	if err != nil {
		return nil, fmt.Errorf("loading workspace result: %w", err)
	}
	if storedResult == nil {
		return nil, fmt.Errorf("no previous result found for workspace %s (run 'crib up' first)", ws.ID)
	}

	// Parse current config.
	cfg, workspaceFolder, err := e.parseAndSubstitute(ws)
	if err != nil {
		return nil, err
	}

	// Compose guards (mirrors Up).
	if len(cfg.DockerComposeFile) > 0 {
		if e.compose == nil {
			return nil, &ErrComposeNotAvailable{}
		}
		if cfg.Service == "" {
			return nil, fmt.Errorf("dockerComposeFile is set but service is not specified")
		}
	}

	// Detect what changed.
	var storedCfg config.DevContainerConfig
	if err := json.Unmarshal(storedResult.MergedConfig, &storedCfg); err != nil {
		return nil, fmt.Errorf("unmarshaling stored config: %w", err)
	}

	change := detectConfigChange(&storedCfg, cfg)

	// If devcontainer.json looks unchanged, check compose file contents.
	// detectConfigChange only compares the compose file list, not their
	// contents. A volume, port, or env change inside a compose file would
	// otherwise be missed.
	if change == changeNone && len(cfg.DockerComposeFile) > 0 {
		cd := configDir(ws)
		composeFiles := resolveComposeFiles(cd, cfg.DockerComposeFile)
		currentHash := computeComposeFilesHash(composeFiles)
		if storedResult.ComposeFilesHash == "" {
			// Pre-existing workspace with no stored hash (created before
			// compose content tracking was added). Treat as changed so the
			// hash gets persisted on this restart.
			e.logger.Debug("no stored compose files hash, forcing recreate to persist hash")
			change = changeSafe
		} else if currentHash != storedResult.ComposeFilesHash {
			e.logger.Debug("compose file contents changed", "stored", storedResult.ComposeFilesHash, "current", currentHash)
			change = changeSafe
		}
	}

	b := e.newBackend(ws, cfg, workspaceFolder)

	switch change {
	case changeNeedsRebuild:
		return nil, fmt.Errorf("config changes require a full rebuild (image, Dockerfile, or features changed); run 'crib rebuild' instead")

	case changeSafe:
		e.reportProgress(PhaseRestart, "Config changes detected, recreating container...")
		result, err := e.restartRecreate(ctx, ws, cfg, workspaceFolder, b, storedResult)
		if result != nil {
			result.Recreated = true
		}
		return result, err

	default:
		// No changes -- simple restart.
		e.reportProgress(PhaseRestart, "Restarting container...")
		return e.restartSimple(ctx, ws, cfg, workspaceFolder, b, storedResult)
	}
}

// restartSimple performs a simple container restart without recreation.
// Uses the backend for container restart and finalize for post-restart steps.
func (e *Engine) restartSimple(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, b containerBackend, storedResult *workspace.Result) (*RestartResult, error) {
	// Find the existing container.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding container: %w", err)
	}
	if container == nil {
		return nil, &ErrNoContainer{WorkspaceID: ws.ID}
	}

	// Dispatch plugins.
	pluginUser := b.pluginUser(ctx)
	if pluginUser == "" {
		// For non-compose, use stored remote user for plugin dispatch.
		pluginUser = storedResult.RemoteUser
	}
	pluginResp, err := e.dispatchPlugins(ctx, ws, cfg, storedResult.ImageName, workspaceFolder, pluginUser)
	if err != nil {
		e.logger.Warn("plugin dispatch failed, continuing without plugins", "error", err)
		pluginResp = nil
	}

	// Restart the container.
	newID, err := b.restart(ctx, container.ID, pluginResp)
	if err != nil {
		return nil, err
	}

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     newID,
		remoteUser:      storedResult.RemoteUser, // pre-set to skip whoami
		workspaceFolder: workspaceFolder,
	}

	result, err := e.finalize(ctx, ws, cfg, finalizeOpts{
		cc:              cc,
		imageName:       storedResult.ImageName,
		hasEntrypoints:  storedResult.HasFeatureEntrypoints,
		pluginResp:      pluginResp,
		storedResult:    storedResult,
		fromSnapshot:    true,
		skipVolumeChown: true,
	})
	if err != nil {
		return nil, err
	}

	rr := toRestartResult(result)
	return rr, nil
}

// restartRecreate stops the container, recreates it with the new config,
// and runs lifecycle hooks via finalize.
func (e *Engine) restartRecreate(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, b containerBackend, storedResult *workspace.Result) (*RestartResult, error) {

	// Remove existing container.
	if err := e.Down(ctx, ws); err != nil {
		e.logger.Warn("failed to remove container before recreate", "error", err)
	}

	// Check for a valid snapshot.
	snapshotImage, hasSnapshot := e.validSnapshot(ctx, ws, cfg)

	// Determine the image to use.
	imgResult := resolveRestartImage(hasSnapshot, snapshotImage, *storedResult, cfg)
	var metadata []*config.ImageMetadata
	var imageUser string

	if imgResult.needsBuild {
		e.reportProgress(PhaseBuild, "No cached image found, rebuilding...")
		buildRes, err := b.buildImage(ctx)
		if err != nil {
			return nil, fmt.Errorf("rebuilding image: %w", err)
		}
		imgResult.imageName = buildRes.imageName
		imgResult.hasEntrypoints = buildRes.hasEntrypoints
		metadata = buildRes.imageMetadata
		imageUser = buildRes.imageUser
	}

	// Dispatch plugins.
	pluginUser := b.pluginUser(ctx)
	pluginResp, err := e.dispatchPlugins(ctx, ws, cfg, imgResult.imageName, workspaceFolder, pluginUser)
	if err != nil {
		return nil, err
	}

	containerID, err := b.createContainer(ctx, createOpts{
		imageName:      imgResult.imageName,
		hasEntrypoints: imgResult.hasEntrypoints,
		metadata:       metadata,
		pluginResp:     pluginResp,
		skipBuild:      hasSnapshot || b.canResumeFromStored() || (storedResult != nil && storedResult.ImageName != ""),
	})
	if err != nil {
		return nil, err
	}

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     containerID,
		workspaceFolder: workspaceFolder,
	}

	// Preserve original image name (not snapshot) for result.
	resultImageName := storedResult.ImageName
	if resultImageName == "" {
		resultImageName = imgResult.imageName
	}

	upResult, err := e.finalize(ctx, ws, cfg, finalizeOpts{
		cc:             cc,
		imageName:      resultImageName,
		hasEntrypoints: imgResult.hasEntrypoints,
		pluginResp:     pluginResp,
		storedResult:   storedResult,
		fromSnapshot:   hasSnapshot,
		imageMetadata:  metadata,
		imageUser:      imageUser,
	})
	if err != nil {
		if upResult != nil {
			rr := toRestartResult(upResult)
			return rr, err
		}
		return nil, err
	}

	rr := toRestartResult(upResult)
	return rr, nil
}

// restartImageResult holds the outcome of resolveRestartImage.
type restartImageResult struct {
	imageName      string
	hasEntrypoints bool
	needsBuild     bool
}

// resolveRestartImage determines which image to use for a container recreate.
// It checks, in order: snapshot, stored image, config image. If none are
// available and the workspace is not compose-based, needsBuild is set true.
func resolveRestartImage(hasSnapshot bool, snapshotImage string, storedResult workspace.Result, cfg *config.DevContainerConfig) restartImageResult {
	switch {
	case hasSnapshot:
		return restartImageResult{
			imageName:      snapshotImage,
			hasEntrypoints: storedResult.HasFeatureEntrypoints,
		}
	case storedResult.ImageName != "":
		return restartImageResult{
			imageName:      storedResult.ImageName,
			hasEntrypoints: storedResult.HasFeatureEntrypoints,
		}
	case cfg.Image != "":
		return restartImageResult{imageName: cfg.Image}
	case len(cfg.DockerComposeFile) > 0:
		return restartImageResult{}
	default:
		return restartImageResult{needsBuild: true}
	}
}

// resolveConfigEnvFromStored resolves ${containerEnv:*} references in
// cfg.RemoteEnv using the stored env as the container env source. Used by
// restart paths that can't probe the container for its native environment.
func resolveConfigEnvFromStored(cfg *config.DevContainerConfig, storedEnv map[string]string) map[string]string {
	if len(cfg.RemoteEnv) == 0 {
		return nil
	}
	resolved, err := config.SubstituteContainerEnv(storedEnv, cfg)
	if err != nil {
		// Fall back to raw config env if substitution fails.
		return cfg.RemoteEnv
	}
	resolvedEnv := resolved.RemoteEnv
	// Also resolve bare ${VAR} references (e.g. ${PATH}).
	resolveBareVarRefs(resolvedEnv, storedEnv)
	return resolvedEnv
}
