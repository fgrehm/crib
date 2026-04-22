package globalconfig

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds user-level crib settings from ~/.config/crib/config.toml.
type Config struct {
	Dotfiles DotfilesConfig `toml:"dotfiles"`
	Plugins  PluginsConfig  `toml:"plugins"`
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
