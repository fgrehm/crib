package engine

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestWrapCommand_AsRoot(t *testing.T) {
	r := &lifecycleRunner{remoteUser: "root"}
	cmd := r.wrapCommand("echo hello", "/workspaces/project")

	if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" {
		t.Errorf("expected [sh -c <script>] wrapper, got %v", cmd)
	}
	if !strings.Contains(cmd[2], "echo hello") {
		t.Errorf("expected command in wrapper, got %v", cmd)
	}
	if !strings.Contains(cmd[2], "cd \"/workspaces/project\"") {
		t.Errorf("expected cd in wrapper, got %v", cmd)
	}
}

func TestWrapCommand_AsUser(t *testing.T) {
	// User switching is handled at the driver level via --user, not by wrapping with su.
	r := &lifecycleRunner{remoteUser: "vscode"}
	cmd := r.wrapCommand("echo hello", "/workspaces/project")

	if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" {
		t.Errorf("expected [sh -c <script>] wrapper, got %v", cmd)
	}
	if !strings.Contains(cmd[2], "echo hello") {
		t.Errorf("expected command in wrapper, got %v", cmd)
	}
}

func TestWrapCommand_EmptyWorkspaceFolder(t *testing.T) {
	r := &lifecycleRunner{remoteUser: "root"}
	cmd := r.wrapCommand("echo test", "")

	// Should not contain cd when workspace folder is empty.
	if strings.Contains(strings.Join(cmd, " "), "cd ") {
		t.Errorf("should not cd when workspace folder is empty, got %v", cmd)
	}
}

func TestEnvSlice_Nil(t *testing.T) {
	if got := envSlice(nil); got != nil {
		t.Errorf("envSlice(nil) = %v, want nil", got)
	}
}

func TestEnvSlice_Empty(t *testing.T) {
	if got := envSlice(map[string]string{}); got != nil {
		t.Errorf("envSlice({}) = %v, want nil", got)
	}
}

func TestEnvSlice_Values(t *testing.T) {
	input := map[string]string{"FOO": "bar", "BAZ": "qux=1"}
	got := envSlice(input)
	sort.Strings(got)

	want := []string{"BAZ=qux=1", "FOO=bar"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("envSlice() = %v, want %v", got, want)
	}
}
