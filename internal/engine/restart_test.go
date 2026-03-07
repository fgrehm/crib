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

func (m *restartMockDriver) ContainerLogs(_ context.Context, _, _ string, _, _ io.Writer, _ *driver.LogsOptions) error {
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
func (m *restartMockDriver) ListContainers(_ context.Context) ([]driver.ContainerDetails, error) {
	return nil, nil
}
func (m *restartMockDriver) CommitContainer(_ context.Context, _, _, _ string) error { return nil }
func (m *restartMockDriver) RemoveImage(_ context.Context, _ string) error           { return nil }
func (m *restartMockDriver) ListVolumes(_ context.Context, _ string) ([]driver.VolumeInfo, error) {
	return nil, nil
}
func (m *restartMockDriver) RemoveVolume(_ context.Context, _ string) error { return nil }

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

func TestRestartSimple_NonCompose_UsesStoredRemoteUser(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-restart-user", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	initialResult := &workspace.Result{
		ContainerID: "c-1",
		ImageName:   "ubuntu:22.04",
		RemoteUser:  "detected-user", // detected at Up() time, not in config
	}
	if err := store.SaveResult(ws.ID, initialResult); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "c-1",
			State: driver.ContainerState{Status: "running"},
		},
	}

	tp := &testPlugin{
		resp: &plugin.PreContainerRunResponse{
			PathPrepend: []string{"/home/detected-user/.local/bin"},
		},
	}
	mgr := plugin.NewManager(slog.Default())
	mgr.Register(tp)

	eng := &Engine{
		driver:      drv,
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	// Config has no RemoteUser — the stored result's RemoteUser should be used.
	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"

	_, err := eng.restartSimple(context.Background(), ws, cfg, "/workspaces/project", initialResult)
	if err != nil {
		t.Fatalf("restartSimple: %v", err)
	}

	if tp.req == nil {
		t.Fatal("plugin was not called")
	}
	if tp.req.RemoteUser != "detected-user" {
		t.Errorf("plugin received RemoteUser = %q, want %q", tp.req.RemoteUser, "detected-user")
	}
}

func TestRestartSimple_NonCompose_PreservesImageName(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-restart-img", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	initialResult := &workspace.Result{
		ContainerID: "c-1",
		ImageName:   "crib-ws-restart-img:features",
		RemoteUser:  "vscode",
	}
	if err := store.SaveResult(ws.ID, initialResult); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "c-1",
			State: driver.ContainerState{Status: "running"},
		},
	}

	eng := &Engine{
		driver:      drv,
		store:       store,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	cfg.RemoteUser = "vscode"

	_, err := eng.restartSimple(context.Background(), ws, cfg, "/workspaces/project", initialResult)
	if err != nil {
		t.Fatalf("restartSimple: %v", err)
	}

	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if saved.ImageName != "crib-ws-restart-img:features" {
		t.Errorf("ImageName = %q, want %q", saved.ImageName, "crib-ws-restart-img:features")
	}
}

func TestRestartSimple_NonCompose_PreservesPathPrepend(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-restart-path", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	// Save an initial result (restartSimple needs storedResult).
	initialResult := &workspace.Result{
		ContainerID: "c-1",
		ImageName:   "ruby:3.2",
		RemoteUser:  "vscode",
		RemoteEnv:   map[string]string{"PATH": "/home/vscode/.bundle/bin:/usr/local/bin:/usr/bin"},
	}
	if err := store.SaveResult(ws.ID, initialResult); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "c-1",
			State: driver.ContainerState{Status: "running"},
		},
	}

	mgr := plugin.NewManager(slog.Default())
	mgr.Register(&testPlugin{
		resp: &plugin.PreContainerRunResponse{
			PathPrepend: []string{"/home/vscode/.bundle/bin"},
		},
	})

	eng := &Engine{
		driver:      drv,
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ruby:3.2"
	cfg.RemoteUser = "vscode"

	result, err := eng.restartSimple(context.Background(), ws, cfg, "/workspaces/project", initialResult)
	if err != nil {
		t.Fatalf("restartSimple: %v", err)
	}
	if result.ContainerID != "c-1" {
		t.Errorf("ContainerID = %q, want c-1", result.ContainerID)
	}

	// Verify the saved RemoteEnv includes both the plugin PATH entry and
	// the probed PATH entries from the stored result.
	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if saved.RemoteEnv == nil {
		t.Fatal("saved RemoteEnv is nil, expected plugin PATH entries")
	}
	path := saved.RemoteEnv["PATH"]
	if !strings.Contains(path, "/home/vscode/.bundle/bin") {
		t.Errorf("saved PATH = %q, want to contain /home/vscode/.bundle/bin", path)
	}
	// The probed PATH from the stored result must survive the restart.
	if !strings.Contains(path, "/usr/local/bin") {
		t.Errorf("saved PATH = %q, want to contain probed /usr/local/bin", path)
	}
}

func TestRestartSimple_NonCompose_PreservesProbedEnv(t *testing.T) {
	// Regression test: after initial "crib up", setupContainer probes the
	// container env and saves it (including mise ruby/node PATH entries).
	// restartSimple skips setupContainer, so it must restore the probed env
	// from the stored result. Without this, "crib run -- ruby ..." fails
	// because the PATH only has plugin dirs, not the probed mise paths.

	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-restart-env", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	// Simulate the result from an initial "crib up" with probed env.
	initialResult := &workspace.Result{
		ContainerID: "c-1",
		ImageName:   "ruby:3.2",
		RemoteUser:  "vscode",
		RemoteEnv: map[string]string{
			"PATH":      "/home/vscode/.bundle/bin:/home/vscode/.local/share/mise/installs/ruby/3.4.7/bin:/home/vscode/.local/share/mise/shims:/usr/local/sbin:/usr/local/bin:/usr/bin",
			"RUBY_ROOT": "/home/vscode/.local/share/mise/installs/ruby/3.4.7",
			"GEM_HOME":  "/home/vscode/.local/share/mise/installs/ruby/3.4.7/lib/ruby/gems",
		},
	}
	if err := store.SaveResult(ws.ID, initialResult); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "c-1",
			State: driver.ContainerState{Status: "running"},
		},
	}

	mgr := plugin.NewManager(slog.Default())
	mgr.Register(&testPlugin{
		resp: &plugin.PreContainerRunResponse{
			PathPrepend: []string{"/home/vscode/.bundle/bin"},
		},
	})

	eng := &Engine{
		driver:      drv,
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ruby:3.2"
	cfg.RemoteUser = "vscode"

	_, err := eng.restartSimple(context.Background(), ws, cfg, "/workspaces/project", initialResult)
	if err != nil {
		t.Fatalf("restartSimple: %v", err)
	}

	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}

	// The probed mise ruby PATH must survive restart.
	path := saved.RemoteEnv["PATH"]
	if !strings.Contains(path, "/home/vscode/.local/share/mise/installs/ruby/3.4.7/bin") {
		t.Errorf("PATH missing mise ruby bin: %q", path)
	}
	if !strings.Contains(path, "/home/vscode/.local/share/mise/shims") {
		t.Errorf("PATH missing mise shims: %q", path)
	}

	// Plugin PathPrepend entry must still be present.
	if !strings.Contains(path, "/home/vscode/.bundle/bin") {
		t.Errorf("PATH missing plugin .bundle/bin: %q", path)
	}

	// Non-PATH probed env vars must also survive.
	if saved.RemoteEnv["RUBY_ROOT"] != "/home/vscode/.local/share/mise/installs/ruby/3.4.7" {
		t.Errorf("RUBY_ROOT = %q, want probed value", saved.RemoteEnv["RUBY_ROOT"])
	}
	if saved.RemoteEnv["GEM_HOME"] != "/home/vscode/.local/share/mise/installs/ruby/3.4.7/lib/ruby/gems" {
		t.Errorf("GEM_HOME = %q, want probed value", saved.RemoteEnv["GEM_HOME"])
	}
}

func TestRestartRecreateSingle_WithSnapshot_PreservesProbedEnv(t *testing.T) {
	// When restartRecreateSingle uses a snapshot (hasSnapshot=true), it
	// skips setupContainer. The probed env from the stored result must
	// survive via mergeStoredRemoteEnv.

	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-recreate-env", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	// Stored result with probed env and a valid snapshot.
	storedResult := &workspace.Result{
		ContainerID:      "old-container",
		ImageName:        "ruby:3.2",
		RemoteUser:       "vscode",
		SnapshotImage:    "crib-ws-recreate-env:snapshot",
		SnapshotHookHash: "44136fa355b3678a", // hash for empty hooks
		RemoteEnv: map[string]string{
			"PATH":      "/home/vscode/.bundle/bin:/home/vscode/.local/share/mise/installs/ruby/3.4.7/bin:/usr/local/bin:/usr/bin",
			"RUBY_ROOT": "/home/vscode/.local/share/mise/installs/ruby/3.4.7",
		},
	}
	if err := store.SaveResult(ws.ID, storedResult); err != nil {
		t.Fatal(err)
	}

	mockDrv := &restartMockDriver{}
	mgr := plugin.NewManager(slog.Default())
	mgr.Register(&testPlugin{
		resp: &plugin.PreContainerRunResponse{
			PathPrepend: []string{"/home/vscode/.bundle/bin"},
		},
	})

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
	cfg.Image = "ruby:3.2"
	cfg.RemoteUser = "vscode"

	result, err := eng.restartRecreateSingle(context.Background(), ws, cfg, "/workspaces/project", storedResult)
	if err != nil {
		t.Fatalf("restartRecreateSingle: %v", err)
	}
	if result.ContainerID == "" {
		t.Fatal("expected non-empty ContainerID")
	}

	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}

	path := saved.RemoteEnv["PATH"]
	if !strings.Contains(path, "/home/vscode/.local/share/mise/installs/ruby/3.4.7/bin") {
		t.Errorf("PATH missing mise ruby: %q", path)
	}
	if !strings.Contains(path, "/home/vscode/.bundle/bin") {
		t.Errorf("PATH missing plugin .bundle/bin: %q", path)
	}
	if saved.RemoteEnv["RUBY_ROOT"] != "/home/vscode/.local/share/mise/installs/ruby/3.4.7" {
		t.Errorf("RUBY_ROOT = %q, want probed value", saved.RemoteEnv["RUBY_ROOT"])
	}
}

func TestRestartSimple_NonCompose_ConfigEnvOverridesStored(t *testing.T) {
	// devcontainer.json remoteEnv values must take precedence over stored
	// values from a previous run. This ensures users can override probed
	// values by editing their devcontainer.json.

	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-restart-override", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	initialResult := &workspace.Result{
		ContainerID: "c-1",
		ImageName:   "ruby:3.2",
		RemoteUser:  "vscode",
		RemoteEnv: map[string]string{
			"EDITOR":    "vim",
			"RUBY_ROOT": "/home/vscode/.local/share/mise/installs/ruby/3.4.7",
			"PATH":      "/home/vscode/.local/share/mise/installs/ruby/3.4.7/bin:/usr/local/bin:/usr/bin",
		},
	}
	if err := store.SaveResult(ws.ID, initialResult); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "c-1",
			State: driver.ContainerState{Status: "running"},
		},
	}

	eng := &Engine{
		driver:      drv,
		store:       store,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ruby:3.2"
	cfg.RemoteUser = "vscode"
	// User overrides EDITOR in devcontainer.json.
	cfg.RemoteEnv = map[string]string{"EDITOR": "nano"}

	_, err := eng.restartSimple(context.Background(), ws, cfg, "/workspaces/project", initialResult)
	if err != nil {
		t.Fatalf("restartSimple: %v", err)
	}

	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}

	// devcontainer.json EDITOR=nano must win over stored EDITOR=vim.
	if saved.RemoteEnv["EDITOR"] != "nano" {
		t.Errorf("EDITOR = %q, want %q (config should override stored)", saved.RemoteEnv["EDITOR"], "nano")
	}
	// Stored RUBY_ROOT should still be present (no conflict).
	if saved.RemoteEnv["RUBY_ROOT"] != "/home/vscode/.local/share/mise/installs/ruby/3.4.7" {
		t.Errorf("RUBY_ROOT = %q, want stored value", saved.RemoteEnv["RUBY_ROOT"])
	}
	// Stored PATH should survive.
	if !strings.Contains(saved.RemoteEnv["PATH"], "/home/vscode/.local/share/mise/installs/ruby/3.4.7/bin") {
		t.Errorf("PATH missing stored mise ruby: %q", saved.RemoteEnv["PATH"])
	}
}

func TestRestartSimple_NonCompose_PluginEnvMerged(t *testing.T) {
	// Plugin Env values (e.g. BUNDLE_PATH, CARGO_HOME) must be included
	// in the saved RemoteEnv after a simple restart. Previously these
	// were silently dropped because only PathPrepend was extracted.

	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-restart-plugenv", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	initialResult := &workspace.Result{
		ContainerID: "c-1",
		ImageName:   "ruby:3.2",
		RemoteUser:  "vscode",
		RemoteEnv: map[string]string{
			"PATH":      "/home/vscode/.bundle/bin:/usr/local/bin:/usr/bin",
			"RUBY_ROOT": "/home/vscode/.local/share/mise/installs/ruby/3.4.7",
		},
	}
	if err := store.SaveResult(ws.ID, initialResult); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "c-1",
			State: driver.ContainerState{Status: "running"},
		},
	}

	mgr := plugin.NewManager(slog.Default())
	mgr.Register(&testPlugin{
		resp: &plugin.PreContainerRunResponse{
			PathPrepend: []string{"/home/vscode/.bundle/bin"},
			Env: map[string]string{
				"BUNDLE_PATH": "/home/vscode/.bundle",
				"HISTFILE":    "/home/vscode/.crib_history/.shell_history",
			},
		},
	})

	eng := &Engine{
		driver:      drv,
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ruby:3.2"
	cfg.RemoteUser = "vscode"

	_, err := eng.restartSimple(context.Background(), ws, cfg, "/workspaces/project", initialResult)
	if err != nil {
		t.Fatalf("restartSimple: %v", err)
	}

	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}

	// Plugin Env values must be present in saved RemoteEnv.
	if saved.RemoteEnv["BUNDLE_PATH"] != "/home/vscode/.bundle" {
		t.Errorf("BUNDLE_PATH = %q, want /home/vscode/.bundle", saved.RemoteEnv["BUNDLE_PATH"])
	}
	if saved.RemoteEnv["HISTFILE"] != "/home/vscode/.crib_history/.shell_history" {
		t.Errorf("HISTFILE = %q, want plugin value", saved.RemoteEnv["HISTFILE"])
	}
	// Stored probed vars should still survive.
	if saved.RemoteEnv["RUBY_ROOT"] != "/home/vscode/.local/share/mise/installs/ruby/3.4.7" {
		t.Errorf("RUBY_ROOT = %q, want stored value", saved.RemoteEnv["RUBY_ROOT"])
	}
	// PATH should include both plugin prepend and stored probed paths.
	path := saved.RemoteEnv["PATH"]
	if !strings.Contains(path, "/home/vscode/.bundle/bin") {
		t.Errorf("PATH missing plugin .bundle/bin: %q", path)
	}
	if !strings.Contains(path, "/usr/local/bin") {
		t.Errorf("PATH missing stored /usr/local/bin: %q", path)
	}
}

func TestRestartSimple_NonCompose_PluginEnvDoesNotOverrideConfig(t *testing.T) {
	// devcontainer.json remoteEnv must win over plugin Env values.

	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-restart-plugcfg", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	initialResult := &workspace.Result{
		ContainerID: "c-1",
		ImageName:   "ruby:3.2",
		RemoteUser:  "vscode",
		RemoteEnv: map[string]string{
			"PATH":   "/usr/local/bin:/usr/bin",
			"EDITOR": "vim",
		},
	}
	if err := store.SaveResult(ws.ID, initialResult); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "c-1",
			State: driver.ContainerState{Status: "running"},
		},
	}

	mgr := plugin.NewManager(slog.Default())
	mgr.Register(&testPlugin{
		resp: &plugin.PreContainerRunResponse{
			Env: map[string]string{
				"EDITOR": "code",
			},
		},
	})

	eng := &Engine{
		driver:      drv,
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ruby:3.2"
	cfg.RemoteUser = "vscode"
	cfg.RemoteEnv = map[string]string{"EDITOR": "nano"}

	_, err := eng.restartSimple(context.Background(), ws, cfg, "/workspaces/project", initialResult)
	if err != nil {
		t.Fatalf("restartSimple: %v", err)
	}

	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}

	// Config EDITOR=nano must win over plugin EDITOR=code.
	if saved.RemoteEnv["EDITOR"] != "nano" {
		t.Errorf("EDITOR = %q, want nano (config should override plugin)", saved.RemoteEnv["EDITOR"])
	}
}
