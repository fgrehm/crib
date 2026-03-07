package engine

import (
	"sort"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
)

func TestCopyStringMap(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := copyStringMap(nil); got != nil {
			t.Errorf("copyStringMap(nil) = %v, want nil", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := copyStringMap(map[string]string{})
		if got == nil || len(got) != 0 {
			t.Errorf("copyStringMap({}) = %v, want empty map", got)
		}
	})

	t.Run("copies values", func(t *testing.T) {
		orig := map[string]string{"a": "1", "b": "2"}
		cp := copyStringMap(orig)
		if cp["a"] != "1" || cp["b"] != "2" {
			t.Errorf("copyStringMap values wrong: %v", cp)
		}
		// Mutating the copy should not affect the original.
		cp["a"] = "changed"
		if orig["a"] != "1" {
			t.Error("mutating copy changed the original")
		}
	})
}

func TestEnvSlice(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := envSlice(nil); got != nil {
			t.Errorf("envSlice(nil) = %v, want nil", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if got := envSlice(map[string]string{}); got != nil {
			t.Errorf("envSlice({}) = %v, want nil", got)
		}
	})

	t.Run("converts to KEY=VALUE", func(t *testing.T) {
		got := envSlice(map[string]string{"FOO": "bar", "BAZ": "qux"})
		sort.Strings(got)
		want := []string{"BAZ=qux", "FOO=bar"}
		if len(got) != len(want) {
			t.Fatalf("envSlice len = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("envSlice[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("empty value", func(t *testing.T) {
		got := envSlice(map[string]string{"KEY": ""})
		if len(got) != 1 || got[0] != "KEY=" {
			t.Errorf("envSlice empty value = %v, want [KEY=]", got)
		}
	})
}

func TestMergeStoredRemoteEnv(t *testing.T) {
	t.Run("nil stored env is a no-op", func(t *testing.T) {
		cfg := &config.DevContainerConfig{}
		mergeStoredRemoteEnv(cfg, nil)
		if cfg.RemoteEnv != nil {
			t.Errorf("RemoteEnv = %v, want nil", cfg.RemoteEnv)
		}
	})

	t.Run("empty stored env is a no-op", func(t *testing.T) {
		cfg := &config.DevContainerConfig{}
		mergeStoredRemoteEnv(cfg, map[string]string{})
		if cfg.RemoteEnv != nil {
			t.Errorf("RemoteEnv = %v, want nil", cfg.RemoteEnv)
		}
	})

	t.Run("restores non-PATH vars as fallbacks", func(t *testing.T) {
		cfg := &config.DevContainerConfig{
			DevContainerConfigBase: config.DevContainerConfigBase{
				RemoteEnv: map[string]string{"EDITOR": "vim"},
			},
		}
		stored := map[string]string{
			"EDITOR":    "emacs",
			"RUBY_ROOT": "/usr/local/ruby",
		}
		mergeStoredRemoteEnv(cfg, stored)

		// EDITOR from cfg takes precedence.
		if cfg.RemoteEnv["EDITOR"] != "vim" {
			t.Errorf("EDITOR = %q, want %q", cfg.RemoteEnv["EDITOR"], "vim")
		}
		// RUBY_ROOT restored from stored.
		if cfg.RemoteEnv["RUBY_ROOT"] != "/usr/local/ruby" {
			t.Errorf("RUBY_ROOT = %q, want %q", cfg.RemoteEnv["RUBY_ROOT"], "/usr/local/ruby")
		}
	})

	t.Run("restores stored PATH when cfg has no PATH", func(t *testing.T) {
		cfg := &config.DevContainerConfig{}
		stored := map[string]string{
			"PATH": "/home/user/.mise/installs/ruby/bin:/usr/local/bin:/usr/bin",
		}
		mergeStoredRemoteEnv(cfg, stored)

		if cfg.RemoteEnv["PATH"] != stored["PATH"] {
			t.Errorf("PATH = %q, want %q", cfg.RemoteEnv["PATH"], stored["PATH"])
		}
	})

	t.Run("merges fresh PATH dirs onto stored PATH", func(t *testing.T) {
		cfg := &config.DevContainerConfig{
			DevContainerConfigBase: config.DevContainerConfigBase{
				RemoteEnv: map[string]string{
					"PATH": "/home/user/.bundle/bin",
				},
			},
		}
		stored := map[string]string{
			"PATH": "/home/user/.bundle/bin:/home/user/.mise/installs/ruby/bin:/usr/local/bin:/usr/bin",
		}
		mergeStoredRemoteEnv(cfg, stored)

		path := cfg.RemoteEnv["PATH"]
		// The stored PATH already has .bundle/bin, so no duplication.
		if strings.Count(path, "/home/user/.bundle/bin") != 1 {
			t.Errorf("PATH has duplicate .bundle/bin: %q", path)
		}
		// The mise ruby path must be present.
		if !strings.Contains(path, "/home/user/.mise/installs/ruby/bin") {
			t.Errorf("PATH missing mise ruby: %q", path)
		}
	})

	t.Run("prepends new plugin dirs to stored PATH", func(t *testing.T) {
		cfg := &config.DevContainerConfig{
			DevContainerConfigBase: config.DevContainerConfigBase{
				RemoteEnv: map[string]string{
					"PATH": "/home/user/.new-plugin/bin",
				},
			},
		}
		stored := map[string]string{
			"PATH": "/home/user/.mise/installs/ruby/bin:/usr/local/bin",
		}
		mergeStoredRemoteEnv(cfg, stored)

		path := cfg.RemoteEnv["PATH"]
		// New plugin dir should be prepended.
		if !strings.HasPrefix(path, "/home/user/.new-plugin/bin:") {
			t.Errorf("PATH should start with new plugin dir: %q", path)
		}
		// Stored paths should follow.
		if !strings.Contains(path, "/home/user/.mise/installs/ruby/bin") {
			t.Errorf("PATH missing mise ruby: %q", path)
		}
	})

	t.Run("initializes nil cfg.RemoteEnv", func(t *testing.T) {
		cfg := &config.DevContainerConfig{}
		stored := map[string]string{"FOO": "bar"}
		mergeStoredRemoteEnv(cfg, stored)

		if cfg.RemoteEnv == nil {
			t.Fatal("RemoteEnv should be initialized")
		}
		if cfg.RemoteEnv["FOO"] != "bar" {
			t.Errorf("FOO = %q, want %q", cfg.RemoteEnv["FOO"], "bar")
		}
	})
}
