package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func setupCribRC(t *testing.T, content string) *cribRC {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".cribrc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	rc, err := loadCribRC()
	if err != nil {
		t.Fatalf("loadCribRC: %v", err)
	}
	return rc
}

func TestLoadCribRC_CacheKey(t *testing.T) {
	rc := setupCribRC(t, "cache = npm, pip, go\n")
	if rc == nil {
		t.Fatal("expected non-nil rc")
	}

	if len(rc.Cache) != 3 {
		t.Fatalf("expected 3 cache providers, got %d: %v", len(rc.Cache), rc.Cache)
	}
	expected := []string{"npm", "pip", "go"}
	for i, want := range expected {
		if rc.Cache[i] != want {
			t.Errorf("Cache[%d] = %q, want %q", i, rc.Cache[i], want)
		}
	}
}

func TestLoadCribRC_BothKeys(t *testing.T) {
	rc := setupCribRC(t, "config = .devcontainer-custom\ncache = npm\n")
	if rc.Config != ".devcontainer-custom" {
		t.Errorf("Config = %q, want .devcontainer-custom", rc.Config)
	}
	if len(rc.Cache) != 1 || rc.Cache[0] != "npm" {
		t.Errorf("Cache = %v, want [npm]", rc.Cache)
	}
}

func TestLoadCribRC_DotfilesDisable(t *testing.T) {
	rc := setupCribRC(t, "dotfiles = false\n")
	if !rc.Dotfiles.Disabled {
		t.Error("expected Dotfiles.Disabled = true")
	}
}

func TestLoadCribRC_DotfilesUnknownValue_Ignored(t *testing.T) {
	rc := setupCribRC(t, "dotfiles = maybe\n")
	if rc.Dotfiles.Disabled {
		t.Error("expected Dotfiles.Disabled = false for unknown value")
	}
}

func TestLoadCribRC_DotfilesRepository(t *testing.T) {
	rc := setupCribRC(t, "dotfiles.repository = git@github.com:user/dots\n")
	if rc.Dotfiles.Repository != "git@github.com:user/dots" {
		t.Errorf("Repository = %q, want git@github.com:user/dots", rc.Dotfiles.Repository)
	}
}

func TestLoadCribRC_DotfilesTargetPath(t *testing.T) {
	rc := setupCribRC(t, "dotfiles.targetPath = ~/my-dots\n")
	if rc.Dotfiles.TargetPath != "~/my-dots" {
		t.Errorf("TargetPath = %q, want ~/my-dots", rc.Dotfiles.TargetPath)
	}
}

func TestLoadCribRC_DotfilesInstallCommand(t *testing.T) {
	rc := setupCribRC(t, "dotfiles.installCommand = make install\n")
	if rc.Dotfiles.InstallCommand != "make install" {
		t.Errorf("InstallCommand = %q, want make install", rc.Dotfiles.InstallCommand)
	}
}

func TestLoadCribRC_DotfilesDisableWithRepository(t *testing.T) {
	rc := setupCribRC(t, "dotfiles = false\ndotfiles.repository = git@github.com:user/dots\n")
	if !rc.Dotfiles.Disabled {
		t.Error("expected Dotfiles.Disabled = true")
	}
	if rc.Dotfiles.Repository != "git@github.com:user/dots" {
		t.Errorf("Repository = %q, want git@github.com:user/dots", rc.Dotfiles.Repository)
	}
}
