package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/workspace"
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

// newTestRunner constructs a lifecycleRunner wired to the given mock driver,
// with a fresh workspace store backed by a temp directory.
func newTestRunner(t *testing.T, mock *mockDriver) (*lifecycleRunner, *workspace.Store, string) {
	t.Helper()
	store := workspace.NewStoreAt(t.TempDir())
	wsID := "test-ws-" + strings.ReplaceAll(t.Name(), "/", "-")
	ws := &workspace.Workspace{ID: wsID}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	r := &lifecycleRunner{
		driver:      mock,
		store:       store,
		workspaceID: wsID,
		containerID: "c-test",
		remoteUser:  "root",
		logger:      slog.Default(),
		stdout:      io.Discard,
		stderr:      io.Discard,
	}
	return r, store, wsID
}

// --- runHook tests ---

func TestRunHook_Empty(t *testing.T) {
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)

	if err := r.runHook(context.Background(), "onCreateCommand", nil, ""); err != nil {
		t.Fatalf("runHook with nil hook: %v", err)
	}
	if err := r.runHook(context.Background(), "onCreateCommand", config.LifecycleHook{}, ""); err != nil {
		t.Fatalf("runHook with empty hook: %v", err)
	}
	if len(mock.execCalls) != 0 {
		t.Errorf("expected no exec calls for empty hook, got %d", len(mock.execCalls))
	}
}

func TestRunHook_Sequential_String(t *testing.T) {
	// String-form hook (single "" key with one element) runs one command.
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)

	hook := config.LifecycleHook{"": {"echo hello"}}
	if err := r.runHook(context.Background(), "postCreateCommand", hook, ""); err != nil {
		t.Fatalf("runHook: %v", err)
	}

	if len(mock.execCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(mock.execCalls))
	}
	got := strings.Join(mock.execCalls[0].cmd, " ")
	if !strings.Contains(got, "echo hello") {
		t.Errorf("exec cmd = %q, want to contain echo hello", got)
	}
}

func TestRunHook_Sequential_Array(t *testing.T) {
	// Array-form hook (single "" key with multiple elements) joins them and runs once.
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)

	hook := config.LifecycleHook{"": {"echo", "hello"}}
	if err := r.runHook(context.Background(), "postCreateCommand", hook, ""); err != nil {
		t.Fatalf("runHook: %v", err)
	}

	if len(mock.execCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(mock.execCalls))
	}
	got := strings.Join(mock.execCalls[0].cmd, " ")
	if !strings.Contains(got, "echo hello") {
		t.Errorf("exec cmd = %q, want to contain echo hello", got)
	}
}

func TestRunHook_Parallel_BothEntriesRun(t *testing.T) {
	// Object-form hook: named entries run in parallel, both must execute.
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)

	hook := config.LifecycleHook{
		"install-node":   {"npm install"},
		"install-python": {"pip install"},
	}
	if err := r.runHook(context.Background(), "onCreateCommand", hook, ""); err != nil {
		t.Fatalf("runHook: %v", err)
	}

	if len(mock.execCalls) != 2 {
		t.Fatalf("expected 2 exec calls (one per named entry), got %d", len(mock.execCalls))
	}

	// Collect what was run (order non-deterministic in parallel).
	var cmds []string
	for _, call := range mock.execCalls {
		cmds = append(cmds, strings.Join(call.cmd, " "))
	}
	sort.Strings(cmds)

	if !strings.Contains(cmds[0], "npm install") && !strings.Contains(cmds[1], "npm install") {
		t.Errorf("npm install not found in exec calls: %v", cmds)
	}
	if !strings.Contains(cmds[0], "pip install") && !strings.Contains(cmds[1], "pip install") {
		t.Errorf("pip install not found in exec calls: %v", cmds)
	}
}

func TestRunHook_Parallel_ErrorPropagates(t *testing.T) {
	// If any parallel entry fails, the hook returns an error.
	mock := &mockDriver{
		errors: map[string]error{
			"sh -c npm install": fmt.Errorf("npm: command not found"),
		},
	}
	r, _, _ := newTestRunner(t, mock)

	hook := config.LifecycleHook{
		"install-node":   {"npm install"},
		"install-python": {"pip install"},
	}
	if err := r.runHook(context.Background(), "onCreateCommand", hook, ""); err == nil {
		t.Fatal("expected error when a parallel entry fails, got nil")
	}
}

func TestRunHook_ProgressCallback(t *testing.T) {
	var messages []string
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)
	r.progress = func(msg string) { messages = append(messages, msg) }

	hook := config.LifecycleHook{"": {"echo hi"}}
	if err := r.runHook(context.Background(), "postStartCommand", hook, ""); err != nil {
		t.Fatalf("runHook: %v", err)
	}

	if len(messages) != 1 || messages[0] != "Running postStartCommand..." {
		t.Errorf("progress messages = %v, want [Running postStartCommand...]", messages)
	}
}

func TestRunHook_NoProgressWhenEmpty(t *testing.T) {
	var messages []string
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)
	r.progress = func(msg string) { messages = append(messages, msg) }

	if err := r.runHook(context.Background(), "postStartCommand", config.LifecycleHook{}, ""); err != nil {
		t.Fatalf("runHook: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected no progress for empty hook, got %v", messages)
	}
}

// --- signalReadyAt tests ---

func TestSignalReadyAt_Match(t *testing.T) {
	var got []string
	r := &lifecycleRunner{progress: func(msg string) { got = append(got, msg) }}
	r.signalReadyAt("updateContentCommand", "updateContentCommand")

	if len(got) != 1 || got[0] != "Container ready." {
		t.Errorf("signalReadyAt match: messages = %v, want [Container ready.]", got)
	}
}

func TestSignalReadyAt_NoMatch(t *testing.T) {
	var got []string
	r := &lifecycleRunner{progress: func(msg string) { got = append(got, msg) }}
	r.signalReadyAt("onCreateCommand", "updateContentCommand")

	if len(got) != 0 {
		t.Errorf("signalReadyAt no-match: messages = %v, want []", got)
	}
}

func TestSignalReadyAt_NilProgress(t *testing.T) {
	// Should not panic when progress is nil.
	r := &lifecycleRunner{progress: nil}
	r.signalReadyAt("updateContentCommand", "updateContentCommand")
}

// --- runLifecycleHooks waitFor tests ---

// collectProgress returns a progress callback that appends to a slice.
func collectProgress(msgs *[]string) func(string) {
	return func(msg string) { *msgs = append(*msgs, msg) }
}

// indexOfMsg returns the first index where pred matches, or -1.
func indexOfMsg(msgs []string, pred func(string) bool) int {
	for i, m := range msgs {
		if pred(m) { return i }
	}
	return -1
}

func TestRunLifecycleHooks_WaitFor_Default(t *testing.T) {
	// Default waitFor is "updateContentCommand".
	// "Container ready." should appear after updateContentCommand's progress
	// message but before postCreateCommand's.
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)
	var msgs []string
	r.progress = collectProgress(&msgs)

	cfg := &config.DevContainerConfig{}
	cfg.OnCreateCommand = config.LifecycleHook{"": {"echo create"}}
	cfg.UpdateContentCommand = config.LifecycleHook{"": {"echo update"}}
	cfg.PostCreateCommand = config.LifecycleHook{"": {"echo postcreate"}}
	// WaitFor = "" → defaults to updateContentCommand

	if err := r.runLifecycleHooks(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runLifecycleHooks: %v", err)
	}

	readyIdx := indexOfMsg(msgs, func(m string) bool { return m == "Container ready." })
	if readyIdx < 0 {
		t.Fatalf("Container ready. not in progress messages: %v", msgs)
	}

	updateIdx := indexOfMsg(msgs, func(m string) bool { return m == "Running updateContentCommand..." })
	postIdx := indexOfMsg(msgs, func(m string) bool { return m == "Running postCreateCommand..." })

	if updateIdx < 0 {
		t.Fatalf("Running updateContentCommand... not in messages: %v", msgs)
	}
	if postIdx < 0 {
		t.Fatalf("Running postCreateCommand... not in messages: %v", msgs)
	}

	if readyIdx <= updateIdx {
		t.Errorf("Container ready. (idx %d) should come after updateContentCommand (idx %d)", readyIdx, updateIdx)
	}
	if postIdx <= readyIdx {
		t.Errorf("Running postCreateCommand... (idx %d) should come after Container ready. (idx %d)", postIdx, readyIdx)
	}
}

func TestRunLifecycleHooks_WaitFor_OnCreate(t *testing.T) {
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)
	var msgs []string
	r.progress = collectProgress(&msgs)

	cfg := &config.DevContainerConfig{}
	cfg.WaitFor = "onCreateCommand"
	cfg.OnCreateCommand = config.LifecycleHook{"": {"echo create"}}
	cfg.UpdateContentCommand = config.LifecycleHook{"": {"echo update"}}

	if err := r.runLifecycleHooks(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runLifecycleHooks: %v", err)
	}

	readyIdx := indexOfMsg(msgs, func(m string) bool { return m == "Container ready." })
	if readyIdx < 0 {
		t.Fatalf("Container ready. not in messages: %v", msgs)
	}

	onCreateIdx := indexOfMsg(msgs, func(m string) bool { return m == "Running onCreateCommand..." })
	updateIdx := indexOfMsg(msgs, func(m string) bool { return m == "Running updateContentCommand..." })

	if onCreateIdx < 0 {
		t.Fatalf("Running onCreateCommand... not in messages: %v", msgs)
	}
	if readyIdx <= onCreateIdx {
		t.Errorf("Container ready. (idx %d) should come after onCreateCommand (idx %d)", readyIdx, onCreateIdx)
	}
	if updateIdx >= 0 && updateIdx <= readyIdx {
		t.Errorf("Running updateContentCommand... (idx %d) should come after Container ready. (idx %d)", updateIdx, readyIdx)
	}
}

func TestRunLifecycleHooks_WaitFor_PostCreate(t *testing.T) {
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)
	var msgs []string
	r.progress = collectProgress(&msgs)

	cfg := &config.DevContainerConfig{}
	cfg.WaitFor = "postCreateCommand"
	cfg.PostCreateCommand = config.LifecycleHook{"": {"echo postcreate"}}
	cfg.PostStartCommand = config.LifecycleHook{"": {"echo poststart"}}

	if err := r.runLifecycleHooks(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runLifecycleHooks: %v", err)
	}

	readyIdx := indexOfMsg(msgs, func(m string) bool { return m == "Container ready." })
	if readyIdx < 0 {
		t.Fatalf("Container ready. not in messages: %v", msgs)
	}

	postCreateIdx := indexOfMsg(msgs, func(m string) bool { return m == "Running postCreateCommand..." })
	postStartIdx := indexOfMsg(msgs, func(m string) bool { return m == "Running postStartCommand..." })

	if postCreateIdx < 0 {
		t.Fatalf("Running postCreateCommand... not in messages: %v", msgs)
	}
	if readyIdx <= postCreateIdx {
		t.Errorf("Container ready. (idx %d) should come after postCreateCommand (idx %d)", readyIdx, postCreateIdx)
	}
	if postStartIdx >= 0 && postStartIdx <= readyIdx {
		t.Errorf("Running postStartCommand... (idx %d) should come after Container ready. (idx %d)", postStartIdx, readyIdx)
	}
}

func TestRunLifecycleHooks_WaitFor_PostStart(t *testing.T) {
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)
	var msgs []string
	r.progress = collectProgress(&msgs)

	cfg := &config.DevContainerConfig{}
	cfg.WaitFor = "postStartCommand"
	cfg.PostStartCommand = config.LifecycleHook{"": {"echo poststart"}}
	cfg.PostAttachCommand = config.LifecycleHook{"": {"echo postattach"}}

	if err := r.runLifecycleHooks(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runLifecycleHooks: %v", err)
	}

	readyIdx := indexOfMsg(msgs, func(m string) bool { return m == "Container ready." })
	if readyIdx < 0 {
		t.Fatalf("Container ready. not in messages: %v", msgs)
	}

	postStartIdx := indexOfMsg(msgs, func(m string) bool { return m == "Running postStartCommand..." })
	postAttachIdx := indexOfMsg(msgs, func(m string) bool { return m == "Running postAttachCommand..." })

	if postStartIdx < 0 {
		t.Fatalf("Running postStartCommand... not in messages: %v", msgs)
	}
	if readyIdx <= postStartIdx {
		t.Errorf("Container ready. (idx %d) should come after postStartCommand (idx %d)", readyIdx, postStartIdx)
	}
	if postAttachIdx >= 0 && postAttachIdx <= readyIdx {
		t.Errorf("Running postAttachCommand... (idx %d) should come after Container ready. (idx %d)", postAttachIdx, readyIdx)
	}
}

func TestRunLifecycleHooks_NoReadyWhenNoHooks(t *testing.T) {
	// When there are no hooks at all, "Container ready." is still emitted
	// (at the waitFor stage, even if nothing ran there).
	mock := &mockDriver{}
	r, _, _ := newTestRunner(t, mock)
	var msgs []string
	r.progress = collectProgress(&msgs)

	cfg := &config.DevContainerConfig{}
	// No hooks configured; waitFor defaults to updateContentCommand.

	if err := r.runLifecycleHooks(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runLifecycleHooks: %v", err)
	}

	readyIdx := indexOfMsg(msgs, func(m string) bool { return m == "Container ready." })
	if readyIdx < 0 {
		t.Errorf("Container ready. should be emitted even when no hooks run: %v", msgs)
	}
}
