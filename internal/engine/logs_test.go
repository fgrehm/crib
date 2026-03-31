package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/workspace"
)

// logsMockDriver captures ContainerLogs calls for verification.
type logsMockDriver struct {
	mockDriver
	logsCalled bool
	logsOpts   *driver.LogsOptions
	container  *driver.ContainerDetails
}

func (m *logsMockDriver) FindContainer(_ context.Context, _ string) (*driver.ContainerDetails, error) {
	return m.container, nil
}

func (m *logsMockDriver) ContainerLogs(_ context.Context, _, _ string, stdout, _ io.Writer, opts *driver.LogsOptions) error {
	m.logsCalled = true
	m.logsOpts = opts
	if stdout != nil {
		io.WriteString(stdout, "test log output\n")
	}
	return nil
}

func TestLogs_SingleContainer(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-logs", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	// Save a result so Logs can load it.
	cfg := &config.DevContainerConfig{}
	cfg.Image = "ubuntu:22.04"
	mergedJSON, _ := json.Marshal(cfg)
	if err := store.SaveResult(ws.ID, &workspace.Result{
		ContainerID:  "container-1",
		MergedConfig: mergedJSON,
	}); err != nil {
		t.Fatal(err)
	}

	mockDrv := &logsMockDriver{
		container: &driver.ContainerDetails{ID: "container-1", State: driver.ContainerState{Status: "running"}},
	}

	var stdout bytes.Buffer
	eng := &Engine{
		driver: mockDrv,
		store:  store,
		logger: slog.Default(),
		stdout: &stdout,
		stderr: io.Discard,
	}

	err := eng.Logs(context.Background(), ws, LogsOptions{Follow: true, Tail: "50"})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}

	if !mockDrv.logsCalled {
		t.Error("expected ContainerLogs to be called")
	}
	if mockDrv.logsOpts == nil {
		t.Fatal("expected LogsOptions to be passed")
	}
	if !mockDrv.logsOpts.Follow {
		t.Error("expected Follow=true")
	}
	if mockDrv.logsOpts.Tail != "50" {
		t.Errorf("Tail = %q, want %q", mockDrv.logsOpts.Tail, "50")
	}
	if stdout.String() != "test log output\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "test log output\n")
	}
}

func TestLogs_ComposeMissing_ReturnsError(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-logs-compose-nil", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveResult(ws.ID, &workspace.Result{
		MergedConfig: []byte(`{"dockerComposeFile":["docker-compose.yml"],"service":"app"}`),
	}); err != nil {
		t.Fatal(err)
	}

	eng := &Engine{
		driver: &mockDriver{},
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
		// compose is nil
	}

	err := eng.Logs(context.Background(), ws, LogsOptions{})
	if err == nil {
		t.Fatal("expected error when compose is nil for compose workspace")
	}
	var target *ErrComposeNotAvailable
	if !errors.As(err, &target) {
		t.Errorf("expected ErrComposeNotAvailable, got: %v", err)
	}
}

func TestLogs_NoResult(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	ws := &workspace.Workspace{ID: "ws-no-result", Source: "/home/user/project"}
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	eng := &Engine{
		driver: &mockDriver{},
		store:  store,
		logger: slog.Default(),
		stdout: io.Discard,
		stderr: io.Discard,
	}

	err := eng.Logs(context.Background(), ws, LogsOptions{})
	if err == nil {
		t.Fatal("expected error for missing result")
	}
}
