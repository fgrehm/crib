package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCribRC_CacheKey(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".cribrc")
	if err := os.WriteFile(rcPath, []byte("cache = npm, pip, go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)

	rc, err := loadCribRC()
	if err != nil {
		t.Fatalf("loadCribRC: %v", err)
	}
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
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".cribrc")
	content := "config = .devcontainer-custom\ncache = npm\n"
	if err := os.WriteFile(rcPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)

	rc, err := loadCribRC()
	if err != nil {
		t.Fatalf("loadCribRC: %v", err)
	}
	if rc.Config != ".devcontainer-custom" {
		t.Errorf("Config = %q, want .devcontainer-custom", rc.Config)
	}
	if len(rc.Cache) != 1 || rc.Cache[0] != "npm" {
		t.Errorf("Cache = %v, want [npm]", rc.Cache)
	}
}

func TestLoadCribRC_DotfilesDisable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".cribrc"), []byte("dotfiles = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	rc, err := loadCribRC()
	if err != nil {
		t.Fatalf("loadCribRC: %v", err)
	}
	if !rc.Dotfiles.Disabled {
		t.Error("expected Dotfiles.Disabled = true")
	}
}

func TestLoadCribRC_DotfilesUnknownValue_Ignored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".cribrc"), []byte("dotfiles = maybe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	rc, err := loadCribRC()
	if err != nil {
		t.Fatalf("loadCribRC: %v", err)
	}
	if rc.Dotfiles.Disabled {
		t.Error("expected Dotfiles.Disabled = false for unknown value")
	}
}

func TestLoadCribRC_DotfilesRepository(t *testing.T) {
	dir := t.TempDir()
	content := "dotfiles.repository = git@github.com:user/dots\n"
	if err := os.WriteFile(filepath.Join(dir, ".cribrc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	rc, err := loadCribRC()
	if err != nil {
		t.Fatalf("loadCribRC: %v", err)
	}
	if rc.Dotfiles.Repository != "git@github.com:user/dots" {
		t.Errorf("Repository = %q, want git@github.com:user/dots", rc.Dotfiles.Repository)
	}
}

func TestLoadCribRC_DotfilesTargetPath(t *testing.T) {
	dir := t.TempDir()
	content := "dotfiles.targetPath = ~/my-dots\n"
	if err := os.WriteFile(filepath.Join(dir, ".cribrc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	rc, err := loadCribRC()
	if err != nil {
		t.Fatalf("loadCribRC: %v", err)
	}
	if rc.Dotfiles.TargetPath != "~/my-dots" {
		t.Errorf("TargetPath = %q, want ~/my-dots", rc.Dotfiles.TargetPath)
	}
}

func TestLoadCribRC_DotfilesInstallCommand(t *testing.T) {
	dir := t.TempDir()
	content := "dotfiles.installCommand = make install\n"
	if err := os.WriteFile(filepath.Join(dir, ".cribrc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	rc, err := loadCribRC()
	if err != nil {
		t.Fatalf("loadCribRC: %v", err)
	}
	if rc.Dotfiles.InstallCommand != "make install" {
		t.Errorf("InstallCommand = %q, want make install", rc.Dotfiles.InstallCommand)
	}
}

func TestLoadCribRC_DotfilesDisableWithRepository(t *testing.T) {
	dir := t.TempDir()
	content := "dotfiles = false\ndotfiles.repository = git@github.com:user/dots\n"
	if err := os.WriteFile(filepath.Join(dir, ".cribrc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	rc, err := loadCribRC()
	if err != nil {
		t.Fatalf("loadCribRC: %v", err)
	}
	if !rc.Dotfiles.Disabled {
		t.Error("expected Dotfiles.Disabled = true")
	}
	if rc.Dotfiles.Repository != "git@github.com:user/dots" {
		t.Errorf("Repository = %q, want git@github.com:user/dots", rc.Dotfiles.Repository)
	}
}
