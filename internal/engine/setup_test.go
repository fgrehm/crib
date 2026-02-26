package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
)

// mockDriver implements the Driver interface for testing.
type mockDriver struct {
	execCalls []mockExecCall
	responses map[string]string
	errors    map[string]error
}

type mockExecCall struct {
	cmd []string
	env []string
}

func (m *mockDriver) FindContainer(ctx context.Context, workspaceID string) (*driver.ContainerDetails, error) {
	return nil, nil
}

func (m *mockDriver) RunContainer(ctx context.Context, workspaceID string, options *driver.RunOptions) error {
	return nil
}

func (m *mockDriver) StartContainer(ctx context.Context, workspaceID, containerID string) error {
	return nil
}

func (m *mockDriver) StopContainer(ctx context.Context, workspaceID, containerID string) error {
	return nil
}

func (m *mockDriver) RestartContainer(ctx context.Context, workspaceID, containerID string) error {
	return nil
}

func (m *mockDriver) DeleteContainer(ctx context.Context, workspaceID, containerID string) error {
	return nil
}

func (m *mockDriver) ExecContainer(ctx context.Context, workspaceID, containerID string, cmd []string, stdin io.Reader, stdout, stderr io.Writer, env []string, user string) error {
	m.execCalls = append(m.execCalls, mockExecCall{cmd: cmd, env: env})

	// Try full command key first, then fall back to legacy prefix matching.
	fullKey := strings.Join(cmd, " ")

	// Check errors map first.
	if m.errors != nil {
		if err, ok := m.errors[fullKey]; ok {
			return err
		}
	}

	// Check responses: full key first, then legacy prefix.
	if response, ok := m.responses[fullKey]; ok && stdout != nil {
		io.WriteString(stdout, response)
		return nil
	}

	// Legacy prefix-based matching for backward compatibility.
	shortKey := cmd[0]
	if len(cmd) > 1 && cmd[0] == "id" {
		shortKey = cmd[0] + " " + cmd[1]
	}

	if m.errors != nil {
		if err, ok := m.errors[shortKey]; ok {
			return err
		}
	}

	if response, ok := m.responses[shortKey]; ok && stdout != nil {
		io.WriteString(stdout, response)
	}
	return nil
}

func (m *mockDriver) ContainerLogs(ctx context.Context, workspaceID, containerID string, stdout, stderr io.Writer) error {
	return nil
}

func (m *mockDriver) BuildImage(ctx context.Context, workspaceID string, options *driver.BuildOptions) error {
	return nil
}

func (m *mockDriver) InspectImage(ctx context.Context, imageName string) (*driver.ImageDetails, error) {
	return nil, nil
}

func (m *mockDriver) TargetArchitecture(ctx context.Context) (string, error) {
	return "amd64", nil
}

func TestParseEnvLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{
			name:  "basic key=value pairs",
			input: "FOO=bar\nBAZ=qux\n",
			want:  map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:  "value contains equals sign",
			input: "URL=postgres://user:pass@host/db?sslmode=disable\n",
			want:  map[string]string{"URL": "postgres://user:pass@host/db?sslmode=disable"},
		},
		{
			name:  "empty value",
			input: "EMPTY=\n",
			want:  map[string]string{"EMPTY": ""},
		},
		{
			name:  "skip lines without equals",
			input: "VALID=yes\nno-equals-here\n",
			want:  map[string]string{"VALID": "yes"},
		},
		{
			name:  "empty input",
			input: "",
			want:  map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEnvLines(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseEnvLines() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDevcontainerEnv(t *testing.T) {
	got := devcontainerEnv("ws-abc", "/home/user/myproject", "/workspaces/myproject")
	sort.Strings(got)

	want := []string{
		"containerWorkspaceFolder=/workspaces/myproject",
		"containerWorkspaceFolderBasename=myproject",
		"devcontainerId=ws-abc",
		"localWorkspaceFolder=/home/user/myproject",
		"localWorkspaceFolderBasename=myproject",
	}
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("devcontainerEnv() = %v, want %v", got, want)
	}
}

func TestExecGetUserID(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"id -u": "1000",
			"id -g": "1001",
		},
	}

	engine := &Engine{
		driver: mockDrv,
		logger: slog.Default(),
	}

	// Test getting UID.
	uid, err := engine.execGetUserID(context.Background(), "ws-1", "container-1", "vscode", "u")
	if err != nil {
		t.Fatalf("execGetUserID failed: %v", err)
	}
	if uid != 1000 {
		t.Errorf("execGetUserID UID = %d, want 1000", uid)
	}

	// Test getting GID.
	gid, err := engine.execGetUserID(context.Background(), "ws-1", "container-1", "vscode", "g")
	if err != nil {
		t.Fatalf("execGetUserID failed: %v", err)
	}
	if gid != 1001 {
		t.Errorf("execGetUserID GID = %d, want 1001", gid)
	}
}

func TestExecGetGroupName(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"id -gn": "vscode\n",
		},
	}

	engine := &Engine{
		driver: mockDrv,
		logger: slog.Default(),
	}

	name, err := engine.execGetGroupName(context.Background(), "ws-1", "container-1", "vscode")
	if err != nil {
		t.Fatalf("execGetGroupName failed: %v", err)
	}
	if name != "vscode" {
		t.Errorf("execGetGroupName = %q, want %q", name, "vscode")
	}
}

func TestSyncRemoteUserUID_Disabled(t *testing.T) {
	// When updateRemoteUserUID is explicitly false, skip sync.
	mockDrv := &mockDriver{responses: map[string]string{}}
	engine := &Engine{driver: mockDrv, logger: slog.Default()}

	false_ := false
	cfg := &config.DevContainerConfig{}
	cfg.UpdateRemoteUserUID = &false_

	inSync, err := engine.syncRemoteUserUID(context.Background(), "ws-1", "container-1", "vscode", cfg)
	if err != nil {
		t.Fatalf("syncRemoteUserUID failed: %v", err)
	}

	// Should not have called ExecContainer.
	if len(mockDrv.execCalls) != 0 {
		t.Errorf("syncRemoteUserUID should not exec when disabled, got %d calls", len(mockDrv.execCalls))
	}
	// Disabled means we can't confirm UIDs are in sync.
	if inSync {
		t.Errorf("syncRemoteUserUID should return inSync=false when disabled")
	}
}

func TestSyncRemoteUserUID_RootUser(t *testing.T) {
	// When remoteUser is root, skip sync.
	mockDrv := &mockDriver{responses: map[string]string{}}
	engine := &Engine{driver: mockDrv, logger: slog.Default()}

	cfg := &config.DevContainerConfig{}

	inSync, err := engine.syncRemoteUserUID(context.Background(), "ws-1", "container-1", "root", cfg)
	if err != nil {
		t.Fatalf("syncRemoteUserUID failed: %v", err)
	}

	// Should not have called ExecContainer.
	if len(mockDrv.execCalls) != 0 {
		t.Errorf("syncRemoteUserUID should not sync for root, got %d calls", len(mockDrv.execCalls))
	}
	if inSync {
		t.Errorf("syncRemoteUserUID should return inSync=false for root user")
	}
}

func TestSyncRemoteUserUID_UIDsMatch(t *testing.T) {
	// When UIDs and GIDs already match, skip sync operations and report in-sync.
	// Use os.Getuid()/os.Getgid() values so the match condition is always satisfied.
	hostUID := os.Getuid()
	hostGID := os.Getgid()

	mockDrv := &mockDriver{
		responses: map[string]string{
			"id -u": strconv.Itoa(hostUID),
			"id -g": strconv.Itoa(hostGID),
		},
	}
	engine := &Engine{driver: mockDrv, logger: slog.Default()}

	cfg := &config.DevContainerConfig{}

	inSync, err := engine.syncRemoteUserUID(context.Background(), "ws-1", "container-1", "vscode", cfg)
	if err != nil {
		t.Fatalf("syncRemoteUserUID failed: %v", err)
	}

	// Should have called id to probe UIDs/GIDs, but no groupmod/usermod/find calls.
	if len(mockDrv.execCalls) > 2 {
		t.Errorf("syncRemoteUserUID should skip sync when UIDs match, got %d calls", len(mockDrv.execCalls))
	}
	// UIDs matched, so inSync should be true (chown unnecessary).
	if !inSync {
		t.Errorf("syncRemoteUserUID should return inSync=true when UIDs already match")
	}
}

func TestDetectUserShell_FromGetent(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"getent passwd vscode": "vscode:x:1000:1000::/home/vscode:/bin/bash\n",
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	shell := eng.detectUserShell(context.Background(), "ws-1", "c-1", "vscode")
	if shell != "/bin/bash" {
		t.Errorf("detectUserShell = %q, want /bin/bash", shell)
	}
}

func TestDetectUserShell_GetentFails_FallsBackToBash(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{},
		errors: map[string]error{
			"getent passwd vscode": fmt.Errorf("getent not found"),
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	shell := eng.detectUserShell(context.Background(), "ws-1", "c-1", "vscode")
	// test -x /bin/bash succeeds by default (no error entry), so fallback returns /bin/bash.
	if shell != "/bin/bash" {
		t.Errorf("detectUserShell = %q, want /bin/bash (fallback)", shell)
	}
}

func TestDetectUserShell_ShellNotExecutable_FallsBack(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"getent passwd vscode": "vscode:x:1000:1000::/home/vscode:/usr/bin/zsh\n",
		},
		errors: map[string]error{
			"test -x /usr/bin/zsh": fmt.Errorf("not executable"),
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	shell := eng.detectUserShell(context.Background(), "ws-1", "c-1", "vscode")
	if shell != "/bin/bash" {
		t.Errorf("detectUserShell = %q, want /bin/bash (fallback after zsh not found)", shell)
	}
}

func TestProbeUserEnv_None(t *testing.T) {
	mockDrv := &mockDriver{responses: map[string]string{}}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	result := eng.probeUserEnv(context.Background(), "ws-1", "c-1", "vscode", "none")
	if result != nil {
		t.Errorf("probeUserEnv(none) = %v, want nil", result)
	}
	if len(mockDrv.execCalls) != 0 {
		t.Errorf("probeUserEnv(none) should not exec, got %d calls", len(mockDrv.execCalls))
	}
}

func TestProbeUserEnv_LoginInteractiveShell(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"getent passwd vscode":   "vscode:x:1000:1000::/home/vscode:/bin/bash\n",
			"/bin/bash -l -i -c env": "PATH=/usr/bin:/home/vscode/.local/share/mise/shims\nHOME=/home/vscode\nSHLVL=1\n",
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	result := eng.probeUserEnv(context.Background(), "ws-1", "c-1", "vscode", "loginInteractiveShell")
	if result == nil {
		t.Fatal("probeUserEnv returned nil")
	}
	if result["PATH"] != "/usr/bin:/home/vscode/.local/share/mise/shims" {
		t.Errorf("PATH = %q, want path with mise shims", result["PATH"])
	}
	if result["HOME"] != "/home/vscode" {
		t.Errorf("HOME = %q, want /home/vscode", result["HOME"])
	}
}

func TestProbeUserEnv_DefaultIsLoginInteractiveShell(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"getent passwd vscode":   "vscode:x:1000:1000::/home/vscode:/bin/bash\n",
			"/bin/bash -l -i -c env": "FOO=bar\n",
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	// Empty string should default to loginInteractiveShell.
	result := eng.probeUserEnv(context.Background(), "ws-1", "c-1", "vscode", "")
	if result == nil {
		t.Fatal("probeUserEnv with empty probe returned nil")
	}
	if result["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar", result["FOO"])
	}
}

func TestProbeUserEnv_LoginShell(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"getent passwd vscode": "vscode:x:1000:1000::/home/vscode:/bin/bash\n",
			"/bin/bash -l -c env":  "FOO=login\n",
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	result := eng.probeUserEnv(context.Background(), "ws-1", "c-1", "vscode", "loginShell")
	if result == nil {
		t.Fatal("probeUserEnv returned nil")
	}
	if result["FOO"] != "login" {
		t.Errorf("FOO = %q, want login", result["FOO"])
	}
}

func TestProbeUserEnv_InteractiveShell(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"getent passwd vscode": "vscode:x:1000:1000::/home/vscode:/bin/bash\n",
			"/bin/bash -i -c env":  "FOO=interactive\n",
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	result := eng.probeUserEnv(context.Background(), "ws-1", "c-1", "vscode", "interactiveShell")
	if result == nil {
		t.Fatal("probeUserEnv returned nil")
	}
	if result["FOO"] != "interactive" {
		t.Errorf("FOO = %q, want interactive", result["FOO"])
	}
}

func TestProbeUserEnv_ProbeFails_ReturnsNil(t *testing.T) {
	mockDrv := &mockDriver{
		responses: map[string]string{
			"getent passwd vscode": "vscode:x:1000:1000::/home/vscode:/bin/bash\n",
		},
		errors: map[string]error{
			"/bin/bash -l -i -c env": fmt.Errorf("bash: cannot set terminal process group"),
		},
	}
	eng := &Engine{driver: mockDrv, logger: slog.Default()}

	result := eng.probeUserEnv(context.Background(), "ws-1", "c-1", "vscode", "")
	if result != nil {
		t.Errorf("probeUserEnv should return nil on probe failure, got %v", result)
	}
}

func TestMergeEnv_ProbedOnly(t *testing.T) {
	probed := map[string]string{"PATH": "/usr/bin:/custom", "HOME": "/home/user"}
	result := mergeEnv(probed, nil)
	if result["PATH"] != "/usr/bin:/custom" {
		t.Errorf("PATH = %q, want /usr/bin:/custom", result["PATH"])
	}
}

func TestMergeEnv_RemoteEnvOverrides(t *testing.T) {
	probed := map[string]string{"PATH": "/usr/bin", "FOO": "probed"}
	remote := map[string]string{"FOO": "remote", "BAR": "new"}
	result := mergeEnv(probed, remote)
	if result["FOO"] != "remote" {
		t.Errorf("FOO = %q, want remote (remoteEnv should win)", result["FOO"])
	}
	if result["BAR"] != "new" {
		t.Errorf("BAR = %q, want new", result["BAR"])
	}
	if result["PATH"] != "/usr/bin" {
		t.Errorf("PATH = %q, want /usr/bin", result["PATH"])
	}
}

func TestMergeEnv_SkipsSessionVars(t *testing.T) {
	probed := map[string]string{
		"PATH":     "/usr/bin",
		"HOSTNAME": "abc123",
		"SHLVL":    "1",
		"PWD":      "/",
		"_":        "/usr/bin/env",
	}
	result := mergeEnv(probed, nil)
	for _, skip := range []string{"HOSTNAME", "SHLVL", "PWD", "_"} {
		if _, ok := result[skip]; ok {
			t.Errorf("%s should be excluded from merged env", skip)
		}
	}
	if result["PATH"] != "/usr/bin" {
		t.Errorf("PATH should be included, got %q", result["PATH"])
	}
}

func TestMergeEnv_BothNil(t *testing.T) {
	result := mergeEnv(nil, nil)
	if result != nil {
		t.Errorf("mergeEnv(nil, nil) = %v, want nil", result)
	}
}
