package engine

import (
	"strings"
	"testing"
)

func TestEnvBuilder_EmptyBuild(t *testing.T) {
	envb := NewEnvBuilder(nil)
	if got := envb.Build(); got != nil {
		t.Errorf("Build() = %v, want nil", got)
	}
}

func TestEnvBuilder_ConfigEnvOnly(t *testing.T) {
	envb := NewEnvBuilder(map[string]string{"EDITOR": "vim", "LANG": "en_US.UTF-8"})
	got := envb.Build()
	if got["EDITOR"] != "vim" {
		t.Errorf("EDITOR = %q, want vim", got["EDITOR"])
	}
	if got["LANG"] != "en_US.UTF-8" {
		t.Errorf("LANG = %q, want en_US.UTF-8", got["LANG"])
	}
}

func TestEnvBuilder_ProbedOnly(t *testing.T) {
	envb := NewEnvBuilder(nil)
	envb.SetProbed(map[string]string{
		"PATH": "/usr/bin:/bin",
		"HOME": "/home/vscode",
	})
	got := envb.Build()
	if got["PATH"] != "/usr/bin:/bin" {
		t.Errorf("PATH = %q, want /usr/bin:/bin", got["PATH"])
	}
	if got["HOME"] != "/home/vscode" {
		t.Errorf("HOME = %q, want /home/vscode", got["HOME"])
	}
}

func TestEnvBuilder_ProbedPlusConfigEnv(t *testing.T) {
	envb := NewEnvBuilder(map[string]string{"EDITOR": "nano"})
	envb.SetProbed(map[string]string{
		"PATH":   "/usr/bin:/bin",
		"EDITOR": "vim",
	})
	got := envb.Build()
	if got["EDITOR"] != "nano" {
		t.Errorf("EDITOR = %q, want nano (configEnv should override probed)", got["EDITOR"])
	}
	if got["PATH"] != "/usr/bin:/bin" {
		t.Errorf("PATH = %q, want /usr/bin:/bin", got["PATH"])
	}
}

func TestEnvBuilder_PluginEnvOverridesProbed(t *testing.T) {
	envb := NewEnvBuilder(nil)
	envb.SetProbed(map[string]string{
		"BUNDLE_PATH": "/old/path",
		"HOME":        "/home/vscode",
	})
	envb.AddPluginEnv(map[string]string{
		"BUNDLE_PATH": "/home/vscode/.bundle",
	})
	got := envb.Build()
	if got["BUNDLE_PATH"] != "/home/vscode/.bundle" {
		t.Errorf("BUNDLE_PATH = %q, want /home/vscode/.bundle (plugin should override probed)", got["BUNDLE_PATH"])
	}
	if got["HOME"] != "/home/vscode" {
		t.Errorf("HOME = %q, want /home/vscode", got["HOME"])
	}
}

func TestEnvBuilder_ConfigEnvOverridesPluginEnv(t *testing.T) {
	envb := NewEnvBuilder(map[string]string{"EDITOR": "nano"})
	envb.AddPluginEnv(map[string]string{
		"EDITOR":      "code",
		"BUNDLE_PATH": "/home/vscode/.bundle",
	})
	got := envb.Build()
	if got["EDITOR"] != "nano" {
		t.Errorf("EDITOR = %q, want nano (configEnv should override plugin)", got["EDITOR"])
	}
	if got["BUNDLE_PATH"] != "/home/vscode/.bundle" {
		t.Errorf("BUNDLE_PATH = %q, want /home/vscode/.bundle", got["BUNDLE_PATH"])
	}
}

func TestEnvBuilder_ContainerPATHPreserved(t *testing.T) {
	envb := NewEnvBuilder(nil)
	envb.SetProbed(map[string]string{
		"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	})
	envb.SetContainerPATH("/usr/local/bundle/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")

	got := envb.Build()
	path := got["PATH"]
	if !strings.Contains(path, "/usr/local/bundle/bin") {
		t.Errorf("PATH missing container-only dir /usr/local/bundle/bin: %q", path)
	}
	// Should be appended, not prepended.
	if strings.HasPrefix(path, "/usr/local/bundle/bin") {
		t.Errorf("container PATH dir should be appended, not prepended: %q", path)
	}
}

func TestEnvBuilder_PluginPathPrepend(t *testing.T) {
	envb := NewEnvBuilder(nil)
	envb.SetProbed(map[string]string{
		"PATH": "/usr/bin:/bin",
	})
	envb.AddPluginPathPrepend([]string{"/home/vscode/.bundle/bin"})

	got := envb.Build()
	path := got["PATH"]
	if !strings.HasPrefix(path, "/home/vscode/.bundle/bin:") {
		t.Errorf("PATH should start with plugin prepend dir: %q", path)
	}
	if !strings.Contains(path, "/usr/bin") {
		t.Errorf("PATH should still contain probed dirs: %q", path)
	}
}

func TestEnvBuilder_FullPathPrecedence(t *testing.T) {
	envb := NewEnvBuilder(nil)
	envb.SetProbed(map[string]string{
		"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	})
	envb.SetContainerPATH("/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	envb.AddPluginPathPrepend([]string{"/home/vscode/.bundle/bin"})

	got := envb.Build()
	path := got["PATH"]

	// Plugin PathPrepend should be first.
	if !strings.HasPrefix(path, "/home/vscode/.bundle/bin:") {
		t.Errorf("PATH should start with plugin prepend: %q", path)
	}
	// Container PATH entries should be preserved (appended).
	if !strings.Contains(path, "/usr/local/go/bin") {
		t.Errorf("PATH missing container go bin: %q", path)
	}
	// Probed dirs should be present.
	if !strings.Contains(path, "/usr/bin") {
		t.Errorf("PATH missing probed /usr/bin: %q", path)
	}
}

func TestEnvBuilder_RestoreFrom(t *testing.T) {
	stored := map[string]string{
		"PATH":      "/home/vscode/.bundle/bin:/home/vscode/.local/share/mise/installs/ruby/3.4.7/bin:/usr/local/bin:/usr/bin",
		"RUBY_ROOT": "/home/vscode/.local/share/mise/installs/ruby/3.4.7",
		"GEM_HOME":  "/home/vscode/.gems",
	}

	envb := NewEnvBuilder(map[string]string{"EDITOR": "nano"})
	envb.RestoreFrom(stored)

	got := envb.Build()

	// Stored PATH should be the base.
	if !strings.Contains(got["PATH"], "/home/vscode/.local/share/mise/installs/ruby/3.4.7/bin") {
		t.Errorf("PATH missing stored mise ruby: %q", got["PATH"])
	}
	// Config should override.
	if got["EDITOR"] != "nano" {
		t.Errorf("EDITOR = %q, want nano", got["EDITOR"])
	}
	// Stored non-PATH vars should be present.
	if got["RUBY_ROOT"] != "/home/vscode/.local/share/mise/installs/ruby/3.4.7" {
		t.Errorf("RUBY_ROOT = %q, want stored value", got["RUBY_ROOT"])
	}
}

func TestEnvBuilder_RestoreFrom_PluginEnvOverridesStored(t *testing.T) {
	stored := map[string]string{
		"PATH":        "/usr/bin",
		"BUNDLE_PATH": "/old/bundle/path",
	}

	envb := NewEnvBuilder(nil)
	envb.AddPluginEnv(map[string]string{
		"BUNDLE_PATH": "/new/bundle/path",
	})
	envb.RestoreFrom(stored)

	got := envb.Build()
	if got["BUNDLE_PATH"] != "/new/bundle/path" {
		t.Errorf("BUNDLE_PATH = %q, want /new/bundle/path (fresh plugin should override stale stored)", got["BUNDLE_PATH"])
	}
}

func TestEnvBuilder_SkipsSessionVars(t *testing.T) {
	envb := NewEnvBuilder(nil)
	envb.SetProbed(map[string]string{
		"PATH":     "/usr/bin",
		"HOSTNAME": "abc123",
		"SHLVL":    "1",
		"PWD":      "/",
		"OLDPWD":   "/home",
		"_":        "/usr/bin/env",
	})

	got := envb.Build()
	for _, skip := range []string{"HOSTNAME", "SHLVL", "PWD", "OLDPWD", "_"} {
		if _, ok := got[skip]; ok {
			t.Errorf("%s should be excluded from built env", skip)
		}
	}
	if got["PATH"] != "/usr/bin" {
		t.Errorf("PATH should be included, got %q", got["PATH"])
	}
}

func TestEnvBuilder_SkipsMiseVars(t *testing.T) {
	envb := NewEnvBuilder(nil)
	envb.SetProbed(map[string]string{
		"PATH":                  "/usr/bin",
		"HOME":                  "/home/user",
		"MISE_SHELL":            "zsh",
		"__MISE_DIFF":           "eAFrXpyfk9Kw...",
		"__MISE_ORIG_PATH":      "/usr/bin",
		"__MISE_SESSION":        "eAHrWJOTn5iS...",
		"__MISE_ZSH_PRECMD_RUN": "0",
	})

	got := envb.Build()
	for _, skip := range []string{"MISE_SHELL", "__MISE_DIFF", "__MISE_ORIG_PATH", "__MISE_SESSION", "__MISE_ZSH_PRECMD_RUN"} {
		if _, ok := got[skip]; ok {
			t.Errorf("%s should be excluded from built env", skip)
		}
	}
	if got["HOME"] != "/home/user" {
		t.Errorf("HOME should be included")
	}
}

func TestEnvBuilder_SetProbedReplacesPrevious(t *testing.T) {
	envb := NewEnvBuilder(nil)
	envb.SetProbed(map[string]string{
		"PATH":    "/usr/bin",
		"OLD_VAR": "old",
	})
	// Post-hook re-probe replaces the pre-hook probe.
	envb.SetProbed(map[string]string{
		"PATH":    "/usr/bin:/home/vscode/.local/bin",
		"NEW_VAR": "new",
	})

	got := envb.Build()
	if got["PATH"] != "/usr/bin:/home/vscode/.local/bin" {
		t.Errorf("PATH = %q, want post-hook value", got["PATH"])
	}
	if _, ok := got["OLD_VAR"]; ok {
		t.Error("OLD_VAR should not be present after SetProbed replaced pre-hook probe")
	}
	if got["NEW_VAR"] != "new" {
		t.Errorf("NEW_VAR = %q, want new", got["NEW_VAR"])
	}
}

func TestEnvBuilder_SetConfigEnvUpdates(t *testing.T) {
	envb := NewEnvBuilder(map[string]string{
		"PATH": "${containerEnv:PATH}:/extra",
	})
	// After resolveRemoteEnv resolves the reference:
	envb.SetConfigEnv(map[string]string{
		"PATH": "/usr/local/bin:/usr/bin:/extra",
	})
	envb.SetProbed(map[string]string{
		"PATH": "/usr/bin:/bin",
		"HOME": "/home/vscode",
	})

	got := envb.Build()
	// Config PATH should win.
	if got["PATH"] != "/usr/local/bin:/usr/bin:/extra" {
		t.Errorf("PATH = %q, want resolved config PATH", got["PATH"])
	}
}

func TestEnvBuilder_DuplicatePathPrepend(t *testing.T) {
	envb := NewEnvBuilder(nil)
	envb.SetProbed(map[string]string{
		"PATH": "/home/vscode/.bundle/bin:/usr/bin",
	})
	// Plugin requests a dir that's already in PATH.
	envb.AddPluginPathPrepend([]string{"/home/vscode/.bundle/bin"})

	got := envb.Build()
	path := got["PATH"]
	if strings.Count(path, "/home/vscode/.bundle/bin") != 1 {
		t.Errorf("PATH has duplicate .bundle/bin: %q", path)
	}
}
