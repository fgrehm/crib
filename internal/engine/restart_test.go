package engine

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestDetectConfigChange_NoChange(t *testing.T) {
	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	cfg.ContainerEnv = map[string]string{"FOO": "bar"}

	stored := *cfg
	if got := detectConfigChange(&stored, cfg); got != changeNone {
		t.Errorf("expected changeNone, got %d", got)
	}
}

func TestDetectConfigChange_ImageChanged(t *testing.T) {
	stored := &config.DevContainerConfig{}
	stored.Image = "ubuntu:22.04"

	current := &config.DevContainerConfig{}
	current.Image = "ubuntu:24.04"

	if got := detectConfigChange(stored, current); got != changeNeedsRebuild {
		t.Errorf("expected changeNeedsRebuild, got %d", got)
	}
}

func TestDetectConfigChange_DockerfileChanged(t *testing.T) {
	stored := &config.DevContainerConfig{}
	stored.Dockerfile = "Dockerfile"

	current := &config.DevContainerConfig{}
	current.Dockerfile = "Dockerfile.dev"

	if got := detectConfigChange(stored, current); got != changeNeedsRebuild {
		t.Errorf("expected changeNeedsRebuild, got %d", got)
	}
}

func TestDetectConfigChange_FeaturesChanged(t *testing.T) {
	stored := &config.DevContainerConfig{}
	stored.Features = map[string]any{"ghcr.io/devcontainers/features/go:1": map[string]any{}}

	current := &config.DevContainerConfig{}
	current.Features = map[string]any{"ghcr.io/devcontainers/features/node:1": map[string]any{}}

	if got := detectConfigChange(stored, current); got != changeNeedsRebuild {
		t.Errorf("expected changeNeedsRebuild, got %d", got)
	}
}

func TestDetectConfigChange_BuildArgsChanged(t *testing.T) {
	v1 := "1"
	v2 := "2"
	stored := &config.DevContainerConfig{}
	stored.Build = &config.ConfigBuildOptions{Args: map[string]*string{"VERSION": &v1}}

	current := &config.DevContainerConfig{}
	current.Build = &config.ConfigBuildOptions{Args: map[string]*string{"VERSION": &v2}}

	if got := detectConfigChange(stored, current); got != changeNeedsRebuild {
		t.Errorf("expected changeNeedsRebuild, got %d", got)
	}
}

func TestDetectConfigChange_EnvChanged(t *testing.T) {
	stored := &config.DevContainerConfig{}
	stored.ContainerEnv = map[string]string{"FOO": "bar"}

	current := &config.DevContainerConfig{}
	current.ContainerEnv = map[string]string{"FOO": "baz"}

	if got := detectConfigChange(stored, current); got != changeSafe {
		t.Errorf("expected changeSafe, got %d", got)
	}
}

func TestDetectConfigChange_MountsChanged(t *testing.T) {
	stored := &config.DevContainerConfig{}
	stored.Mounts = []config.Mount{{Type: "bind", Source: "/a", Target: "/b"}}

	current := &config.DevContainerConfig{}
	current.Mounts = []config.Mount{{Type: "bind", Source: "/a", Target: "/c"}}

	if got := detectConfigChange(stored, current); got != changeSafe {
		t.Errorf("expected changeSafe, got %d", got)
	}
}

func TestDetectConfigChange_RunArgsChanged(t *testing.T) {
	stored := &config.DevContainerConfig{}
	stored.RunArgs = []string{"--network=host"}

	current := &config.DevContainerConfig{}
	current.RunArgs = []string{"--network=bridge"}

	if got := detectConfigChange(stored, current); got != changeSafe {
		t.Errorf("expected changeSafe, got %d", got)
	}
}

func TestDetectConfigChange_RemoteUserChanged(t *testing.T) {
	stored := &config.DevContainerConfig{}
	stored.RemoteUser = "vscode"

	current := &config.DevContainerConfig{}
	current.RemoteUser = "developer"

	if got := detectConfigChange(stored, current); got != changeSafe {
		t.Errorf("expected changeSafe, got %d", got)
	}
}

func TestDetectConfigChange_PrivilegedChanged(t *testing.T) {
	f := false
	tr := true
	stored := &config.DevContainerConfig{}
	stored.Privileged = &f

	current := &config.DevContainerConfig{}
	current.Privileged = &tr

	if got := detectConfigChange(stored, current); got != changeSafe {
		t.Errorf("expected changeSafe, got %d", got)
	}
}

func TestDetectConfigChange_WorkspaceMountChanged(t *testing.T) {
	stored := &config.DevContainerConfig{}
	stored.WorkspaceMount = "type=bind,src=/old,dst=/workspace"

	current := &config.DevContainerConfig{}
	current.WorkspaceMount = "type=bind,src=/new,dst=/workspace"

	if got := detectConfigChange(stored, current); got != changeSafe {
		t.Errorf("expected changeSafe, got %d", got)
	}
}

func TestDetectConfigChange_ComposeServiceChanged(t *testing.T) {
	stored := &config.DevContainerConfig{}
	stored.Service = "app"

	current := &config.DevContainerConfig{}
	current.Service = "web"

	if got := detectConfigChange(stored, current); got != changeSafe {
		t.Errorf("expected changeSafe, got %d", got)
	}
}

// restartMockDriver extends mockDriver with stateful behavior needed for
// the restartRecreateSingle flow (container appears after RunContainer).
type restartMockDriver struct {
	mu            sync.Mutex
	execCalls     []mockExecCall
	runCalls      []*driver.RunOptions
	containerUp   bool // set to true after RunContainer
	findCallCount int
}

func (m *restartMockDriver) FindContainer(_ context.Context, _ string) (*driver.ContainerDetails, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.findCallCount++
	if m.findCallCount == 1 {
		// First call: existing container to delete.
		return &driver.ContainerDetails{ID: "old-container", State: driver.ContainerState{Status: "running"}}, nil
	}
	if m.containerUp {
		return &driver.ContainerDetails{ID: "new-container", State: driver.ContainerState{Status: "running"}}, nil
	}
	return nil, nil
}

func (m *restartMockDriver) RunContainer(_ context.Context, _ string, opts *driver.RunOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runCalls = append(m.runCalls, opts)
	m.containerUp = true
	return nil
}

func (m *restartMockDriver) DeleteContainer(_ context.Context, _, _ string) error { return nil }
func (m *restartMockDriver) StartContainer(_ context.Context, _, _ string) error  { return nil }
func (m *restartMockDriver) StopContainer(_ context.Context, _, _ string) error   { return nil }
func (m *restartMockDriver) RestartContainer(_ context.Context, _, _ string) error {
	return nil
}

func (m *restartMockDriver) ExecContainer(_ context.Context, _, _ string, cmd []string, _ io.Reader, _ io.Writer, _ io.Writer, env []string, _ string) error {
	m.mu.Lock()
	m.execCalls = append(m.execCalls, mockExecCall{cmd: cmd, env: env})
	m.mu.Unlock()
	return nil
}

func (m *restartMockDriver) ContainerLogs(_ context.Context, _, _ string, _, _ io.Writer) error {
	return nil
}
func (m *restartMockDriver) BuildImage(_ context.Context, _ string, _ *driver.BuildOptions) error {
	return nil
}
func (m *restartMockDriver) InspectImage(_ context.Context, _ string) (*driver.ImageDetails, error) {
	return nil, nil
}
func (m *restartMockDriver) TargetArchitecture(_ context.Context) (string, error) {
	return "amd64", nil
}

func TestRestartRecreateSingle_RunsPlugins(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-restart", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	// Create a staged file for plugin copy testing.
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "creds.json")
	if err := os.WriteFile(srcFile, []byte(`{"token":"abc"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr := plugin.NewManager(slog.Default())
	mgr.Register(&testPlugin{
		resp: &plugin.PreContainerRunResponse{
			Mounts:  []config.Mount{{Type: "bind", Source: "/host/ssh", Target: "/container/ssh"}},
			Env:     map[string]string{"SSH_AUTH_SOCK": "/tmp/ssh.sock"},
			RunArgs: []string{"--network=host"},
			Copies: []plugin.FileCopy{
				{Source: srcFile, Target: "/home/vscode/.creds.json", Mode: "0600", User: "vscode"},
			},
		},
	})

	mockDrv := &restartMockDriver{}
	eng := &Engine{
		driver:      mockDrv,
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	cfg.RemoteUser = "vscode"

	storedResult := &workspace.Result{
		ContainerID: "old-container",
		ImageName:   "ubuntu:22.04",
	}

	result, err := eng.restartRecreateSingle(context.Background(), ws, cfg, "/workspaces/project", storedResult)
	if err != nil {
		t.Fatalf("restartRecreateSingle: %v", err)
	}

	if result.ContainerID != "new-container" {
		t.Errorf("ContainerID = %q, want new-container", result.ContainerID)
	}

	// Verify RunContainer received plugin-injected options.
	if len(mockDrv.runCalls) != 1 {
		t.Fatalf("expected 1 RunContainer call, got %d", len(mockDrv.runCalls))
	}
	runOpts := mockDrv.runCalls[0]

	// Check plugin mount was merged.
	foundMount := false
	for _, m := range runOpts.Mounts {
		if m.Source == "/host/ssh" && m.Target == "/container/ssh" {
			foundMount = true
		}
	}
	if !foundMount {
		t.Errorf("plugin mount not found in RunOptions.Mounts: %v", runOpts.Mounts)
	}

	// Check plugin env was merged.
	foundEnv := false
	for _, e := range runOpts.Env {
		if e == "SSH_AUTH_SOCK=/tmp/ssh.sock" {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Errorf("plugin env SSH_AUTH_SOCK not found in RunOptions.Env: %v", runOpts.Env)
	}

	// Check plugin runArgs were merged.
	if len(runOpts.ExtraArgs) == 0 || runOpts.ExtraArgs[len(runOpts.ExtraArgs)-1] != "--network=host" {
		t.Errorf("plugin runArgs not found in RunOptions.ExtraArgs: %v", runOpts.ExtraArgs)
	}

	// Check plugin file copy was executed via exec.
	foundCopy := false
	for _, call := range mockDrv.execCalls {
		cmdStr := strings.Join(call.cmd, " ")
		if strings.Contains(cmdStr, ".creds.json") && strings.Contains(cmdStr, "cat >") {
			foundCopy = true
		}
	}
	if !foundCopy {
		t.Error("plugin file copy not executed after container recreation")
	}
}

func TestRestartRecreateSingle_NoPlugins(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-restart-np", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &restartMockDriver{}
	eng := &Engine{
		driver:   mockDrv,
		store:    store,
		logger:   slog.Default(),
		stdout:   io.Discard,
		stderr:   io.Discard,
		progress: func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	cfg.RemoteUser = "vscode"

	storedResult := &workspace.Result{
		ContainerID: "old-container",
		ImageName:   "ubuntu:22.04",
	}

	result, err := eng.restartRecreateSingle(context.Background(), ws, cfg, "/workspaces/project", storedResult)
	if err != nil {
		t.Fatalf("restartRecreateSingle: %v", err)
	}

	if result.ContainerID != "new-container" {
		t.Errorf("ContainerID = %q, want new-container", result.ContainerID)
	}

	// With no plugins, RunContainer should still be called but with no plugin injections.
	if len(mockDrv.runCalls) != 1 {
		t.Fatalf("expected 1 RunContainer call, got %d", len(mockDrv.runCalls))
	}
}

func TestRunResumeHooks_PropagatesVerbose(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-verbose"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &mockDriver{responses: map[string]string{}}
	eng := &Engine{
		driver:   mockDrv,
		store:    store,
		logger:   slog.Default(),
		stdout:   io.Discard,
		stderr:   io.Discard,
		progress: func(string) {},
		verbose:  true,
	}

	cfg := &config.DevContainerConfig{}
	cfg.PostStartCommand = config.LifecycleHook{"": {"echo hello"}}

	// runResumeHooks should not panic and should execute the hook.
	// The key thing we're testing is that the verbose field is set on the
	// lifecycleRunner (previously it was missing, causing --verbose to not
	// print hook commands during restart).
	err := eng.runResumeHooks(context.Background(), ws, cfg, "container-1", "/workspaces/project", "vscode")
	if err != nil {
		t.Fatalf("runResumeHooks: %v", err)
	}

	// Verify the hook was executed.
	found := false
	for _, call := range mockDrv.execCalls {
		cmdStr := strings.Join(call.cmd, " ")
		if strings.Contains(cmdStr, "echo hello") {
			found = true
		}
	}
	if !found {
		t.Error("postStartCommand was not executed during runResumeHooks")
	}
}
