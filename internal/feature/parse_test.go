package feature

import (
	"path/filepath"
	"testing"
)

func testdataPath(name string) string {
	return filepath.Join("testdata", name)
}

func TestParseFeatureConfig(t *testing.T) {
	tests := []struct {
		name      string
		folder    string
		wantErr   bool
		wantID    string
		checkFunc func(t *testing.T, fc *FeatureConfig)
	}{
		{
			name:   "minimal",
			folder: testdataPath("minimal"),
			wantID: "minimal",
		},
		{
			name:   "with options",
			folder: testdataPath("with-options"),
			wantID: "with-options",
			checkFunc: func(t *testing.T, fc *FeatureConfig) {
				if len(fc.Options) != 2 {
					t.Errorf("got %d options, want 2", len(fc.Options))
				}
				opt, ok := fc.Options["version"]
				if !ok {
					t.Fatal("missing 'version' option")
				}
				if opt.Type != "string" {
					t.Errorf("version type = %q, want %q", opt.Type, "string")
				}
				if string(opt.Default) != "latest" {
					t.Errorf("version default = %q, want %q", opt.Default, "latest")
				}
				if len(opt.Proposals) != 2 {
					t.Errorf("got %d proposals, want 2", len(opt.Proposals))
				}

				boolOpt, ok := fc.Options["install_tools"]
				if !ok {
					t.Fatal("missing 'install_tools' option")
				}
				if boolOpt.Type != "boolean" {
					t.Errorf("install_tools type = %q, want %q", boolOpt.Type, "boolean")
				}
				if !boolOpt.Default.IsTrue() {
					t.Error("install_tools default should be true")
				}
			},
		},
		{
			name:   "with dependencies",
			folder: testdataPath("with-deps"),
			wantID: "with-deps",
			checkFunc: func(t *testing.T, fc *FeatureConfig) {
				if len(fc.DependsOn) != 1 {
					t.Errorf("got %d dependsOn, want 1", len(fc.DependsOn))
				}
				if _, ok := fc.DependsOn["ghcr.io/devcontainers/features/common-utils:2"]; !ok {
					t.Error("missing expected dependsOn key")
				}
				if len(fc.InstallsAfter) != 1 {
					t.Errorf("got %d installsAfter, want 1", len(fc.InstallsAfter))
				}
			},
		},
		{
			name:   "full JSONC config",
			folder: testdataPath("full"),
			wantID: "full-feature",
			checkFunc: func(t *testing.T, fc *FeatureConfig) {
				if fc.Name != "Full Feature" {
					t.Errorf("name = %q, want %q", fc.Name, "Full Feature")
				}
				if fc.Version != "2.1.0" {
					t.Errorf("version = %q, want %q", fc.Version, "2.1.0")
				}
				if fc.Description != "A feature with all fields populated" {
					t.Errorf("unexpected description: %q", fc.Description)
				}
				if fc.Entrypoint != "/usr/local/bin/entrypoint.sh" {
					t.Errorf("entrypoint = %q", fc.Entrypoint)
				}
				if !fc.Deprecated {
					t.Error("expected deprecated = true")
				}
				if len(fc.Options) != 1 {
					t.Errorf("got %d options, want 1", len(fc.Options))
				}
				if opt, ok := fc.Options["version"]; ok {
					if len(opt.Enum) != 3 {
						t.Errorf("got %d enum values, want 3", len(opt.Enum))
					}
				}
				if len(fc.DependsOn) != 1 {
					t.Errorf("got %d dependsOn, want 1", len(fc.DependsOn))
				}
				if len(fc.InstallsAfter) != 1 {
					t.Errorf("got %d installsAfter, want 1", len(fc.InstallsAfter))
				}
				if len(fc.CapAdd) != 1 || fc.CapAdd[0] != "SYS_PTRACE" {
					t.Errorf("capAdd = %v", fc.CapAdd)
				}
				if fc.Init == nil || !*fc.Init {
					t.Error("expected init = true")
				}
				if fc.Privileged == nil || *fc.Privileged {
					t.Error("expected privileged = false")
				}
				if len(fc.SecurityOpt) != 1 {
					t.Errorf("got %d securityOpt, want 1", len(fc.SecurityOpt))
				}
				if len(fc.Mounts) != 1 {
					t.Errorf("got %d mounts, want 1", len(fc.Mounts))
				}
				if fc.ContainerEnv["MY_VAR"] != "my_value" {
					t.Errorf("containerEnv[MY_VAR] = %q", fc.ContainerEnv["MY_VAR"])
				}
				if len(fc.OnCreateCommand) == 0 {
					t.Error("expected onCreateCommand")
				}
				if len(fc.PostCreateCommand) == 0 {
					t.Error("expected postCreateCommand")
				}
			},
		},
		{
			name:    "missing file",
			folder:  testdataPath("nonexistent"),
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			folder:  testdataPath("invalid"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc, err := ParseFeatureConfig(tt.folder)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fc.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", fc.ID, tt.wantID)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, fc)
			}
		})
	}
}
