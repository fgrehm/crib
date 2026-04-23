package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fgrehm/crib/internal/globalconfig"
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

func TestLoadProjectCribRC_WorkspaceFlowsToRuntimeCfg(t *testing.T) {
	origDir := dirFlag
	t.Cleanup(func() { dirFlag = origDir })

	projectDir := t.TempDir()
	content := `
[workspace]
env = { FROM_CRIBRC = "yes" }
mount = ["type=bind,source=/host/p,target=/container/p"]
run_args = ["--cap-add", "SYS_PTRACE"]
`
	if err := os.WriteFile(filepath.Join(projectDir, ".cribrc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	dirFlag = projectDir

	rc, err := loadProjectCribRC()
	if err != nil {
		t.Fatalf("loadProjectCribRC: %v", err)
	}

	// Simulate the PersistentPreRunE copy into runtimeCfg that setupPlugins
	// consumes. Without this copy (the bug the review caught), .cribrc's
	// [workspace] section would be silently dropped.
	var cfg runtimeConfig
	cfg.ProjectWorkspace = rc.Workspace

	if cfg.ProjectWorkspace.Env["FROM_CRIBRC"] != "yes" {
		t.Errorf("ProjectWorkspace.Env[FROM_CRIBRC] = %q, want yes", cfg.ProjectWorkspace.Env["FROM_CRIBRC"])
	}
	if len(cfg.ProjectWorkspace.Mounts) != 1 || cfg.ProjectWorkspace.Mounts[0] != "type=bind,source=/host/p,target=/container/p" {
		t.Errorf("ProjectWorkspace.Mounts = %v", cfg.ProjectWorkspace.Mounts)
	}
	if len(cfg.ProjectWorkspace.RunArgs) != 2 {
		t.Errorf("ProjectWorkspace.RunArgs = %v", cfg.ProjectWorkspace.RunArgs)
	}

	// And the merge with the global workspace must preserve these values,
	// producing the options setupPlugins hands to the engine.
	merged := mergeWorkspaceOptions(globalconfig.WorkspaceConfig{}, cfg.ProjectWorkspace)
	if merged.Env["FROM_CRIBRC"] != "yes" {
		t.Errorf("merged.Env[FROM_CRIBRC] = %q", merged.Env["FROM_CRIBRC"])
	}
	if len(merged.Mounts) != 1 {
		t.Errorf("merged.Mounts = %v", merged.Mounts)
	}
	if len(merged.RunArgs) != 2 {
		t.Errorf("merged.RunArgs = %v", merged.RunArgs)
	}
}

func TestMergeWorkspaceOptions_EmptyInputs(t *testing.T) {
	out := mergeWorkspaceOptions(globalconfig.WorkspaceConfig{}, globalconfig.WorkspaceConfig{})
	if out.Env != nil || out.Mounts != nil || out.RunArgs != nil {
		t.Errorf("expected zero-value output, got %+v", out)
	}
}

func TestMergeWorkspaceOptions_EnvProjectWins(t *testing.T) {
	out := mergeWorkspaceOptions(
		globalconfig.WorkspaceConfig{Env: map[string]string{
			"GLOBAL_ONLY": "g",
			"CONFLICT":    "global-loser",
		}},
		globalconfig.WorkspaceConfig{Env: map[string]string{
			"PROJECT_ONLY": "p",
			"CONFLICT":     "project-wins",
		}},
	)
	if out.Env["GLOBAL_ONLY"] != "g" {
		t.Errorf("GLOBAL_ONLY = %q", out.Env["GLOBAL_ONLY"])
	}
	if out.Env["PROJECT_ONLY"] != "p" {
		t.Errorf("PROJECT_ONLY = %q", out.Env["PROJECT_ONLY"])
	}
	if out.Env["CONFLICT"] != "project-wins" {
		t.Errorf("CONFLICT = %q, want project-wins", out.Env["CONFLICT"])
	}
}

func TestMergeWorkspaceOptions_MountsConcatGlobalFirst(t *testing.T) {
	out := mergeWorkspaceOptions(
		globalconfig.WorkspaceConfig{Mounts: []string{"type=bind,source=/g,target=/g"}},
		globalconfig.WorkspaceConfig{Mounts: []string{"type=bind,source=/p,target=/p"}},
	)
	if len(out.Mounts) != 2 {
		t.Fatalf("Mounts = %v, want 2 entries", out.Mounts)
	}
	if out.Mounts[0] != "type=bind,source=/g,target=/g" {
		t.Errorf("Mounts[0] = %q (global should come first)", out.Mounts[0])
	}
	if out.Mounts[1] != "type=bind,source=/p,target=/p" {
		t.Errorf("Mounts[1] = %q (project should come second)", out.Mounts[1])
	}
}

func TestMergeWorkspaceOptions_RunArgsProjectAfterGlobal(t *testing.T) {
	out := mergeWorkspaceOptions(
		globalconfig.WorkspaceConfig{RunArgs: []string{"--cpus", "2"}},
		globalconfig.WorkspaceConfig{RunArgs: []string{"--cpus", "4"}},
	)
	// Global first, project second — last-flag-wins in the runtime resolves
	// --cpus to the project value.
	want := []string{"--cpus", "2", "--cpus", "4"}
	if len(out.RunArgs) != len(want) {
		t.Fatalf("RunArgs = %v, want %v", out.RunArgs, want)
	}
	for i, w := range want {
		if out.RunArgs[i] != w {
			t.Errorf("RunArgs[%d] = %q, want %q", i, out.RunArgs[i], w)
		}
	}
}

func TestMergeWorkspaceOptions_OnlyGlobal(t *testing.T) {
	out := mergeWorkspaceOptions(
		globalconfig.WorkspaceConfig{
			Env:     map[string]string{"G": "1"},
			Mounts:  []string{"type=bind,source=/a,target=/a"},
			RunArgs: []string{"--privileged"},
		},
		globalconfig.WorkspaceConfig{},
	)
	if out.Env["G"] != "1" || len(out.Mounts) != 1 || len(out.RunArgs) != 1 {
		t.Errorf("unexpected merged output: %+v", out)
	}
}

func TestMergeWorkspaceOptions_OnlyProject(t *testing.T) {
	out := mergeWorkspaceOptions(
		globalconfig.WorkspaceConfig{},
		globalconfig.WorkspaceConfig{
			Env:     map[string]string{"P": "1"},
			Mounts:  []string{"type=bind,source=/a,target=/a"},
			RunArgs: []string{"--privileged"},
		},
	)
	if out.Env["P"] != "1" || len(out.Mounts) != 1 || len(out.RunArgs) != 1 {
		t.Errorf("unexpected merged output: %+v", out)
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
