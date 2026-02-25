package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestStrArray_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    StrArray
		wantErr bool
	}{
		{"single string", `"hello"`, StrArray{"hello"}, false},
		{"array", `["a","b","c"]`, StrArray{"a", "b", "c"}, false},
		{"empty array", `[]`, StrArray{}, false},
		{"invalid", `123`, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got StrArray
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestStrIntArray_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    StrIntArray
		wantErr bool
	}{
		{"single string", `"8080:80"`, StrIntArray{"8080:80"}, false},
		{"single number", `8080`, StrIntArray{"8080"}, false},
		{"mixed array", `[8080, "9090:90", 3000]`, StrIntArray{"8080", "9090:90", "3000"}, false},
		{"string array", `["8080", "9090"]`, StrIntArray{"8080", "9090"}, false},
		{"empty array", `[]`, StrIntArray{}, false},
		{"invalid element", `[true]`, nil, true},
		{"invalid type", `true`, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got StrIntArray
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLifecycleHook_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    LifecycleHook
		wantErr bool
	}{
		{
			"single string",
			`"echo hello"`,
			LifecycleHook{"": {"echo hello"}},
			false,
		},
		{
			"array",
			`["echo", "hello"]`,
			LifecycleHook{"": {"echo", "hello"}},
			false,
		},
		{
			"object with string values",
			`{"setup": "npm install", "build": "npm run build"}`,
			LifecycleHook{"setup": {"npm install"}, "build": {"npm run build"}},
			false,
		},
		{
			"object with array values",
			`{"compile": ["gcc", "-o", "main", "main.c"]}`,
			LifecycleHook{"compile": {"gcc", "-o", "main", "main.c"}},
			false,
		},
		{
			"invalid type",
			`123`,
			nil,
			true,
		},
		{
			"invalid object value",
			`{"key": 123}`,
			nil,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got LifecycleHook
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, wantArr := range tt.want {
				gotArr, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if len(gotArr) != len(wantArr) {
					t.Errorf("key %q: got %v, want %v", k, gotArr, wantArr)
					continue
				}
				for i := range gotArr {
					if gotArr[i] != wantArr[i] {
						t.Errorf("key %q index %d: got %q, want %q", k, i, gotArr[i], wantArr[i])
					}
				}
			}
		})
	}
}

func TestStrBool_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    StrBool
		isTrue  bool
		wantErr bool
	}{
		{"bool true", `true`, "true", true, false},
		{"bool false", `false`, "false", false, false},
		{"string true", `"true"`, "true", true, false},
		{"string True", `"True"`, "True", true, false},
		{"string false", `"false"`, "false", false, false},
		{"string optional", `"optional"`, "optional", false, false},
		{"invalid", `123`, "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got StrBool
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if got.IsTrue() != tt.isTrue {
				t.Errorf("IsTrue() = %v, want %v", got.IsTrue(), tt.isTrue)
			}
		})
	}
}

func TestMount_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Mount
		wantErr bool
	}{
		{
			"string format with src/dst",
			`"type=bind,src=/tmp,dst=/tmp"`,
			Mount{Type: "bind", Source: "/tmp", Target: "/tmp"},
			false,
		},
		{
			"string format with source/target",
			`"type=volume,source=mydata,target=/data"`,
			Mount{Type: "volume", Source: "mydata", Target: "/data"},
			false,
		},
		{
			"string format with destination",
			`"type=bind,src=/a,destination=/b"`,
			Mount{Type: "bind", Source: "/a", Target: "/b"},
			false,
		},
		{
			"object format",
			`{"type":"bind","source":"/host","target":"/container"}`,
			Mount{Type: "bind", Source: "/host", Target: "/container"},
			false,
		},
		{
			"object format with external",
			`{"type":"volume","source":"data","target":"/data","external":true}`,
			Mount{Type: "volume", Source: "data", Target: "/data", External: true},
			false,
		},
		{
			"invalid",
			`123`,
			Mount{},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Mount
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseMount(t *testing.T) {
	tests := []struct {
		input string
		want  Mount
	}{
		{
			"type=bind,src=/tmp,dst=/tmp",
			Mount{Type: "bind", Source: "/tmp", Target: "/tmp"},
		},
		{
			"type=volume,source=data,target=/data",
			Mount{Type: "volume", Source: "data", Target: "/data"},
		},
		{
			"type=bind",
			Mount{Type: "bind"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseMount(tt.input)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMount_String(t *testing.T) {
	m := Mount{Type: "bind", Source: "/host", Target: "/container"}
	got := m.String()
	want := "type=bind,src=/host,dst=/container"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGetContextPath(t *testing.T) {
	tests := []struct {
		name   string
		config *DevContainerConfig
		want   string
	}{
		{
			"build context from build options",
			&DevContainerConfig{
				DockerfileContainer: DockerfileContainer{
					Build: &ConfigBuildOptions{Context: ".."},
				},
				Origin: "/project/.devcontainer/devcontainer.json",
			},
			filepath.Join("/project/.devcontainer", ".."),
		},
		{
			"legacy context field",
			&DevContainerConfig{
				DockerfileContainer: DockerfileContainer{
					Context: "src",
				},
				Origin: "/project/.devcontainer/devcontainer.json",
			},
			filepath.Join("/project/.devcontainer", "src"),
		},
		{
			"default to config directory",
			&DevContainerConfig{
				Origin: "/project/.devcontainer/devcontainer.json",
			},
			"/project/.devcontainer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetContextPath(tt.config)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetDockerfilePath(t *testing.T) {
	tests := []struct {
		name   string
		config *DevContainerConfig
		want   string
	}{
		{
			"from build options",
			&DevContainerConfig{
				DockerfileContainer: DockerfileContainer{
					Build: &ConfigBuildOptions{Dockerfile: "Dockerfile.dev"},
				},
				Origin: "/project/.devcontainer/devcontainer.json",
			},
			"/project/.devcontainer/Dockerfile.dev",
		},
		{
			"legacy dockerfile field",
			&DevContainerConfig{
				DockerfileContainer: DockerfileContainer{
					Dockerfile: "Dockerfile",
				},
				Origin: "/project/.devcontainer/devcontainer.json",
			},
			"/project/.devcontainer/Dockerfile",
		},
		{
			"no dockerfile",
			&DevContainerConfig{
				Origin: "/project/.devcontainer/devcontainer.json",
			},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDockerfilePath(tt.config)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDevContainerConfig_FullUnmarshal(t *testing.T) {
	input := `{
		"name": "test",
		"image": "ubuntu:22.04",
		"forwardPorts": [8080, "9090:9090"],
		"remoteUser": "vscode",
		"containerEnv": {"FOO": "bar"},
		"mounts": [
			"type=bind,src=/tmp,dst=/tmp",
			{"type": "volume", "source": "data", "target": "/data"}
		],
		"onCreateCommand": "echo hello",
		"postCreateCommand": {"setup": "npm install"},
		"features": {
			"ghcr.io/devcontainers/features/node:1": {"version": "18"}
		},
		"customizations": {
			"vscode": {"extensions": ["ms-python.python"]}
		}
	}`

	var got DevContainerConfig
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Name != "test" {
		t.Errorf("Name = %q, want %q", got.Name, "test")
	}
	if got.Image != "ubuntu:22.04" {
		t.Errorf("Image = %q, want %q", got.Image, "ubuntu:22.04")
	}
	if len(got.ForwardPorts) != 2 {
		t.Errorf("ForwardPorts length = %d, want 2", len(got.ForwardPorts))
	}
	if got.RemoteUser != "vscode" {
		t.Errorf("RemoteUser = %q, want %q", got.RemoteUser, "vscode")
	}
	if got.ContainerEnv["FOO"] != "bar" {
		t.Errorf("ContainerEnv[FOO] = %q, want %q", got.ContainerEnv["FOO"], "bar")
	}
	if len(got.Mounts) != 2 {
		t.Errorf("Mounts length = %d, want 2", len(got.Mounts))
	}
	if got.Mounts[0].Source != "/tmp" {
		t.Errorf("Mounts[0].Source = %q, want %q", got.Mounts[0].Source, "/tmp")
	}
	if got.Mounts[1].Type != "volume" {
		t.Errorf("Mounts[1].Type = %q, want %q", got.Mounts[1].Type, "volume")
	}
	if len(got.OnCreateCommand) != 1 {
		t.Errorf("OnCreateCommand length = %d, want 1", len(got.OnCreateCommand))
	}
	if len(got.PostCreateCommand) != 1 {
		t.Errorf("PostCreateCommand length = %d, want 1", len(got.PostCreateCommand))
	}
	if len(got.Features) != 1 {
		t.Errorf("Features length = %d, want 1", len(got.Features))
	}
	if got.Customizations == nil {
		t.Error("Customizations is nil")
	}
}
