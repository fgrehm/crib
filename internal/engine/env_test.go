package engine

import (
	"sort"
	"testing"
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

func TestMergeEnv_SkipsNoisyHostVars(t *testing.T) {
	probed := map[string]string{
		"PATH":                     "/usr/bin",
		"HOME":                     "/home/user",
		"LS_COLORS":                "rs=0:di=01;34:...",
		"LSCOLORS":                 "Gxfxcxdxbxegedabagacad",
		"LESSCLOSE":                "/usr/bin/lesspipe %s %s",
		"LESSOPEN":                 "| /usr/bin/lesspipe %s",
		"TERM_PROGRAM":             "tmux",
		"TERM_PROGRAM_VERSION":     "3.4",
		"COLORTERM":                "truecolor",
		"VTE_VERSION":              "7200",
		"WINDOWID":                 "12345",
		"DISPLAY":                  ":0",
		"WAYLAND_DISPLAY":          "wayland-0",
		"DESKTOP_SESSION":          "gnome",
		"SESSION_MANAGER":          "local/hostname:@/tmp/.ICE-unix/1234",
		"XDG_SESSION_TYPE":         "wayland",
		"XDG_SESSION_CLASS":        "user",
		"XDG_SESSION_ID":           "2",
		"XDG_CURRENT_DESKTOP":      "GNOME",
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/1000/bus",
		"GPG_AGENT_INFO":           "/run/user/1000/gnupg/S.gpg-agent:0:1",
	}
	result := mergeEnv(probed, nil)

	noisyVars := []string{
		"LS_COLORS", "LSCOLORS", "LESSCLOSE", "LESSOPEN",
		"TERM_PROGRAM", "TERM_PROGRAM_VERSION", "COLORTERM", "VTE_VERSION",
		"WINDOWID", "DISPLAY", "WAYLAND_DISPLAY",
		"DESKTOP_SESSION", "SESSION_MANAGER",
		"XDG_SESSION_TYPE", "XDG_SESSION_CLASS", "XDG_SESSION_ID",
		"XDG_CURRENT_DESKTOP", "DBUS_SESSION_BUS_ADDRESS", "GPG_AGENT_INFO",
	}
	for _, v := range noisyVars {
		if _, ok := result[v]; ok {
			t.Errorf("%s should be filtered from probed env", v)
		}
	}

	// Regular vars should survive.
	if result["PATH"] != "/usr/bin" {
		t.Errorf("PATH should be included, got %q", result["PATH"])
	}
	if result["HOME"] != "/home/user" {
		t.Errorf("HOME should be included, got %q", result["HOME"])
	}
}

func TestMergeEnv_RemoteEnvCanOverrideNoisyFilter(t *testing.T) {
	probed := map[string]string{
		"PATH":    "/usr/bin",
		"DISPLAY": ":0",
	}
	remote := map[string]string{
		"DISPLAY": ":1",
	}
	result := mergeEnv(probed, remote)

	// Explicit remoteEnv should override the filter.
	if result["DISPLAY"] != ":1" {
		t.Errorf("DISPLAY = %q, want :1 (explicit remoteEnv should override filter)", result["DISPLAY"])
	}
}
