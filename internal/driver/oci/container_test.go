package oci

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
)

func newTestDockerDriver() *OCIDriver {
	return &OCIDriver{
		helper:  NewHelper("docker", slog.Default()),
		runtime: RuntimeDocker,
		logger:  slog.Default(),
	}
}

func newTestPodmanDriver() *OCIDriver {
	return &OCIDriver{
		helper:  NewHelper("podman", slog.Default()),
		runtime: RuntimePodman,
		logger:  slog.Default(),
	}
}

func TestBuildRunArgs_Minimal(t *testing.T) {
	d := newTestDockerDriver()

	opts := &driver.RunOptions{
		Image: "ubuntu:22.04",
	}

	args := d.buildRunArgs("myproject", opts)
	got := strings.Join(args, " ")

	assertContains(t, got, "run -d --name crib-myproject")
	assertContains(t, got, "--label crib.workspace=myproject")
	assertContains(t, got, "ubuntu:22.04")
}

func TestBuildRunArgs_AllOptions(t *testing.T) {
	d := newTestDockerDriver()

	opts := &driver.RunOptions{
		Image:       "myimage:latest",
		User:        "vscode",
		Entrypoint:  "/bin/sh",
		Cmd:         []string{"-c", "sleep infinity"},
		Env:         []string{"FOO=bar", "BAZ=qux"},
		CapAdd:      []string{"SYS_PTRACE"},
		SecurityOpt: []string{"seccomp=unconfined"},
		Labels:      map[string]string{"custom": "value"},
		Privileged:  true,
		Init:        true,
		WorkspaceMount: config.Mount{
			Type:   "bind",
			Source: "/home/user/project",
			Target: "/workspaces/project",
		},
		Mounts: []config.Mount{
			{Type: "volume", Source: "mydata", Target: "/data"},
		},
	}

	args := d.buildRunArgs("test-ws", opts)
	got := strings.Join(args, " ")

	assertContains(t, got, "--name crib-test-ws")
	assertContains(t, got, "--label crib.workspace=test-ws")
	assertContains(t, got, "--label custom=value")
	assertContains(t, got, "--user vscode")
	assertContains(t, got, "-e FOO=bar")
	assertContains(t, got, "-e BAZ=qux")
	assertContains(t, got, "--init")
	assertContains(t, got, "--privileged")
	assertContains(t, got, "--cap-add SYS_PTRACE")
	assertContains(t, got, "--security-opt seccomp=unconfined")
	assertContains(t, got, "--mount type=bind,src=/home/user/project,dst=/workspaces/project")
	assertContains(t, got, "--mount type=volume,src=mydata,dst=/data")
	assertContains(t, got, "--entrypoint /bin/sh")

	// Image and cmd should be at the end.
	if !strings.HasSuffix(got, "myimage:latest -c sleep infinity") {
		t.Errorf("expected args to end with image and cmd, got: %s", got)
	}
}

func TestBuildRunArgs_WorkspaceLabelAlwaysPresent(t *testing.T) {
	d := newTestDockerDriver()

	opts := &driver.RunOptions{
		Image: "alpine",
	}

	args := d.buildRunArgs("ws1", opts)
	found := false
	for i, a := range args {
		if a == "--label" && i+1 < len(args) && args[i+1] == "crib.workspace=ws1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("workspace label not found in args: %v", args)
	}
}

func TestBuildRunArgs_NoOptionalFlags(t *testing.T) {
	d := newTestDockerDriver()

	opts := &driver.RunOptions{
		Image: "alpine",
	}

	args := d.buildRunArgs("ws1", opts)
	got := strings.Join(args, " ")

	// These flags should NOT be present with empty options.
	for _, flag := range []string{"--user", "--init", "--privileged", "--cap-add", "--security-opt", "--entrypoint", "--mount", "-e"} {
		if strings.Contains(got, flag) {
			t.Errorf("unexpected flag %q in args: %s", flag, got)
		}
	}
}

func TestBuildRunArgs_RootlessPodmanInjectsUserns(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 1000 } // non-root

	d := newTestPodmanDriver()

	opts := &driver.RunOptions{
		Image: "alpine",
	}

	args := d.buildRunArgs("ws1", opts)
	got := strings.Join(args, " ")

	assertContains(t, got, "--userns=keep-id")
}

func TestBuildRunArgs_RootPodmanSkipsUserns(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 0 } // root

	d := newTestPodmanDriver()

	opts := &driver.RunOptions{
		Image: "alpine",
	}

	args := d.buildRunArgs("ws1", opts)
	got := strings.Join(args, " ")

	if strings.Contains(got, "--userns") {
		t.Errorf("--userns should not be injected for root podman, got: %s", got)
	}
}

func TestBuildRunArgs_DockerSkipsUserns(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 1000 }

	d := newTestDockerDriver()

	opts := &driver.RunOptions{
		Image: "alpine",
	}

	args := d.buildRunArgs("ws1", opts)
	got := strings.Join(args, " ")

	if strings.Contains(got, "--userns") {
		t.Errorf("--userns should not be injected for docker, got: %s", got)
	}
}

func TestBuildRunArgs_UserUsernsOverrideSkipsAutoInject(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 1000 }

	d := newTestPodmanDriver()

	opts := &driver.RunOptions{
		Image:     "alpine",
		ExtraArgs: []string{"--userns=host"},
	}

	args := d.buildRunArgs("ws1", opts)

	// Should have the user-specified --userns=host but NOT --userns=keep-id.
	got := strings.Join(args, " ")
	assertContains(t, got, "--userns=host")

	count := strings.Count(got, "--userns")
	if count != 1 {
		t.Errorf("expected exactly one --userns flag, found %d in: %s", count, got)
	}
}

func TestBuildRunArgs_Ports(t *testing.T) {
	d := newTestDockerDriver()

	opts := &driver.RunOptions{
		Image: "alpine",
		Ports: []string{"8080:8080", "9090:3000"},
	}

	args := d.buildRunArgs("ws1", opts)
	got := strings.Join(args, " ")

	assertContains(t, got, "--publish 8080:8080")
	assertContains(t, got, "--publish 9090:3000")

	// Ports should appear before the image name.
	imageIdx := strings.Index(got, "alpine")
	portIdx := strings.Index(got, "--publish")
	if portIdx > imageIdx {
		t.Errorf("--publish should appear before image, got: %s", got)
	}
}

func TestBuildRunArgs_ExtraArgsPassthrough(t *testing.T) {
	origGetuid := getuid
	t.Cleanup(func() { getuid = origGetuid })
	getuid = func() int { return 0 } // root, so no auto-inject

	d := newTestDockerDriver()

	opts := &driver.RunOptions{
		Image:     "alpine",
		Cmd:       []string{"sleep", "infinity"},
		ExtraArgs: []string{"--network=host", "--gpus", "all"},
	}

	args := d.buildRunArgs("ws1", opts)
	got := strings.Join(args, " ")

	assertContains(t, got, "--network=host")
	assertContains(t, got, "--gpus all")

	// ExtraArgs should appear before the image name.
	imageIdx := strings.Index(got, "alpine")
	networkIdx := strings.Index(got, "--network=host")
	if networkIdx > imageIdx {
		t.Errorf("ExtraArgs should appear before image, got: %s", got)
	}

	// Image and cmd should still be at the end.
	if !strings.HasSuffix(got, "alpine sleep infinity") {
		t.Errorf("expected args to end with image and cmd, got: %s", got)
	}
}

func TestParseLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"\n", 0},
		{"abc\n", 1},
		{"abc\ndef\n", 2},
		{"  abc  \n  def  \n", 2},
	}
	for _, tt := range tests {
		got := parseLines(tt.input)
		if len(got) != tt.want {
			t.Errorf("parseLines(%q) returned %d items, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestScrubArgs(t *testing.T) {
	args := []string{
		"exec", "--user", "root",
		"-e", "GH_TOKEN=secret123",
		"-e", "PATH=/usr/bin:/bin",
		"-e", "SSH_AUTH_SOCK=/tmp/ssh.sock",
		"-e", "AWS_SECRET_ACCESS_KEY=mysecret",
		"-e", "DATABASE_PASSWORD=hunter2",
		"-e", "HOME=/root",
		"-e", "API_KEY=abc123",
		"container123", "sh", "-c", "echo hello",
	}

	scrubbed := scrubArgs(args)

	// Sensitive values should be redacted.
	for i, arg := range scrubbed {
		if i > 0 && args[i-1] == "-e" {
			switch {
			case strings.HasPrefix(arg, "GH_TOKEN="):
				if arg != "GH_TOKEN=***" {
					t.Errorf("GH_TOKEN not scrubbed: %s", arg)
				}
			case strings.HasPrefix(arg, "SSH_AUTH_SOCK="):
				if arg != "SSH_AUTH_SOCK=***" {
					t.Errorf("SSH_AUTH_SOCK not scrubbed: %s", arg)
				}
			case strings.HasPrefix(arg, "AWS_SECRET_ACCESS_KEY="):
				if arg != "AWS_SECRET_ACCESS_KEY=***" {
					t.Errorf("AWS_SECRET_ACCESS_KEY not scrubbed: %s", arg)
				}
			case strings.HasPrefix(arg, "DATABASE_PASSWORD="):
				if arg != "DATABASE_PASSWORD=***" {
					t.Errorf("DATABASE_PASSWORD not scrubbed: %s", arg)
				}
			case strings.HasPrefix(arg, "API_KEY="):
				if arg != "API_KEY=***" {
					t.Errorf("API_KEY not scrubbed: %s", arg)
				}
			case strings.HasPrefix(arg, "PATH="):
				if arg != "PATH=/usr/bin:/bin" {
					t.Errorf("PATH should not be scrubbed: %s", arg)
				}
			case strings.HasPrefix(arg, "HOME="):
				if arg != "HOME=/root" {
					t.Errorf("HOME should not be scrubbed: %s", arg)
				}
			}
		}
	}

	// Non-env args should be unchanged.
	if scrubbed[0] != "exec" || scrubbed[1] != "--user" || scrubbed[2] != "root" {
		t.Errorf("non-env args were modified: %v", scrubbed[:3])
	}
}

func TestParseContainerPort(t *testing.T) {
	tests := []struct {
		input    string
		wantPort int
		wantProto string
	}{
		{"8080/tcp", 8080, "tcp"},
		{"53/udp", 53, "udp"},
		{"3000", 3000, "tcp"},       // no protocol defaults to tcp
		{"invalid/tcp", 0, "tcp"},   // non-numeric port
	}
	for _, tt := range tests {
		port, proto := parseContainerPort(tt.input)
		if port != tt.wantPort || proto != tt.wantProto {
			t.Errorf("parseContainerPort(%q) = (%d, %q), want (%d, %q)",
				tt.input, port, proto, tt.wantPort, tt.wantProto)
		}
	}
}

func TestInspectContainer_ToContainerDetails_Ports(t *testing.T) {
	ic := &inspectContainer{}
	ic.ID = "abc123"
	ic.State.Status = "running"
	ic.NetworkSettings.Ports = map[string][]struct {
		HostIp   string `json:"HostIp"`
		HostPort string `json:"HostPort"`
	}{
		"8080/tcp": {{HostIp: "0.0.0.0", HostPort: "8080"}},
		"53/udp":   {{HostIp: "0.0.0.0", HostPort: "5353"}},
	}

	details := ic.toContainerDetails()

	if details.ID != "abc123" {
		t.Errorf("ID = %q, want %q", details.ID, "abc123")
	}
	if len(details.Ports) != 2 {
		t.Fatalf("Ports length = %d, want 2", len(details.Ports))
	}

	// Build a lookup by container port for order-independent checks.
	byContainer := map[int]driver.PortBinding{}
	for _, p := range details.Ports {
		byContainer[p.ContainerPort] = p
	}

	tcp := byContainer[8080]
	if tcp.HostPort != 8080 || tcp.Protocol != "tcp" {
		t.Errorf("tcp port = %+v, want HostPort=8080, Protocol=tcp", tcp)
	}

	udp := byContainer[53]
	if udp.HostPort != 5353 || udp.Protocol != "udp" {
		t.Errorf("udp port = %+v, want HostPort=5353, Protocol=udp", udp)
	}
}

func TestInspectContainer_ToContainerDetails_NoPorts(t *testing.T) {
	ic := &inspectContainer{}
	ic.ID = "xyz"
	ic.State.Status = "exited"

	details := ic.toContainerDetails()

	if len(details.Ports) != 0 {
		t.Errorf("Ports should be empty, got %v", details.Ports)
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}
