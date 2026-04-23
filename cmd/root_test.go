package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionString_Dev(t *testing.T) {
	origV, origC, origD := version, commit, date
	defer func() { version, commit, date = origV, origC, origD }()

	version = "0.2.0-dev"
	commit = "abc1234"
	date = "2026-02-27T10:00:00Z"

	got := versionString()
	want := "crib 0.2.0-dev (abc1234, 2026-02-27T10:00:00Z)"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionString_DevUnknownDate(t *testing.T) {
	origV, origC, origD := version, commit, date
	defer func() { version, commit, date = origV, origC, origD }()

	version = "0.2.0-dev"
	commit = "abc1234"
	date = "unknown"

	got := versionString()
	want := "crib 0.2.0-dev (abc1234)"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionString_Release(t *testing.T) {
	origV, origC, origD := version, commit, date
	defer func() { version, commit, date = origV, origC, origD }()

	version = "0.2.0"
	commit = "abc1234"
	date = "2026-02-27T10:00:00Z"

	got := versionString()
	want := "crib 0.2.0"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionString_UnknownCommit(t *testing.T) {
	origV, origC, origD := version, commit, date
	defer func() { version, commit, date = origV, origC, origD }()

	version = "0.3.0-dev"
	commit = "unknown"
	date = "unknown"

	got := versionString()
	want := "crib 0.3.0-dev"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

func TestLoadProjectCribRC_UsesCwdByDefault(t *testing.T) {
	origDir := dirFlag
	t.Cleanup(func() { dirFlag = origDir })
	dirFlag = ""

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".cribrc"), []byte(`config = "from-cwd"`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectDir)

	rc, err := loadProjectCribRC()
	if err != nil {
		t.Fatalf("loadProjectCribRC: %v", err)
	}
	if rc.Config != "from-cwd" {
		t.Errorf("Config = %q, want from-cwd", rc.Config)
	}
}

func TestLoadProjectCribRC_RespectsDirFlag(t *testing.T) {
	origDir := dirFlag
	t.Cleanup(func() { dirFlag = origDir })

	cwdDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwdDir, ".cribrc"), []byte(`config = "from-cwd"`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwdDir)

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".cribrc"), []byte(`config = "from-dirflag"`), 0o644); err != nil {
		t.Fatal(err)
	}

	dirFlag = projectDir
	rc, err := loadProjectCribRC()
	if err != nil {
		t.Fatalf("loadProjectCribRC: %v", err)
	}
	if rc.Config != "from-dirflag" {
		t.Errorf("Config = %q, want from-dirflag (--dir should win over cwd)", rc.Config)
	}
}

func TestLoadProjectCribRC_DirFlagMissingFile(t *testing.T) {
	origDir := dirFlag
	t.Cleanup(func() { dirFlag = origDir })

	dirFlag = t.TempDir() // directory exists but has no .cribrc
	rc, err := loadProjectCribRC()
	if err != nil {
		t.Fatalf("expected no error for missing .cribrc, got: %v", err)
	}
	if rc == nil {
		t.Fatal("expected zero CribRC, got nil")
	}
	if rc.Config != "" {
		t.Errorf("expected empty CribRC, got %+v", rc)
	}
}

func TestResetPerExecutionFlags(t *testing.T) {
	root := &cobra.Command{Use: "test"}
	sub := &cobra.Command{Use: "up", RunE: func(*cobra.Command, []string) error { return nil }}
	addPluginFlags(sub)
	root.AddCommand(sub)

	// First "run": --disable-plugin ssh provided.
	root.SetArgs([]string{"up", "--disable-plugin", "ssh"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first execute: %v", err)
	}
	got := disabledPluginsForCommand(sub)
	if len(got) != 1 || got[0] != "ssh" {
		t.Fatalf("first run: got %v, want [ssh]", got)
	}

	// Reset before the next parse, then "run" without the flag.
	resetPerExecutionFlags(root)
	root.SetArgs([]string{"up"})
	if err := root.Execute(); err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if got := disabledPluginsForCommand(sub); len(got) != 0 {
		t.Errorf("second run: got %v, want empty (stale value leaked)", got)
	}
	if f := sub.Flags().Lookup("disable-plugin"); f.Changed {
		t.Errorf("second run: flag Changed=true after reset")
	}

	// Third "run": flag provided again, should not accumulate with the first run.
	resetPerExecutionFlags(root)
	root.SetArgs([]string{"up", "--disable-plugin", "dotfiles"})
	if err := root.Execute(); err != nil {
		t.Fatalf("third execute: %v", err)
	}
	got = disabledPluginsForCommand(sub)
	if len(got) != 1 || got[0] != "dotfiles" {
		t.Errorf("third run: got %v, want [dotfiles] only", got)
	}
}
