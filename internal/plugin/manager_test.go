package plugin

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/fgrehm/crib/internal/config"
)

// stubPlugin is a test helper that implements Plugin with configurable behavior.
type stubPlugin struct {
	name         string
	resp         *PreContainerRunResponse
	err          error
	postCreateFn func(context.Context, *PostContainerCreateRequest) (*PostContainerCreateResponse, error)
}

func (s *stubPlugin) Name() string { return s.name }
func (s *stubPlugin) PreContainerRun(_ context.Context, _ *PreContainerRunRequest) (*PreContainerRunResponse, error) {
	return s.resp, s.err
}
func (s *stubPlugin) PostContainerCreate(ctx context.Context, req *PostContainerCreateRequest) (*PostContainerCreateResponse, error) {
	if s.postCreateFn != nil {
		return s.postCreateFn(ctx, req)
	}
	return nil, nil
}

func testManager() *Manager {
	return NewManager(slog.Default())
}

func testRequest() *PreContainerRunRequest {
	return &PreContainerRunRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    "/tmp/workspaces/test-ws",
		SourceDir:       "/home/user/project",
		Runtime:         "docker",
		ImageName:       "ubuntu:22.04",
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspaces/project",
		ContainerName:   "crib-test-ws",
	}
}

func TestRunPreContainerRun_SinglePlugin(t *testing.T) {
	mgr := testManager()
	mgr.Register(&stubPlugin{
		name: "test-plugin",
		resp: &PreContainerRunResponse{
			Mounts:  []config.Mount{{Type: "bind", Source: "/host/path", Target: "/container/path"}},
			Env:     map[string]string{"FOO": "bar"},
			RunArgs: []string{"--network=host"},
		},
	})

	resp, err := mgr.RunPreContainerRun(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(resp.Mounts))
	}
	if resp.Mounts[0].Source != "/host/path" {
		t.Errorf("expected mount source /host/path, got %s", resp.Mounts[0].Source)
	}
	if resp.Env["FOO"] != "bar" {
		t.Errorf("expected env FOO=bar, got %s", resp.Env["FOO"])
	}
	if len(resp.RunArgs) != 1 || resp.RunArgs[0] != "--network=host" {
		t.Errorf("expected runArgs [--network=host], got %v", resp.RunArgs)
	}
}

func TestRunPreContainerRun_MultiplePlugins(t *testing.T) {
	mgr := testManager()
	mgr.Register(&stubPlugin{
		name: "plugin-a",
		resp: &PreContainerRunResponse{
			Mounts:  []config.Mount{{Type: "bind", Source: "/a", Target: "/mnt/a"}},
			Env:     map[string]string{"SHARED": "from-a", "ONLY_A": "yes"},
			RunArgs: []string{"--cap-add=SYS_PTRACE"},
		},
	})
	mgr.Register(&stubPlugin{
		name: "plugin-b",
		resp: &PreContainerRunResponse{
			Mounts:  []config.Mount{{Type: "bind", Source: "/b", Target: "/mnt/b"}},
			Env:     map[string]string{"SHARED": "from-b", "ONLY_B": "yes"},
			RunArgs: []string{"--security-opt=seccomp=unconfined"},
		},
	})

	resp, err := mgr.RunPreContainerRun(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mounts appended in plugin order.
	if len(resp.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(resp.Mounts))
	}
	if resp.Mounts[0].Source != "/a" || resp.Mounts[1].Source != "/b" {
		t.Errorf("mounts not in expected order: %v", resp.Mounts)
	}

	// Env merged, last plugin wins on conflict.
	if resp.Env["SHARED"] != "from-b" {
		t.Errorf("expected SHARED=from-b (last wins), got %s", resp.Env["SHARED"])
	}
	if resp.Env["ONLY_A"] != "yes" {
		t.Errorf("expected ONLY_A=yes, got %s", resp.Env["ONLY_A"])
	}
	if resp.Env["ONLY_B"] != "yes" {
		t.Errorf("expected ONLY_B=yes, got %s", resp.Env["ONLY_B"])
	}

	// RunArgs appended in plugin order.
	if len(resp.RunArgs) != 2 {
		t.Fatalf("expected 2 runArgs, got %d", len(resp.RunArgs))
	}
	if resp.RunArgs[0] != "--cap-add=SYS_PTRACE" || resp.RunArgs[1] != "--security-opt=seccomp=unconfined" {
		t.Errorf("runArgs not in expected order: %v", resp.RunArgs)
	}
}

func TestRunPreContainerRun_PluginError_FailOpen(t *testing.T) {
	mgr := testManager()
	mgr.Register(&stubPlugin{
		name: "failing-plugin",
		err:  errors.New("something broke"),
	})
	mgr.Register(&stubPlugin{
		name: "good-plugin",
		resp: &PreContainerRunResponse{
			Env: map[string]string{"GOOD": "true"},
		},
	})

	resp, err := mgr.RunPreContainerRun(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("expected no error (fail-open), got: %v", err)
	}
	if resp.Env["GOOD"] != "true" {
		t.Errorf("expected GOOD=true from remaining plugin, got %s", resp.Env["GOOD"])
	}
}

func TestRunPreContainerRun_NilResponse(t *testing.T) {
	mgr := testManager()
	mgr.Register(&stubPlugin{
		name: "noop-plugin",
		resp: nil,
	})
	mgr.Register(&stubPlugin{
		name: "real-plugin",
		resp: &PreContainerRunResponse{
			Env: map[string]string{"KEY": "value"},
		},
	})

	resp, err := mgr.RunPreContainerRun(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Env["KEY"] != "value" {
		t.Errorf("expected KEY=value, got %s", resp.Env["KEY"])
	}
	if len(resp.Mounts) != 0 {
		t.Errorf("expected 0 mounts, got %d", len(resp.Mounts))
	}
}

func TestRunPreContainerRun_CopiesAppended(t *testing.T) {
	mgr := testManager()
	mgr.Register(&stubPlugin{
		name: "plugin-a",
		resp: &PreContainerRunResponse{
			Copies: []FileCopy{{Source: "/host/a", Target: "/container/a", Mode: "0600", User: "vscode"}},
		},
	})
	mgr.Register(&stubPlugin{
		name: "plugin-b",
		resp: &PreContainerRunResponse{
			Copies: []FileCopy{{Source: "/host/b", Target: "/container/b"}},
		},
	})

	resp, err := mgr.RunPreContainerRun(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Copies) != 2 {
		t.Fatalf("expected 2 copies, got %d", len(resp.Copies))
	}
	if resp.Copies[0].Source != "/host/a" || resp.Copies[1].Source != "/host/b" {
		t.Errorf("copies not in expected order: %v", resp.Copies)
	}
}

func TestRunPreContainerRun_EnvLastPluginWins(t *testing.T) {
	// a then b: b's value wins.
	mgr := testManager()
	mgr.Register(&stubPlugin{name: "plugin-a", resp: &PreContainerRunResponse{Env: map[string]string{"FOO": "a"}}})
	mgr.Register(&stubPlugin{name: "plugin-b", resp: &PreContainerRunResponse{Env: map[string]string{"FOO": "b"}}})

	resp, err := mgr.RunPreContainerRun(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Env["FOO"] != "b" {
		t.Errorf("expected FOO=b (last-plugin-wins), got %q", resp.Env["FOO"])
	}

	// b then a: a's value wins.
	mgr2 := testManager()
	mgr2.Register(&stubPlugin{name: "plugin-b", resp: &PreContainerRunResponse{Env: map[string]string{"FOO": "b"}}})
	mgr2.Register(&stubPlugin{name: "plugin-a", resp: &PreContainerRunResponse{Env: map[string]string{"FOO": "a"}}})

	resp2, err := mgr2.RunPreContainerRun(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp2.Env["FOO"] != "a" {
		t.Errorf("expected FOO=a (last-plugin-wins, reversed order), got %q", resp2.Env["FOO"])
	}
}

func TestRunPostContainerCreate_Dispatches(t *testing.T) {
	mgr := testManager()
	var called []string
	mgr.Register(&stubPlugin{
		name: "plugin-a",
		postCreateFn: func(_ context.Context, _ *PostContainerCreateRequest) (*PostContainerCreateResponse, error) {
			called = append(called, "a")
			return nil, nil
		},
	})
	mgr.Register(&stubPlugin{
		name: "plugin-b",
		postCreateFn: func(_ context.Context, _ *PostContainerCreateRequest) (*PostContainerCreateResponse, error) {
			called = append(called, "b")
			return nil, nil
		},
	})

	mgr.RunPostContainerCreate(context.Background(), &PostContainerCreateRequest{
		WorkspaceID: "test-ws",
		ContainerID: "abc123",
		RemoteUser:  "vscode",
	})

	if len(called) != 2 || called[0] != "a" || called[1] != "b" {
		t.Errorf("expected [a b], got %v", called)
	}
}

func TestRunPostContainerCreate_FailOpen(t *testing.T) {
	mgr := testManager()
	var called bool
	mgr.Register(&stubPlugin{
		name: "failing",
		postCreateFn: func(_ context.Context, _ *PostContainerCreateRequest) (*PostContainerCreateResponse, error) {
			return nil, errors.New("install failed")
		},
	})
	mgr.Register(&stubPlugin{
		name: "good",
		postCreateFn: func(_ context.Context, _ *PostContainerCreateRequest) (*PostContainerCreateResponse, error) {
			called = true
			return nil, nil
		},
	})

	mgr.RunPostContainerCreate(context.Background(), &PostContainerCreateRequest{
		WorkspaceID: "test-ws",
		ContainerID: "abc123",
	})

	if !called {
		t.Error("second plugin should still run after first fails")
	}
}

func TestRunPreContainerRun_NoPlugins(t *testing.T) {
	mgr := testManager()

	resp, err := mgr.RunPreContainerRun(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Mounts) != 0 {
		t.Errorf("expected 0 mounts, got %d", len(resp.Mounts))
	}
	if len(resp.Env) != 0 {
		t.Errorf("expected 0 env, got %d", len(resp.Env))
	}
	if len(resp.RunArgs) != 0 {
		t.Errorf("expected 0 runArgs, got %d", len(resp.RunArgs))
	}
	if len(resp.Copies) != 0 {
		t.Errorf("expected 0 copies, got %d", len(resp.Copies))
	}
}
