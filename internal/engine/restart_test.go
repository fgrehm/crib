package engine

import (
	"testing"

	"github.com/fgrehm/crib/internal/config"
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
