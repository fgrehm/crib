package plugin

import (
	"context"
	"log/slog"
)

// Manager holds registered plugins and dispatches events to them.
type Manager struct {
	plugins []Plugin
	logger  *slog.Logger
}

// NewManager creates a Manager with the given logger.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{logger: logger}
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
		for k, v := range resp.Env {
			merged.Env[k] = v
		}
	}

	return merged, nil
}
