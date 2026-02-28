package engine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

// testPlugin returns a fixed response for testing.
type testPlugin struct {
	resp *plugin.PreContainerRunResponse
}

func (p *testPlugin) Name() string { return "test" }
func (p *testPlugin) PreContainerRun(_ context.Context, _ *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
	return p.resp, nil
}

func TestRunPreContainerRunPlugins_MergesIntoRunOpts(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-1", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	mgr := plugin.NewManager(slog.Default())
	mgr.Register(&testPlugin{
		resp: &plugin.PreContainerRunResponse{
			Mounts:  []config.Mount{{Type: "bind", Source: "/host/a", Target: "/container/a"}},
			Env:     map[string]string{"PLUGIN_VAR": "hello"},
			RunArgs: []string{"--network=host"},
		},
	})

	eng := &Engine{
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
	}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"

	runOpts := &driver.RunOptions{
		Image:  "ubuntu:22.04",
		Env:    []string{"EXISTING=yes"},
		Mounts: []config.Mount{{Type: "bind", Source: "/src", Target: "/dst"}},
	}

	resp, err := eng.runPreContainerRunPlugins(context.Background(), ws, cfg, runOpts, "ubuntu:22.04", "/workspaces/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mounts should be appended.
	if len(runOpts.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(runOpts.Mounts))
	}
	if runOpts.Mounts[1].Source != "/host/a" {
		t.Errorf("expected appended mount source /host/a, got %s", runOpts.Mounts[1].Source)
	}

	// Env should be appended.
	found := false
	for _, e := range runOpts.Env {
		if e == "PLUGIN_VAR=hello" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected PLUGIN_VAR=hello in env, got %v", runOpts.Env)
	}

	// ExtraArgs should be appended.
	if len(runOpts.ExtraArgs) != 1 || runOpts.ExtraArgs[0] != "--network=host" {
		t.Errorf("expected ExtraArgs [--network=host], got %v", runOpts.ExtraArgs)
	}

	// No copies, so resp should have empty copies.
	if len(resp.Copies) != 0 {
		t.Errorf("expected 0 copies, got %d", len(resp.Copies))
	}
}

func TestRunPreContainerRunPlugins_NilManager(t *testing.T) {
	eng := &Engine{
		logger: slog.Default(),
	}

	ws := &workspace.Workspace{ID: "ws-1"}
	cfg := &config.DevContainerConfig{}
	runOpts := &driver.RunOptions{}

	resp, err := eng.runPreContainerRunPlugins(context.Background(), ws, cfg, runOpts, "img", "/workspaces/project")
	if err != nil {
		t.Fatalf("unexpected error with nil plugins: %v", err)
	}

	// RunOpts should be unchanged.
	if len(runOpts.Mounts) != 0 || len(runOpts.Env) != 0 || len(runOpts.ExtraArgs) != 0 {
		t.Errorf("runOpts should be unchanged when plugins is nil")
	}
	if resp != nil {
		t.Errorf("expected nil response when plugins is nil")
	}
}

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

	eng.execPluginCopies(context.Background(), "ws-1", "container-123", copies)

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
	if !strings.Contains(cmdStr, "chmod 0600") {
		t.Errorf("expected chmod 0600 in command, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "chown vscode") {
		t.Errorf("expected chown vscode in command, got: %s", cmdStr)
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

	eng.execPluginCopies(context.Background(), "ws-1", "c-1", copies)

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

	eng.execPluginCopies(context.Background(), "ws-1", "c-1", copies)

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

	eng.execPluginCopies(context.Background(), "ws-1", "c-1", nil)

	if len(mockDrv.execCalls) != 0 {
		t.Errorf("expected 0 exec calls for empty copies, got %d", len(mockDrv.execCalls))
	}
}
