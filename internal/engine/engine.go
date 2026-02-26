package engine

import (
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
	"github.com/fgrehm/crib/internal/workspace"
)

// Engine orchestrates devcontainer lifecycle operations.
type Engine struct {
	driver   driver.Driver
	compose  *compose.Helper
	store    *workspace.Store
	logger   *slog.Logger
	stdout   io.Writer
	stderr   io.Writer
	progress func(string)
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

// SetProgress sets a callback for user-facing progress messages.
func (e *Engine) SetProgress(fn func(string)) {
	e.progress = fn
}

// reportProgress sends a message to the progress callback (if set)
// and logs it at debug level.
func (e *Engine) reportProgress(msg string) {
	if e.progress != nil {
		e.progress(msg)
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

	// WorkspaceFolder is the path inside the container where the project is mounted.
	WorkspaceFolder string

	// RemoteUser is the user to run commands as inside the container.
	RemoteUser string
}

// Up brings a devcontainer up for the given workspace.
func (e *Engine) Up(ctx context.Context, ws *workspace.Workspace, opts UpOptions) (*UpResult, error) {
	e.logger.Debug("up", "workspace", ws.ID, "source", ws.Source)

	// Parse and substitute devcontainer.json.
	configPath := filepath.Join(ws.Source, ws.DevContainerPath)
	cfg, err := config.Parse(configPath)
	if err != nil {
		return nil, fmt.Errorf("parsing devcontainer config: %w", err)
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
		return nil, fmt.Errorf("substituting variables: %w", err)
	}

	// Re-resolve after full substitution in case workspaceFolder referenced
	// other variables (e.g. ${devcontainerId}).
	workspaceFolder = resolveWorkspaceFolder(cfg, ws.Source)

	// Run initializeCommand on the host before image build/pull.
	if err := e.runInitializeCommand(ctx, ws, cfg); err != nil {
		return nil, fmt.Errorf("initializeCommand: %w", err)
	}

	// Route by config type.
	var result *UpResult
	var upErr error
	if len(cfg.DockerComposeFile) > 0 {
		result, upErr = e.upCompose(ctx, ws, cfg, workspaceFolder, opts)
	} else {
		result, upErr = e.upSingle(ctx, ws, cfg, workspaceFolder, opts)
	}

	// Save final result with probed environment. An early result (without
	// remoteEnv) is saved in setupAndReturn before lifecycle hooks run so
	// crib exec/shell work while hooks are still executing.
	if result != nil {
		e.saveResult(ws, cfg, result)
	}

	if upErr != nil {
		return nil, upErr
	}

	return result, nil
}

// saveResult persists the workspace result to disk so crib exec/shell can
// find the container, workspace folder, user, and environment.
func (e *Engine) saveResult(ws *workspace.Workspace, cfg *config.DevContainerConfig, result *UpResult) {
	ws.LastUsedAt = time.Now()
	if err := e.store.Save(ws); err != nil {
		e.logger.Warn("failed to update workspace timestamps", "error", err)
	}

	mergedJSON, _ := json.Marshal(cfg)
	wsResult := &workspace.Result{
		ContainerID:     result.ContainerID,
		MergedConfig:    mergedJSON,
		WorkspaceFolder: result.WorkspaceFolder,
		RemoteEnv:       cfg.RemoteEnv,
		RemoteUser:      result.RemoteUser,
	}
	if err := e.store.SaveResult(ws.ID, wsResult); err != nil {
		e.logger.Warn("failed to save workspace result", "error", err)
	}
}

// Stop stops the container for the given workspace.
func (e *Engine) Stop(ctx context.Context, ws *workspace.Workspace) error {
	e.logger.Debug("stop", "workspace", ws.ID)

	// For compose workspaces, use compose stop to stop all services.
	if result, err := e.store.LoadResult(ws.ID); err == nil && result != nil {
		var cfg config.DevContainerConfig
		if json.Unmarshal(result.MergedConfig, &cfg) == nil && len(cfg.DockerComposeFile) > 0 {
			if e.compose != nil {
				configDir := filepath.Dir(filepath.Join(ws.Source, ws.DevContainerPath))
				composeFiles := resolveComposeFiles(configDir, cfg.DockerComposeFile)
				projectName := compose.ProjectName(ws.ID)
				env := devcontainerEnv(ws.ID, ws.Source, result.WorkspaceFolder)
				return e.compose.Stop(ctx, projectName, composeFiles, e.stdout, e.stderr, env)
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

	return e.driver.StopContainer(ctx, ws.ID, container.ID)
}

// Delete removes the container and workspace state.
func (e *Engine) Delete(ctx context.Context, ws *workspace.Workspace) error {
	e.logger.Debug("delete", "workspace", ws.ID)

	// For compose workspaces, use compose down to remove all services.
	if result, err := e.store.LoadResult(ws.ID); err == nil && result != nil {
		var cfg config.DevContainerConfig
		if json.Unmarshal(result.MergedConfig, &cfg) == nil && len(cfg.DockerComposeFile) > 0 {
			if e.compose != nil {
				configDir := filepath.Dir(filepath.Join(ws.Source, ws.DevContainerPath))
				composeFiles := resolveComposeFiles(configDir, cfg.DockerComposeFile)
				projectName := compose.ProjectName(ws.ID)
				env := devcontainerEnv(ws.ID, ws.Source, result.WorkspaceFolder)
				if err := e.compose.Down(ctx, projectName, composeFiles, e.stdout, e.stderr, env); err != nil {
					e.logger.Warn("failed to bring down compose services", "error", err)
				}
			}
			return e.store.Delete(ws.ID)
		}
	}

	// Non-compose path: remove the individual container.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return fmt.Errorf("finding container: %w", err)
	}
	if container != nil {
		if err := e.driver.DeleteContainer(ctx, ws.ID, container.ID); err != nil {
			return fmt.Errorf("deleting container: %w", err)
		}
	}

	return e.store.Delete(ws.ID)
}

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
}

// configChangeKind classifies what changed between stored and current config.
type configChangeKind int

const (
	changeNone       configChangeKind = iota // Nothing changed.
	changeSafe                               // Volumes, ports, env, mounts — container recreate is sufficient.
	changeNeedsRebuild                       // Image, Dockerfile, features — full rebuild required.
)

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
	configPath := filepath.Join(ws.Source, ws.DevContainerPath)
	cfg, err := config.Parse(configPath)
	if err != nil {
		return nil, fmt.Errorf("parsing devcontainer config: %w", err)
	}

	workspaceFolder := resolveWorkspaceFolder(cfg, ws.Source)
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
		return nil, fmt.Errorf("substituting variables: %w", err)
	}
	workspaceFolder = resolveWorkspaceFolder(cfg, ws.Source)

	// Detect what changed.
	var storedCfg config.DevContainerConfig
	if err := json.Unmarshal(storedResult.MergedConfig, &storedCfg); err != nil {
		return nil, fmt.Errorf("unmarshaling stored config: %w", err)
	}

	change := detectConfigChange(&storedCfg, cfg)

	switch change {
	case changeNeedsRebuild:
		return nil, fmt.Errorf("config changes require a full rebuild (image, Dockerfile, or features changed); run 'crib rebuild' instead")

	case changeSafe:
		e.reportProgress("Config changes detected, recreating container...")
		result, err := e.restartWithRecreate(ctx, ws, cfg, workspaceFolder)
		if err != nil {
			return nil, err
		}
		result.Recreated = true
		return result, nil

	default:
		// No changes — simple restart.
		e.reportProgress("Restarting container...")
		return e.restartSimple(ctx, ws, cfg, workspaceFolder, storedResult)
	}
}

// restartSimple performs a simple container restart without recreation.
func (e *Engine) restartSimple(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string, storedResult *workspace.Result) (*RestartResult, error) {
	// For compose workspaces, use compose restart.
	if len(cfg.DockerComposeFile) > 0 {
		if e.compose == nil {
			return nil, fmt.Errorf("compose is not available")
		}
		configDir := filepath.Dir(filepath.Join(ws.Source, ws.DevContainerPath))
		composeFiles := resolveComposeFiles(configDir, cfg.DockerComposeFile)
		projectName := compose.ProjectName(ws.ID)
		env := devcontainerEnv(ws.ID, ws.Source, workspaceFolder)
		if err := e.compose.Restart(ctx, projectName, composeFiles, e.stdout, e.stderr, env); err != nil {
			return nil, fmt.Errorf("restarting compose services: %w", err)
		}
	} else {
		// Non-compose: restart the individual container.
		container, err := e.driver.FindContainer(ctx, ws.ID)
		if err != nil {
			return nil, fmt.Errorf("finding container: %w", err)
		}
		if container == nil {
			return nil, fmt.Errorf("no container found for workspace %s", ws.ID)
		}
		if err := e.driver.RestartContainer(ctx, ws.ID, container.ID); err != nil {
			return nil, fmt.Errorf("restarting container: %w", err)
		}
	}

	// Run resume-flow hooks.
	containerID := storedResult.ContainerID
	remoteUser := storedResult.RemoteUser
	if err := e.runResumeHooks(ctx, ws, cfg, containerID, workspaceFolder, remoteUser); err != nil {
		e.logger.Warn("resume hooks failed", "error", err)
	}

	// Update timestamps.
	e.saveResult(ws, cfg, &UpResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	})

	return &RestartResult{
		ContainerID:     containerID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	}, nil
}

// restartWithRecreate stops the container, recreates it with the new config,
// and runs resume-flow hooks (not the full creation lifecycle).
func (e *Engine) restartWithRecreate(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string) (*RestartResult, error) {
	// Stop first.
	if err := e.Stop(ctx, ws); err != nil {
		e.logger.Warn("failed to stop before recreate", "error", err)
	}

	// For compose: down + up (picks up volume/env/port changes).
	if len(cfg.DockerComposeFile) > 0 {
		if e.compose == nil {
			return nil, fmt.Errorf("compose is not available")
		}
		configDir := filepath.Dir(filepath.Join(ws.Source, ws.DevContainerPath))
		composeFiles := resolveComposeFiles(configDir, cfg.DockerComposeFile)
		projectName := compose.ProjectName(ws.ID)
		env := devcontainerEnv(ws.ID, ws.Source, workspaceFolder)

		// Down removes old containers so Up creates new ones with updated config.
		if err := e.compose.Down(ctx, projectName, composeFiles, e.stdout, e.stderr, env); err != nil {
			return nil, fmt.Errorf("compose down: %w", err)
		}

		// Generate override and bring services up.
		overridePath, err := e.generateComposeOverride(ws, cfg, workspaceFolder, configDir, composeFiles, "")
		if err != nil {
			return nil, fmt.Errorf("generating compose override: %w", err)
		}
		defer func() { _ = os.Remove(overridePath) }()

		allFiles := append(composeFiles[:len(composeFiles):len(composeFiles)], overridePath)
		services := ensureServiceIncluded(cfg.RunServices, cfg.Service)

		e.reportProgress("Starting services...")
		if err := e.compose.Up(ctx, projectName, allFiles, services, e.stdout, e.stderr, env); err != nil {
			return nil, fmt.Errorf("compose up: %w", err)
		}

		container, err := e.findComposeContainer(ctx, ws.ID, projectName, allFiles, env, "after restart recreate")
		if err != nil {
			return nil, err
		}

		remoteUser := e.resolveRemoteUser(ctx, ws, cfg, container.ID)
		if err := e.runResumeHooks(ctx, ws, cfg, container.ID, workspaceFolder, remoteUser); err != nil {
			e.logger.Warn("resume hooks failed", "error", err)
		}

		e.saveResult(ws, cfg, &UpResult{
			ContainerID:     container.ID,
			WorkspaceFolder: workspaceFolder,
			RemoteUser:      remoteUser,
		})

		return &RestartResult{
			ContainerID:     container.ID,
			WorkspaceFolder: workspaceFolder,
			RemoteUser:      remoteUser,
		}, nil
	}

	// Non-compose: delete old container and create a new one with updated config.
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding container: %w", err)
	}
	if container != nil {
		if err := e.driver.DeleteContainer(ctx, ws.ID, container.ID); err != nil {
			return nil, fmt.Errorf("deleting container: %w", err)
		}
	}

	// We need the image name. For image-based, it's in the config.
	// For Dockerfile-based, use the stored result's image.
	storedResult, _ := e.store.LoadResult(ws.ID)
	imageName := cfg.Image
	if imageName == "" && storedResult != nil {
		imageName = storedResult.ImageName
	}
	if imageName == "" {
		return nil, fmt.Errorf("cannot determine image name; run 'crib rebuild' instead")
	}

	runOpts := e.buildRunOptions(cfg, imageName, ws.Source, workspaceFolder)
	e.reportProgress("Creating container...")
	if err := e.driver.RunContainer(ctx, ws.ID, runOpts); err != nil {
		return nil, fmt.Errorf("creating container: %w", err)
	}

	container, err = e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("finding new container: %w", err)
	}
	if container == nil {
		return nil, fmt.Errorf("container not found after recreation")
	}

	remoteUser := e.resolveRemoteUser(ctx, ws, cfg, container.ID)
	if err := e.runResumeHooks(ctx, ws, cfg, container.ID, workspaceFolder, remoteUser); err != nil {
		e.logger.Warn("resume hooks failed", "error", err)
	}

	e.saveResult(ws, cfg, &UpResult{
		ContainerID:     container.ID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	})

	return &RestartResult{
		ContainerID:     container.ID,
		WorkspaceFolder: workspaceFolder,
		RemoteUser:      remoteUser,
	}, nil
}

// resolveRemoteUser determines the remote user for a container, following the
// same logic as setupAndReturn but without running full setup.
func (e *Engine) resolveRemoteUser(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, containerID string) string {
	remoteUser := cfg.RemoteUser
	if remoteUser == "" {
		remoteUser = cfg.ContainerUser
	}
	if remoteUser == "" {
		remoteUser = e.detectContainerUser(ctx, ws.ID, containerID)
	}
	if remoteUser == "" {
		remoteUser = "root"
	}
	return remoteUser
}

// runResumeHooks executes only the resume-flow lifecycle hooks
// (postStartCommand + postAttachCommand) for a container.
func (e *Engine) runResumeHooks(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, containerID, workspaceFolder, remoteUser string) error {
	runner := &lifecycleRunner{
		driver:      e.driver,
		store:       e.store,
		workspaceID: ws.ID,
		containerID: containerID,
		remoteUser:  remoteUser,
		remoteEnv:   cfg.RemoteEnv,
		logger:      e.logger,
		stdout:      e.stdout,
		stderr:      e.stderr,
		progress:    e.progress,
	}
	return runner.runResumeHooks(ctx, cfg, workspaceFolder)
}

// detectConfigChange compares a stored config with a freshly parsed config
// and classifies the changes.
func detectConfigChange(stored, current *config.DevContainerConfig) configChangeKind {
	// Check image-affecting changes.
	if stored.Image != current.Image {
		return changeNeedsRebuild
	}
	if stored.Dockerfile != current.Dockerfile {
		return changeNeedsRebuild
	}
	if !buildOptsEqual(stored.Build, current.Build) {
		return changeNeedsRebuild
	}
	if !featuresEqual(stored.Features, current.Features) {
		return changeNeedsRebuild
	}

	// Check safe changes (container runtime config).
	if !stringMapsEqual(stored.ContainerEnv, current.ContainerEnv) {
		return changeSafe
	}
	// Note: RemoteEnv is intentionally not compared here. The stored config
	// includes probed environment values (from userEnvProbe) merged into
	// RemoteEnv during setup, which won't be present in a freshly parsed
	// config. Also, remoteEnv is injected at exec time via -e flags, so
	// changes don't require container recreation.
	if stored.ContainerUser != current.ContainerUser {
		return changeSafe
	}
	if stored.RemoteUser != current.RemoteUser {
		return changeSafe
	}
	if stored.WorkspaceMount != current.WorkspaceMount {
		return changeSafe
	}
	if stored.WorkspaceFolder != current.WorkspaceFolder {
		return changeSafe
	}
	if !mountsEqual(stored.Mounts, current.Mounts) {
		return changeSafe
	}
	if !strSlicesEqual(stored.RunArgs, current.RunArgs) {
		return changeSafe
	}
	if !strSlicesEqual([]string(stored.AppPort), []string(current.AppPort)) {
		return changeSafe
	}
	if !strSlicesEqual([]string(stored.ForwardPorts), []string(current.ForwardPorts)) {
		return changeSafe
	}
	if !boolPtrEqual(stored.Init, current.Init) {
		return changeSafe
	}
	if !boolPtrEqual(stored.Privileged, current.Privileged) {
		return changeSafe
	}
	if !strSlicesEqual(stored.CapAdd, current.CapAdd) {
		return changeSafe
	}
	if !strSlicesEqual(stored.SecurityOpt, current.SecurityOpt) {
		return changeSafe
	}
	if !boolPtrEqual(stored.OverrideCommand, current.OverrideCommand) {
		return changeSafe
	}

	// Check compose-specific safe changes.
	if !strSlicesEqual([]string(stored.DockerComposeFile), []string(current.DockerComposeFile)) {
		return changeSafe
	}
	if stored.Service != current.Service {
		return changeSafe
	}
	if !strSlicesEqual(stored.RunServices, current.RunServices) {
		return changeSafe
	}

	return changeNone
}

// --- comparison helpers ---

func stringMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func strSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func mountsEqual(a, b []config.Mount) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func buildOptsEqual(a, b *config.ConfigBuildOptions) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Dockerfile != b.Dockerfile || a.Context != b.Context || a.Target != b.Target {
		return false
	}
	if !strSlicesEqual([]string(a.CacheFrom), []string(b.CacheFrom)) {
		return false
	}
	if !strSlicesEqual(a.Options, b.Options) {
		return false
	}
	// Compare args.
	if len(a.Args) != len(b.Args) {
		return false
	}
	for k, v := range a.Args {
		bv, ok := b.Args[k]
		if !ok {
			return false
		}
		if (v == nil) != (bv == nil) {
			return false
		}
		if v != nil && *v != *bv {
			return false
		}
	}
	return true
}

func featuresEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	// Compare via JSON serialization for deep equality of arbitrary types.
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

// Status returns the current container details for a workspace, or nil if not found.
func (e *Engine) Status(ctx context.Context, ws *workspace.Workspace) (*driver.ContainerDetails, error) {
	return e.driver.FindContainer(ctx, ws.ID)
}

// resolveComposeFiles resolves compose file paths relative to configDir.
func resolveComposeFiles(configDir string, paths []string) []string {
	files := make([]string, len(paths))
	for i, f := range paths {
		files[i] = filepath.Join(configDir, f)
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
