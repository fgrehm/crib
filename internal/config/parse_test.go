package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFind(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, dir string)
		wantFile string
	}{
		{
			"devcontainer/devcontainer.json",
			func(t *testing.T, dir string) {
				t.Helper()
				mkdirAll(t, filepath.Join(dir, ".devcontainer"))
				writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"ubuntu"}`)
			},
			filepath.Join(".devcontainer", "devcontainer.json"),
		},
		{
			".devcontainer.json at root",
			func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, ".devcontainer.json"), `{"image":"ubuntu"}`)
			},
			".devcontainer.json",
		},
		{
			"subfolder config",
			func(t *testing.T, dir string) {
				t.Helper()
				mkdirAll(t, filepath.Join(dir, ".devcontainer", "python"))
				writeFile(t, filepath.Join(dir, ".devcontainer", "python", "devcontainer.json"), `{"image":"python"}`)
			},
			filepath.Join(".devcontainer", "python", "devcontainer.json"),
		},
		{
			"prefers .devcontainer/ over .devcontainer.json",
			func(t *testing.T, dir string) {
				t.Helper()
				mkdirAll(t, filepath.Join(dir, ".devcontainer"))
				writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"ubuntu"}`)
				writeFile(t, filepath.Join(dir, ".devcontainer.json"), `{"image":"other"}`)
			},
			filepath.Join(".devcontainer", "devcontainer.json"),
		},
		{
			"no config found",
			func(t *testing.T, dir string) {
				t.Helper()
			},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			got, err := Find(dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantFile == "" {
				if got != "" {
					t.Errorf("expected empty, got %q", got)
				}
				return
			}

			want := filepath.Join(dir, tt.wantFile)
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestParse_ImageBased(t *testing.T) {
	config, err := Parse(testdataPath("minimal-image.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Image != "ubuntu:22.04" {
		t.Errorf("Image = %q, want %q", config.Image, "ubuntu:22.04")
	}
	if config.Origin == "" {
		t.Error("Origin should be set")
	}
}

func TestParse_DockerfileBased(t *testing.T) {
	config, err := Parse(testdataPath("minimal-dockerfile.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Build == nil {
		t.Fatal("Build should not be nil")
	}
	if config.Build.Dockerfile != "Dockerfile" {
		t.Errorf("Dockerfile = %q, want %q", config.Build.Dockerfile, "Dockerfile")
	}
}

func TestParse_ComposeBased(t *testing.T) {
	config, err := Parse(testdataPath("minimal-compose.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(config.DockerComposeFile) != 1 || config.DockerComposeFile[0] != "docker-compose.yml" {
		t.Errorf("DockerComposeFile = %v, want [docker-compose.yml]", config.DockerComposeFile)
	}
	if config.Service != "app" {
		t.Errorf("Service = %q, want %q", config.Service, "app")
	}
	if len(config.RunServices) != 2 {
		t.Errorf("RunServices length = %d, want 2", len(config.RunServices))
	}
}

func TestParse_JSONC(t *testing.T) {
	config, err := Parse(testdataPath("full-config.jsonc"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Name != "Full Config" {
		t.Errorf("Name = %q, want %q", config.Name, "Full Config")
	}
	if config.RemoteUser != "vscode" {
		t.Errorf("RemoteUser = %q, want %q", config.RemoteUser, "vscode")
	}
	if len(config.ForwardPorts) != 2 {
		t.Errorf("ForwardPorts length = %d, want 2", len(config.ForwardPorts))
	}
	if len(config.Mounts) != 2 {
		t.Errorf("Mounts length = %d, want 2", len(config.Mounts))
	}
	if config.WorkspaceFolder != "/workspace" {
		t.Errorf("WorkspaceFolder = %q, want %q", config.WorkspaceFolder, "/workspace")
	}
	if config.ShutdownAction != "stopContainer" {
		t.Errorf("ShutdownAction = %q, want %q", config.ShutdownAction, "stopContainer")
	}
}

func TestParse_LegacyReplacement(t *testing.T) {
	config, err := Parse(testdataPath("legacy-extensions.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Extensions should be moved to customizations.vscode.extensions.
	if len(config.Extensions) != 0 {
		t.Errorf("Extensions should be empty after legacy replacement, got %v", config.Extensions)
	}
	if len(config.Settings) != 0 {
		t.Errorf("Settings should be empty after legacy replacement, got %v", config.Settings)
	}

	vscode, ok := config.Customizations["vscode"].(map[string]any)
	if !ok {
		t.Fatal("customizations.vscode should be a map")
	}
	exts, ok := vscode["extensions"].([]string)
	if !ok {
		t.Fatal("customizations.vscode.extensions should be []string")
	}
	if len(exts) != 2 {
		t.Errorf("extensions length = %d, want 2", len(exts))
	}

	settings, ok := vscode["settings"].(map[string]any)
	if !ok {
		t.Fatal("customizations.vscode.settings should be a map")
	}
	if settings["editor.tabSize"] != float64(4) {
		t.Errorf("editor.tabSize = %v, want 4", settings["editor.tabSize"])
	}
}

func TestParse_LifecycleHooks(t *testing.T) {
	config, err := Parse(testdataPath("with-lifecycle-hooks.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// String form: "echo hello"
	if cmds, ok := config.OnCreateCommand[""]; !ok || len(cmds) != 1 || cmds[0] != "echo hello" {
		t.Errorf("OnCreateCommand = %v, want string form", config.OnCreateCommand)
	}

	// Array form: ["git", "pull"]
	if cmds, ok := config.UpdateContentCommand[""]; !ok || len(cmds) != 2 {
		t.Errorf("UpdateContentCommand = %v, want array form", config.UpdateContentCommand)
	}

	// Object form: {"install": "npm install", "build": ["make", "build"]}
	if len(config.PostCreateCommand) != 2 {
		t.Errorf("PostCreateCommand length = %d, want 2", len(config.PostCreateCommand))
	}
}

func TestParse_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	writeFile(t, path, `{invalid json}`)

	_, err := Parse(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParse_NonexistentFile(t *testing.T) {
	_, err := Parse("/nonexistent/devcontainer.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestFindAndParse(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"node:20"}`)

	config, err := FindAndParse(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Image != "node:20" {
		t.Errorf("Image = %q, want %q", config.Image, "node:20")
	}
}

func TestFindAndParse_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := FindAndParse(dir)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestParseBytes(t *testing.T) {
	data := []byte(`{
		// comment
		"image": "alpine:3.18",
		"remoteUser": "root",
	}`)

	config, err := ParseBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Image != "alpine:3.18" {
		t.Errorf("Image = %q, want %q", config.Image, "alpine:3.18")
	}
	if config.Origin != "" {
		t.Errorf("Origin should be empty for ParseBytes, got %q", config.Origin)
	}
}

// --- Test helpers ---

func testdataPath(name string) string {
	return filepath.Join("testdata", name)
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
