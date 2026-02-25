package config

import (
	"testing"
)

func TestImageMetadataFromConfig(t *testing.T) {
	config := &DevContainerConfig{
		DevContainerConfigBase: DevContainerConfigBase{
			RemoteUser: "vscode",
			RemoteEnv:  map[string]string{"FOO": "bar"},
		},
		DevContainerActions: DevContainerActions{
			OnCreateCommand: LifecycleHook{"": {"echo hello"}},
		},
		NonComposeBase: NonComposeBase{
			ContainerEnv: map[string]string{"BAZ": "qux"},
		},
		ImageContainer: ImageContainer{Image: "ubuntu"},
	}

	meta := ImageMetadataFromConfig(config)

	if meta.RemoteUser != "vscode" {
		t.Errorf("RemoteUser = %q, want %q", meta.RemoteUser, "vscode")
	}
	if meta.RemoteEnv["FOO"] != "bar" {
		t.Errorf("RemoteEnv[FOO] = %q, want %q", meta.RemoteEnv["FOO"], "bar")
	}
	if meta.ContainerEnv["BAZ"] != "qux" {
		t.Errorf("ContainerEnv[BAZ] = %q, want %q", meta.ContainerEnv["BAZ"], "qux")
	}
	if len(meta.OnCreateCommand) != 1 {
		t.Errorf("OnCreateCommand length = %d, want 1", len(meta.OnCreateCommand))
	}
}

func TestParseImageMetadata(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		want    int
		wantErr bool
	}{
		{
			"empty string",
			"",
			0,
			false,
		},
		{
			"array of entries",
			`[{"id":"feature1","remoteUser":"dev"},{"id":"feature2","entrypoint":"/entrypoint.sh"}]`,
			2,
			false,
		},
		{
			"single object",
			`{"id":"feature1","remoteUser":"dev"}`,
			1,
			false,
		},
		{
			"invalid json",
			`{invalid}`,
			0,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := ParseImageMetadata(tt.label)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entries) != tt.want {
				t.Errorf("got %d entries, want %d", len(entries), tt.want)
			}
		})
	}
}
