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

	cfg, workspaceFolder, err := e.parseAndSubstitute(ws)
	if err != nil {
		return nil, err
	}

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
				cd := configDir(ws)
				composeFiles := resolveComposeFiles(cd, cfg.DockerComposeFile)
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
				cd := configDir(ws)
				composeFiles := resolveComposeFiles(cd, cfg.DockerComposeFile)
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

// Status returns the current container details for a workspace, or nil if not found.
func (e *Engine) Status(ctx context.Context, ws *workspace.Workspace) (*driver.ContainerDetails, error) {
	return e.driver.FindContainer(ctx, ws.ID)
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

// resolveRemoteUser determines the remote user for a container, using the
// config's remoteUser/containerUser with fallback to detecting the container's
// default user via whoami.
func (e *Engine) resolveRemoteUser(ctx context.Context, workspaceID string, cfg *config.DevContainerConfig, containerID string) string {
	remoteUser := cfg.RemoteUser
	if remoteUser == "" {
		remoteUser = cfg.ContainerUser
	}
	if remoteUser == "" {
		remoteUser = e.detectContainerUser(ctx, workspaceID, containerID)
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

// recreateComposeServices tears down and recreates compose services for the
// given workspace. It generates a compose override, brings services up, and
// returns the primary service container ID. featureImage is the image name to
// override the primary service with (empty string to skip the override).
func (e *Engine) recreateComposeServices(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder, featureImage string) (string, error) {
	cd := configDir(ws)
	composeFiles := resolveComposeFiles(cd, cfg.DockerComposeFile)
	projectName := compose.ProjectName(ws.ID)
	env := devcontainerEnv(ws.ID, ws.Source, workspaceFolder)

	// Down removes old containers so Up creates new ones with updated config.
	if err := e.compose.Down(ctx, projectName, composeFiles, e.stdout, e.stderr, env); err != nil {
		return "", fmt.Errorf("compose down: %w", err)
	}

	// Generate override and bring services up.
	overridePath, err := e.generateComposeOverride(ws, cfg, workspaceFolder, cd, composeFiles, featureImage)
	if err != nil {
		return "", fmt.Errorf("generating compose override: %w", err)
	}
	defer func() { _ = os.Remove(overridePath) }()

	allFiles := append(composeFiles[:len(composeFiles):len(composeFiles)], overridePath)
	services := ensureServiceIncluded(cfg.RunServices, cfg.Service)

	e.reportProgress("Starting services...")
	if err := e.compose.Up(ctx, projectName, allFiles, services, e.stdout, e.stderr, env); err != nil {
		return "", fmt.Errorf("compose up: %w", err)
	}

	container, err := e.findComposeContainer(ctx, ws.ID, projectName, allFiles, env, "after recreate")
	if err != nil {
		return "", err
	}

	return container.ID, nil
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
