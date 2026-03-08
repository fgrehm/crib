package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestBuildRunOptions_Minimal(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

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

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

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

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

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

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

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

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

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

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

	if len(opts.Env) != 1 || opts.Env[0] != "FOO=bar" {
		t.Errorf("Env = %v, want [FOO=bar]", opts.Env)
	}
}

func TestBuildRunOptions_RunArgsPassthrough(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}
	cfg.RunArgs = []string{"--network=host", "--gpus", "all"}

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

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

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

	if len(opts.ExtraArgs) != 0 {
		t.Errorf("ExtraArgs should be empty, got %v", opts.ExtraArgs)
	}
}

func TestBuildRunOptions_ForwardPorts(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}
	cfg.ForwardPorts = config.StrIntArray{"8080", "9090:3000"}

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

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

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

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

	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", false)
	if err != nil {
		t.Fatal(err)
	}

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

func TestBuildRunOptions_FeatureEntrypoints(t *testing.T) {
	e := &Engine{}
	cfg := &config.DevContainerConfig{}

	// With feature entrypoints: should NOT override ENTRYPOINT, CMD is full command.
	opts, err := e.buildRunOptions(cfg, "alpine:3.20", "/project", "/workspaces/project", true)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Entrypoint != "" {
		t.Errorf("Entrypoint = %q, want empty (feature entrypoint baked into image)", opts.Entrypoint)
	}
	if len(opts.Cmd) == 0 || opts.Cmd[0] != "/bin/sh" {
		t.Errorf("Cmd[0] = %q, want /bin/sh (full command for feature entrypoint)", opts.Cmd[0])
	}
}

func TestApplyFeatureMetadata(t *testing.T) {
	opts := &driver.RunOptions{}
	metadata := []*config.ImageMetadata{
		{
			NonComposeBase: config.NonComposeBase{
				Privileged:   new(true),
				Init:         new(true),
				CapAdd:       []string{"SYS_PTRACE"},
				SecurityOpt:  []string{"seccomp=unconfined"},
				Mounts:       []config.Mount{{Type: "volume", Source: "data", Target: "/data"}},
				ContainerEnv: map[string]string{"FOO": "bar"},
			},
		},
	}
	applyFeatureMetadata(opts, metadata, nil)

	if !opts.Privileged {
		t.Error("Privileged should be true")
	}
	if !opts.Init {
		t.Error("Init should be true")
	}
	if len(opts.CapAdd) != 1 || opts.CapAdd[0] != "SYS_PTRACE" {
		t.Errorf("CapAdd = %v, want [SYS_PTRACE]", opts.CapAdd)
	}
	if len(opts.SecurityOpt) != 1 || opts.SecurityOpt[0] != "seccomp=unconfined" {
		t.Errorf("SecurityOpt = %v, want [seccomp=unconfined]", opts.SecurityOpt)
	}
	if len(opts.Mounts) != 1 || opts.Mounts[0].Target != "/data" {
		t.Errorf("Mounts = %v, want [{volume data /data}]", opts.Mounts)
	}
	if len(opts.Env) != 1 || opts.Env[0] != "FOO=bar" {
		t.Errorf("Env = %v, want [FOO=bar]", opts.Env)
	}
}

func TestApplyFeatureMetadata_Substitution(t *testing.T) {
	opts := &driver.RunOptions{}
	metadata := []*config.ImageMetadata{
		{
			NonComposeBase: config.NonComposeBase{
				Mounts: []config.Mount{
					{Type: "volume", Source: "dind-var-lib-docker-${devcontainerId}", Target: "/var/lib/docker"},
				},
				ContainerEnv: map[string]string{"WS": "${devcontainerId}"},
			},
		},
	}
	subCtx := &config.SubstitutionContext{
		DevContainerID:           "ws-abc",
		LocalWorkspaceFolder:     "/host/project",
		ContainerWorkspaceFolder: "/workspaces/project",
	}
	applyFeatureMetadata(opts, metadata, subCtx)

	if len(opts.Mounts) != 1 || opts.Mounts[0].Source != "dind-var-lib-docker-ws-abc" {
		t.Errorf("Mount source = %q, want %q", opts.Mounts[0].Source, "dind-var-lib-docker-ws-abc")
	}
	if len(opts.Env) != 1 || opts.Env[0] != "WS=ws-abc" {
		t.Errorf("Env = %v, want [WS=ws-abc]", opts.Env)
	}
}

func TestCollectPorts_Empty(t *testing.T) {
	got := collectPorts(nil, nil)
	if len(got) != 0 {
		t.Errorf("collectPorts(nil, nil) = %v, want empty", got)
	}
}

func TestCollectPorts_BareAndPair(t *testing.T) {
	got := collectPorts(config.StrIntArray{"8080", "9090:3000"}, nil)
	want := []string{"8080:8080", "9090:3000"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCollectPorts_DedupSameFormat(t *testing.T) {
	// "8080" normalizes to "8080:8080", same as explicit "8080:8080".
	got := collectPorts(
		config.StrIntArray{"8080"},
		config.StrIntArray{"8080:8080"},
	)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (should dedup)", len(got))
	}
	if got[0] != "8080:8080" {
		t.Errorf("got[0] = %q, want %q", got[0], "8080:8080")
	}
}

func TestCollectPorts_Range(t *testing.T) {
	got := collectPorts(config.StrIntArray{"8000-8010"}, nil)
	if len(got) != 1 || got[0] != "8000-8010:8000-8010" {
		t.Errorf("got = %v, want [\"8000-8010:8000-8010\"]", got)
	}
}

func TestCollectPorts_RangeWithHost(t *testing.T) {
	got := collectPorts(config.StrIntArray{"9000-9010:8000-8010"}, nil)
	if len(got) != 1 || got[0] != "9000-9010:8000-8010" {
		t.Errorf("got = %v, want [\"9000-9010:8000-8010\"]", got)
	}
}

func TestPortSpecToBindings(t *testing.T) {
	specs := []string{"8080:80", "9090:3000"}
	got := portSpecToBindings(specs)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].HostPort != 8080 || got[0].ContainerPort != 80 || got[0].Protocol != "tcp" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].HostPort != 9090 || got[1].ContainerPort != 3000 {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestPortSpecToBindings_Empty(t *testing.T) {
	got := portSpecToBindings(nil)
	if len(got) != 0 {
		t.Errorf("portSpecToBindings(nil) = %v, want empty", got)
	}
}

func TestPortSpecToBindings_RangeSpec(t *testing.T) {
	specs := []string{"8080:80", "8000-8010:8000-8010", "9090:3000"}
	got := portSpecToBindings(specs)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].HostPort != 8080 || got[0].ContainerPort != 80 {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].RawSpec != "8000-8010:8000-8010" {
		t.Errorf("got[1].RawSpec = %q, want \"8000-8010:8000-8010\"", got[1].RawSpec)
	}
	if got[2].HostPort != 9090 || got[2].ContainerPort != 3000 {
		t.Errorf("got[2] = %+v", got[2])
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

		remoteUser := configRemoteUser(cfg)
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

	user := eng.detectContainerUser(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1"})
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

	user := eng.detectContainerUser(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1"})
	if user != "" {
		t.Errorf("detectContainerUser = %q, want empty (root should fall through)", user)
	}
}

func TestChownPluginVolumes_OnlyVolumes(t *testing.T) {
	mockDrv := &mockDriver{responses: map[string]string{}}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	mounts := []config.Mount{
		{Type: "volume", Source: "cache-vol", Target: "/home/vscode/.cargo"},
		{Type: "bind", Source: "/host/path", Target: "/container/path"},
		{Type: "volume", Source: "apt-vol", Target: "/var/cache/apt"},
	}

	eng.chownPluginVolumes(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1", remoteUser: "vscode"}, mounts)

	// Should only chown volumes, not binds.
	if len(mockDrv.execCalls) != 2 {
		t.Fatalf("expected 2 exec calls, got %d", len(mockDrv.execCalls))
	}
	if mockDrv.execCalls[0].cmd[1] != "vscode:" || mockDrv.execCalls[0].cmd[2] != "/home/vscode/.cargo" {
		t.Errorf("first chown call = %v, want chown vscode: /home/vscode/.cargo", mockDrv.execCalls[0].cmd)
	}
	if mockDrv.execCalls[1].cmd[2] != "/var/cache/apt" {
		t.Errorf("second chown call target = %q, want /var/cache/apt", mockDrv.execCalls[1].cmd[2])
	}
}

func TestChownPluginVolumes_SkipsEmpty(t *testing.T) {
	mockDrv := &mockDriver{responses: map[string]string{}}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	eng.chownPluginVolumes(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1", remoteUser: "vscode"}, nil)

	if len(mockDrv.execCalls) != 0 {
		t.Errorf("expected 0 exec calls for nil mounts, got %d", len(mockDrv.execCalls))
	}
}

func TestChownPluginVolumes_ErrorContinues(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{},
		errors: map[string]error{
			"chown vscode: /first": fmt.Errorf("permission denied"),
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	mounts := []config.Mount{
		{Type: "volume", Source: "vol1", Target: "/first"},
		{Type: "volume", Source: "vol2", Target: "/second"},
	}

	eng.chownPluginVolumes(context.Background(), containerContext{workspaceID: "ws-1", containerID: "c-1", remoteUser: "vscode"}, mounts)

	// Should attempt both even if first fails.
	if len(mockDrv.execCalls) != 2 {
		t.Fatalf("expected 2 exec calls (continues after error), got %d", len(mockDrv.execCalls))
	}
}

func TestUpSingle_AlreadyRunning_PreservesPathPrepend(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-path", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "existing-c",
			State: driver.ContainerState{Status: "running"},
		},
	}

	mgr := plugin.NewManager(slog.Default())
	mgr.Register(&testPlugin{
		resp: &plugin.PreContainerRunResponse{
			PathPrepend: []string{"/home/vscode/.bundle/bin"},
		},
	})

	eng := &Engine{
		driver:      drv,
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ruby:3.2"
	cfg.RemoteUser = "vscode"

	result, err := eng.upSingle(context.Background(), ws, cfg, "/workspaces/project", UpOptions{})
	if err != nil {
		t.Fatalf("upSingle: %v", err)
	}
	if result.ContainerID != "existing-c" {
		t.Errorf("ContainerID = %q, want existing-c", result.ContainerID)
	}

	// cfg.RemoteEnv is mutated in place by setupContainer. The caller (Up)
	// passes it to saveResult after upSingle returns. Verify it contains
	// the plugin PATH entry.
	if cfg.RemoteEnv == nil {
		t.Fatal("cfg.RemoteEnv is nil after upSingle, expected plugin PATH entries")
	}
	path := cfg.RemoteEnv["PATH"]
	if !strings.Contains(path, "/home/vscode/.bundle/bin") {
		t.Errorf("cfg.RemoteEnv PATH = %q, want to contain /home/vscode/.bundle/bin", path)
	}
}

func TestUpSingle_AlreadyRunning_PassesRemoteUserToPlugins(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-user", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "existing-c",
			State: driver.ContainerState{Status: "running"},
		},
	}

	tp := &testPlugin{
		resp: &plugin.PreContainerRunResponse{
			PathPrepend: []string{"/home/vscode/.local/bin"},
		},
	}
	mgr := plugin.NewManager(slog.Default())
	mgr.Register(tp)

	eng := &Engine{
		driver:      drv,
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	cfg.RemoteUser = "vscode"

	_, err := eng.upSingle(context.Background(), ws, cfg, "/workspaces/project", UpOptions{})
	if err != nil {
		t.Fatalf("upSingle: %v", err)
	}

	if tp.req == nil {
		t.Fatal("plugin was not called")
	}
	if tp.req.RemoteUser != "vscode" {
		t.Errorf("plugin received RemoteUser = %q, want %q", tp.req.RemoteUser, "vscode")
	}
}

func TestUpSingle_AlreadyRunning_FallsBackToContainerUser(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-user2", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	drv := &fixedFindContainerDriver{
		container: &driver.ContainerDetails{
			ID:    "existing-c",
			State: driver.ContainerState{Status: "running"},
		},
	}

	tp := &testPlugin{
		resp: &plugin.PreContainerRunResponse{},
	}
	mgr := plugin.NewManager(slog.Default())
	mgr.Register(tp)

	eng := &Engine{
		driver:      drv,
		store:       store,
		plugins:     mgr,
		runtimeName: "docker",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		progress:    func(string) {},
	}

	// No RemoteUser, only ContainerUser.
	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	cfg.ContainerUser = "devuser"

	_, err := eng.upSingle(context.Background(), ws, cfg, "/workspaces/project", UpOptions{})
	if err != nil {
		t.Fatalf("upSingle: %v", err)
	}

	if tp.req == nil {
		t.Fatal("plugin was not called")
	}
	// dispatchPlugins falls back to configRemoteUser which returns ContainerUser
	// when RemoteUser is empty.
	if tp.req.RemoteUser != "devuser" {
		t.Errorf("plugin received RemoteUser = %q, want %q", tp.req.RemoteUser, "devuser")
	}
}

func TestConfigRemoteUser(t *testing.T) {
	tests := []struct {
		remoteUser    string
		containerUser string
		want          string
	}{
		{"vscode", "", "vscode"},
		{"", "dev", "dev"},
		{"vscode", "dev", "vscode"},
		{"", "", ""},
	}
	for _, tt := range tests {
		cfg := &config.DevContainerConfig{}
		cfg.RemoteUser = tt.remoteUser
		cfg.ContainerUser = tt.containerUser
		got := configRemoteUser(cfg)
		if got != tt.want {
			t.Errorf("configRemoteUser(remote=%q, container=%q) = %q, want %q",
				tt.remoteUser, tt.containerUser, got, tt.want)
		}
	}
}
