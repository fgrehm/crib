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
	name string
	resp *PreContainerRunResponse
	err  error
}

func (s *stubPlugin) Name() string { return s.name }
func (s *stubPlugin) PreContainerRun(_ context.Context, _ *PreContainerRunRequest) (*PreContainerRunResponse, error) {
	return s.resp, s.err
}

// stubPostCreatePlugin implements both Plugin and PostContainerCreator.
type stubPostCreatePlugin struct {
	stubPlugin
	postCreateCalled bool
	postCreateErr    error
}

func (s *stubPostCreatePlugin) PostContainerCreate(_ context.Context, _ *PostContainerCreateRequest) error {
	s.postCreateCalled = true
	return s.postCreateErr
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

func testPostCreateRequest() *PostContainerCreateRequest {
	return &PostContainerCreateRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    "/tmp/workspaces/test-ws",
		ContainerID:     "abc123",
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspaces/project",
		Runtime:         "docker",
		ExecFunc: func(_ context.Context, _ []string, _ string) error {
			return nil
		},
		CopyFileFunc: func(_ context.Context, _ []byte, _, _, _ string) error {
			return nil
		},
	}
}

func TestRunPostContainerCreate_DispatchesToImplementors(t *testing.T) {
	mgr := testManager()
	pcc := &stubPostCreatePlugin{stubPlugin: stubPlugin{name: "sandbox"}}
	plain := &stubPlugin{name: "plain"}
	mgr.Register(pcc)
	mgr.Register(plain)

	mgr.RunPostContainerCreate(context.Background(), testPostCreateRequest())

	if !pcc.postCreateCalled {
		t.Error("expected PostContainerCreate to be called on sandbox plugin")
	}
}

func TestRunPostContainerCreate_SkipsNonImplementors(t *testing.T) {
	mgr := testManager()
	plain := &stubPlugin{name: "plain"}
	mgr.Register(plain)

	// Should not panic or error.
	mgr.RunPostContainerCreate(context.Background(), testPostCreateRequest())
}

// stubPostCreateEnabledPlugin adds PostContainerCreateEnabler to stubPostCreatePlugin.
type stubPostCreateEnabledPlugin struct {
	stubPostCreatePlugin
	enabled bool
}

func (s *stubPostCreateEnabledPlugin) IsPostContainerCreateEnabled(_ *PostContainerCreateRequest) bool {
	return s.enabled
}

func TestRunPostContainerCreate_SkipsDisabledPlugins(t *testing.T) {
	mgr := testManager()
	disabled := &stubPostCreateEnabledPlugin{
		stubPostCreatePlugin: stubPostCreatePlugin{stubPlugin: stubPlugin{name: "disabled"}},
		enabled:              false,
	}
	enabled := &stubPostCreateEnabledPlugin{
		stubPostCreatePlugin: stubPostCreatePlugin{stubPlugin: stubPlugin{name: "enabled"}},
		enabled:              true,
	}
	mgr.Register(disabled)
	mgr.Register(enabled)

	mgr.RunPostContainerCreate(context.Background(), testPostCreateRequest())

	if disabled.postCreateCalled {
		t.Error("expected disabled plugin to be skipped")
	}
	if !enabled.postCreateCalled {
		t.Error("expected enabled plugin to be called")
	}
}

func TestRunPostContainerCreate_ErrorFailOpen(t *testing.T) {
	mgr := testManager()
	failing := &stubPostCreatePlugin{
		stubPlugin:    stubPlugin{name: "failing"},
		postCreateErr: errors.New("install failed"),
	}
	good := &stubPostCreatePlugin{
		stubPlugin: stubPlugin{name: "good"},
	}
	mgr.Register(failing)
	mgr.Register(good)

	mgr.RunPostContainerCreate(context.Background(), testPostCreateRequest())

	if !good.postCreateCalled {
		t.Error("expected good plugin to still run after failing plugin")
	}
}
