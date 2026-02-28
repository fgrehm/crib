package engine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestRunInitializeCommand_String(t *testing.T) {
	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "init-ran")

	e := &Engine{
		logger: slog.Default(),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	ws := &workspace.Workspace{Source: tmpDir}
	cfg := &config.DevContainerConfig{}
	cfg.InitializeCommand = config.LifecycleHook{
		"": {"touch " + marker},
	}

	if err := e.runInitializeCommand(context.Background(), ws, cfg); err != nil {
		t.Fatalf("runInitializeCommand: %v", err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("expected marker file %s to exist: %v", marker, err)
	}
}

func TestRunInitializeCommand_Array(t *testing.T) {
	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "init-array-ran")

	e := &Engine{
		logger: slog.Default(),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	ws := &workspace.Workspace{Source: tmpDir}
	cfg := &config.DevContainerConfig{}
	cfg.InitializeCommand = config.LifecycleHook{
		"": {"touch", marker},
	}

	if err := e.runInitializeCommand(context.Background(), ws, cfg); err != nil {
		t.Fatalf("runInitializeCommand: %v", err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("expected marker file %s to exist: %v", marker, err)
	}
}

func TestRunInitializeCommand_Empty(t *testing.T) {
	e := &Engine{
		logger: slog.Default(),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	ws := &workspace.Workspace{Source: t.TempDir()}
	cfg := &config.DevContainerConfig{}

	if err := e.runInitializeCommand(context.Background(), ws, cfg); err != nil {
		t.Fatalf("expected no error for empty hook, got: %v", err)
	}
}

func TestRunInitializeCommand_Failure(t *testing.T) {
	e := &Engine{
		logger: slog.Default(),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	ws := &workspace.Workspace{Source: t.TempDir()}
	cfg := &config.DevContainerConfig{}
	cfg.InitializeCommand = config.LifecycleHook{
		"": {"false"},
	}

	err := e.runInitializeCommand(context.Background(), ws, cfg)
	if err == nil {
		t.Fatal("expected error for failing command, got nil")
	}
}

func TestRunInitializeCommand_Object_AllEntriesRun(t *testing.T) {
	// Object-form: named entries run in parallel, all must execute.
	tmpDir := t.TempDir()
	markerA := filepath.Join(tmpDir, "hook-a")
	markerB := filepath.Join(tmpDir, "hook-b")

	e := &Engine{
		logger: slog.Default(),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	ws := &workspace.Workspace{Source: tmpDir}
	cfg := &config.DevContainerConfig{}
	cfg.InitializeCommand = config.LifecycleHook{
		"hook-a": {"touch " + markerA},
		"hook-b": {"touch " + markerB},
	}

	if err := e.runInitializeCommand(context.Background(), ws, cfg); err != nil {
		t.Fatalf("runInitializeCommand: %v", err)
	}

	if _, err := os.Stat(markerA); err != nil {
		t.Errorf("hook-a did not run: %v", err)
	}
	if _, err := os.Stat(markerB); err != nil {
		t.Errorf("hook-b did not run: %v", err)
	}
}

func TestRunInitializeCommand_Object_FailureReturnsError(t *testing.T) {
	e := &Engine{
		logger: slog.Default(),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	ws := &workspace.Workspace{Source: t.TempDir()}
	cfg := &config.DevContainerConfig{}
	cfg.InitializeCommand = config.LifecycleHook{
		"ok":   {"true"},
		"fail": {"false"},
	}

	if err := e.runInitializeCommand(context.Background(), ws, cfg); err == nil {
		t.Fatal("expected error when a named entry fails, got nil")
	}
}

func TestRunInitializeCommand_WorkingDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "pwd-check")

	e := &Engine{
		logger: slog.Default(),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	ws := &workspace.Workspace{Source: tmpDir}
	cfg := &config.DevContainerConfig{}
	// Use pwd to verify working directory, write it to a file.
	cfg.InitializeCommand = config.LifecycleHook{
		"": {"sh -c 'pwd > " + marker + "'"},
	}

	if err := e.runInitializeCommand(context.Background(), ws, cfg); err != nil {
		t.Fatalf("runInitializeCommand: %v", err)
	}

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("reading marker file: %v", err)
	}

	got := string(data)
	// Trim newline.
	got = got[:len(got)-1]
	if got != tmpDir {
		t.Errorf("working directory = %q, want %q", got, tmpDir)
	}
}
