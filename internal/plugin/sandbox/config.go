package sandbox

// sandboxConfig holds the parsed sandbox configuration from
// customizations.crib.sandbox in devcontainer.json.
type sandboxConfig struct {
	DenyRead          []string // extra paths to deny reads on
	DenyWrite         []string // extra paths to deny writes on
	AllowWrite        []string // extra writable paths beyond workspace + /tmp
	HideFiles         []string // extra files to mask (relative to workspace folder)
	BlockLocalNetwork bool
	Aliases           []string // agent commands to wrap (e.g. "claude", "pi")
}

// parseConfig extracts sandbox config from customizations.crib.
// Returns nil if no sandbox config is present (plugin is a no-op).
func parseConfig(customizations map[string]any) *sandboxConfig {
	if customizations == nil {
		return nil
	}
	raw, ok := customizations["sandbox"]
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	cfg := &sandboxConfig{}

	cfg.DenyRead = toStringSlice(m["denyRead"])
	cfg.DenyWrite = toStringSlice(m["denyWrite"])
	cfg.AllowWrite = toStringSlice(m["allowWrite"])
	cfg.HideFiles = toStringSlice(m["hideFiles"])
	cfg.Aliases = toStringSlice(m["aliases"])

	if v, ok := m["blockLocalNetwork"].(bool); ok {
		cfg.BlockLocalNetwork = v
	}
	return cfg
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
