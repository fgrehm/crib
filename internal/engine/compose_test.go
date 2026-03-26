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
	"github.com/fgrehm/crib/internal/feature"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// newComposeTestEngine creates an Engine with a workspace store backed by a
// temp directory. It saves the workspace so the directory exists for the
// compose override file.
func newComposeTestEngine(t *testing.T, runtime string, ws *workspace.Workspace) *Engine {
	t.Helper()
	store := workspace.NewStoreAt(t.TempDir())
	if err := store.Save(ws); err != nil {
		t.Fatalf("saving workspace: %v", err)
	}
	return &Engine{
		compose: compose.NewHelperFromRuntime(runtime),
		store:   store,
	}
}

func TestGenerateComposeOverride_RootlessPodmanInjectsUserns(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 1000 }

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "podman", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}

	if !strings.Contains(string(data), "userns_mode: keep-id") {
		t.Errorf("expected userns_mode in override, got:\n%s", data)
	}
	if !strings.Contains(string(data), "x-podman:") || !strings.Contains(string(data), "in_pod: false") {
		t.Errorf("expected x-podman in_pod: false in override, got:\n%s", data)
	}
}

func TestGenerateComposeOverride_RootPodmanSkipsUserns(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 0 }

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "podman", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

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

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

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

	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "podman", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	dir := t.TempDir()

	// Write a compose file that already has userns_mode set.
	composeFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composeFile, []byte("services:\n  app:\n    userns_mode: \"host\"\n"), 0o644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", []string{composeFile}, "", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}

	if strings.Contains(string(data), "userns_mode") {
		t.Errorf("userns_mode should not be injected when already in compose files, got:\n%s", data)
	}
}

func TestGenerateComposeOverride_WithFeatureImage(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "crib-test-ws:crib-abc123", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

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
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "" /* featureImage already baked in */, nil)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

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
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	pluginResp := &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{
			{Type: "bind", Source: "/host/history", Target: "/home/vscode/.crib_history"},
			{Type: "bind", Source: "/host/ssh", Target: "/tmp/ssh-agent.sock"},
		},
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", pluginResp)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	// Workspace mount should still be present (long form).
	if !strings.Contains(content, "source: /tmp/project") || !strings.Contains(content, "target: /workspaces/project") {
		t.Errorf("expected workspace mount, got:\n%s", content)
	}
	// Plugin mounts should be present.
	if !strings.Contains(content, "source: /host/history") || !strings.Contains(content, "target: /home/vscode/.crib_history") {
		t.Errorf("expected plugin history mount, got:\n%s", content)
	}
	if !strings.Contains(content, "source: /host/ssh") || !strings.Contains(content, "target: /tmp/ssh-agent.sock") {
		t.Errorf("expected plugin ssh mount, got:\n%s", content)
	}
}

func TestGenerateComposeOverride_PluginEnv(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	pluginResp := &plugin.PreContainerRunResponse{
		Env: map[string]string{
			"HISTFILE":      "/home/vscode/.crib_history/.shell_history",
			"SSH_AUTH_SOCK": "/tmp/ssh-agent.sock",
		},
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", pluginResp)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

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
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"
	cfg.ContainerEnv = map[string]string{"APP_ENV": "development"}

	pluginResp := &plugin.PreContainerRunResponse{
		Env: map[string]string{"HISTFILE": "/home/vscode/.crib_history/.shell_history"},
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", pluginResp)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

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
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	// With nil plugin response.
	path1, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride with nil plugin failed: %v", err)
	}
	data1, _ := os.ReadFile(path1)

	// With empty plugin response (overwrites the same file).
	_, err = e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", &plugin.PreContainerRunResponse{})
	if err != nil {
		t.Fatalf("generateComposeOverride with empty plugin failed: %v", err)
	}
	data2, _ := os.ReadFile(path1)

	// Both should produce equivalent output (no extra environment/volumes sections).
	if string(data1) != string(data2) {
		t.Errorf("nil and empty plugin response should produce same output.\nnil:\n%s\nempty:\n%s", data1, data2)
	}
}

func TestGenerateComposeOverride_PluginVolumeMountsGetNameDeclaration(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	pluginResp := &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{
			{Type: "volume", Source: "crib-cache-test-ws-npm", Target: "/home/vscode/.npm"},
			{Type: "bind", Source: "/host/history", Target: "/home/vscode/.crib_history"},
		},
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", pluginResp)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	// Top-level volume declaration must include name: to prevent compose from
	// prefixing with the project name.
	if !strings.Contains(content, "  crib-cache-test-ws-npm:\n    name: crib-cache-test-ws-npm\n") {
		t.Errorf("expected top-level volume with name: declaration, got:\n%s", content)
	}

	// Bind mounts should not appear in top-level volumes.
	if strings.Contains(content, "/host/history") && strings.Contains(content, "name: /host/history") {
		t.Errorf("bind mount should not get a top-level volume declaration")
	}
}

func TestGenerateComposeOverride_IncludesFeatureImage(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "crib-test-ws:features", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "image: crib-test-ws:features") {
		t.Errorf("expected feature image in override, got:\n%s", content)
	}
}

func TestGenerateComposeOverride_OmitsImageWhenEmpty(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}

	if strings.Contains(string(data), "image:") {
		t.Errorf("expected no image line when featureImage is empty, got:\n%s", data)
	}
}

func TestGenerateComposeOverride_NoBuildSection(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	// Override must NOT include a build: section for image-only services.
	// Adding build: to the override makes Docker Compose attempt a build
	// even when the service has no Dockerfile.
	if strings.Contains(content, "build:") {
		t.Errorf("override should not contain build section for image-only service, got:\n%s", content)
	}

	// Container labels should still be present.
	if !strings.Contains(content, "crib.workspace") {
		t.Errorf("expected container label, got:\n%s", content)
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
	user := eng.resolveComposeUser(context.Background(), cfg, nil)
	if user != "" {
		t.Errorf("expected empty user when config has remoteUser, got %q", user)
	}

	cfg.RemoteUser = ""
	cfg.ContainerUser = "devuser"
	user = eng.resolveComposeUser(context.Background(), cfg, nil)
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

func TestGenerateComposeOverride_FeatureCapabilities(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	priv := true
	init := true
	metadata := []*config.ImageMetadata{
		{
			NonComposeBase: config.NonComposeBase{
				Privileged:  &priv,
				Init:        &init,
				CapAdd:      []string{"SYS_PTRACE", "NET_ADMIN"},
				SecurityOpt: []string{"seccomp=unconfined"},
			},
		},
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", nil, metadata...)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "privileged: true") {
		t.Errorf("expected privileged: true, got:\n%s", content)
	}
	if !strings.Contains(content, "init: true") {
		t.Errorf("expected init: true, got:\n%s", content)
	}
	if !strings.Contains(content, "SYS_PTRACE") {
		t.Errorf("expected SYS_PTRACE in cap_add, got:\n%s", content)
	}
	if !strings.Contains(content, "NET_ADMIN") {
		t.Errorf("expected NET_ADMIN in cap_add, got:\n%s", content)
	}
	if !strings.Contains(content, "seccomp=unconfined") {
		t.Errorf("expected seccomp=unconfined in security_opt, got:\n%s", content)
	}
}

func TestGenerateComposeOverride_FeatureEntrypointSetsCommandOnly(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	metadata := []*config.ImageMetadata{
		{
			ID:         "docker-in-docker",
			Entrypoint: "/usr/local/share/docker-init.sh",
		},
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "crib-test-ws:features", nil, metadata...)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	// With feature entrypoints: should NOT override entrypoint, only set command.
	if strings.Contains(content, "entrypoint:") {
		t.Errorf("should not override entrypoint when feature has entrypoint, got:\n%s", content)
	}
	if !strings.Contains(content, "command:") {
		t.Errorf("expected command section, got:\n%s", content)
	}
	if !strings.Contains(content, "/bin/sh") {
		t.Errorf("expected /bin/sh in command, got:\n%s", content)
	}
}

func TestGenerateComposeOverride_NoFeatureEntrypointSetsEntrypoint(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", nil)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	// Without feature entrypoints: should set entrypoint and command.
	if !strings.Contains(content, "entrypoint:") || !strings.Contains(content, "/bin/sh") {
		t.Errorf("expected entrypoint with /bin/sh, got:\n%s", content)
	}
}

func TestGenerateComposeOverride_FeatureMounts(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	metadata := []*config.ImageMetadata{
		{
			NonComposeBase: config.NonComposeBase{
				Mounts: []config.Mount{
					{Type: "volume", Source: "dind-var-lib-docker-${devcontainerId}", Target: "/var/lib/docker"},
				},
			},
		},
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", nil, metadata...)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	// Variable substitution should resolve ${devcontainerId}.
	if !strings.Contains(content, "source: dind-var-lib-docker-test-ws") || !strings.Contains(content, "target: /var/lib/docker") {
		t.Errorf("expected substituted feature mount, got:\n%s", content)
	}

	// Named volume should get a top-level declaration.
	if !strings.Contains(content, "dind-var-lib-docker-test-ws:") || !strings.Contains(content, "name: dind-var-lib-docker-test-ws") {
		t.Errorf("expected top-level named volume declaration, got:\n%s", content)
	}
}

func TestGenerateComposeOverride_FeatureEnv(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	metadata := []*config.ImageMetadata{
		{
			NonComposeBase: config.NonComposeBase{
				ContainerEnv: map[string]string{
					"DOCKER_HOST": "unix:///var/run/docker.sock",
					"WS_ID":       "${devcontainerId}",
				},
			},
		},
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", nil, metadata...)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "DOCKER_HOST:") {
		t.Errorf("expected DOCKER_HOST in environment, got:\n%s", content)
	}
	// Variable substitution should resolve ${devcontainerId} in env values.
	if !strings.Contains(content, "WS_ID: test-ws") {
		t.Errorf("expected substituted WS_ID value, got:\n%s", content)
	}
}

func TestGenerateComposeOverride_FeatureEnvMergedWithConfigAndPlugin(t *testing.T) {
	ws := &workspace.Workspace{ID: "test-ws", Source: "/tmp/project"}
	e := newComposeTestEngine(t, "docker", ws)

	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"
	cfg.ContainerEnv = map[string]string{"APP_ENV": "development"}

	pluginResp := &plugin.PreContainerRunResponse{
		Env: map[string]string{"HISTFILE": "/home/vscode/.crib_history/.shell_history"},
	}
	metadata := []*config.ImageMetadata{
		{
			NonComposeBase: config.NonComposeBase{
				ContainerEnv: map[string]string{"DOCKER_HOST": "unix:///var/run/docker.sock"},
			},
		},
	}

	path, err := e.generateComposeOverride(ws, cfg, "/workspaces/project", nil, "", pluginResp, metadata...)
	if err != nil {
		t.Fatalf("generateComposeOverride failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	content := string(data)

	// All three env sources should be present.
	if !strings.Contains(content, "APP_ENV:") {
		t.Errorf("expected APP_ENV from config, got:\n%s", content)
	}
	if !strings.Contains(content, "DOCKER_HOST:") {
		t.Errorf("expected DOCKER_HOST from feature, got:\n%s", content)
	}
	if !strings.Contains(content, "HISTFILE:") {
		t.Errorf("expected HISTFILE from plugin, got:\n%s", content)
	}
}

// Regression: feature containerEnv (e.g. PATH=/nvm/bin:${PATH}) is baked into
// the image via Dockerfile ENV. featureToMetadata must exclude it so
// collectFeatureOverrides doesn't include it in the compose environment section,
// which would override the image's correctly-expanded values.
func TestCollectFeatureOverrides_ExcludesFeatureContainerEnv(t *testing.T) {
	f := &feature.FeatureSet{
		Config: &feature.FeatureConfig{
			ID: "node",
			ContainerEnv: map[string]string{
				"PATH": "/usr/local/share/nvm/current/bin:${PATH}",
			},
			CapAdd: []string{"SYS_PTRACE"},
		},
	}

	metadata := []*config.ImageMetadata{featureToMetadata(f)}
	ov := collectFeatureOverrides(metadata, nil)

	if len(ov.Env) != 0 {
		t.Errorf("feature containerEnv should not appear in compose env, got %v", ov.Env)
	}
	if len(ov.CapAdd) != 1 || ov.CapAdd[0] != "SYS_PTRACE" {
		t.Errorf("expected SYS_PTRACE in CapAdd, got %v", ov.CapAdd)
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
