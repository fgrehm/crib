package engine

import (
	"context"
	"log/slog"
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
		Image: "ubuntu:22.04",
		Env:   []string{"EXISTING=yes"},
		Mounts: []config.Mount{{Type: "bind", Source: "/src", Target: "/dst"}},
	}

	err := eng.runPreContainerRunPlugins(context.Background(), ws, cfg, runOpts, "ubuntu:22.04", "/workspaces/project")
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
}

func TestRunPreContainerRunPlugins_NilManager(t *testing.T) {
	eng := &Engine{
		logger: slog.Default(),
	}

	ws := &workspace.Workspace{ID: "ws-1"}
	cfg := &config.DevContainerConfig{}
	runOpts := &driver.RunOptions{}

	err := eng.runPreContainerRunPlugins(context.Background(), ws, cfg, runOpts, "img", "/workspaces/project")
	if err != nil {
		t.Fatalf("unexpected error with nil plugins: %v", err)
	}

	// RunOpts should be unchanged.
	if len(runOpts.Mounts) != 0 || len(runOpts.Env) != 0 || len(runOpts.ExtraArgs) != 0 {
		t.Errorf("runOpts should be unchanged when plugins is nil")
	}
}
