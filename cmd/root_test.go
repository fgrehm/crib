package cmd

import (
	"bytes"
	"log/slog"
	"strings"
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

func TestResolveDotfilesPlugin(t *testing.T) {
	globalSet := globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
		TargetPath: "~/dotfiles",
	}
	globalEmpty := globalconfig.DotfilesConfig{}

	tests := []struct {
		name           string
		gcfg           globalconfig.DotfilesConfig
		rc             dotfilesRC
		wantRun        bool
		wantRepo       string
		wantTargetPath string
		wantInstall    string
	}{
		{
			name:     "global set, no project override → runs",
			gcfg:     globalSet,
			rc:       dotfilesRC{},
			wantRun:  true,
			wantRepo: "https://github.com/user/dotfiles",
		},
		{
			name:    "global set, project disabled → does not run",
			gcfg:    globalSet,
			rc:      dotfilesRC{Disabled: true},
			wantRun: false,
		},
		{
			name:     "global set, project repo override → runs with override",
			gcfg:     globalSet,
			rc:       dotfilesRC{Repository: "https://github.com/user/other"},
			wantRun:  true,
			wantRepo: "https://github.com/user/other",
		},
		{
			name:           "global set, project targetPath override → merged",
			gcfg:           globalSet,
			rc:             dotfilesRC{TargetPath: "~/custom"},
			wantRun:        true,
			wantRepo:       "https://github.com/user/dotfiles",
			wantTargetPath: "~/custom",
		},
		{
			name:        "global set, project installCommand override → merged",
			gcfg:        globalSet,
			rc:          dotfilesRC{InstallCommand: "make install"},
			wantRun:     true,
			wantRepo:    "https://github.com/user/dotfiles",
			wantInstall: "make install",
		},
		{
			name:     "no global repo, project repo → runs with project settings",
			gcfg:     globalEmpty,
			rc:       dotfilesRC{Repository: "https://github.com/user/dots"},
			wantRun:  true,
			wantRepo: "https://github.com/user/dots",
		},
		{
			name:           "no global repo, project repo, no targetPath → default applied",
			gcfg:           globalEmpty,
			rc:             dotfilesRC{Repository: "https://github.com/user/dots"},
			wantRun:        true,
			wantRepo:       "https://github.com/user/dots",
			wantTargetPath: "~/dotfiles",
		},
		{
			name:    "no global repo, no project override → does not run",
			gcfg:    globalEmpty,
			rc:      dotfilesRC{},
			wantRun: false,
		},
		{
			name:    "no global repo, project disabled → does not run",
			gcfg:    globalEmpty,
			rc:      dotfilesRC{Disabled: true},
			wantRun: false,
		},
		{
			name:    "no global repo, project targetPath only → does not run",
			gcfg:    globalEmpty,
			rc:      dotfilesRC{TargetPath: "~/custom"},
			wantRun: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := resolveDotfilesPlugin(tt.gcfg, tt.rc)
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

func TestCollectDisabledPlugins(t *testing.T) {
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
				{"ssh"},           // global
				{"dotfiles"},      // .cribrc
				{"package-cache"}, // --disable-plugin
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
			got := collectDisabledPlugins(tt.layers...)
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

func TestIsKnownPlugin(t *testing.T) {
	for _, name := range []string{"coding-agents", "shell-history", "ssh", "dotfiles", "package-cache"} {
		if !isKnownPlugin(name) {
			t.Errorf("isKnownPlugin(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "sssh", "unknown", "SSH"} {
		if isKnownPlugin(name) {
			t.Errorf("isKnownPlugin(%q) = true, want false", name)
		}
	}
}

// captureLogger installs a slog logger that writes to buf and returns a
// restore func. Tests patch the package-level logger so warn calls in
// cmd/root.go are observable.
func captureLogger(t *testing.T, buf *bytes.Buffer) func() {
	t.Helper()
	orig := logger
	logger = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return func() { logger = orig }
}

func TestWarnUnknownDisabledPlugins(t *testing.T) {
	tests := []struct {
		name       string
		disabled   map[string]bool
		wantNames  []string // names that should appear in a warning line
		wantSilent []string // names that should NOT produce warnings
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
			defer captureLogger(t, &buf)()

			warnUnknownDisabledPlugins(tt.disabled)

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
					t.Errorf("expected names in sorted order; %q appeared before expected predecessor in %q", n, out)
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
