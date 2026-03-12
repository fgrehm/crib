package sandbox

import (
	"context"

	"github.com/fgrehm/crib/internal/plugin"
)

// Plugin provides coding agent sandboxing via bubblewrap.
// It generates wrapper scripts that restrict filesystem and network access
// for agent processes. No-op when customizations.crib.sandbox is absent.
type Plugin struct{}

// New creates a sandbox plugin.
func New() *Plugin {
	return &Plugin{}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string { return "sandbox" }

// PreContainerRun reads sandbox config and returns RunArgs for network
// capabilities when blockLocalNetwork is enabled. Returns nil (no-op) when
// no sandbox config is present.
func (p *Plugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	cfg := parseConfig(req.Customizations)
	if cfg == nil {
		return nil, nil
	}

	resp := &plugin.PreContainerRunResponse{}

	if cfg.BlockLocalNetwork || cfg.BlockCloudProviders {
		resp.RunArgs = append(resp.RunArgs, "--cap-add=NET_ADMIN", "--cap-add=NET_RAW")
	}

	return resp, nil
}
