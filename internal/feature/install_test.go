package feature

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
)

func TestPrepareContextDirectoryStructure(t *testing.T) {
	contextDir := t.TempDir()
	featureDir := setupFeatureDir(t, "test-feature")

	features := []*FeatureSet{
		{
			ConfigID: "test-feature",
			Folder:   featureDir,
			Config:   &FeatureConfig{ID: "test-feature"},
		},
	}

	featuresPath, err := PrepareContext(contextDir, features, "vscode", "vscode")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check features directory exists.
	if _, err := os.Stat(featuresPath); err != nil {
		t.Fatalf("features dir not created: %v", err)
	}

	// Check numbered feature directory.
	featureSubdir := filepath.Join(featuresPath, "0")
	if _, err := os.Stat(featureSubdir); err != nil {
		t.Fatalf("feature subdir not created: %v", err)
	}

	// Check install.sh was copied.
	installPath := filepath.Join(featureSubdir, "install.sh")
	if _, err := os.Stat(installPath); err != nil {
		t.Fatalf("install.sh not copied: %v", err)
	}
}

func TestPrepareContextBuiltinEnv(t *testing.T) {
	contextDir := t.TempDir()
	featureDir := setupFeatureDir(t, "test-feature")

	features := []*FeatureSet{
		{
			ConfigID: "test-feature",
			Folder:   featureDir,
			Config:   &FeatureConfig{ID: "test-feature"},
		},
	}

	featuresPath, err := PrepareContext(contextDir, features, "vscode", "devuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	builtinPath := filepath.Join(featuresPath, builtinEnvFile)
	data, err := os.ReadFile(builtinPath)
	if err != nil {
		t.Fatalf("reading builtin env: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `_CONTAINER_USER="vscode"`) {
		t.Errorf("missing container user, got:\n%s", content)
	}
	if !strings.Contains(content, `_REMOTE_USER="devuser"`) {
		t.Errorf("missing remote user, got:\n%s", content)
	}
	if !strings.Contains(content, `_CONTAINER_USER_HOME="/home/vscode"`) {
		t.Errorf("missing container user home, got:\n%s", content)
	}
	if !strings.Contains(content, `_REMOTE_USER_HOME="/home/devuser"`) {
		t.Errorf("missing remote user home, got:\n%s", content)
	}
}

func TestPrepareContextBuiltinEnvRoot(t *testing.T) {
	contextDir := t.TempDir()
	featureDir := setupFeatureDir(t, "test-feature")

	features := []*FeatureSet{
		{
			ConfigID: "test-feature",
			Folder:   featureDir,
			Config:   &FeatureConfig{ID: "test-feature"},
		},
	}

	featuresPath, err := PrepareContext(contextDir, features, "root", "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(featuresPath, builtinEnvFile))
	if err != nil {
		t.Fatalf("reading builtin env: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `_CONTAINER_USER_HOME="/root"`) {
		t.Errorf("root user should have /root home, got:\n%s", content)
	}
}

func TestPrepareContextFeatureEnv(t *testing.T) {
	contextDir := t.TempDir()
	featureDir := setupFeatureDir(t, "test-feature")

	features := []*FeatureSet{
		{
			ConfigID: "test-feature",
			Folder:   featureDir,
			Config: &FeatureConfig{
				ID: "test-feature",
				Options: map[string]FeatureOption{
					"version": {Default: config.StrBool("latest")},
				},
			},
			Options: map[string]any{"version": "3.12"},
		},
	}

	featuresPath, err := PrepareContext(contextDir, features, "root", "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	envPath := filepath.Join(featuresPath, "0", featureEnvFile)
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading feature env: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `VERSION="3.12"`) {
		t.Errorf("missing version env var, got:\n%s", content)
	}
}

func TestPrepareContextWrapperScript(t *testing.T) {
	contextDir := t.TempDir()
	featureDir := setupFeatureDir(t, "test-feature")

	features := []*FeatureSet{
		{
			ConfigID: "test-feature",
			Folder:   featureDir,
			Config: &FeatureConfig{
				ID:      "test-feature",
				Name:    "Test Feature",
				Version: "1.0.0",
			},
		},
	}

	featuresPath, err := PrepareContext(contextDir, features, "root", "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scriptPath := filepath.Join(featuresPath, "0", "devcontainer-features-install.sh")
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("reading wrapper script: %v", err)
	}

	content := string(data)
	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Error("missing shebang")
	}
	if !strings.Contains(content, "set -e") {
		t.Error("missing set -e")
	}
	if !strings.Contains(content, "Feature: test-feature") {
		t.Error("missing feature ID echo")
	}
	if !strings.Contains(content, "Name: Test Feature") {
		t.Error("missing feature name echo")
	}
	if !strings.Contains(content, "Version: 1.0.0") {
		t.Error("missing feature version echo")
	}
	if !strings.Contains(content, "devcontainer-features.builtin.env") {
		t.Error("missing builtin env sourcing")
	}
	if !strings.Contains(content, "./install.sh") {
		t.Error("missing install.sh execution")
	}

	// Check executable permission.
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat script: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("wrapper script should be executable")
	}
}

func TestPrepareContextCleanup(t *testing.T) {
	contextDir := t.TempDir()
	featureDir := setupFeatureDir(t, "test-feature")

	// Create a pre-existing features directory.
	oldDir := filepath.Join(contextDir, ContextFeatureFolder)
	if err := os.MkdirAll(filepath.Join(oldDir, "old-stuff"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "old-stuff", "old-file"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	features := []*FeatureSet{
		{
			ConfigID: "test-feature",
			Folder:   featureDir,
			Config:   &FeatureConfig{ID: "test-feature"},
		},
	}

	featuresPath, err := PrepareContext(contextDir, features, "root", "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Old content should be gone.
	if _, err := os.Stat(filepath.Join(featuresPath, "old-stuff")); !os.IsNotExist(err) {
		t.Error("old directory should have been cleaned up")
	}
}

func TestPrepareContextMultipleFeatures(t *testing.T) {
	contextDir := t.TempDir()
	featureDirA := setupFeatureDir(t, "feature-a")
	featureDirB := setupFeatureDir(t, "feature-b")

	features := []*FeatureSet{
		{
			ConfigID: "feature-a",
			Folder:   featureDirA,
			Config:   &FeatureConfig{ID: "feature-a"},
		},
		{
			ConfigID: "feature-b",
			Folder:   featureDirB,
			Config:   &FeatureConfig{ID: "feature-b"},
		},
	}

	featuresPath, err := PrepareContext(contextDir, features, "root", "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := range features {
		dir := filepath.Join(featuresPath, strconv.Itoa(i))
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("feature dir %d not created: %v", i, err)
		}
		scriptPath := filepath.Join(dir, "devcontainer-features-install.sh")
		if _, err := os.Stat(scriptPath); err != nil {
			t.Errorf("install script %d not created: %v", i, err)
		}
	}
}

func TestEscapeQuotes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"it's", "it'\\''s"},
		{"a'b'c", "a'\\''b'\\''c"},
		{"no quotes here", "no quotes here"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeQuotes(tt.input)
			if got != tt.want {
				t.Errorf("escapeQuotes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInstallWrapperScriptIgnoresEntrypoint(t *testing.T) {
	// The entrypoint field in devcontainer-feature.json is the container
	// entrypoint (runtime), not the install script. The install script is
	// always install.sh.
	fc := &FeatureConfig{
		ID:         "custom",
		Entrypoint: "/usr/local/share/docker-init.sh",
	}

	script := installWrapperScript("custom", fc, nil)
	if !strings.Contains(script, "./install.sh") {
		t.Errorf("script should always use install.sh, got:\n%s", script)
	}
	if strings.Contains(script, "docker-init.sh") {
		t.Errorf("script should not reference the entrypoint, got:\n%s", script)
	}
}

// setupFeatureDir creates a temporary feature directory with a minimal install.sh.
func setupFeatureDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	installScript := "#!/bin/sh\necho 'Installing " + name + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "install.sh"), []byte(installScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
