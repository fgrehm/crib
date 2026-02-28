package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestGenerateComposeOverride_RootlessPodmanInjectsUserns(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 1000 }

	e := &Engine{
		compose: compose.NewHelperFromRuntime("podman"),
	}

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	dir := t.TempDir()
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "")
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}

	if !strings.Contains(string(data), `userns_mode: "keep-id"`) {
		t.Errorf("expected userns_mode: \"keep-id\" in override, got:\n%s", data)
	}
	if !strings.Contains(string(data), "x-podman:\n  in_pod: false") {
		t.Errorf("expected x-podman in_pod: false in override, got:\n%s", data)
	}
}

func TestGenerateComposeOverride_RootPodmanSkipsUserns(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 0 }

	e := &Engine{
		compose: compose.NewHelperFromRuntime("podman"),
	}

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	dir := t.TempDir()
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "")
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}

	if strings.Contains(string(data), "userns_mode") {
		t.Errorf("userns_mode should not be injected for root podman, got:\n%s", data)
	}
}

func TestGenerateComposeOverride_DockerSkipsUserns(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 1000 }

	e := &Engine{
		compose: compose.NewHelperFromRuntime("docker"),
	}

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	dir := t.TempDir()
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "")
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}

	if strings.Contains(string(data), "userns_mode") {
		t.Errorf("userns_mode should not be injected for docker, got:\n%s", data)
	}
}

func TestGenerateComposeOverride_SkipsUsernsWhenAlreadySet(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 1000 }

	e := &Engine{
		compose: compose.NewHelperFromRuntime("podman"),
	}

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	dir := t.TempDir()

	// Write a compose file that already has userns_mode set.
	composeFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composeFile, []byte("services:\n  app:\n    userns_mode: \"host\"\n"), 0o644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, []string{composeFile}, "")
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}

	if strings.Contains(string(data), "userns_mode") {
		t.Errorf("userns_mode should not be injected when already in compose files, got:\n%s", data)
	}
}

func TestGenerateComposeOverride_WithFeatureImage(t *testing.T) {
	e := &Engine{
		compose: compose.NewHelperFromRuntime("docker"),
	}

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	dir := t.TempDir()
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "crib-test-ws:crib-abc123")
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}

	if !strings.Contains(string(data), "image: crib-test-ws:crib-abc123") {
		t.Errorf("expected image override in YAML, got:\n%s", data)
	}
}

// TestGenerateComposeOverride_RestartPath verifies that generateComposeOverride
// produces a valid override when called from the restart-after-stop path (no
// feature image). The override must include the workspace label and must not
// inject an image override, since the feature image is already baked in from
// the initial up.
func TestGenerateComposeOverride_RestartPath(t *testing.T) {
	e := &Engine{
		compose: compose.NewHelperFromRuntime("docker"),
	}

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	dir := t.TempDir()
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "" /* featureImage already baked in */)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "crib.workspace") {
		t.Errorf("expected crib.workspace label in restart override, got:\n%s", content)
	}
	if strings.Contains(content, "image:") {
		t.Errorf("restart override must not include image override (feature image already baked in), got:\n%s", content)
	}
}

func TestRemoveService(t *testing.T) {
	got := removeService([]string{"app", "db", "cache"}, "app")
	if len(got) != 2 || got[0] != "db" || got[1] != "cache" {
		t.Errorf("removeService = %v, want [db cache]", got)
	}

	got = removeService([]string{"app"}, "app")
	if len(got) != 0 {
		t.Errorf("removeService single = %v, want []", got)
	}

	got = removeService([]string{"db", "cache"}, "app")
	if len(got) != 2 {
		t.Errorf("removeService absent = %v, want [db cache]", got)
	}
}

func TestWritePodmanDownOverride_RootlessPodman(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 1000 }

	e := &Engine{compose: compose.NewHelperFromRuntime("podman")}

	dir := t.TempDir()
	composeFile := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(composeFile, []byte("services:\n  app:\n    image: alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	path, ok := e.writePodmanDownOverride([]string{composeFile})
	if !ok {
		t.Fatal("expected override to be written for rootless podman")
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "x-podman:") || !strings.Contains(content, "in_pod: false") {
		t.Errorf("unexpected override content:\n%s", content)
	}
}

func TestWritePodmanDownOverride_Docker(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 1000 }

	e := &Engine{compose: compose.NewHelperFromRuntime("docker")}

	dir := t.TempDir()
	composeFile := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(composeFile, []byte("services:\n  app:\n    image: alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, ok := e.writePodmanDownOverride([]string{composeFile})
	if ok {
		t.Error("docker should not create podman override")
	}
}

func TestWritePodmanDownOverride_SkipsWhenUsernsSet(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 1000 }

	e := &Engine{compose: compose.NewHelperFromRuntime("podman")}

	dir := t.TempDir()
	composeFile := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(composeFile, []byte("services:\n  app:\n    userns_mode: host\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, ok := e.writePodmanDownOverride([]string{composeFile})
	if ok {
		t.Error("should skip override when compose files already set userns_mode")
	}
}

func TestWritePodmanDownOverride_RootPodman(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 0 }

	e := &Engine{compose: compose.NewHelperFromRuntime("podman")}

	dir := t.TempDir()
	composeFile := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(composeFile, []byte("services:\n  app:\n    image: alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, ok := e.writePodmanDownOverride([]string{composeFile})
	if ok {
		t.Error("root podman should not create override")
	}
}
