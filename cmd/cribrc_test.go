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
