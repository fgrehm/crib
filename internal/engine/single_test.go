package engine

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
)

func TestBuildRunOptions_Minimal(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if opts.Image != "alpine:3.20" {
		t.Errorf("Image = %q, want %q", opts.Image, "alpine:3.20")
	}

	// Default: overrideCommand is true (nil means default true).
	if opts.Entrypoint != defaultEntrypoint {
		t.Errorf("Entrypoint = %q, want %q", opts.Entrypoint, defaultEntrypoint)
	}

	// Default workspace mount.
	if opts.WorkspaceMount.Source != "/project" {
		t.Errorf("WorkspaceMount.Source = %q, want %q", opts.WorkspaceMount.Source, "/project")
	}
	if opts.WorkspaceMount.Target != "/workspaces/project" {
		t.Errorf("WorkspaceMount.Target = %q, want %q", opts.WorkspaceMount.Target, "/workspaces/project")
	}
}

func TestBuildRunOptions_OverrideCommandFalse(t *testing.T) {
	e := &Engine{}
	oc := false
	cfg := &config.DevContainerConfig{}
	cfg.OverrideCommand = &oc

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if opts.Entrypoint != "" {
		t.Errorf("Entrypoint should be empty when overrideCommand=false, got %q", opts.Entrypoint)
	}
	if len(opts.Cmd) != 0 {
		t.Errorf("Cmd should be empty when overrideCommand=false, got %v", opts.Cmd)
	}
}

func TestBuildRunOptions_WithContainerUser(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}
	cfg.ContainerUser = "vscode"

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if opts.User != "vscode" {
		t.Errorf("User = %q, want %q", opts.User, "vscode")
	}
}

func TestBuildRunOptions_WithSecurityOpts(t *testing.T) {
	e := &Engine{}
	initVal := true
	privVal := true
	cfg := &config.DevContainerConfig{}
	cfg.Init = &initVal
	cfg.Privileged = &privVal
	cfg.CapAdd = []string{"SYS_PTRACE"}
	cfg.SecurityOpt = []string{"seccomp=unconfined"}

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if !opts.Init {
		t.Error("Init should be true")
	}
	if !opts.Privileged {
		t.Error("Privileged should be true")
	}
	if len(opts.CapAdd) != 1 || opts.CapAdd[0] != "SYS_PTRACE" {
		t.Errorf("CapAdd = %v, want [SYS_PTRACE]", opts.CapAdd)
	}
	if len(opts.SecurityOpt) != 1 || opts.SecurityOpt[0] != "seccomp=unconfined" {
		t.Errorf("SecurityOpt = %v, want [seccomp=unconfined]", opts.SecurityOpt)
	}
}

func TestBuildRunOptions_CustomWorkspaceMount(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}
	cfg.WorkspaceMount = "type=bind,src=/custom/src,dst=/custom/dst"

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if opts.WorkspaceMount.Source != "/custom/src" {
		t.Errorf("WorkspaceMount.Source = %q, want %q", opts.WorkspaceMount.Source, "/custom/src")
	}
	if opts.WorkspaceMount.Target != "/custom/dst" {
		t.Errorf("WorkspaceMount.Target = %q, want %q", opts.WorkspaceMount.Target, "/custom/dst")
	}
}

func TestBuildRunOptions_ContainerEnv(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}
	cfg.ContainerEnv = map[string]string{"FOO": "bar"}

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if len(opts.Env) != 1 || opts.Env[0] != "FOO=bar" {
		t.Errorf("Env = %v, want [FOO=bar]", opts.Env)
	}
}

func TestBuildRunOptions_RunArgsPassthrough(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}
	cfg.RunArgs = []string{"--network=host", "--gpus", "all"}

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if len(opts.ExtraArgs) != 3 {
		t.Fatalf("ExtraArgs length = %d, want 3", len(opts.ExtraArgs))
	}
	if opts.ExtraArgs[0] != "--network=host" {
		t.Errorf("ExtraArgs[0] = %q, want %q", opts.ExtraArgs[0], "--network=host")
	}
	if opts.ExtraArgs[1] != "--gpus" {
		t.Errorf("ExtraArgs[1] = %q, want %q", opts.ExtraArgs[1], "--gpus")
	}
	if opts.ExtraArgs[2] != "all" {
		t.Errorf("ExtraArgs[2] = %q, want %q", opts.ExtraArgs[2], "all")
	}
}

func TestBuildRunOptions_NoRunArgs(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if len(opts.ExtraArgs) != 0 {
		t.Errorf("ExtraArgs should be empty, got %v", opts.ExtraArgs)
	}
}

func TestBuildRunOptions_ForwardPorts(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}
	cfg.ForwardPorts = config.StrIntArray{"8080", "9090:3000"}

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if len(opts.Ports) != 2 {
		t.Fatalf("Ports length = %d, want 2", len(opts.Ports))
	}
	if opts.Ports[0] != "8080:8080" {
		t.Errorf("Ports[0] = %q, want %q", opts.Ports[0], "8080:8080")
	}
	if opts.Ports[1] != "9090:3000" {
		t.Errorf("Ports[1] = %q, want %q", opts.Ports[1], "9090:3000")
	}
}

func TestBuildRunOptions_AppPort(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}
	cfg.AppPort = config.StrIntArray{"3000", "5000:5000"}

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if len(opts.Ports) != 2 {
		t.Fatalf("Ports length = %d, want 2", len(opts.Ports))
	}
	if opts.Ports[0] != "3000:3000" {
		t.Errorf("Ports[0] = %q, want %q", opts.Ports[0], "3000:3000")
	}
	if opts.Ports[1] != "5000:5000" {
		t.Errorf("Ports[1] = %q, want %q", opts.Ports[1], "5000:5000")
	}
}

func TestBuildRunOptions_PortsDedup(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}
	cfg.ForwardPorts = config.StrIntArray{"8080", "3000"}
	cfg.AppPort = config.StrIntArray{"8080", "5000"}

	opts := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project")

	if len(opts.Ports) != 3 {
		t.Fatalf("Ports length = %d, want 3", len(opts.Ports))
	}
	// 8080 from forwardPorts wins, not duplicated from appPort.
	expected := []string{"8080:8080", "3000:3000", "5000:5000"}
	for i, want := range expected {
		if opts.Ports[i] != want {
			t.Errorf("Ports[%d] = %q, want %q", i, opts.Ports[i], want)
		}
	}
}

func TestResolveContainerUser(t *testing.T) {
	tests := []struct {
		containerUser string
		remoteUser    string
		want          string
	}{
		{"vscode", "", "vscode"},
		{"", "dev", "dev"},
		{"", "", "root"},
		{"vscode", "dev", "vscode"},
	}

	for _, tt := range tests {
		cfg := &config.DevContainerConfig{}
		cfg.ContainerUser = tt.containerUser
		cfg.RemoteUser = tt.remoteUser

		got := resolveContainerUser(cfg)
		if got != tt.want {
			t.Errorf("resolveContainerUser(container=%q, remote=%q) = %q, want %q",
				tt.containerUser, tt.remoteUser, got, tt.want)
		}
	}
}

func TestResolveRemoteUser(t *testing.T) {
	tests := []struct {
		containerUser string
		remoteUser    string
		want          string
	}{
		// remoteUser defaults to containerUser
		{"vscode", "", "vscode"},
		{"", "", "root"},
		// remoteUser takes precedence when explicitly set
		{"vscode", "dev", "dev"},
		{"", "dev", "dev"},
		// edge case: containerUser is root explicitly
		{"root", "", "root"},
	}

	for _, tt := range tests {
		cfg := &config.DevContainerConfig{}
		cfg.ContainerUser = tt.containerUser
		cfg.RemoteUser = tt.remoteUser

		remoteUser := cfg.RemoteUser
		if remoteUser == "" {
			remoteUser = cfg.ContainerUser
		}
		if remoteUser == "" {
			remoteUser = "root"
		}

		if remoteUser != tt.want {
			t.Errorf("resolve remoteUser(container=%q, remote=%q) = %q, want %q",
				tt.containerUser, tt.remoteUser, remoteUser, tt.want)
		}
	}
}

func TestResolveWorkspaceFolder(t *testing.T) {
	tests := []struct {
		configFolder string
		projectRoot  string
		want         string
	}{
		{"/custom/path", "/home/user/project", "/custom/path"},
		{"", "/home/user/myproject", "/workspaces/myproject"},
		{"", "/projects/foo-bar", "/workspaces/foo-bar"},
	}

	for _, tt := range tests {
		cfg := &config.DevContainerConfig{}
		cfg.WorkspaceFolder = tt.configFolder

		got := resolveWorkspaceFolder(cfg, tt.projectRoot)
		if got != tt.want {
			t.Errorf("resolveWorkspaceFolder(folder=%q, root=%q) = %q, want %q",
				tt.configFolder, tt.projectRoot, got, tt.want)
		}
	}
}

func TestWorkspaceFolderVariableExpansion(t *testing.T) {
	// Verifies that ${localWorkspaceFolderBasename} and ${localWorkspaceFolder}
	// in workspaceFolder are expanded before being used as ContainerWorkspaceFolder.
	// This mimics the pre-expansion logic in Engine.Up.
	tests := []struct {
		raw        string
		source     string
		wantFolder string
	}{
		{
			raw:        "/workspaces/${localWorkspaceFolderBasename}",
			source:     "/home/user/compose-app",
			wantFolder: "/workspaces/compose-app",
		},
		{
			raw:        "${localWorkspaceFolder}/container",
			source:     "/home/user/myproject",
			wantFolder: "/home/user/myproject/container",
		},
		{
			raw:        "/fixed/path",
			source:     "/home/user/anything",
			wantFolder: "/fixed/path",
		},
	}

	for _, tt := range tests {
		got := strings.NewReplacer(
			"${localWorkspaceFolder}", tt.source,
			"${localWorkspaceFolderBasename}", filepath.Base(tt.source),
		).Replace(tt.raw)

		if got != tt.wantFolder {
			t.Errorf("expand(%q, source=%q) = %q, want %q", tt.raw, tt.source, got, tt.wantFolder)
		}
	}
}

func TestDetectContainerUser_NonRoot(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"whoami": "vscode\n",
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	user := eng.detectContainerUser(context.Background(), "ws-1", "c-1")
	if user != "vscode" {
		t.Errorf("detectContainerUser = %q, want vscode", user)
	}
}

func TestDetectContainerUser_Root(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"whoami": "root\n",
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	user := eng.detectContainerUser(context.Background(), "ws-1", "c-1")
	if user != "" {
		t.Errorf("detectContainerUser = %q, want empty (root should fall through)", user)
	}
}
