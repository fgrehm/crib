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

// Names returns the names of registered plugins in registration order.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.plugins))
	for _, p := range m.plugins {
		names = append(names, p.Name())
	}
	return names
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
// registered plugins. Called between create-time hooks (postCreateCommand)
// and start-time hooks (postStartCommand). Errors are logged and skipped
// (fail-open).
func (m *Manager) RunPostContainerCreate(ctx context.Context, req *PostContainerCreateRequest) {
	for _, p := range m.plugins {
		if m.progress != nil {
			m.progress("  Running plugin: " + p.Name())
		}
		if _, err := p.PostContainerCreate(ctx, req); err != nil {
			m.logger.Warn("plugin post-create error, skipping", "plugin", p.Name(), "error", err)
		}
	}
}
