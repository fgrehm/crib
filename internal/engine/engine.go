package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	ocidriver "github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// containerContext identifies a running container for exec operations.
// Passed by value; callers may fill fields incrementally (e.g. remoteUser
// resolved after container creation).
type containerContext struct {
	workspaceID     string
	containerID     string
	remoteUser      string
	workspaceFolder string
}

// composeInvocation bundles the parameters needed to invoke docker compose.
type composeInvocation struct {
	projectName string
	files       []string
	env         []string
	service     string // primary devcontainer service name
}

// newComposeInvocation constructs a composeInvocation from workspace and config.
func newComposeInvocation(ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string) composeInvocation {
	cd := configDir(ws)
	return composeInvocation{
		projectName: compose.ProjectName(ws.ID),
		files:       resolveComposeFiles(cd, cfg.DockerComposeFile),
		env:         devcontainerEnv(ws.ID, ws.Source, workspaceFolder),
		service:     cfg.Service,
	}
}

// Engine orchestrates devcontainer lifecycle operations.
type Engine struct {
	driver           driver.Driver
	compose          *compose.Helper
	store            *workspace.Store
	plugins          *plugin.Manager
	runtimeName      string
	buildCacheMounts []string // BuildKit cache mount targets for feature builds
	logger           *slog.Logger
	stdout           io.Writer
	stderr           io.Writer
	verbose          bool
	progress         func(ProgressEvent)
}

// New creates an Engine with the given dependencies.
func New(d driver.Driver, composeHelper *compose.Helper, store *workspace.Store, logger *slog.Logger) *Engine {
	return &Engine{
		driver:  d,
		compose: composeHelper,
		store:   store,
		logger:  logger,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
}

// SetOutput overrides the default stdout and stderr writers.
func (e *Engine) SetOutput(stdout, stderr io.Writer) {
	e.stdout = stdout
	e.stderr = stderr
}

// SetVerbose enables verbose output (e.g. compose stdout).
func (e *Engine) SetVerbose(v bool) {
	e.verbose = v
}

// composeStdout returns the writer for compose stdout. In verbose mode, this
// is the engine's stdout writer. Otherwise, output is discarded to reduce noise
// from container name listings during up/down/restart.
func (e *Engine) composeStdout() io.Writer {
	if e.verbose {
		return e.stdout
	}
	return io.Discard
}

// composeStderr returns the writer for compose stderr. In verbose mode, this
// is the engine's stderr writer so operational warnings (SIGTERM timeouts,
// IPAM messages, etc.) are visible for debugging. Otherwise, output is
// discarded. On failure, compose.Run's internal buffer still captures stderr
// and includes it in the returned error.
func (e *Engine) composeStderr() io.Writer {
	if e.verbose {
		return e.stderr
	}
	return io.Discard
}

// composeStderrTee returns a writer that always captures compose stderr into
// buf, and also forwards to the engine's stderr in verbose mode. Use this
// when the caller needs the output for error diagnostics even in non-verbose
// mode (e.g., "container not found after up").
func (e *Engine) composeStderrTee(buf *bytes.Buffer) io.Writer {
	if e.verbose {
		return io.MultiWriter(buf, e.stderr)
	}
	return buf
}

// SetProgress sets a callback for user-facing progress events.
func (e *Engine) SetProgress(fn func(ProgressEvent)) {
	e.progress = fn
	if e.plugins != nil {
		e.plugins.SetProgress(progressToString(fn))
	}
}

// progressToString adapts a ProgressEvent callback to a plain string callback
// for use by the plugin manager (which lives in a separate package).
// TODO: remove once plugin package migrates to ProgressEvent.
func progressToString(fn func(ProgressEvent)) func(string) {
	if fn == nil {
		return nil
	}
	return func(msg string) {
		fn(ProgressEvent{Phase: PhasePlugins, Message: msg})
	}
}

// SetPlugins attaches a plugin manager to the engine.
func (e *Engine) SetPlugins(m *plugin.Manager) {
	e.plugins = m
	if e.progress != nil {
		m.SetProgress(progressToString(e.progress))
	}
}

// SetRuntime stores the runtime name (e.g. "docker", "podman") for plugin requests.
func (e *Engine) SetRuntime(name string) {
	e.runtimeName = name
}

// SetBuildCacheMounts configures BuildKit cache mount targets for feature
// install RUN instructions (e.g. "/var/cache/apt", "/root/.npm").
func (e *Engine) SetBuildCacheMounts(mounts []string) {
	e.buildCacheMounts = mounts
}

// reportProgress sends a progress event to the callback (if set)
// and logs the message at debug level.
func (e *Engine) reportProgress(phase ProgressPhase, msg string) {
	if e.progress != nil {
		e.progress(ProgressEvent{Phase: phase, Message: msg})
	}
	e.logger.Debug(msg)
}

// UpOptions controls the behavior of the Up operation.
type UpOptions struct {
	// Recreate forces container recreation even if one already exists.
	Recreate bool
}

// UpResult holds the outcome of a successful Up operation.
type UpResult struct {
	// ContainerID is the container ID.
	ContainerID string

	// ImageName is the name of the built image (for compose feature images).
	ImageName string

	// WorkspaceFolder is the path inside the container where the project is mounted.
	WorkspaceFolder string

	// RemoteUser is the user to run commands as inside the container.
	RemoteUser string

	// Ports lists the published port bindings.
	Ports []driver.PortBinding

	// HasFeatureEntrypoints is true when the image has feature-declared
	// entrypoints baked in. Persisted to result.json for restart paths.
	HasFeatureEntrypoints bool
}

// Up brings a devcontainer up for the given workspace.
func (e *Engine) Up(ctx context.Context, ws *workspace.Workspace, opts UpOptions) (*UpResult, error) {
	e.logger.Debug("up", "workspace", ws.ID, "source", ws.Source)

	cfg, workspaceFolder, err := e.parseAndSubstitute(ws)
	if err != nil {
		return nil, err
	}

	// Run initializeCommand on the host before image build/pull.
	if err := e.runInitializeCommand(ctx, ws, cfg); err != nil {
		return nil, fmt.Errorf("initializeCommand: %w", err)
	}
	if cfg.WaitFor == "initializeCommand" {
		e.reportProgress(PhaseInit, "Container ready.")
	}

	// Compose guards.
	if len(cfg.DockerComposeFile) > 0 {
		if e.compose == nil {
			return nil, fmt.Errorf("compose is not available (install docker compose or podman compose)")
		}
		if cfg.Service == "" {
			return nil, fmt.Errorf("dockerComposeFile is set but service is not specified")
		}
	}

	b := e.newBackend(ws, cfg, workspaceFolder)

	// Check for an existing container.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding container: %w", err)
	}

	if container != nil && !opts.Recreate {
		return e.upExisting(ctx, ws, cfg, workspaceFolder, b, container)
	}

	// Remove existing container if recreating.
	if container != nil && opts.Recreate {
		e.reportProgress(PhaseCreate, "Removing container...")
		if err := e.store.ClearHookMarkers(ws.ID); err != nil {
			e.logger.Warn("failed to clear hook markers", "error", err)
		}
		if err := b.deleteExisting(ctx); err != nil {
			return nil, fmt.Errorf("deleting container for recreation: %w", err)
		}
	}

	return e.upCreate(ctx, ws, cfg, workspaceFolder, b, opts.Recreate)
}

// upExisting handles the case where a container already exists.
func (e *Engine) upExisting(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, b containerBackend, container *driver.ContainerDetails) (*UpResult, error) {
	// Load stored result for image name and feature entrypoints.
	var storedImageName string
	var storedHasEntrypoints bool
	if stored, err := e.store.LoadResult(ws.ID); err == nil && stored != nil {
		storedImageName = stored.ImageName
		storedHasEntrypoints = stored.HasFeatureEntrypoints
	}

	// Dispatch plugins.
	pluginUser := b.pluginUser(ctx)
	pluginResp, err := e.dispatchPlugins(ctx, ws, cfg, storedImageName, workspaceFolder, pluginUser)
	if err != nil {
		e.logger.Warn("plugin dispatch failed for existing container", "error", err)
		pluginResp = nil
	}

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     container.ID,
		workspaceFolder: workspaceFolder,
	}

	if !container.State.IsRunning() {
		e.reportProgress(PhaseCreate, "Starting container...")
		newID, err := b.start(ctx, container.ID, pluginResp)
		if err != nil {
			return nil, err
		}
		cc.containerID = newID
	} else {
		e.reportProgress(PhaseCreate, "Container already running")
	}

	return e.finalize(ctx, ws, cfg, finalizeOpts{
		cc:             cc,
		imageName:      storedImageName,
		hasEntrypoints: storedHasEntrypoints,
		pluginResp:     pluginResp,
	})
}

// upCreate handles creating a new container (no existing container or recreate).
func (e *Engine) upCreate(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, b containerBackend, isRecreate bool) (*UpResult, error) {
	// Check for snapshot or stored result to resume from.
	if !isRecreate {
		if storedResult, loadErr := e.store.LoadResult(ws.ID); loadErr == nil && storedResult != nil {
			// Check for valid snapshot.
			if snapshotImage, ok := e.validSnapshot(ctx, ws, cfg); ok {
				return e.upFromImage(ctx, ws, cfg, workspaceFolder, b, snapshotImage, storedResult, true)
			}
			// Compose can resume from stored result without snapshot.
			if b.canResumeFromStored() {
				return e.upFromImage(ctx, ws, cfg, workspaceFolder, b, storedResult.ImageName, storedResult, false)
			}
		}
	}

	// Fresh build path.
	buildRes, err := b.buildImage(ctx)
	if err != nil {
		return nil, err
	}

	// Dispatch plugins.
	pluginUser := b.pluginUser(ctx)
	pluginResp, err := e.dispatchPlugins(ctx, ws, cfg, buildRes.imageName, workspaceFolder, pluginUser)
	if err != nil {
		return nil, err
	}

	containerID, err := b.createContainer(ctx, createOpts{
		imageName:      buildRes.imageName,
		hasEntrypoints: buildRes.hasEntrypoints,
		metadata:       buildRes.imageMetadata,
		pluginResp:     pluginResp,
	})
	if err != nil {
		return nil, err
	}

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     containerID,
		workspaceFolder: workspaceFolder,
	}
	return e.finalize(ctx, ws, cfg, finalizeOpts{
		cc:             cc,
		imageName:      buildRes.imageName,
		hasEntrypoints: buildRes.hasEntrypoints,
		pluginResp:     pluginResp,
		imageMetadata:  buildRes.imageMetadata,
	})
}

// upFromImage creates a container from a snapshot or stored image.
func (e *Engine) upFromImage(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, b containerBackend, imageName string, storedResult *workspace.Result, isSnapshot bool) (*UpResult, error) {
	e.logger.Debug("up from image", "image", imageName, "snapshot", isSnapshot)

	// Dispatch plugins.
	pluginUser := b.pluginUser(ctx)
	pluginResp, err := e.dispatchPlugins(ctx, ws, cfg, imageName, workspaceFolder, pluginUser)
	if err != nil {
		return nil, err
	}

	hasEntrypoints := storedResult.HasFeatureEntrypoints

	containerID, err := b.createContainer(ctx, createOpts{
		imageName:      imageName,
		hasEntrypoints: hasEntrypoints,
		pluginResp:     pluginResp,
		skipBuild:      true,
	})
	if err != nil {
		return nil, err
	}

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     containerID,
		workspaceFolder: workspaceFolder,
	}

	// Use the original image name (not snapshot) for the result.
	resultImageName := storedResult.ImageName

	return e.finalize(ctx, ws, cfg, finalizeOpts{
		cc:             cc,
		imageName:      resultImageName,
		hasEntrypoints: hasEntrypoints,
		pluginResp:     pluginResp,
		storedResult:   storedResult,
		fromSnapshot:   isSnapshot,
	})
}

// saveResult persists the workspace result to disk so crib exec/shell can
// find the container, workspace folder, user, and environment. Loads any
// existing result first and merges in the new values, preserving fields
// managed elsewhere (e.g. snapshot metadata from commitSnapshot).
func (e *Engine) saveResult(ws *workspace.Workspace, cfg *config.DevContainerConfig, result *UpResult) {
	ws.LastUsedAt = time.Now()
	if err := e.store.Save(ws); err != nil {
		e.logger.Warn("failed to update workspace timestamps", "error", err)
	}

	// Load existing result as the base for a merge. Fields managed by other
	// code paths (SnapshotImage, SnapshotHookHash) are preserved automatically.
	wsResult, _ := e.store.LoadResult(ws.ID)
	if wsResult == nil {
		wsResult = &workspace.Result{}
	}

	mergedJSON, _ := json.Marshal(cfg)
	wsResult.ContainerID = result.ContainerID
	wsResult.ImageName = result.ImageName
	wsResult.MergedConfig = mergedJSON
	wsResult.WorkspaceFolder = result.WorkspaceFolder
	wsResult.RemoteEnv = cfg.RemoteEnv
	wsResult.RemoteUser = result.RemoteUser
	wsResult.HasFeatureEntrypoints = result.HasFeatureEntrypoints

	if len(cfg.DockerComposeFile) > 0 {
		cd := configDir(ws)
		composeFiles := resolveComposeFiles(cd, cfg.DockerComposeFile)
		wsResult.ComposeFilesHash = computeComposeFilesHash(composeFiles)
	} else {
		wsResult.ComposeFilesHash = ""
	}

	if err := e.store.SaveResult(ws.ID, wsResult); err != nil {
		e.logger.Warn("failed to save workspace result", "error", err)
	}
}

// Down stops and removes the container for the given workspace, but keeps
// workspace state in the store so that a subsequent "up" can recreate it.
// Hook markers are cleared so the next "up" runs all lifecycle hooks.
func (e *Engine) Down(ctx context.Context, ws *workspace.Workspace) error {
	e.logger.Debug("down", "workspace", ws.ID)

	// Clear hook markers so the next "up" runs all lifecycle hooks.
	if err := e.store.ClearHookMarkers(ws.ID); err != nil {
		e.logger.Warn("failed to clear hook markers", "error", err)
	}

	// For compose workspaces, use compose down to stop and remove all services.
	if result, err := e.store.LoadResult(ws.ID); err == nil && result != nil {
		var cfg config.DevContainerConfig
		if json.Unmarshal(result.MergedConfig, &cfg) == nil && len(cfg.DockerComposeFile) > 0 {
			if e.compose != nil {
				inv := newComposeInvocation(ws, &cfg, result.WorkspaceFolder)
				return e.composeDown(ctx, inv, ws.ID, false)
			}
		}
	}

	// Non-compose path: stop and remove the individual container.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return fmt.Errorf("finding container: %w", err)
	}
	if container == nil {
		return fmt.Errorf("no container found for workspace %s", ws.ID)
	}

	return e.driver.DeleteContainer(ctx, ws.ID, container.ID)
}

// Stop stops the container for the given workspace without removing it.
// Hook markers are preserved so that a subsequent "up" runs only resume-flow
// hooks (postStartCommand, postAttachCommand).
func (e *Engine) Stop(ctx context.Context, ws *workspace.Workspace) error {
	e.logger.Debug("stop", "workspace", ws.ID)

	// For compose workspaces, use compose stop.
	if result, err := e.store.LoadResult(ws.ID); err == nil && result != nil {
		var cfg config.DevContainerConfig
		if json.Unmarshal(result.MergedConfig, &cfg) == nil && len(cfg.DockerComposeFile) > 0 {
			if e.compose != nil {
				inv := newComposeInvocation(ws, &cfg, result.WorkspaceFolder)
				return e.composeStop(ctx, inv, ws.ID)
			}
		}
	}

	// Non-compose path: stop the individual container.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return fmt.Errorf("finding container: %w", err)
	}
	if container == nil {
		return fmt.Errorf("no container found for workspace %s", ws.ID)
	}

	if !container.State.IsRunning() {
		e.logger.Debug("container already stopped", "workspace", ws.ID, "containerID", container.ID)
		return nil
	}

	return e.driver.StopContainer(ctx, ws.ID, container.ID)
}

// RemovePreview describes what Remove() will delete.
type RemovePreview struct {
	ContainerID string   // empty if no container found
	Images      []string // image references that will be removed
}

// PreviewRemove returns what Remove() would delete without actually removing anything.
func (e *Engine) PreviewRemove(ctx context.Context, ws *workspace.Workspace) *RemovePreview {
	preview := &RemovePreview{}

	if container, err := e.driver.FindContainer(ctx, ws.ID); err == nil && container != nil {
		preview.ContainerID = container.ID
	}

	if result, err := e.store.LoadResult(ws.ID); err == nil && result != nil {
		if result.SnapshotImage != "" {
			preview.Images = append(preview.Images, result.SnapshotImage)
		}
		if result.ImageName != "" && strings.HasPrefix(result.ImageName, "crib-") {
			preview.Images = append(preview.Images, result.ImageName)
		}
	}

	label := ocidriver.WorkspaceLabel(ws.ID)
	if images, err := e.driver.ListImages(ctx, label); err == nil {
		seen := make(map[string]bool)
		for _, img := range preview.Images {
			seen[img] = true
		}
		for _, img := range images {
			if !seen[img.Reference] {
				preview.Images = append(preview.Images, img.Reference)
			}
		}
	}

	return preview
}

// Remove stops and removes the container, then deletes all workspace state.
func (e *Engine) Remove(ctx context.Context, ws *workspace.Workspace) error {
	e.logger.Debug("remove", "workspace", ws.ID)

	// Remove snapshot image before tearing down.
	e.clearSnapshot(ctx, ws)

	// Best-effort container removal (workspace may have no container).
	// For compose workspaces, use compose down --volumes to also remove
	// named volumes declared in the compose file (e.g. database data).
	composeTornDown := false
	if result, err := e.store.LoadResult(ws.ID); err == nil && result != nil {
		var cfg config.DevContainerConfig
		if json.Unmarshal(result.MergedConfig, &cfg) == nil && len(cfg.DockerComposeFile) > 0 {
			if e.compose != nil {
				inv := newComposeInvocation(ws, &cfg, result.WorkspaceFolder)
				if err := e.composeDown(ctx, inv, ws.ID, true); err != nil {
					e.logger.Warn("failed to remove compose services", "error", err)
				}
				composeTornDown = true
			}
		}
	}

	if !composeTornDown {
		if err := e.Down(ctx, ws); err != nil {
			e.logger.Warn("failed to remove container", "error", err)
		}
	}

	// Clean up workspace images (best-effort).
	e.cleanupWorkspaceImages(ctx, ws.ID)

	return e.store.Delete(ws.ID)
}

// Status returns the current container details for a workspace, or nil if not found.
// StatusResult holds the outcome of a Status query.
type StatusResult struct {
	// Container is the primary container details (nil if not found).
	Container *driver.ContainerDetails

	// Services holds the status of compose services (nil for non-compose workspaces).
	Services []compose.ServiceStatus
}

func (e *Engine) Status(ctx context.Context, ws *workspace.Workspace) (*StatusResult, error) {
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, err
	}

	result := &StatusResult{Container: container}

	// For compose workspaces, also fetch service statuses.
	if e.compose != nil {
		if stored, err := e.store.LoadResult(ws.ID); err == nil && stored != nil {
			var cfg config.DevContainerConfig
			if json.Unmarshal(stored.MergedConfig, &cfg) == nil && len(cfg.DockerComposeFile) > 0 {
				inv := newComposeInvocation(ws, &cfg, stored.WorkspaceFolder)
				if statuses, err := e.compose.ListServiceStatuses(ctx, inv.projectName, inv.files, inv.env); err == nil {
					result.Services = statuses
				} else {
					e.logger.Debug("failed to list compose services", "error", err)
				}
			}
		}
	}

	return result, nil
}

// cleanupWorkspaceImages removes the build image and any remaining labeled
// images for a workspace. Best-effort: failures are logged, not returned.
func (e *Engine) cleanupWorkspaceImages(ctx context.Context, wsID string) {
	// Collect images to remove: labeled images + unlabeled build image (legacy).
	seen := make(map[string]bool)

	label := ocidriver.WorkspaceLabel(wsID)
	if images, err := e.driver.ListImages(ctx, label); err == nil {
		for _, img := range images {
			seen[img.Reference] = true
		}
	}

	// Also target the stored build image in case it predates labeling.
	if result, err := e.store.LoadResult(wsID); err == nil && result != nil {
		if result.ImageName != "" && strings.HasPrefix(result.ImageName, "crib-") {
			seen[result.ImageName] = true
		}
	}

	for ref := range seen {
		if err := e.driver.RemoveImage(ctx, ref); err != nil {
			e.logger.Debug("failed to remove workspace image", "image", ref, "error", err)
		}
	}
}

// --- shared helpers ---

// parseAndSubstitute parses and performs variable substitution on the
// devcontainer config for the given workspace. Returns the fully resolved
// config and the workspace folder path inside the container.
func (e *Engine) parseAndSubstitute(ws *workspace.Workspace) (*config.DevContainerConfig, string, error) {
	cfgPath := filepath.Join(ws.Source, ws.DevContainerPath)
	cfg, err := config.Parse(cfgPath)
	if err != nil {
		return nil, "", fmt.Errorf("parsing devcontainer config: %w", err)
	}

	workspaceFolder := resolveWorkspaceFolder(cfg, ws.Source)
	// Pre-expand local-path variables in workspaceFolder so the substitution
	// context gets a concrete path for ${containerWorkspaceFolder} references.
	workspaceFolder = strings.NewReplacer(
		"${localWorkspaceFolder}", ws.Source,
		"${localWorkspaceFolderBasename}", filepath.Base(ws.Source),
	).Replace(workspaceFolder)

	subCtx := &config.SubstitutionContext{
		DevContainerID:           ws.ID,
		LocalWorkspaceFolder:     ws.Source,
		ContainerWorkspaceFolder: workspaceFolder,
		Env:                      envMap(),
	}
	cfg, err = config.Substitute(subCtx, cfg)
	if err != nil {
		return nil, "", fmt.Errorf("substituting variables: %w", err)
	}

	// Re-resolve after full substitution in case workspaceFolder referenced
	// other variables (e.g. ${devcontainerId}).
	workspaceFolder = resolveWorkspaceFolder(cfg, ws.Source)

	return cfg, workspaceFolder, nil
}

// configRemoteUser returns remoteUser from config, falling back to
// containerUser. Returns empty string if neither is set.
func configRemoteUser(cfg *config.DevContainerConfig) string {
	if cfg.RemoteUser != "" {
		return cfg.RemoteUser
	}
	return cfg.ContainerUser
}

// resolveRemoteUser determines the remote user for a container, using the
// config's remoteUser/containerUser with fallback to detecting the container's
// default user via whoami.
func (e *Engine) resolveRemoteUser(ctx context.Context, cc containerContext, cfg *config.DevContainerConfig) string {
	remoteUser := configRemoteUser(cfg)
	if remoteUser == "" {
		remoteUser = e.detectContainerUser(ctx, cc)
	}
	if remoteUser == "" {
		remoteUser = "root"
	}
	return remoteUser
}

// configDir returns the directory containing the devcontainer config file.
func configDir(ws *workspace.Workspace) string {
	return filepath.Dir(filepath.Join(ws.Source, ws.DevContainerPath))
}

// resolveComposeFiles resolves compose file paths relative to configDir.
func resolveComposeFiles(cd string, paths []string) []string {
	files := make([]string, len(paths))
	for i, f := range paths {
		files[i] = filepath.Join(cd, f)
	}
	return files
}

// devcontainerEnv builds the devcontainer variable env slice for passing to
// docker compose subprocesses so ${VAR} references in compose files resolve.
func devcontainerEnv(workspaceID, localFolder, containerFolder string) []string {
	return []string{
		"localWorkspaceFolder=" + localFolder,
		"localWorkspaceFolderBasename=" + filepath.Base(localFolder),
		"containerWorkspaceFolder=" + containerFolder,
		"containerWorkspaceFolderBasename=" + filepath.Base(containerFolder),
		"devcontainerId=" + workspaceID,
	}
}
