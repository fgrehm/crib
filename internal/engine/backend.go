package engine

import (
	"context"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// containerBackend encapsulates the container-type-specific operations.
// Implementations handle container lifecycle only; plugin dispatch, file
// copies, env wiring, lifecycle hooks, and result saving live in the
// shared orchestration layer.
type containerBackend interface {
	// pluginUser returns the remote user for plugin dispatch.
	//
	// For compose backends, resolves from compose config (ignores fallbacks).
	// For single backends, returns configRemoteUser(cfg) if set, otherwise
	// iterates fallbacks in order and returns the first non-empty value.
	//
	// The contract: config always wins over fallbacks.
	pluginUser(ctx context.Context, fallbacks ...string) string

	// start brings up a stopped container (and dependent services for compose).
	// pluginResp is passed for compose override regeneration.
	// Returns the container ID (may differ from input for compose).
	start(ctx context.Context, containerID string, pluginResp *plugin.PreContainerRunResponse) (string, error)

	// buildImage builds the container image(s).
	// Single: Dockerfile build or image pull.
	// Compose: feature layer only (service build runs inside createContainer).
	buildImage(ctx context.Context) (*buildResult, error)

	// createContainer creates and starts a new container from the given image.
	// Single: build RunOptions, merge pluginResp, RunContainer.
	// Compose: generate override, compose build, compose up.
	createContainer(ctx context.Context, opts createOpts) (string, error)

	// deleteExisting removes all containers for the workspace.
	// Single: driver.DeleteContainer. Compose: composeDown.
	deleteExisting(ctx context.Context) error

	// restart restarts the container without recreation.
	// Single: driver.RestartContainer.
	// Compose: regenerate override, compose stop, compose start.
	// pluginResp is passed for compose override regeneration.
	restart(ctx context.Context, containerID string, pluginResp *plugin.PreContainerRunResponse) (string, error)

	// canResumeFromStored returns true if the backend can bring services up
	// from a stored result without rebuilding. Compose: true (images exist).
	// Single: false (Dockerfile-built images may have been pruned).
	canResumeFromStored() bool
}

// createOpts bundles parameters for createContainer.
type createOpts struct {
	imageName      string
	hasEntrypoints bool
	metadata       []*config.ImageMetadata // nil when creating from stored/snapshot
	pluginResp     *plugin.PreContainerRunResponse
	skipBuild      bool // true when resuming from stored result (images exist)
}

// Compile-time interface checks.
var _ containerBackend = (*singleBackend)(nil)
var _ containerBackend = (*composeBackend)(nil)

// newBackend creates the appropriate backend based on config type.
func (e *Engine) newBackend(ws *workspace.Workspace, cfg *config.DevContainerConfig, workspaceFolder string) containerBackend {
	if len(cfg.DockerComposeFile) > 0 {
		return &composeBackend{
			e:               e,
			ws:              ws,
			cfg:             cfg,
			workspaceFolder: workspaceFolder,
			inv:             newComposeInvocation(ws, cfg, workspaceFolder),
		}
	}
	return &singleBackend{
		e:               e,
		ws:              ws,
		cfg:             cfg,
		workspaceFolder: workspaceFolder,
	}
}
