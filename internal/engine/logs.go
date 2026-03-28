package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/workspace"
)

// LogsOptions controls the behavior of the Logs operation.
type LogsOptions struct {
	Follow bool   // stream logs as they are produced
	Tail   string // number of lines from the end ("all" or a number)
}

// Logs streams container logs for the given workspace.
// For compose workspaces, shows logs from all services.
func (e *Engine) Logs(ctx context.Context, ws *workspace.Workspace, opts LogsOptions) error {
	// Load stored result to get container info and detect compose.
	storedResult, err := e.store.LoadResult(ws.ID)
	if err != nil {
		return fmt.Errorf("loading workspace result: %w", err)
	}
	if storedResult == nil {
		return fmt.Errorf("no previous result found for workspace %s (run 'crib up' first)", ws.ID)
	}

	// Check if this is a compose workspace.
	var cfg config.DevContainerConfig
	if err := json.Unmarshal(storedResult.MergedConfig, &cfg); err != nil {
		return fmt.Errorf("unmarshaling stored config: %w", err)
	}
	if len(cfg.DockerComposeFile) > 0 {
		return e.logsCompose(ctx, ws, storedResult, &cfg, opts)
	}

	return e.logsSingle(ctx, ws, storedResult, opts)
}

// logsSingle streams logs from a single container.
func (e *Engine) logsSingle(ctx context.Context, ws *workspace.Workspace, storedResult *workspace.Result, opts LogsOptions) error { //nolint:unparam // storedResult kept for API symmetry with logsCompose
	container, err := e.driver.FindContainer(ctx, ws.ID)
	if err != nil {
		return fmt.Errorf("finding container: %w", err)
	}
	if container == nil {
		return fmt.Errorf("no container found for workspace %s", ws.ID)
	}

	driverOpts := &driver.LogsOptions{
		Follow: opts.Follow,
		Tail:   opts.Tail,
	}
	return e.driver.ContainerLogs(ctx, ws.ID, container.ID, e.stdout, e.stderr, driverOpts)
}

// logsCompose streams logs from all compose services.
func (e *Engine) logsCompose(ctx context.Context, ws *workspace.Workspace, storedResult *workspace.Result, cfg *config.DevContainerConfig, opts LogsOptions) error {
	if e.compose == nil {
		return fmt.Errorf("compose is not available")
	}

	cd := configDir(ws)
	composeFiles := resolveComposeFiles(cd, cfg.DockerComposeFile)
	projectName := compose.ProjectName(ws.ID)
	env := devcontainerEnv(ws.ID, ws.Source, storedResult.WorkspaceFolder)

	return e.compose.Logs(ctx, projectName, composeFiles, opts.Follow, opts.Tail, e.stdout, e.stderr, env)
}
