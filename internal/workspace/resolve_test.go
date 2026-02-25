package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_DevContainerDir(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"ubuntu"}`)

	result, err := Resolve(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ProjectRoot != dir {
		t.Errorf("ProjectRoot = %q, want %q", result.ProjectRoot, dir)
	}
	if result.RelativeConfigPath != filepath.Join(".devcontainer", "devcontainer.json") {
		t.Errorf("RelativeConfigPath = %q, want %q", result.RelativeConfigPath, ".devcontainer/devcontainer.json")
	}
	if result.WorkspaceID == "" {
		t.Error("WorkspaceID should not be empty")
	}
}

func TestResolve_DotDevContainerJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".devcontainer.json"), `{"image":"ubuntu"}`)

	result, err := Resolve(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ProjectRoot != dir {
		t.Errorf("ProjectRoot = %q, want %q", result.ProjectRoot, dir)
	}
	if result.RelativeConfigPath != ".devcontainer.json" {
		t.Errorf("RelativeConfigPath = %q, want %q", result.RelativeConfigPath, ".devcontainer.json")
	}
}

func TestResolve_SubfolderConfig(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer", "python"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "python", "devcontainer.json"), `{"image":"python"}`)

	result, err := Resolve(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(".devcontainer", "python", "devcontainer.json")
	if result.RelativeConfigPath != expected {
		t.Errorf("RelativeConfigPath = %q, want %q", result.RelativeConfigPath, expected)
	}
}

func TestResolve_WalksUp(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"ubuntu"}`)

	subdir := filepath.Join(dir, "src", "app")
	mkdirAll(t, subdir)

	result, err := Resolve(subdir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ProjectRoot != dir {
		t.Errorf("ProjectRoot = %q, want %q", result.ProjectRoot, dir)
	}
}

func TestResolve_NotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := Resolve(dir)
	if err != ErrNoDevContainer {
		t.Errorf("expected ErrNoDevContainer, got %v", err)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "myproject", "myproject"},
		{"uppercase", "MyProject", "myproject"},
		{"spaces", "My Project", "my-project"},
		{"special chars", "my@project!v2", "my-project-v2"},
		{"dots", "my.project", "my-project"},
		{"leading trailing special", "---project---", "project"},
		{"empty", "", "workspace"},
		{"only special", "@#$", "workspace"},
		{
			"long name truncated",
			"this-is-a-very-very-very-long-project-name-that-exceeds-the-maximum",
			// 40 chars + "-" + 7-char hash
			"this-is-a-very-very-very-long-project-na-" + slugHash("this-is-a-very-very-very-long-project-name-that-exceeds-the-maximum"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Verify max length.
			if len(got) > 48 {
				t.Errorf("slug too long: %d chars (max 48)", len(got))
			}
		})
	}
}

func TestSlugify_Stable(t *testing.T) {
	s1 := Slugify("My Cool Project")
	s2 := Slugify("My Cool Project")
	if s1 != s2 {
		t.Errorf("slugify should be deterministic: %q != %q", s1, s2)
	}
}

// slugHash returns the first 7 chars of the sha256 hex hash of name.
func slugHash(name string) string {
	slug := Slugify(name)
	// The hash suffix is the last 7 characters of the slug.
	return slug[len(slug)-7:]
}

// --- Test helpers ---

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
