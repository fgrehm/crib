package codingagents

import (
	"context"
	"log/slog"

	"github.com/fgrehm/crib/internal/plugin"
)

// Plugin provides coding-agent credential sharing for Claude Code and pi.
// Both agents behave identically under the shared `credentials` customization
// ("host" default, or "workspace") and can run side-by-side in the same
// container. Per-agent handling lives in claude.go and pi.go.
type Plugin struct {
	plugin.BasePlugin
	homeDir string // overridable for testing; defaults to os.UserHomeDir()
}

// New creates a coding-agents plugin that uses the real user home directory.
func New() *Plugin {
	return &Plugin{}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string { return "coding-agents" }

// PreContainerRun dispatches credential handling for Claude Code and pi.
// Claude is primary; a pi failure is logged at Warn and skipped so it can
// never block Claude credential injection.
func (p *Plugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	mode := getCredentialsMode(req.Customizations)

	resp, err := p.handleClaude(req, mode)
	if err != nil {
		return nil, err
	}

	// pi handling is best-effort: a pi-specific failure must not break Claude
	// credential injection. Log at Warn and continue rather than bubbling up.
	piResp, err := p.handlePi(req, mode)
	if err != nil {
		slog.Warn("coding-agents: pi credential handling failed, skipping pi injection", "error", err)
	} else if piResp != nil {
		if resp == nil {
			resp = &plugin.PreContainerRunResponse{}
		}
		resp.Mounts = append(resp.Mounts, piResp.Mounts...)
		resp.Copies = append(resp.Copies, piResp.Copies...)
	}

	return resp, nil
}

// getCredentialsMode reads the credentials mode from customizations.crib.coding-agents.
// Returns "host" (default) or "workspace".
func getCredentialsMode(customizations map[string]any) string {
	if customizations == nil {
		return "host"
	}
	caConfig, ok := customizations["coding-agents"]
	if !ok {
		return "host"
	}
	m, ok := caConfig.(map[string]any)
	if !ok {
		return "host"
	}
	if creds, ok := m["credentials"].(string); ok && creds == "workspace" {
		return "workspace"
	}
	return "host"
}
