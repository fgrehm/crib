package globalconfig

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds user-level crib settings from ~/.config/crib/config.toml.
type Config struct {
	Dotfiles  DotfilesConfig  `toml:"dotfiles"`
	Plugins   PluginsConfig   `toml:"plugins"`
	Workspace WorkspaceConfig `toml:"workspace"`
}

// DotfilesConfig configures dotfiles repository cloning and installation.
type DotfilesConfig struct {
	Repository     string `toml:"repository"`
	TargetPath     string `toml:"targetPath"`
	InstallCommand string `toml:"installCommand"`
}

// PluginsConfig disables bundled plugins globally. Disable lists specific
// plugins by name; DisableAll is a kill switch that skips plugin registration
// entirely.
type PluginsConfig struct {
	Disable    []string `toml:"disable"`
	DisableAll bool     `toml:"disable_all"`
}

// WorkspaceConfig is applied to every container on top of project-level
// configuration. Project values win on key conflicts.
type WorkspaceConfig struct {
	Env     map[string]string `toml:"env"`
	Mounts  []string          `toml:"mount"`
	RunArgs []string          `toml:"run_args"`
}

// CribRC holds values loaded from a per-project .cribrc file.
type CribRC struct {
	// Config sets the devcontainer config directory (equivalent to -C/--config).
	Config string `toml:"config"`

	// Cache lists package cache providers. Accepts a TOML array or a
	// comma-separated string for backward compatibility with the pre-TOML
	// .cribrc format.
	Cache StringList `toml:"cache"`

	// Dotfiles overrides global dotfiles settings and carries the kill switch.
	Dotfiles DotfilesRC `toml:"dotfiles"`

	// Plugins disables bundled plugins for the current project.
	Plugins PluginsRC `toml:"plugins"`

	// Workspace is the per-project workspace section (env, mounts, run_args).
	Workspace WorkspaceConfig `toml:"workspace"`
}

// DotfilesRC mirrors DotfilesConfig plus a `dotfiles = "false"` kill switch
// handled in UnmarshalTOML.
type DotfilesRC struct {
	Disabled       bool
	Repository     string `toml:"repository"`
	TargetPath     string `toml:"targetPath"`
	InstallCommand string `toml:"installCommand"`
}

// PluginsRC mirrors PluginsConfig but also honors a `plugins = "false"`
// kill switch handled in UnmarshalTOML.
type PluginsRC struct {
	DisableAll bool
	Disable    StringList `toml:"disable"`
}

// pluginsRCAlias breaks UnmarshalTOML recursion — see dotfilesRCAlias.
type pluginsRCAlias PluginsRC

// StringList accepts either a TOML array of strings or a single
// comma-separated string. It trims each entry and drops empties. The
// comma-separated form exists for backward compatibility with the pre-TOML
// .cribrc format (e.g. `plugins.disable = ssh, dotfiles`).
type StringList []string

// UnmarshalTOML implements toml.Unmarshaler.
func (s *StringList) UnmarshalTOML(v any) error {
	switch val := v.(type) {
	case string:
		*s = splitCSV(val)
		return nil
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			str, ok := item.(string)
			if !ok {
				return fmt.Errorf("expected string in list, got %T", item)
			}
			str = strings.TrimSpace(str)
			if str != "" {
				out = append(out, str)
			}
		}
		*s = out
		return nil
	default:
		return fmt.Errorf("expected string or array of strings, got %T", v)
	}
}

// decodeTable re-encodes a decoded TOML table as TOML bytes and decodes it
// into dst. Used by UnmarshalTOML implementations that accept either a
// scalar (handled inline) or a nested table (delegated here) so struct tags
// on dst stay authoritative.
func decodeTable(m map[string]any, dst any) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(m); err != nil {
		return fmt.Errorf("re-encoding TOML table: %w", err)
	}
	if _, err := toml.Decode(buf.String(), dst); err != nil {
		return fmt.Errorf("decoding TOML table: %w", err)
	}
	return nil
}

func splitCSV(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// dotfilesRCAlias exists so UnmarshalTOML can delegate table decoding back
// to the library without recursing into DotfilesRC's own UnmarshalTOML. The
// alias inherits every struct tag, so adding a field to DotfilesRC requires
// no change here.
type dotfilesRCAlias DotfilesRC

// UnmarshalTOML accepts either a nested table (dotfiles.repository, etc.) or
// a legacy kill switch: the boolean `dotfiles = false` (pre-TOML .cribrc) and
// the quoted string `dotfiles = "false"` both set Disabled. Table decoding is
// delegated to the tagged struct so field names stay in one place.
func (d *DotfilesRC) UnmarshalTOML(v any) error {
	if b, ok := v.(bool); ok {
		if !b {
			d.Disabled = true
		}
		return nil
	}
	if s, ok := v.(string); ok {
		if s == "false" {
			d.Disabled = true
		}
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("dotfiles: expected table or string, got %T", v)
	}
	var a dotfilesRCAlias
	if err := decodeTable(m, &a); err != nil {
		return err
	}
	*d = DotfilesRC(a)
	return nil
}

// UnmarshalTOML accepts either a nested table (plugins.disable, etc.) or a
// legacy kill switch: the boolean `plugins = false` (pre-TOML .cribrc) and
// the quoted string `plugins = "false"` both set DisableAll. Table decoding
// is delegated to the tagged struct so field names stay in one place.
func (p *PluginsRC) UnmarshalTOML(v any) error {
	if b, ok := v.(bool); ok {
		if !b {
			p.DisableAll = true
		}
		return nil
	}
	if s, ok := v.(string); ok {
		if s == "false" {
			p.DisableAll = true
		}
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("plugins: expected table or string, got %T", v)
	}
	var a pluginsRCAlias
	if err := decodeTable(m, &a); err != nil {
		return err
	}
	*p = PluginsRC(a)
	return nil
}

// Load reads the global config from the default path.
// Returns a zero Config (not an error) if the file does not exist.
func Load() (*Config, error) {
	return LoadFrom(DefaultPath())
}

// LoadFrom reads the global config from the given path.
// Returns a zero Config (not an error) if the file does not exist.
func LoadFrom(path string) (*Config, error) {
	var cfg Config
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &cfg, nil
		}
		return nil, err
	}
	cfg.applyDefaults()
	return &cfg, nil
}

// LoadCribRC reads a .cribrc file at the given path. Returns a zero CribRC
// (not an error) if the file does not exist.
func LoadCribRC(path string) (*CribRC, error) {
	var rc CribRC
	_, err := toml.DecodeFile(path, &rc)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &rc, nil
		}
		return nil, err
	}
	return &rc, nil
}

// DefaultPath returns the config file location, respecting XDG_CONFIG_HOME.
func DefaultPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "crib", "config.toml")
}

func (c *Config) applyDefaults() {
	if c.Dotfiles.Repository != "" && c.Dotfiles.TargetPath == "" {
		c.Dotfiles.TargetPath = "~/dotfiles"
	}
}
