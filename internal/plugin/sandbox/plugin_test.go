package sandbox

import (
	"context"
	"testing"

	"github.com/fgrehm/crib/internal/plugin"
)

func testReq(customizations map[string]any) *plugin.PreContainerRunRequest {
	return &plugin.PreContainerRunRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    "/tmp/workspaces/test-ws",
		SourceDir:       "/home/user/project",
		Runtime:         "docker",
		ImageName:       "ubuntu:22.04",
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspaces/project",
		ContainerName:   "crib-test-ws",
		Customizations:  customizations,
	}
}

func TestName(t *testing.T) {
	p := New()
	if p.Name() != "sandbox" {
		t.Errorf("expected name sandbox, got %s", p.Name())
	}
}

func TestPreContainerRun_NoConfig_Noop(t *testing.T) {
	p := New()
	resp, err := p.PreContainerRun(context.Background(), testReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Error("expected nil response when no sandbox config")
	}
}

func TestPreContainerRun_EmptyConfig_NoRunArgs(t *testing.T) {
	p := New()
	resp, err := p.PreContainerRun(context.Background(), testReq(map[string]any{
		"sandbox": map[string]any{},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.RunArgs) != 0 {
		t.Errorf("expected no RunArgs when network blocking disabled, got %v", resp.RunArgs)
	}
}

func TestPreContainerRun_BlockLocalNetwork_AddsNetCaps(t *testing.T) {
	p := New()
	resp, err := p.PreContainerRun(context.Background(), testReq(map[string]any{
		"sandbox": map[string]any{
			"blockLocalNetwork": true,
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.RunArgs) != 1 {
		t.Fatalf("expected 1 RunArg, got %d: %v", len(resp.RunArgs), resp.RunArgs)
	}
	if resp.RunArgs[0] != "--cap-add=NET_ADMIN" {
		t.Errorf("unexpected RunArgs: %v", resp.RunArgs)
	}
}

func TestPreContainerRun_BlockCloudProviders_AddsNetCaps(t *testing.T) {
	p := New()
	resp, err := p.PreContainerRun(context.Background(), testReq(map[string]any{
		"sandbox": map[string]any{
			"blockCloudProviders": true,
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.RunArgs) != 1 {
		t.Fatalf("expected 1 RunArg, got %d", len(resp.RunArgs))
	}
}
