package engine

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestSingleBackend_PluginUser_ReturnsEmpty(t *testing.T) {
	b := &singleBackend{}
	if got := b.pluginUser(context.Background()); got != "" {
		t.Errorf("pluginUser() = %q, want empty", got)
	}
}

func TestSingleBackend_CanResumeFromStored_ReturnsFalse(t *testing.T) {
	b := &singleBackend{}
	if b.canResumeFromStored() {
		t.Error("canResumeFromStored() = true, want false")
	}
}

func TestSingleBackend_Start_CallsDriverStart(t *testing.T) {
	drv := &mockDriver{}
	eng := &Engine{driver: drv, logger: slog.Default(), progress: func(ProgressEvent) {}}

	b := &singleBackend{
		e:  eng,
		ws: &workspace.Workspace{ID: "ws-start"},
	}

	id, err := b.start(context.Background(), "container-1", nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if id != "container-1" {
		t.Errorf("start returned %q, want container-1", id)
	}
}

func TestSingleBackend_Restart_CallsDriverRestart(t *testing.T) {
	drv := &mockDriver{}
	eng := &Engine{driver: drv, logger: slog.Default(), progress: func(ProgressEvent) {}}

	b := &singleBackend{
		e:  eng,
		ws: &workspace.Workspace{ID: "ws-restart"},
	}

	id, err := b.restart(context.Background(), "container-1", nil)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if id != "container-1" {
		t.Errorf("restart returned %q, want container-1", id)
	}
}

func TestSingleBackend_CreateContainer_MergesPluginResponse(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-create", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &snapshotUpMockDriver{containerID: "new-container"}
	eng := &Engine{
		driver:   mockDrv,
		store:    store,
		logger:   slog.Default(),
		stdout:   io.Discard,
		stderr:   io.Discard,
		progress: func(ProgressEvent) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	cfg.RemoteUser = "vscode"

	b := &singleBackend{
		e:               eng,
		ws:              ws,
		cfg:             cfg,
		workspaceFolder: "/workspaces/project",
	}

	pluginResp := &plugin.PreContainerRunResponse{
		Mounts:  []config.Mount{{Type: "bind", Source: "/host/ssh", Target: "/container/ssh"}},
		Env:     map[string]string{"SSH_AUTH_SOCK": "/tmp/ssh.sock"},
		RunArgs: []string{"--network=host"},
	}

	containerID, err := b.createContainer(context.Background(), createOpts{
		imageName:  "ubuntu:22.04",
		pluginResp: pluginResp,
	})
	if err != nil {
		t.Fatalf("createContainer: %v", err)
	}
	if containerID != "new-container" {
		t.Errorf("containerID = %q, want new-container", containerID)
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
	foundArgs := false
	for _, a := range runOpts.ExtraArgs {
		if a == "--network=host" {
			foundArgs = true
		}
	}
	if !foundArgs {
		t.Errorf("plugin runArgs not found in RunOptions.ExtraArgs: %v", runOpts.ExtraArgs)
	}
}

func TestSingleBackend_CreateContainer_AppliesFeatureMetadata(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-feat-meta", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &snapshotUpMockDriver{containerID: "new-container"}
	eng := &Engine{
		driver:   mockDrv,
		store:    store,
		logger:   slog.Default(),
		stdout:   io.Discard,
		stderr:   io.Discard,
		progress: func(ProgressEvent) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"

	b := &singleBackend{
		e:               eng,
		ws:              ws,
		cfg:             cfg,
		workspaceFolder: "/workspaces/project",
	}

	trueVal := true
	metadata := []*config.ImageMetadata{
		{
			NonComposeBase: config.NonComposeBase{
				Privileged: &trueVal,
				CapAdd:     []string{"SYS_PTRACE"},
			},
		},
	}

	_, err := b.createContainer(context.Background(), createOpts{
		imageName: "ubuntu:22.04",
		metadata:  metadata,
	})
	if err != nil {
		t.Fatalf("createContainer: %v", err)
	}

	if len(mockDrv.runCalls) != 1 {
		t.Fatalf("expected 1 RunContainer call, got %d", len(mockDrv.runCalls))
	}
	runOpts := mockDrv.runCalls[0]

	if !runOpts.Privileged {
		t.Error("expected Privileged=true from feature metadata")
	}
	if len(runOpts.CapAdd) == 0 || runOpts.CapAdd[0] != "SYS_PTRACE" {
		t.Errorf("CapAdd = %v, want [SYS_PTRACE]", runOpts.CapAdd)
	}
}

func TestSingleBackend_DeleteExisting_NoContainer(t *testing.T) {
	// When no container exists, deleteExisting should not error.
	drv := &mockDriver{}
	eng := &Engine{driver: drv, logger: slog.Default()}

	b := &singleBackend{
		e:  eng,
		ws: &workspace.Workspace{ID: "ws-del"},
	}

	if err := b.deleteExisting(context.Background()); err != nil {
		t.Fatalf("deleteExisting: %v", err)
	}
}

func TestSingleBackend_DeleteExisting_WithContainer(t *testing.T) {
	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "container-to-delete",
			State: driver.ContainerState{Status: "running"},
		},
	}
	eng := &Engine{driver: drv, logger: slog.Default()}

	b := &singleBackend{
		e:  eng,
		ws: &workspace.Workspace{ID: "ws-del"},
	}

	if err := b.deleteExisting(context.Background()); err != nil {
		t.Fatalf("deleteExisting: %v", err)
	}
}

func TestSingleBackend_Start_IgnoresPluginResp(t *testing.T) {
	drv := &mockDriver{}
	eng := &Engine{driver: drv, logger: slog.Default(), progress: func(ProgressEvent) {}}

	b := &singleBackend{
		e:  eng,
		ws: &workspace.Workspace{ID: "ws-start-noplug"},
	}

	// Passing a plugin response should be ignored for single start.
	resp := &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{{Type: "bind", Source: "/a", Target: "/b"}},
	}

	id, err := b.start(context.Background(), "container-1", resp)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if id != "container-1" {
		t.Errorf("start returned %q, want container-1", id)
	}
}

func TestSingleBackend_CreateContainer_FeatureEntrypoints(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-feat-ep", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &snapshotUpMockDriver{containerID: "new-container"}
	eng := &Engine{
		driver:   mockDrv,
		store:    store,
		logger:   slog.Default(),
		stdout:   io.Discard,
		stderr:   io.Discard,
		progress: func(ProgressEvent) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"

	b := &singleBackend{
		e:               eng,
		ws:              ws,
		cfg:             cfg,
		workspaceFolder: "/workspaces/project",
	}

	containerID, err := b.createContainer(context.Background(), createOpts{
		imageName:      "ubuntu:22.04",
		hasEntrypoints: true,
	})
	if err != nil {
		t.Fatalf("createContainer: %v", err)
	}
	if containerID == "" {
		t.Fatal("expected non-empty container ID")
	}

	if len(mockDrv.runCalls) != 1 {
		t.Fatalf("expected 1 RunContainer call, got %d", len(mockDrv.runCalls))
	}
	runOpts := mockDrv.runCalls[0]

	// With feature entrypoints, Entrypoint should be empty.
	if runOpts.Entrypoint != "" {
		t.Errorf("Entrypoint = %q, want empty", runOpts.Entrypoint)
	}
	// Cmd should start with /bin/sh (full command for feature entrypoint).
	if len(runOpts.Cmd) == 0 || runOpts.Cmd[0] != "/bin/sh" {
		t.Errorf("Cmd = %v, want [/bin/sh ...]", runOpts.Cmd)
	}
}

func TestSingleBackend_Restart_IgnoresPluginResp(t *testing.T) {
	drv := &mockDriver{}
	eng := &Engine{driver: drv, logger: slog.Default(), progress: func(ProgressEvent) {}}

	b := &singleBackend{
		e:  eng,
		ws: &workspace.Workspace{ID: "ws-restart-noplug"},
	}

	resp := &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{{Type: "bind", Source: "/a", Target: "/b"}},
	}

	id, err := b.restart(context.Background(), "container-1", resp)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if id != "container-1" {
		t.Errorf("restart returned %q, want container-1", id)
	}
}

// Verify that the restartMockDriver's FindContainer returns the right
// container ID after RunContainer creates the container.
func TestSingleBackend_CreateContainer_FindsNewContainer(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-find-new", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &snapshotUpMockDriver{containerID: "new-container"}
	eng := &Engine{
		driver:   mockDrv,
		store:    store,
		logger:   slog.Default(),
		stdout:   io.Discard,
		stderr:   io.Discard,
		progress: func(ProgressEvent) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "alpine:3.20"

	b := &singleBackend{
		e:               eng,
		ws:              ws,
		cfg:             cfg,
		workspaceFolder: "/workspaces/project",
	}

	containerID, err := b.createContainer(context.Background(), createOpts{
		imageName: "alpine:3.20",
	})
	if err != nil {
		t.Fatalf("createContainer: %v", err)
	}

	// restartMockDriver returns "new-container" after RunContainer.
	if containerID != "new-container" {
		t.Errorf("containerID = %q, want new-container", containerID)
	}
}

// restartMockDriver has a quirk: the first FindContainer returns "old-container".
// For singleBackend.createContainer, we need the first FindContainer call to be
// the post-RunContainer lookup. Use a simpler approach with our own mock.
func TestSingleBackend_CreateContainer_ProgressReported(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-progress", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &snapshotUpMockDriver{containerID: "new-container"}
	var progressMessages []string
	eng := &Engine{
		driver:   mockDrv,
		store:    store,
		logger:   slog.Default(),
		stdout:   io.Discard,
		stderr:   io.Discard,
		progress: func(ev ProgressEvent) { progressMessages = append(progressMessages, ev.Message) },
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "alpine:3.20"

	b := &singleBackend{
		e:               eng,
		ws:              ws,
		cfg:             cfg,
		workspaceFolder: "/workspaces/project",
	}

	_, err := b.createContainer(context.Background(), createOpts{imageName: "alpine:3.20"})
	if err != nil {
		t.Fatalf("createContainer: %v", err)
	}

	found := false
	for _, msg := range progressMessages {
		if strings.Contains(msg, "Creating container") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected progress message about creating container, got: %v", progressMessages)
	}
}
