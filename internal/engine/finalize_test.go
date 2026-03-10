package engine

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestFinalize_FreshSetup_RunsPluginCopiesAndChown(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-fin-fresh", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	// Create a staged file for plugin copy testing.
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "creds.json")
	if err := os.WriteFile(srcFile, []byte(`{"token":"abc"}`), 0o600); err != nil {
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
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     "container-1",
		workspaceFolder: "/workspaces/project",
	}

	pluginResp := &plugin.PreContainerRunResponse{
		Copies: []plugin.FileCopy{
			{Source: srcFile, Target: "/home/vscode/.creds.json", Mode: "0600", User: "vscode"},
		},
		Mounts: []config.Mount{
			{Type: "volume", Source: "cache-vol", Target: "/home/vscode/.npm"},
			{Type: "bind", Source: "/host/path", Target: "/container/path"},
		},
	}

	result, err := eng.finalize(context.Background(), ws, cfg, finalizeOpts{
		cc:         cc,
		imageName:  "ubuntu:22.04",
		pluginResp: pluginResp,
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	if result.ContainerID != "container-1" {
		t.Errorf("ContainerID = %q, want container-1", result.ContainerID)
	}

	// Verify plugin copy was executed.
	foundCopy := false
	foundChown := false
	for _, call := range mockDrv.execCalls {
		cmdStr := strings.Join(call.cmd, " ")
		if strings.Contains(cmdStr, ".creds.json") && strings.Contains(cmdStr, "cat >") {
			foundCopy = true
		}
		if len(call.cmd) >= 3 && call.cmd[0] == "chown" && call.cmd[2] == "/home/vscode/.npm" {
			foundChown = true
		}
	}
	if !foundCopy {
		t.Error("plugin file copy not executed")
	}
	if !foundChown {
		t.Error("plugin volume chown not executed")
	}
}

func TestFinalize_FreshSetup_CallsSetupContainerAndCommitsSnapshot(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-fin-setup", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &snapshotUpMockDriver{
		snapshotImage: "", // no snapshot
		containerID:   "container-1",
	}

	eng := &Engine{
		driver:   mockDrv,
		store:    store,
		logger:   slog.Default(),
		stdout:   io.Discard,
		stderr:   io.Discard,
		progress: func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"
	cfg.OnCreateCommand = config.LifecycleHook{"": {"echo onCreate"}}

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     "container-1",
		workspaceFolder: "/workspaces/project",
	}

	_, err := eng.finalize(context.Background(), ws, cfg, finalizeOpts{
		cc:        cc,
		imageName: "ubuntu:22.04",
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// Verify the onCreate hook was executed (via setupContainer).
	foundOnCreate := false
	for _, call := range mockDrv.execCalls {
		cmdStr := strings.Join(call.cmd, " ")
		if strings.Contains(cmdStr, "echo onCreate") {
			foundOnCreate = true
		}
	}
	if !foundOnCreate {
		t.Error("onCreate hook should run in fresh setup path")
	}

	// Verify snapshot was committed.
	if mockDrv.commitCalls == 0 {
		t.Error("snapshot should be committed after fresh setup")
	}
}

func TestFinalize_FromSnapshot_RestoresStoredEnvAndRunsResumeHooks(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-fin-snap", Source: "/home/user/project"}
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
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"
	cfg.OnCreateCommand = config.LifecycleHook{"": {"echo onCreate"}}
	cfg.PostStartCommand = config.LifecycleHook{"": {"echo postStart"}}

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     "container-1",
		workspaceFolder: "/workspaces/project",
	}

	storedResult := &workspace.Result{
		ImageName:  "ubuntu:22.04",
		RemoteUser: "vscode",
		RemoteEnv: map[string]string{
			"PATH":      "/home/vscode/.bundle/bin:/usr/local/bin:/usr/bin",
			"RUBY_ROOT": "/home/vscode/.local/share/mise/installs/ruby/3.4.7",
		},
	}

	pluginResp := &plugin.PreContainerRunResponse{
		PathPrepend: []string{"/home/vscode/.bundle/bin"},
	}

	result, err := eng.finalize(context.Background(), ws, cfg, finalizeOpts{
		cc:           cc,
		imageName:    "ubuntu:22.04",
		pluginResp:   pluginResp,
		storedResult: storedResult,
		fromSnapshot: true,
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	if result.ContainerID != "container-1" {
		t.Errorf("ContainerID = %q, want container-1", result.ContainerID)
	}

	// Verify saved env includes probed values from stored result.
	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if saved.RemoteEnv["RUBY_ROOT"] != "/home/vscode/.local/share/mise/installs/ruby/3.4.7" {
		t.Errorf("RUBY_ROOT = %q, want stored value", saved.RemoteEnv["RUBY_ROOT"])
	}
	path := saved.RemoteEnv["PATH"]
	if !strings.Contains(path, "/home/vscode/.bundle/bin") {
		t.Errorf("PATH missing plugin .bundle/bin: %q", path)
	}
	if !strings.Contains(path, "/usr/local/bin") {
		t.Errorf("PATH missing stored /usr/local/bin: %q", path)
	}

	// Verify postStart hook ran but NOT onCreate.
	foundOnCreate := false
	foundPostStart := false
	for _, call := range mockDrv.execCalls {
		cmdStr := strings.Join(call.cmd, " ")
		if strings.Contains(cmdStr, "echo onCreate") {
			foundOnCreate = true
		}
		if strings.Contains(cmdStr, "echo postStart") {
			foundPostStart = true
		}
	}
	if foundOnCreate {
		t.Error("onCreate should NOT run in snapshot path")
	}
	if !foundPostStart {
		t.Error("postStart should run in snapshot path")
	}
}

func TestFinalize_FromSnapshot_SkipsSetupContainerAndSnapshot(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-fin-skip", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mockDrv := &snapshotUpMockDriver{
		containerID: "container-1",
	}
	eng := &Engine{
		driver:   mockDrv,
		store:    store,
		logger:   slog.Default(),
		stdout:   io.Discard,
		stderr:   io.Discard,
		progress: func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     "container-1",
		workspaceFolder: "/workspaces/project",
	}

	storedResult := &workspace.Result{
		ImageName:  "ubuntu:22.04",
		RemoteUser: "vscode",
		RemoteEnv:  map[string]string{"PATH": "/usr/bin"},
	}

	_, err := eng.finalize(context.Background(), ws, cfg, finalizeOpts{
		cc:           cc,
		imageName:    "ubuntu:22.04",
		storedResult: storedResult,
		fromSnapshot: true,
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// Snapshot should NOT be re-committed in the snapshot path.
	if mockDrv.commitCalls > 0 {
		t.Error("should not commit snapshot when using fromSnapshot path")
	}
}

func TestFinalize_SkipVolumeChown(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-fin-nochown", Source: "/home/user/project"}
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
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     "container-1",
		workspaceFolder: "/workspaces/project",
	}

	pluginResp := &plugin.PreContainerRunResponse{
		Mounts: []config.Mount{
			{Type: "volume", Source: "cache-vol", Target: "/home/vscode/.npm"},
		},
	}

	storedResult := &workspace.Result{
		ImageName:  "ubuntu:22.04",
		RemoteUser: "vscode",
		RemoteEnv:  map[string]string{"PATH": "/usr/bin"},
	}

	_, err := eng.finalize(context.Background(), ws, cfg, finalizeOpts{
		cc:              cc,
		imageName:       "ubuntu:22.04",
		pluginResp:      pluginResp,
		storedResult:    storedResult,
		fromSnapshot:    true,
		skipVolumeChown: true,
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// Verify chown was NOT called when skipVolumeChown is true.
	for _, call := range mockDrv.execCalls {
		if len(call.cmd) >= 1 && call.cmd[0] == "chown" {
			t.Error("chown should not be called when skipVolumeChown is true")
		}
	}
}

func TestFinalize_EarlySave_BeforeLifecycleHooks(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-fin-early", Source: "/home/user/project"}
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
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"
	cfg.PostStartCommand = config.LifecycleHook{"": {"echo postStart"}}

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     "container-1",
		workspaceFolder: "/workspaces/project",
	}

	storedResult := &workspace.Result{
		ImageName:  "ubuntu:22.04",
		RemoteUser: "vscode",
		RemoteEnv:  map[string]string{"PATH": "/usr/bin"},
	}

	_, err := eng.finalize(context.Background(), ws, cfg, finalizeOpts{
		cc:             cc,
		imageName:      "ubuntu:22.04",
		hasEntrypoints: true,
		storedResult:   storedResult,
		fromSnapshot:   true,
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// Verify result was saved with full metadata (imageName, hasEntrypoints).
	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if saved.ImageName != "ubuntu:22.04" {
		t.Errorf("saved ImageName = %q, want ubuntu:22.04", saved.ImageName)
	}
	if !saved.HasFeatureEntrypoints {
		t.Error("saved HasFeatureEntrypoints should be true")
	}
}

func TestFinalize_PreservesPathPrepend_FreshSetup(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-fin-path-fresh", Source: "/home/user/project"}
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
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     "container-1",
		workspaceFolder: "/workspaces/project",
	}

	pluginResp := &plugin.PreContainerRunResponse{
		PathPrepend: []string{"/home/vscode/.bundle/bin"},
	}

	result, err := eng.finalize(context.Background(), ws, cfg, finalizeOpts{
		cc:         cc,
		imageName:  "ubuntu:22.04",
		pluginResp: pluginResp,
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if result.ContainerID != "container-1" {
		t.Errorf("ContainerID = %q, want container-1", result.ContainerID)
	}

	// cfg.RemoteEnv is mutated by setupContainer. The saved result should
	// contain the plugin PATH entry.
	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if saved.RemoteEnv == nil {
		t.Fatal("saved RemoteEnv is nil")
	}
	path := saved.RemoteEnv["PATH"]
	if !strings.Contains(path, "/home/vscode/.bundle/bin") {
		t.Errorf("PATH missing plugin .bundle/bin: %q", path)
	}
}

func TestFinalize_PreservesPathPrepend_FromSnapshot(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-fin-path-snap", Source: "/home/user/project"}
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
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     "container-1",
		workspaceFolder: "/workspaces/project",
	}

	pluginResp := &plugin.PreContainerRunResponse{
		PathPrepend: []string{"/home/vscode/.bundle/bin"},
	}

	storedResult := &workspace.Result{
		ImageName:  "ubuntu:22.04",
		RemoteUser: "vscode",
		RemoteEnv: map[string]string{
			"PATH":      "/usr/local/bin:/usr/bin",
			"RUBY_ROOT": "/some/path",
		},
	}

	result, err := eng.finalize(context.Background(), ws, cfg, finalizeOpts{
		cc:           cc,
		imageName:    "ubuntu:22.04",
		pluginResp:   pluginResp,
		storedResult: storedResult,
		fromSnapshot: true,
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if result.ContainerID != "container-1" {
		t.Errorf("ContainerID = %q, want container-1", result.ContainerID)
	}

	saved, err := store.LoadResult(ws.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	path := saved.RemoteEnv["PATH"]
	if !strings.Contains(path, "/home/vscode/.bundle/bin") {
		t.Errorf("PATH missing plugin .bundle/bin: %q", path)
	}
	if !strings.Contains(path, "/usr/local/bin") {
		t.Errorf("PATH missing stored /usr/local/bin: %q", path)
	}
}

func TestFinalize_RemoteUserSkippedWhenPreset(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-fin-user", Source: "/home/user/project"}
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
	}

	cfg := &config.DevContainerConfig{}
	// No remoteUser in config.

	cc := containerContext{
		workspaceID:     ws.ID,
		containerID:     "container-1",
		workspaceFolder: "/workspaces/project",
		remoteUser:      "preset-user", // pre-set from stored result
	}

	storedResult := &workspace.Result{
		ImageName:  "ubuntu:22.04",
		RemoteUser: "preset-user",
		RemoteEnv:  map[string]string{"PATH": "/usr/bin"},
	}

	result, err := eng.finalize(context.Background(), ws, cfg, finalizeOpts{
		cc:           cc,
		imageName:    "ubuntu:22.04",
		storedResult: storedResult,
		fromSnapshot: true,
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// Remote user should be the preset value, not re-resolved.
	if result.RemoteUser != "preset-user" {
		t.Errorf("RemoteUser = %q, want preset-user", result.RemoteUser)
	}

	// No whoami exec should have been called.
	for _, call := range mockDrv.execCalls {
		if len(call.cmd) >= 1 && call.cmd[0] == "whoami" {
			t.Error("whoami should not be called when remoteUser is preset")
		}
	}
}
