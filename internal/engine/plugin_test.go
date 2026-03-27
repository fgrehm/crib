package engine

import (
	"context"
	"fmt"
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

// testPlugin returns a fixed response for testing.
type testPlugin struct {
	plugin.BasePlugin
	resp *plugin.PreContainerRunResponse
	req  *plugin.PreContainerRunRequest // captured from last call
}

func (p *testPlugin) Name() string { return "test" }
func (p *testPlugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	p.req = req
	return p.resp, nil
}

// TestRunPreContainerRunPlugins_MergesIntoRunOpts is now covered by
// TestSingleBackend_CreateContainer_MergesPluginResponse in backend_single_test.go.

func TestDispatchPlugins_ReturnsResponseWithoutMerging(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-1", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	tp := &testPlugin{
		resp: &plugin.PreContainerRunResponse{
			Mounts:  []config.Mount{{Type: "bind", Source: "/host/a", Target: "/container/a"}},
			Env:     map[string]string{"PLUGIN_VAR": "hello"},
			RunArgs: []string{"--network=host"},
			Copies:  []plugin.FileCopy{{Source: "/tmp/src", Target: "/tmp/dst"}},
		},
	}
	mgr := plugin.NewManager(slog.Default())
	mgr.Register(tp)

	eng := &Engine{
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"

	resp, err := eng.dispatchPlugins(context.Background(), ws, cfg, "ubuntu:22.04", "/workspaces/project", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Response should contain all plugin data.
	if len(resp.Mounts) != 1 || resp.Mounts[0].Source != "/host/a" {
		t.Errorf("expected plugin mount, got %v", resp.Mounts)
	}
	if resp.Env["PLUGIN_VAR"] != "hello" {
		t.Errorf("expected plugin env, got %v", resp.Env)
	}
	if len(resp.Copies) != 1 {
		t.Errorf("expected 1 copy, got %d", len(resp.Copies))
	}
}

func TestDispatchPlugins_NilManager(t *testing.T) {
	eng := &Engine{logger: slog.Default()}
	ws := &workspace.Workspace{ID: "ws-1"}
	cfg := &config.DevContainerConfig{}

	resp, err := eng.dispatchPlugins(context.Background(), ws, cfg, "img", "/workspaces/project", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response when plugins is nil, got %v", resp)
	}
}

// TestRunPreContainerRunPlugins_* tests are now covered by:
// - TestSingleBackend_CreateContainer_* in backend_single_test.go (merge behavior)
// - TestDispatchPlugins_* in this file (dispatch behavior: nil manager, explicit user override)

func TestExecPluginCopies(t *testing.T) {
	// Create a staging file on "host".
	staging := t.TempDir()
	srcFile := filepath.Join(staging, "test.json")
	if err := os.WriteFile(srcFile, []byte(`{"key":"value"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mockDrv := &mockDriver{}
	eng := &Engine{
		driver: mockDrv,
		logger: slog.Default(),
	}

	copies := []plugin.FileCopy{
		{Source: srcFile, Target: "/home/vscode/.config/test.json", Mode: "0600", User: "vscode"},
	}

	eng.execPluginCopies(context.Background(), containerContext{workspaceID: "ws-1", containerID: "container-123"}, copies)

	// Should have made one exec call.
	if len(mockDrv.execCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(mockDrv.execCalls))
	}

	call := mockDrv.execCalls[0]
	// The command should create parent dir, write via cat, chmod, and chown.
	cmdStr := strings.Join(call.cmd, " ")
	if !strings.Contains(cmdStr, "mkdir -p") {
		t.Errorf("expected mkdir -p in command, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "/home/vscode/.config/test.json") {
		t.Errorf("expected target path in command, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "chmod '0600'") {
		t.Errorf("expected chmod '0600' in command, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "chown 'vscode' '/home/vscode/.config' '/home/vscode/.config/test.json'") {
		t.Errorf("expected chown of both dir and file in command, got: %s", cmdStr)
	}
}

func TestExecPluginCopies_NoMode(t *testing.T) {
	staging := t.TempDir()
	srcFile := filepath.Join(staging, "test.json")
	if err := os.WriteFile(srcFile, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mockDrv := &mockDriver{}
	eng := &Engine{
		driver: mockDrv,
		logger: slog.Default(),
	}

	copies := []plugin.FileCopy{
		{Source: srcFile, Target: "/home/vscode/.config.json", User: "vscode"},
	}

	eng.execPluginCopies(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1"}, copies)

	cmdStr := strings.Join(mockDrv.execCalls[0].cmd, " ")
	// No chmod when Mode is empty.
	if strings.Contains(cmdStr, "chmod") {
		t.Errorf("should not chmod when Mode is empty, got: %s", cmdStr)
	}
}

func TestExecPluginCopies_NoUser(t *testing.T) {
	staging := t.TempDir()
	srcFile := filepath.Join(staging, "test.json")
	if err := os.WriteFile(srcFile, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mockDrv := &mockDriver{}
	eng := &Engine{
		driver: mockDrv,
		logger: slog.Default(),
	}

	copies := []plugin.FileCopy{
		{Source: srcFile, Target: "/root/.config.json"},
	}

	eng.execPluginCopies(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1"}, copies)

	cmdStr := strings.Join(mockDrv.execCalls[0].cmd, " ")
	// No chown when User is empty.
	if strings.Contains(cmdStr, "chown") {
		t.Errorf("should not chown when User is empty, got: %s", cmdStr)
	}
}

func TestExecPluginCopies_Empty(t *testing.T) {
	mockDrv := &mockDriver{}
	eng := &Engine{
		driver: mockDrv,
		logger: slog.Default(),
	}

	eng.execPluginCopies(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1"}, nil)

	if len(mockDrv.execCalls) != 0 {
		t.Errorf("expected 0 exec calls for empty copies, got %d", len(mockDrv.execCalls))
	}
}

func TestExecPluginCopies_MissingSource(t *testing.T) {
	mockDrv := &mockDriver{}
	eng := &Engine{
		driver: mockDrv,
		logger: slog.Default(),
	}

	staging := t.TempDir()
	goodFile := filepath.Join(staging, "good.json")
	if err := os.WriteFile(goodFile, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	copies := []plugin.FileCopy{
		{Source: "/nonexistent/missing.json", Target: "/home/vscode/.config/missing.json"},
		{Source: goodFile, Target: "/home/vscode/.config/good.json"},
	}

	eng.execPluginCopies(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1"}, copies)

	// Missing source should be skipped, good file should still be copied.
	if len(mockDrv.execCalls) != 1 {
		t.Fatalf("expected 1 exec call (skipping missing source), got %d", len(mockDrv.execCalls))
	}
	cmdStr := strings.Join(mockDrv.execCalls[0].cmd, " ")
	if !strings.Contains(cmdStr, "good.json") {
		t.Errorf("expected good.json in command, got: %s", cmdStr)
	}
}

func TestExecPluginCopies_ExecFailure(t *testing.T) {
	staging := t.TempDir()
	file1 := filepath.Join(staging, "a.json")
	file2 := filepath.Join(staging, "b.json")
	if err := os.WriteFile(file1, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Make all exec calls fail.
	mockDrv := &mockDriver{
		errors: map[string]error{},
	}
	// We can't predict the exact command key, so use a failing driver wrapper.
	failDrv := &failingExecDriver{mockDriver: mockDrv}
	eng := &Engine{
		driver: failDrv,
		logger: slog.Default(),
	}

	copies := []plugin.FileCopy{
		{Source: file1, Target: "/home/vscode/a.json"},
		{Source: file2, Target: "/home/vscode/b.json"},
	}

	eng.execPluginCopies(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1"}, copies)

	// After first exec failure, remaining copies should be skipped.
	if failDrv.execCount != 1 {
		t.Errorf("expected 1 exec attempt (bail out after failure), got %d", failDrv.execCount)
	}
}

func TestExecPluginCopies_IfNotExists(t *testing.T) {
	staging := t.TempDir()
	srcFile := filepath.Join(staging, "config.json")
	if err := os.WriteFile(srcFile, []byte(`{"key":"value"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mockDrv := &mockDriver{}
	eng := &Engine{
		driver: mockDrv,
		logger: slog.Default(),
	}

	copies := []plugin.FileCopy{
		{Source: srcFile, Target: "/home/vscode/.config.json", IfNotExists: true},
	}

	eng.execPluginCopies(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1"}, copies)

	if len(mockDrv.execCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(mockDrv.execCalls))
	}
	cmdStr := strings.Join(mockDrv.execCalls[0].cmd, " ")
	// Command should be guarded with [ ! -f '...' ] || { ... }
	if !strings.Contains(cmdStr, "[ -f '/home/vscode/.config.json' ]") {
		t.Errorf("expected existence check in command, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "||") {
		t.Errorf("expected || guard in command, got: %s", cmdStr)
	}
	// Should still contain the write logic inside the guard.
	if !strings.Contains(cmdStr, "cat >") {
		t.Errorf("expected cat > in command, got: %s", cmdStr)
	}
}

func TestDispatchPlugins_ExplicitRemoteUserOverride(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-1", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	tp := &testPlugin{resp: &plugin.PreContainerRunResponse{}}
	mgr := plugin.NewManager(slog.Default())
	mgr.Register(tp)

	eng := &Engine{
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
	}

	// Config has no remoteUser/containerUser, but caller passes explicit user.
	cfg := &config.DevContainerConfig{}

	_, err := eng.dispatchPlugins(context.Background(), ws, cfg, "ubuntu:22.04", "/workspaces/project", "vscode")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tp.req.RemoteUser != "vscode" {
		t.Errorf("expected RemoteUser=vscode from explicit override, got %s", tp.req.RemoteUser)
	}
}

func TestDispatchPlugins_ExplicitRemoteUserOverridesConfig(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-1", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	tp := &testPlugin{resp: &plugin.PreContainerRunResponse{}}
	mgr := plugin.NewManager(slog.Default())
	mgr.Register(tp)

	eng := &Engine{
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
	}

	// Config has remoteUser set, but explicit override takes precedence.
	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "devuser"

	_, err := eng.dispatchPlugins(context.Background(), ws, cfg, "ubuntu:22.04", "/workspaces/project", "nodeuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tp.req.RemoteUser != "nodeuser" {
		t.Errorf("expected RemoteUser=nodeuser from explicit override, got %s", tp.req.RemoteUser)
	}
}

// failingExecDriver wraps mockDriver but makes ExecContainer always return an error.
type failingExecDriver struct {
	*mockDriver
	execCount int
}

func (f *failingExecDriver) ExecContainer(ctx context.Context, workspaceID, containerID string, cmd []string, stdin io.Reader, stdout, stderr io.Writer, env []string, user string) error {
	f.execCount++
	return fmt.Errorf("exec failed")
}

func TestExecPluginCopies_MissingSourceThenExecFailure(t *testing.T) {
	staging := t.TempDir()
	goodFile := filepath.Join(staging, "good.json")
	if err := os.WriteFile(goodFile, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	failDrv := &failingExecDriver{mockDriver: &mockDriver{}}
	eng := &Engine{
		driver: failDrv,
		logger: slog.Default(),
	}

	copies := []plugin.FileCopy{
		{Source: "/nonexistent/missing.json", Target: "/home/vscode/missing.json"},
		{Source: goodFile, Target: "/home/vscode/good.json"},
		{Source: goodFile, Target: "/home/vscode/other.json"},
	}

	eng.execPluginCopies(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1"}, copies)

	// Missing source is skipped (continue), good.json triggers exec which fails,
	// other.json should NOT be attempted (bail out after first exec failure).
	if failDrv.execCount != 1 {
		t.Errorf("expected 1 exec attempt (skip missing, bail after first exec fail), got %d", failDrv.execCount)
	}
}
