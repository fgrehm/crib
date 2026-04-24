package pluginsetup

import (
	"bytes"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/globalconfig"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func captureLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestCollectDisabled(t *testing.T) {
	tests := []struct {
		name   string
		layers [][]string
		want   map[string]bool
	}{
		{
			name:   "no layers",
			layers: nil,
			want:   map[string]bool{},
		},
		{
			name:   "single layer",
			layers: [][]string{{"ssh"}},
			want:   map[string]bool{"ssh": true},
		},
		{
			name: "merges across global, cribrc, and flag",
			layers: [][]string{
				{"ssh"},
				{"dotfiles"},
				{"package-cache"},
			},
			want: map[string]bool{"ssh": true, "dotfiles": true, "package-cache": true},
		},
		{
			name:   "dedupes across layers",
			layers: [][]string{{"ssh", "dotfiles"}, {"ssh"}},
			want:   map[string]bool{"ssh": true, "dotfiles": true},
		},
		{
			name:   "trims whitespace and filters empty entries",
			layers: [][]string{{"  ssh  ", "", "  "}, {"dotfiles"}},
			want:   map[string]bool{"ssh": true, "dotfiles": true},
		},
		{
			name:   "nil layer entries are tolerated",
			layers: [][]string{nil, {"ssh"}, nil},
			want:   map[string]bool{"ssh": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectDisabled(tt.layers...)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestIsKnown(t *testing.T) {
	for _, name := range knownPlugins {
		if !isKnown(name) {
			t.Errorf("isKnown(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "sssh", "unknown", "SSH"} {
		if isKnown(name) {
			t.Errorf("isKnown(%q) = true, want false", name)
		}
	}
}

func TestWarnUnknown(t *testing.T) {
	tests := []struct {
		name       string
		disabled   map[string]bool
		wantNames  []string
		wantSilent []string
	}{
		{
			name:       "all known names",
			disabled:   map[string]bool{"ssh": true, "dotfiles": true},
			wantSilent: []string{"ssh", "dotfiles"},
		},
		{
			name:      "unknown name warns",
			disabled:  map[string]bool{"sssh": true},
			wantNames: []string{"sssh"},
		},
		{
			name:       "mixed: only unknowns warn",
			disabled:   map[string]bool{"ssh": true, "typo": true, "dotfiles": true},
			wantNames:  []string{"typo"},
			wantSilent: []string{"ssh", "dotfiles"},
		},
		{
			name:     "empty set never warns",
			disabled: map[string]bool{},
		},
		{
			name:      "multiple unknowns log in sorted order",
			disabled:  map[string]bool{"zebra": true, "alpha": true, "mike": true},
			wantNames: []string{"alpha", "mike", "zebra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			warnUnknown(tt.disabled, captureLogger(&buf))

			out := buf.String()
			prevIdx := -1
			for _, n := range tt.wantNames {
				if !strings.Contains(out, "unknown plugin in disable list") {
					t.Errorf("expected warning message, got %q", out)
				}
				idx := strings.Index(out, "name="+n)
				if idx == -1 {
					t.Errorf("expected warning to include name=%s, got %q", n, out)
					continue
				}
				if idx < prevIdx {
					t.Errorf("expected sorted order; %q appeared out of order in %q", n, out)
				}
				prevIdx = idx
			}
			for _, n := range tt.wantSilent {
				if strings.Contains(out, "name="+n+" ") || strings.HasSuffix(strings.TrimRight(out, "\n"), "name="+n) {
					t.Errorf("did not expect warning for known name %q, got %q", n, out)
				}
			}
			if len(tt.wantNames) == 0 && strings.Contains(out, "unknown plugin in disable list") {
				t.Errorf("did not expect any warning, got %q", out)
			}
		})
	}
}

func TestResolveDotfiles(t *testing.T) {
	globalSet := globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
		TargetPath: "~/dotfiles",
	}
	globalEmpty := globalconfig.DotfilesConfig{}

	tests := []struct {
		name           string
		gcfg           globalconfig.DotfilesConfig
		rc             globalconfig.DotfilesRC
		wantRun        bool
		wantRepo       string
		wantTargetPath string
		wantInstall    string
	}{
		{
			name:     "global set, no project override → runs",
			gcfg:     globalSet,
			rc:       globalconfig.DotfilesRC{},
			wantRun:  true,
			wantRepo: "https://github.com/user/dotfiles",
		},
		{
			name:    "global set, project disabled → does not run",
			gcfg:    globalSet,
			rc:      globalconfig.DotfilesRC{Disabled: true},
			wantRun: false,
		},
		{
			name:     "global set, project repo override → runs with override",
			gcfg:     globalSet,
			rc:       globalconfig.DotfilesRC{Repository: "https://github.com/user/other"},
			wantRun:  true,
			wantRepo: "https://github.com/user/other",
		},
		{
			name:           "global set, project targetPath override → merged",
			gcfg:           globalSet,
			rc:             globalconfig.DotfilesRC{TargetPath: "~/custom"},
			wantRun:        true,
			wantRepo:       "https://github.com/user/dotfiles",
			wantTargetPath: "~/custom",
		},
		{
			name:        "global set, project installCommand override → merged",
			gcfg:        globalSet,
			rc:          globalconfig.DotfilesRC{InstallCommand: "make install"},
			wantRun:     true,
			wantRepo:    "https://github.com/user/dotfiles",
			wantInstall: "make install",
		},
		{
			name:     "no global repo, project repo → runs with project settings",
			gcfg:     globalEmpty,
			rc:       globalconfig.DotfilesRC{Repository: "https://github.com/user/dots"},
			wantRun:  true,
			wantRepo: "https://github.com/user/dots",
		},
		{
			name:           "no global repo, project repo, no targetPath → default applied",
			gcfg:           globalEmpty,
			rc:             globalconfig.DotfilesRC{Repository: "https://github.com/user/dots"},
			wantRun:        true,
			wantRepo:       "https://github.com/user/dots",
			wantTargetPath: "~/dotfiles",
		},
		{
			name:    "no global repo, no project override → does not run",
			gcfg:    globalEmpty,
			rc:      globalconfig.DotfilesRC{},
			wantRun: false,
		},
		{
			name:    "no global repo, project disabled → does not run",
			gcfg:    globalEmpty,
			rc:      globalconfig.DotfilesRC{Disabled: true},
			wantRun: false,
		},
		{
			name:    "no global repo, project targetPath only → does not run",
			gcfg:    globalEmpty,
			rc:      globalconfig.DotfilesRC{TargetPath: "~/custom"},
			wantRun: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := ResolveDotfiles(tt.gcfg, tt.rc)
			if ok != tt.wantRun {
				t.Fatalf("wantRun=%v got %v", tt.wantRun, ok)
			}
			if !ok {
				return
			}
			if tt.wantRepo != "" && cfg.Repository != tt.wantRepo {
				t.Errorf("Repository = %q, want %q", cfg.Repository, tt.wantRepo)
			}
			if tt.wantTargetPath != "" && cfg.TargetPath != tt.wantTargetPath {
				t.Errorf("TargetPath = %q, want %q", cfg.TargetPath, tt.wantTargetPath)
			}
			if tt.wantInstall != "" && cfg.InstallCommand != tt.wantInstall {
				t.Errorf("InstallCommand = %q, want %q", cfg.InstallCommand, tt.wantInstall)
			}
		})
	}
}

func TestConfigure_DefaultRegistersBundledPlugins(t *testing.T) {
	result := Configure(Opts{}, discardLogger())
	if result.Manager == nil {
		t.Fatal("Manager is nil")
	}
	names := result.Plugins
	// dotfiles only registers when a repo is configured; expect 3 of 4 by
	// default (coding-agents, shell-history, ssh). package-cache requires
	// providers.
	wantSubset := []string{"coding-agents", "shell-history", "ssh"}
	for _, name := range wantSubset {
		if !contains(names, name) {
			t.Errorf("expected plugin %q to be registered, got %v", name, names)
		}
	}
	if contains(names, "dotfiles") {
		t.Errorf("dotfiles should not register without a repo, got %v", names)
	}
	if contains(names, "package-cache") {
		t.Errorf("package-cache should not register without providers, got %v", names)
	}
}

func TestConfigure_GlobalDisableAllSkipsEverything(t *testing.T) {
	result := Configure(Opts{
		GlobalDisableAll: true,
		GlobalDotfiles:   globalconfig.DotfilesConfig{Repository: "x"},
		CacheProviders:   []string{"npm"},
	}, discardLogger())
	if names := result.Plugins; len(names) != 0 {
		t.Errorf("expected no plugins registered, got %v", names)
	}
	if len(result.BuildCacheMounts) != 0 {
		t.Errorf("expected no build cache mounts, got %v", result.BuildCacheMounts)
	}
}

func TestConfigure_ProjectDisableAllSkipsEverything(t *testing.T) {
	result := Configure(Opts{
		ProjectDisableAll: true,
	}, discardLogger())
	if names := result.Plugins; len(names) != 0 {
		t.Errorf("expected no plugins registered, got %v", names)
	}
}

func TestConfigure_DisableSpecific(t *testing.T) {
	result := Configure(Opts{
		GlobalDisable:  []string{"ssh"},
		ProjectDisable: []string{"coding-agents"},
		CLIDisable:     []string{"shell-history"},
	}, discardLogger())
	names := result.Plugins
	for _, banned := range []string{"ssh", "coding-agents", "shell-history"} {
		if contains(names, banned) {
			t.Errorf("expected %q to be disabled, got %v", banned, names)
		}
	}
}

func TestConfigure_DotfilesRegisteredWhenRepoPresent(t *testing.T) {
	result := Configure(Opts{
		GlobalDotfiles: globalconfig.DotfilesConfig{
			Repository: "git@github.com:user/dots",
			TargetPath: "~/dotfiles",
		},
	}, discardLogger())
	if !contains(result.Plugins, "dotfiles") {
		t.Errorf("expected dotfiles plugin, got %v", result.Plugins)
	}
}

func TestConfigure_CacheProvidersRegisterPackageCache(t *testing.T) {
	result := Configure(Opts{
		CacheProviders: []string{"npm"},
	}, discardLogger())
	if !contains(result.Plugins, "package-cache") {
		t.Errorf("expected package-cache plugin, got %v", result.Plugins)
	}
	if len(result.BuildCacheMounts) == 0 {
		t.Error("expected build cache mounts when package-cache is registered")
	}
}

func TestConfigure_UnknownPluginNameWarns(t *testing.T) {
	var buf bytes.Buffer
	Configure(Opts{
		CLIDisable: []string{"typo"},
	}, captureLogger(&buf))
	if !strings.Contains(buf.String(), "unknown plugin in disable list") {
		t.Errorf("expected warning, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "name=typo") {
		t.Errorf("expected name=typo in warning, got %q", buf.String())
	}
}

func TestConfigure_UnknownCacheProviderWarns(t *testing.T) {
	var buf bytes.Buffer
	Configure(Opts{
		CacheProviders: []string{"npm", "bogus"},
	}, captureLogger(&buf))
	if !strings.Contains(buf.String(), "unknown cache providers") {
		t.Errorf("expected warning for unknown providers, got %q", buf.String())
	}
}

func contains(xs []string, want string) bool {
	return slices.Contains(xs, want)
}
