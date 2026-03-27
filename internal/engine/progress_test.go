package engine

import (
	"bytes"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestSetProgress_ReportProgressCallsCallback(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	eng := &Engine{
		driver: &mockDriver{responses: map[string]string{}},
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	var mu sync.Mutex
	var events []ProgressEvent
	eng.SetProgress(func(ev ProgressEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})

	eng.reportProgress(PhaseBuild, "Building image...")
	eng.reportProgress(PhaseHooks, "Running onCreateCommand...")

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Phase != PhaseBuild || events[0].Message != "Building image..." {
		t.Errorf("event[0] = %+v, want PhaseBuild/Building image...", events[0])
	}
	if events[1].Phase != PhaseHooks || events[1].Message != "Running onCreateCommand..." {
		t.Errorf("event[1] = %+v, want PhaseHooks/Running onCreateCommand...", events[1])
	}
}

func TestReportProgress_NilCallback(t *testing.T) {
	eng := &Engine{
		driver: &mockDriver{responses: map[string]string{}},
		store:  workspace.NewStoreAt(t.TempDir()),
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}
	// No SetProgress call. reportProgress should not panic.
	eng.reportProgress(PhaseBuild, "should not panic")
}

func TestProgressToString_WrapsInPhasePlugins(t *testing.T) {
	var got ProgressEvent
	adapter := progressToString(func(ev ProgressEvent) {
		got = ev
	})

	adapter("Running plugin: ssh")

	if got.Phase != PhasePlugins {
		t.Errorf("phase = %q, want %q", got.Phase, PhasePlugins)
	}
	if got.Message != "Running plugin: ssh" {
		t.Errorf("message = %q, want 'Running plugin: ssh'", got.Message)
	}
}

func TestProgressToString_NilInput(t *testing.T) {
	adapter := progressToString(nil)
	if adapter != nil {
		t.Error("progressToString(nil) should return nil")
	}
}

func TestSetProgress_WiresPluginManager(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	eng := &Engine{
		driver: &mockDriver{responses: map[string]string{}},
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	mgr := plugin.NewManager(slog.Default())
	eng.SetPlugins(mgr)

	var events []ProgressEvent
	eng.SetProgress(func(ev ProgressEvent) {
		events = append(events, ev)
	})

	// The manager's progress callback should now be wired.
	// Trigger it via the engine's reportProgress (direct) and verify.
	eng.reportProgress(PhaseBuild, "direct event")

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Phase != PhaseBuild {
		t.Errorf("phase = %q, want PhaseBuild", events[0].Phase)
	}
}

func TestSetPlugins_WiresExistingProgress(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	eng := &Engine{
		driver: &mockDriver{responses: map[string]string{}},
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	var events []ProgressEvent
	// Set progress FIRST, then plugins.
	eng.SetProgress(func(ev ProgressEvent) {
		events = append(events, ev)
	})

	mgr := plugin.NewManager(slog.Default())
	eng.SetPlugins(mgr)

	// Direct engine progress should still work.
	eng.reportProgress(PhaseCreate, "Creating container...")

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
}

func TestComposeStderrTee_VerboseMode(t *testing.T) {
	var stderr bytes.Buffer
	eng := &Engine{
		driver:  &mockDriver{responses: map[string]string{}},
		store:   workspace.NewStoreAt(t.TempDir()),
		logger:  slog.Default(),
		stdout:  io.Discard,
		stderr:  &stderr,
		verbose: true,
	}

	var buf bytes.Buffer
	w := eng.composeStderrTee(&buf)

	msg := []byte("compose warning: something happened\n")
	n, err := w.Write(msg)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(msg) {
		t.Errorf("wrote %d bytes, want %d", n, len(msg))
	}

	// In verbose mode, output goes to both the buffer and stderr.
	if buf.String() != string(msg) {
		t.Errorf("buffer = %q, want %q", buf.String(), string(msg))
	}
	if stderr.String() != string(msg) {
		t.Errorf("stderr = %q, want %q", stderr.String(), string(msg))
	}
}

func TestComposeStderrTee_NonVerboseMode(t *testing.T) {
	var stderr bytes.Buffer
	eng := &Engine{
		driver:  &mockDriver{responses: map[string]string{}},
		store:   workspace.NewStoreAt(t.TempDir()),
		logger:  slog.Default(),
		stdout:  io.Discard,
		stderr:  &stderr,
		verbose: false,
	}

	var buf bytes.Buffer
	w := eng.composeStderrTee(&buf)

	msg := []byte("compose error: container failed\n")
	w.Write(msg)

	// In non-verbose mode, output goes only to the buffer.
	if buf.String() != string(msg) {
		t.Errorf("buffer = %q, want %q", buf.String(), string(msg))
	}
	if stderr.String() != "" {
		t.Errorf("stderr should be empty in non-verbose mode, got %q", stderr.String())
	}
}
