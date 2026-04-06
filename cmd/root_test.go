package cmd

import (
	"testing"

	"github.com/fgrehm/crib/internal/globalconfig"
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
