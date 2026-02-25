package config

import (
	"testing"
)

func TestSubstitute(t *testing.T) {
	ctx := &SubstitutionContext{
		DevContainerID:           "test-id",
		LocalWorkspaceFolder:     "/home/user/myproject",
		ContainerWorkspaceFolder: "/workspace/myproject",
		Env: map[string]string{
			"HOME":   "/home/user",
			"GOPATH": "/home/user/go",
		},
	}

	tests := []struct {
		name      string
		config    *DevContainerConfig
		checkFunc func(t *testing.T, result *DevContainerConfig)
	}{
		{
			"localWorkspaceFolder",
			&DevContainerConfig{
				NonComposeBase: NonComposeBase{
					ContainerEnv: map[string]string{
						"SRC": "${localWorkspaceFolder}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.ContainerEnv["SRC"] != "/home/user/myproject" {
					t.Errorf("got %q, want %q", result.ContainerEnv["SRC"], "/home/user/myproject")
				}
			},
		},
		{
			"localWorkspaceFolderBasename",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					WorkspaceFolder: "/workspace/${localWorkspaceFolderBasename}",
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.WorkspaceFolder != "/workspace/myproject" {
					t.Errorf("got %q, want %q", result.WorkspaceFolder, "/workspace/myproject")
				}
			},
		},
		{
			"containerWorkspaceFolder",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteEnv: map[string]string{
						"PROJECT": "${containerWorkspaceFolder}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.RemoteEnv["PROJECT"] != "/workspace/myproject" {
					t.Errorf("got %q, want %q", result.RemoteEnv["PROJECT"], "/workspace/myproject")
				}
			},
		},
		{
			"containerWorkspaceFolderBasename",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteEnv: map[string]string{
						"NAME": "${containerWorkspaceFolderBasename}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.RemoteEnv["NAME"] != "myproject" {
					t.Errorf("got %q, want %q", result.RemoteEnv["NAME"], "myproject")
				}
			},
		},
		{
			"devcontainerId",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteEnv: map[string]string{
						"ID": "${devcontainerId}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.RemoteEnv["ID"] != "test-id" {
					t.Errorf("got %q, want %q", result.RemoteEnv["ID"], "test-id")
				}
			},
		},
		{
			"localEnv with value",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteEnv: map[string]string{
						"MY_HOME": "${localEnv:HOME}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.RemoteEnv["MY_HOME"] != "/home/user" {
					t.Errorf("got %q, want %q", result.RemoteEnv["MY_HOME"], "/home/user")
				}
			},
		},
		{
			"localEnv with default",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteEnv: map[string]string{
						"EDITOR": "${localEnv:EDITOR:vim}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.RemoteEnv["EDITOR"] != "vim" {
					t.Errorf("got %q, want %q", result.RemoteEnv["EDITOR"], "vim")
				}
			},
		},
		{
			"localEnv missing no default",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteEnv: map[string]string{
						"MISSING": "${localEnv:NONEXISTENT}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.RemoteEnv["MISSING"] != "" {
					t.Errorf("got %q, want empty string", result.RemoteEnv["MISSING"])
				}
			},
		},
		{
			"env alias for localEnv",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteEnv: map[string]string{
						"VAL": "${env:HOME}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.RemoteEnv["VAL"] != "/home/user" {
					t.Errorf("got %q, want %q", result.RemoteEnv["VAL"], "/home/user")
				}
			},
		},
		{
			"containerEnv left as-is",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteEnv: map[string]string{
						"VAL": "${containerEnv:PATH}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.RemoteEnv["VAL"] != "${containerEnv:PATH}" {
					t.Errorf("got %q, want %q", result.RemoteEnv["VAL"], "${containerEnv:PATH}")
				}
			},
		},
		{
			"unknown variable left as-is",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteEnv: map[string]string{
						"VAL": "${unknownVar}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.RemoteEnv["VAL"] != "${unknownVar}" {
					t.Errorf("got %q, want %q", result.RemoteEnv["VAL"], "${unknownVar}")
				}
			},
		},
		{
			"multiple variables in one string",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteEnv: map[string]string{
						"COMBINED": "${localWorkspaceFolder}:${localEnv:GOPATH}",
					},
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				want := "/home/user/myproject:/home/user/go"
				if result.RemoteEnv["COMBINED"] != want {
					t.Errorf("got %q, want %q", result.RemoteEnv["COMBINED"], want)
				}
			},
		},
		{
			"no variables returns equivalent config",
			&DevContainerConfig{
				ImageContainer: ImageContainer{Image: "ubuntu:22.04"},
				DevContainerConfigBase: DevContainerConfigBase{
					RemoteUser: "vscode",
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.Image != "ubuntu:22.04" {
					t.Errorf("Image = %q, want %q", result.Image, "ubuntu:22.04")
				}
				if result.RemoteUser != "vscode" {
					t.Errorf("RemoteUser = %q, want %q", result.RemoteUser, "vscode")
				}
			},
		},
		{
			"substitution in workspace folder",
			&DevContainerConfig{
				DevContainerConfigBase: DevContainerConfigBase{
					WorkspaceFolder: "/workspace/${localWorkspaceFolderBasename}",
				},
			},
			func(t *testing.T, result *DevContainerConfig) {
				t.Helper()
				if result.WorkspaceFolder != "/workspace/myproject" {
					t.Errorf("got %q, want %q", result.WorkspaceFolder, "/workspace/myproject")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Substitute(ctx, tt.config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.checkFunc(t, result)
		})
	}
}

func TestSubstitute_PreservesOrigin(t *testing.T) {
	config := &DevContainerConfig{
		Origin: "/path/to/devcontainer.json",
	}
	ctx := &SubstitutionContext{}

	result, err := Substitute(ctx, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Origin != config.Origin {
		t.Errorf("Origin = %q, want %q", result.Origin, config.Origin)
	}
}

func TestSubstituteContainerEnv(t *testing.T) {
	containerEnv := map[string]string{
		"PATH": "/usr/local/bin:/usr/bin",
		"HOME": "/home/vscode",
	}

	config := &DevContainerConfig{
		DevContainerConfigBase: DevContainerConfigBase{
			RemoteEnv: map[string]string{
				"MY_PATH": "${containerEnv:PATH}",
				"MY_HOME": "${containerEnv:HOME}",
				"LOCAL":   "${localWorkspaceFolder}",
				"MISSING": "${containerEnv:NONEXISTENT:fallback}",
			},
		},
	}

	result, err := SubstituteContainerEnv(containerEnv, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RemoteEnv["MY_PATH"] != "/usr/local/bin:/usr/bin" {
		t.Errorf("MY_PATH = %q, want %q", result.RemoteEnv["MY_PATH"], "/usr/local/bin:/usr/bin")
	}
	if result.RemoteEnv["MY_HOME"] != "/home/vscode" {
		t.Errorf("MY_HOME = %q, want %q", result.RemoteEnv["MY_HOME"], "/home/vscode")
	}
	// localWorkspaceFolder should be left as-is by SubstituteContainerEnv.
	if result.RemoteEnv["LOCAL"] != "${localWorkspaceFolder}" {
		t.Errorf("LOCAL = %q, want %q", result.RemoteEnv["LOCAL"], "${localWorkspaceFolder}")
	}
	if result.RemoteEnv["MISSING"] != "fallback" {
		t.Errorf("MISSING = %q, want %q", result.RemoteEnv["MISSING"], "fallback")
	}
}
