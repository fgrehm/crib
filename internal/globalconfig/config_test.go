package globalconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTOMLConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFrom_MissingFile(t *testing.T) {
	cfg, err := LoadFrom("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg.Dotfiles.Repository != "" {
		t.Errorf("expected empty repository, got %q", cfg.Dotfiles.Repository)
	}
}

func TestLoadFrom_ValidTOML(t *testing.T) {
	path := writeTOMLConfig(t, `
[dotfiles]
repository = "https://github.com/user/dotfiles"
targetPath = "~/my-dotfiles"
installCommand = "setup.sh"
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Dotfiles.Repository != "https://github.com/user/dotfiles" {
		t.Errorf("Repository = %q", cfg.Dotfiles.Repository)
	}
	if cfg.Dotfiles.TargetPath != "~/my-dotfiles" {
		t.Errorf("TargetPath = %q", cfg.Dotfiles.TargetPath)
	}
	if cfg.Dotfiles.InstallCommand != "setup.sh" {
		t.Errorf("InstallCommand = %q", cfg.Dotfiles.InstallCommand)
	}
}

func TestLoadFrom_MalformedTOML(t *testing.T) {
	path := writeTOMLConfig(t, "[dotfiles\nbroken")

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for malformed TOML")
	}
}

func TestLoadFrom_DefaultTargetPath(t *testing.T) {
	path := writeTOMLConfig(t, `
[dotfiles]
repository = "https://github.com/user/dotfiles"
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Dotfiles.TargetPath != "~/dotfiles" {
		t.Errorf("TargetPath default = %q, want ~/dotfiles", cfg.Dotfiles.TargetPath)
	}
}

func TestLoad_RespectsXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configDir := filepath.Join(dir, "crib")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(`
[dotfiles]
repository = "git@github.com:user/dots.git"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Dotfiles.Repository != "git@github.com:user/dots.git" {
		t.Errorf("Repository = %q", cfg.Dotfiles.Repository)
	}
}

func TestLoad_MissingFileReturnsZero(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Dotfiles.Repository != "" {
		t.Errorf("expected empty config, got repository=%q", cfg.Dotfiles.Repository)
	}
}

func TestLoadFrom_EmptyFile(t *testing.T) {
	path := writeTOMLConfig(t, "")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Dotfiles.Repository != "" {
		t.Errorf("expected empty config from empty file")
	}
}

func TestLoadFrom_PluginsDisableList(t *testing.T) {
	path := writeTOMLConfig(t, `
[plugins]
disable = ["ssh", "dotfiles"]
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	want := []string{"ssh", "dotfiles"}
	if len(cfg.Plugins.Disable) != len(want) {
		t.Fatalf("Disable len = %d, want %d (%v)", len(cfg.Plugins.Disable), len(want), cfg.Plugins.Disable)
	}
	for i, v := range want {
		if cfg.Plugins.Disable[i] != v {
			t.Errorf("Disable[%d] = %q, want %q", i, cfg.Plugins.Disable[i], v)
		}
	}
	if cfg.Plugins.DisableAll {
		t.Error("DisableAll = true, want false")
	}
}

func TestLoadFrom_PluginsDisableAll(t *testing.T) {
	path := writeTOMLConfig(t, `
[plugins]
disable_all = true
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if !cfg.Plugins.DisableAll {
		t.Error("expected DisableAll = true")
	}
}

func TestLoadFrom_PluginsEmptySection(t *testing.T) {
	path := writeTOMLConfig(t, `
[plugins]
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(cfg.Plugins.Disable) != 0 {
		t.Errorf("expected empty Disable list, got %v", cfg.Plugins.Disable)
	}
	if cfg.Plugins.DisableAll {
		t.Error("expected DisableAll = false")
	}
}

func TestLoadFrom_PluginsMissingSection(t *testing.T) {
	path := writeTOMLConfig(t, `
[dotfiles]
repository = "x"
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(cfg.Plugins.Disable) != 0 || cfg.Plugins.DisableAll {
		t.Errorf("expected zero-value Plugins, got %+v", cfg.Plugins)
	}
}
