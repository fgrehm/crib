package sandbox

import (
	"testing"
)

func TestParseConfig_Nil(t *testing.T) {
	if parseConfig(nil) != nil {
		t.Error("expected nil for nil customizations")
	}
}

func TestParseConfig_NoSandboxKey(t *testing.T) {
	if parseConfig(map[string]any{"other": true}) != nil {
		t.Error("expected nil when sandbox key missing")
	}
}

func TestParseConfig_InvalidType(t *testing.T) {
	if parseConfig(map[string]any{"sandbox": "not-a-map"}) != nil {
		t.Error("expected nil for non-map sandbox value")
	}
}

func TestParseConfig_Empty(t *testing.T) {
	cfg := parseConfig(map[string]any{"sandbox": map[string]any{}})
	if cfg == nil {
		t.Fatal("expected non-nil config for empty sandbox map")
	}
	if cfg.BlockLocalNetwork {
		t.Error("expected blockLocalNetwork=false by default")
	}
	if len(cfg.Aliases) != 0 {
		t.Error("expected empty aliases")
	}
}

func TestParseConfig_Full(t *testing.T) {
	cfg := parseConfig(map[string]any{
		"sandbox": map[string]any{
			"denyRead":          []any{"~/.ssh/config"},
			"denyWrite":         []any{"~/.ssh", "~/.claude"},
			"allowWrite":        []any{"/var/log"},
			"hideFiles":         []any{".env.staging", "config/secrets.yml"},
			"blockLocalNetwork": true,
			"aliases":           []any{"claude", "pi", "aider"},
		},
	})
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if !cfg.BlockLocalNetwork {
		t.Error("expected blockLocalNetwork=true")
	}
	if len(cfg.DenyRead) != 1 || cfg.DenyRead[0] != "~/.ssh/config" {
		t.Errorf("unexpected denyRead: %v", cfg.DenyRead)
	}
	if len(cfg.DenyWrite) != 2 {
		t.Errorf("expected 2 denyWrite entries, got %d", len(cfg.DenyWrite))
	}
	if len(cfg.AllowWrite) != 1 || cfg.AllowWrite[0] != "/var/log" {
		t.Errorf("unexpected allowWrite: %v", cfg.AllowWrite)
	}
	if len(cfg.HideFiles) != 2 || cfg.HideFiles[0] != ".env.staging" {
		t.Errorf("unexpected hideFiles: %v", cfg.HideFiles)
	}
	if len(cfg.Aliases) != 3 {
		t.Errorf("expected 3 aliases, got %d", len(cfg.Aliases))
	}
}

func TestToStringSlice_NilInput(t *testing.T) {
	if toStringSlice(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestToStringSlice_NonArrayInput(t *testing.T) {
	if toStringSlice("not-an-array") != nil {
		t.Error("expected nil for non-array input")
	}
}

func TestToStringSlice_MixedTypes(t *testing.T) {
	result := toStringSlice([]any{"a", 42, "b", true})
	if len(result) != 2 || result[0] != "a" || result[1] != "b" {
		t.Errorf("expected [a b], got %v", result)
	}
}
