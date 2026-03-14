package plugin

import (
	"context"
	"log/slog"
	"maps"
)

// Manager holds registered plugins and dispatches events to them.
type Manager struct {
	plugins  []Plugin
	logger   *slog.Logger
	progress func(string)
}

// NewManager creates a Manager with the given logger.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{logger: logger}
}

// SetProgress sets a callback for user-facing progress messages (e.g. "Running plugin: foo").
func (m *Manager) SetProgress(fn func(string)) {
	m.progress = fn
}

// Register adds a plugin to the manager.
func (m *Manager) Register(p Plugin) {
	m.plugins = append(m.plugins, p)
}

// RunPreContainerRun dispatches the pre-container-run event to all registered
// plugins and merges their responses. Mounts and RunArgs are appended in plugin
// order. Env vars are merged with last-plugin-wins on conflicts. Plugin errors
// are logged and skipped (fail-open).
func (m *Manager) RunPreContainerRun(ctx context.Context, req *PreContainerRunRequest) (*PreContainerRunResponse, error) {
	merged := &PreContainerRunResponse{
		Env: make(map[string]string),
	}

	for _, p := range m.plugins {
		if m.progress != nil {
			m.progress("  Running plugin: " + p.Name())
		}
		resp, err := p.PreContainerRun(ctx, req)
		if err != nil {
			m.logger.Warn("plugin error, skipping", "plugin", p.Name(), "error", err)
			continue
		}
		if resp == nil {
			continue
		}

		merged.Mounts = append(merged.Mounts, resp.Mounts...)
		merged.RunArgs = append(merged.RunArgs, resp.RunArgs...)
		merged.Copies = append(merged.Copies, resp.Copies...)
		merged.PathPrepend = append(merged.PathPrepend, resp.PathPrepend...)
		maps.Copy(merged.Env, resp.Env)
	}

	return merged, nil
}

// RunPostContainerCreate dispatches the post-container-create event to all
// plugins that implement PostContainerCreator. Plugin errors are logged and
// skipped (fail-open).
func (m *Manager) RunPostContainerCreate(ctx context.Context, req *PostContainerCreateRequest) {
	for _, p := range m.plugins {
		pcc, ok := p.(PostContainerCreator)
		if !ok {
			continue
		}
		if e, ok := p.(PostContainerCreateEnabler); ok && !e.IsPostContainerCreateEnabled(req) {
			continue
		}
		if m.progress != nil {
			m.progress("  Running post-create: " + p.Name())
		}
		if err := pcc.PostContainerCreate(ctx, req); err != nil {
			m.logger.Warn("post-create plugin error, skipping", "plugin", p.Name(), "error", err)
		}
	}
}
