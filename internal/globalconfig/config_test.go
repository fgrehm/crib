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

func TestLoadFrom_Workspace(t *testing.T) {
	path := writeTOMLConfig(t, `
[workspace]
env = { FOO = "bar", BAZ = "qux" }
mount = ["type=bind,source=/a,target=/a"]
run_args = ["--cap-add", "SYS_PTRACE"]
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Workspace.Env["FOO"] != "bar" || cfg.Workspace.Env["BAZ"] != "qux" {
		t.Errorf("Workspace.Env = %v", cfg.Workspace.Env)
	}
	if len(cfg.Workspace.Mounts) != 1 || cfg.Workspace.Mounts[0] != "type=bind,source=/a,target=/a" {
		t.Errorf("Workspace.Mounts = %v", cfg.Workspace.Mounts)
	}
	if len(cfg.Workspace.RunArgs) != 2 || cfg.Workspace.RunArgs[0] != "--cap-add" || cfg.Workspace.RunArgs[1] != "SYS_PTRACE" {
		t.Errorf("Workspace.RunArgs = %v", cfg.Workspace.RunArgs)
	}
}

func TestLoadFrom_WorkspaceEmpty(t *testing.T) {
	path := writeTOMLConfig(t, `[workspace]`)
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(cfg.Workspace.Env) != 0 || len(cfg.Workspace.Mounts) != 0 || len(cfg.Workspace.RunArgs) != 0 {
		t.Errorf("expected zero-value Workspace, got %+v", cfg.Workspace)
	}
}

func TestLoadFrom_WorkspaceMissing(t *testing.T) {
	path := writeTOMLConfig(t, `
[dotfiles]
repository = "x"
`)
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Workspace.Env != nil || cfg.Workspace.Mounts != nil || cfg.Workspace.RunArgs != nil {
		t.Errorf("expected nil Workspace fields, got %+v", cfg.Workspace)
	}
}

func writeCribRC(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".cribrc")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadCribRC_Missing(t *testing.T) {
	rc, err := LoadCribRC(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if rc == nil {
		t.Fatal("expected non-nil zero CribRC")
	}
	if rc.Config != "" || len(rc.Cache) != 0 || rc.Plugins.DisableAll {
		t.Errorf("expected zero CribRC, got %+v", rc)
	}
}

func TestLoadCribRC_Empty(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, ""))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if rc.Config != "" || len(rc.Cache) != 0 {
		t.Errorf("expected empty rc, got %+v", rc)
	}
}

func TestLoadCribRC_ConfigKey(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `config = ".devcontainer-custom"`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if rc.Config != ".devcontainer-custom" {
		t.Errorf("Config = %q", rc.Config)
	}
}

func TestLoadCribRC_CacheArray(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `cache = ["npm", "pip", "go"]`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	want := []string{"npm", "pip", "go"}
	if len(rc.Cache) != len(want) {
		t.Fatalf("Cache len = %d, want %d (%v)", len(rc.Cache), len(want), rc.Cache)
	}
	for i, w := range want {
		if rc.Cache[i] != w {
			t.Errorf("Cache[%d] = %q, want %q", i, rc.Cache[i], w)
		}
	}
}

func TestLoadCribRC_CacheCSV(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `cache = "npm, pip, go"`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	want := []string{"npm", "pip", "go"}
	if len(rc.Cache) != len(want) {
		t.Fatalf("Cache len = %d, want %d (%v)", len(rc.Cache), len(want), rc.Cache)
	}
	for i, w := range want {
		if rc.Cache[i] != w {
			t.Errorf("Cache[%d] = %q, want %q", i, rc.Cache[i], w)
		}
	}
}

func TestLoadCribRC_PluginsDisableArray(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `
[plugins]
disable = ["ssh", "dotfiles"]
`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if len(rc.Plugins.Disable) != 2 || rc.Plugins.Disable[0] != "ssh" || rc.Plugins.Disable[1] != "dotfiles" {
		t.Errorf("Plugins.Disable = %v", rc.Plugins.Disable)
	}
}

func TestLoadCribRC_PluginsDisableCSV(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `plugins.disable = "ssh, dotfiles"`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if len(rc.Plugins.Disable) != 2 || rc.Plugins.Disable[0] != "ssh" || rc.Plugins.Disable[1] != "dotfiles" {
		t.Errorf("Plugins.Disable = %v", rc.Plugins.Disable)
	}
}

func TestLoadCribRC_PluginsDisableSingle(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `plugins.disable = "ssh"`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if len(rc.Plugins.Disable) != 1 || rc.Plugins.Disable[0] != "ssh" {
		t.Errorf("Plugins.Disable = %v", rc.Plugins.Disable)
	}
}

func TestLoadCribRC_PluginsKillSwitch(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `plugins = "false"`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if !rc.Plugins.DisableAll {
		t.Error("expected Plugins.DisableAll = true")
	}
}

func TestLoadCribRC_PluginsKillSwitchUnknown_Ignored(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `plugins = "maybe"`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if rc.Plugins.DisableAll {
		t.Error("expected DisableAll = false for unknown scalar")
	}
}

func TestLoadCribRC_DotfilesDisable(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `dotfiles = "false"`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if !rc.Dotfiles.Disabled {
		t.Error("expected Dotfiles.Disabled = true")
	}
}

func TestLoadCribRC_DotfilesOverrides(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `
[dotfiles]
repository = "git@github.com:user/dots"
targetPath = "~/my-dots"
installCommand = "make install"
`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if rc.Dotfiles.Repository != "git@github.com:user/dots" {
		t.Errorf("Repository = %q", rc.Dotfiles.Repository)
	}
	if rc.Dotfiles.TargetPath != "~/my-dots" {
		t.Errorf("TargetPath = %q", rc.Dotfiles.TargetPath)
	}
	if rc.Dotfiles.InstallCommand != "make install" {
		t.Errorf("InstallCommand = %q", rc.Dotfiles.InstallCommand)
	}
	if rc.Dotfiles.Disabled {
		t.Error("expected Dotfiles.Disabled = false")
	}
}

func TestLoadCribRC_Workspace(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `
[workspace]
env = { FOO = "bar" }
mount = ["type=bind,source=/a,target=/a"]
run_args = ["--cap-add", "SYS_PTRACE"]
`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if rc.Workspace.Env["FOO"] != "bar" {
		t.Errorf("Workspace.Env = %v", rc.Workspace.Env)
	}
	if len(rc.Workspace.Mounts) != 1 {
		t.Errorf("Workspace.Mounts = %v", rc.Workspace.Mounts)
	}
	if len(rc.Workspace.RunArgs) != 2 {
		t.Errorf("Workspace.RunArgs = %v", rc.Workspace.RunArgs)
	}
}

func TestLoadCribRC_AllKeysTogether(t *testing.T) {
	rc, err := LoadCribRC(writeCribRC(t, `
config = ".devcontainer-custom"
cache = ["npm", "pip"]

[dotfiles]
repository = "git@github.com:user/dots"

[plugins]
disable = ["ssh"]
`))
	if err != nil {
		t.Fatalf("LoadCribRC: %v", err)
	}
	if rc.Config != ".devcontainer-custom" {
		t.Errorf("Config = %q", rc.Config)
	}
	if len(rc.Cache) != 2 {
		t.Errorf("Cache = %v", rc.Cache)
	}
	if rc.Dotfiles.Repository != "git@github.com:user/dots" {
		t.Errorf("Dotfiles.Repository = %q", rc.Dotfiles.Repository)
	}
	if len(rc.Plugins.Disable) != 1 || rc.Plugins.Disable[0] != "ssh" {
		t.Errorf("Plugins.Disable = %v", rc.Plugins.Disable)
	}
}
