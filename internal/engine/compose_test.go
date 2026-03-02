package engine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
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
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "", nil)
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
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "", nil)
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
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "", nil)
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

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, []string{composeFile}, "", nil)
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
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "crib-test-ws:crib-abc123", nil)
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
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "" /* featureImage already baked in */, nil)
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

func TestGenerateComposeOverride_PluginMounts(t *testing.T) {
	e := &Engine{
		compose: compose.NewHelperFromRuntime("docker"),
	}

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	pluginResp := &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{
			{Type: "bind", Source: "/host/history", Target: "/home/vscode/.crib_history"},
			{Type: "bind", Source: "/host/ssh", Target: "/tmp/ssh-agent.sock"},
		},
	}

	dir := t.TempDir()
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "", pluginResp)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	// Workspace mount should still be present.
	if !strings.Contains(content, "/tmp/project:/workspaces/project") {
		t.Errorf("expected workspace mount, got:\n%s", content)
	}
	// Plugin mounts should be present.
	if !strings.Contains(content, "/host/history:/home/vscode/.crib_history") {
		t.Errorf("expected plugin history mount, got:\n%s", content)
	}
	if !strings.Contains(content, "/host/ssh:/tmp/ssh-agent.sock") {
		t.Errorf("expected plugin ssh mount, got:\n%s", content)
	}
}

func TestGenerateComposeOverride_PluginEnv(t *testing.T) {
	e := &Engine{
		compose: compose.NewHelperFromRuntime("docker"),
	}

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	pluginResp := &plugin.PreContainerRunResponse{
		Env: map[string]string{
			"HISTFILE":       "/home/vscode/.crib_history/.shell_history",
			"SSH_AUTH_SOCK":  "/tmp/ssh-agent.sock",
		},
	}

	dir := t.TempDir()
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "", pluginResp)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "HISTFILE:") {
		t.Errorf("expected HISTFILE env var, got:\n%s", content)
	}
	if !strings.Contains(content, "SSH_AUTH_SOCK:") {
		t.Errorf("expected SSH_AUTH_SOCK env var, got:\n%s", content)
	}
}

func TestGenerateComposeOverride_PluginEnvMergedWithConfigEnv(t *testing.T) {
	e := &Engine{
		compose: compose.NewHelperFromRuntime("docker"),
	}

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"
	cfg.ContainerEnv = map[string]string{"APP_ENV": "development"}

	pluginResp := &plugin.PreContainerRunResponse{
		Env: map[string]string{"HISTFILE": "/home/vscode/.crib_history/.shell_history"},
	}

	dir := t.TempDir()
	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "", pluginResp)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	// Both config and plugin env vars should be present.
	if !strings.Contains(content, "APP_ENV:") {
		t.Errorf("expected APP_ENV from config, got:\n%s", content)
	}
	if !strings.Contains(content, "HISTFILE:") {
		t.Errorf("expected HISTFILE from plugin, got:\n%s", content)
	}
}

func TestGenerateComposeOverride_NilPluginResponse(t *testing.T) {
	e := &Engine{
		compose: compose.NewHelperFromRuntime("docker"),
	}

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	dir := t.TempDir()

	// With nil plugin response.
	path1, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride with nil plugin failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path1) })

	// With empty plugin response.
	path2, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", dir, nil, "", &plugin.PreContainerRunResponse{})
	if err != nil {
		t.Fatalf("generateComposeOverride with empty plugin failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(path2) })

	data1, _ := os.ReadFile(path1)
	data2, _ := os.ReadFile(path2)

	// Both should produce equivalent output (no extra environment/volumes sections).
	if string(data1) != string(data2) {
		t.Errorf("nil and empty plugin response should produce same output.\nnil:\n%s\nempty:\n%s", data1, data2)
	}
}

func TestResolveComposeUser_ConfigUserTakesPrecedence(t *testing.T) {
	eng := &Engine{
		logger: slog.Default(),
		driver: &mockDriver{},
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "devuser"

	// When config already specifies a user, resolveComposeUser returns empty
	// (meaning: don't override, let dispatchPlugins use cfg fields).
	user := eng.resolveComposeUser(context.Background(), cfg, t.TempDir(), nil)
	if user != "" {
		t.Errorf("expected empty user when config has remoteUser, got %q", user)
	}

	cfg.RemoteUser = ""
	cfg.ContainerUser = "devuser"
	user = eng.resolveComposeUser(context.Background(), cfg, t.TempDir(), nil)
	if user != "" {
		t.Errorf("expected empty user when config has containerUser, got %q", user)
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
