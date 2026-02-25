package oci

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
)

func newTestDriver(t *testing.T) *OCIDriver {
	t.Helper()
	d, err := NewOCIDriver(slog.Default())
	if err != nil {
		t.Skipf("skipping: no container runtime available: %v", err)
	}
	return d
}

func TestIntegrationContainerLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	d := newTestDriver(t)
	wsID := "test-lifecycle"

	// Clean up any leftover container from a previous failed run.
	_ = d.DeleteContainer(ctx, wsID, ContainerName(wsID))

	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, ContainerName(wsID))
	})

	// Run a container.
	err := d.RunContainer(ctx, wsID, &driver.RunOptions{
		Image:      "alpine:3.20",
		Entrypoint: "/bin/sh",
		Cmd:        []string{"-c", "sleep infinity"},
	})
	if err != nil {
		t.Fatalf("RunContainer: %v", err)
	}

	// Find the container.
	container, err := d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer: %v", err)
	}
	if container == nil {
		t.Fatal("FindContainer returned nil")
	}

	// Verify the container has the workspace label.
	if got := container.Config.Labels[LabelWorkspace]; got != wsID {
		t.Errorf("workspace label = %q, want %q", got, wsID)
	}

	// Verify the container is running.
	if status := strings.ToLower(container.State.Status); status != "running" {
		t.Errorf("container status = %q, want running", status)
	}

	// Stop the container.
	if err := d.StopContainer(ctx, wsID, container.ID); err != nil {
		t.Fatalf("StopContainer: %v", err)
	}

	// Verify it's stopped.
	container, err = d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer after stop: %v", err)
	}
	if container == nil {
		t.Fatal("FindContainer returned nil after stop")
	}
	if status := strings.ToLower(container.State.Status); status != "exited" {
		t.Errorf("container status after stop = %q, want exited", status)
	}

	// Start the container again.
	if err := d.StartContainer(ctx, wsID, container.ID); err != nil {
		t.Fatalf("StartContainer: %v", err)
	}

	container, err = d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer after start: %v", err)
	}
	if status := strings.ToLower(container.State.Status); status != "running" {
		t.Errorf("container status after start = %q, want running", status)
	}

	// Delete the container.
	if err := d.DeleteContainer(ctx, wsID, container.ID); err != nil {
		t.Fatalf("DeleteContainer: %v", err)
	}

	// Verify it's gone.
	container, err = d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer after delete: %v", err)
	}
	if container != nil {
		t.Error("FindContainer returned non-nil after delete")
	}
}

func TestIntegrationBuildAndInspect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	d := newTestDriver(t)
	wsID := "test-build"
	// BuildImage with no Image/PrebuildHash uses ImageName(wsID, "latest").
	imageName := ImageName(wsID, "latest")

	// Create a temporary Dockerfile.
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.20\nLABEL test=true\n"), 0o644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	t.Cleanup(func() {
		_, _ = d.helper.Output(ctx, "rmi", imageName)
	})

	// Build the image.
	err := d.BuildImage(ctx, wsID, &driver.BuildOptions{
		Dockerfile: dockerfile,
		Context:    tmpDir,
	})
	if err != nil {
		t.Fatalf("BuildImage: %v", err)
	}

	// Inspect the image.
	img, err := d.InspectImage(ctx, imageName)
	if err != nil {
		t.Fatalf("InspectImage: %v", err)
	}
	if img.ID == "" {
		t.Error("InspectImage returned empty ID")
	}
	if got := img.Config.Labels["test"]; got != "true" {
		t.Errorf("image label test = %q, want %q", got, "true")
	}
}

func TestIntegrationExec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	d := newTestDriver(t)
	wsID := "test-exec"

	_ = d.DeleteContainer(ctx, wsID, ContainerName(wsID))

	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, ContainerName(wsID))
	})

	// Run a container.
	err := d.RunContainer(ctx, wsID, &driver.RunOptions{
		Image:      "alpine:3.20",
		Entrypoint: "/bin/sh",
		Cmd:        []string{"-c", "sleep infinity"},
	})
	if err != nil {
		t.Fatalf("RunContainer: %v", err)
	}

	container, err := d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer: %v", err)
	}

	// Exec a command.
	var stdout bytes.Buffer
	err = d.ExecContainer(ctx, wsID, container.ID, []string{"echo", "hello"}, nil, &stdout, nil, nil, "")
	if err != nil {
		t.Fatalf("ExecContainer: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	if got != "hello" {
		t.Errorf("exec output = %q, want %q", got, "hello")
	}

	// Test exec with stdin.
	var stdout2 bytes.Buffer
	stdin := strings.NewReader("world\n")
	err = d.ExecContainer(ctx, wsID, container.ID, []string{"cat"}, stdin, &stdout2, nil, nil, "")
	if err != nil {
		t.Fatalf("ExecContainer with stdin: %v", err)
	}

	got = strings.TrimSpace(stdout2.String())
	if got != "world" {
		t.Errorf("exec with stdin output = %q, want %q", got, "world")
	}
}

func TestIntegrationContainerLogs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	d := newTestDriver(t)
	wsID := "test-logs"

	_ = d.DeleteContainer(ctx, wsID, ContainerName(wsID))

	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, ContainerName(wsID))
	})

	// Run a container that prints something.
	err := d.RunContainer(ctx, wsID, &driver.RunOptions{
		Image:      "alpine:3.20",
		Entrypoint: "/bin/sh",
		Cmd:        []string{"-c", "echo container-started && sleep infinity"},
	})
	if err != nil {
		t.Fatalf("RunContainer: %v", err)
	}

	container, err := d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer: %v", err)
	}

	var stdout bytes.Buffer
	err = d.ContainerLogs(ctx, wsID, container.ID, &stdout, nil)
	if err != nil {
		t.Fatalf("ContainerLogs: %v", err)
	}

	if !strings.Contains(stdout.String(), "container-started") {
		t.Errorf("logs = %q, want to contain %q", stdout.String(), "container-started")
	}
}

func TestIntegrationRunContainerWithMounts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	d := newTestDriver(t)
	wsID := "test-mounts"

	_ = d.DeleteContainer(ctx, wsID, ContainerName(wsID))

	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, ContainerName(wsID))
	})

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "testfile"), []byte("mounted"), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	err := d.RunContainer(ctx, wsID, &driver.RunOptions{
		Image:      "alpine:3.20",
		Entrypoint: "/bin/sh",
		Cmd:        []string{"-c", "sleep infinity"},
		WorkspaceMount: config.Mount{
			Type:   "bind",
			Source: tmpDir,
			Target: "/workspaces/project",
		},
	})
	if err != nil {
		t.Fatalf("RunContainer: %v", err)
	}

	container, err := d.FindContainer(ctx, wsID)
	if err != nil {
		t.Fatalf("FindContainer: %v", err)
	}

	// Verify the mount works by reading the test file.
	var stdout bytes.Buffer
	err = d.ExecContainer(ctx, wsID, container.ID, []string{"cat", "/workspaces/project/testfile"}, nil, &stdout, nil, nil, "")
	if err != nil {
		t.Fatalf("ExecContainer: %v", err)
	}

	if got := strings.TrimSpace(stdout.String()); got != "mounted" {
		t.Errorf("mounted file content = %q, want %q", got, "mounted")
	}
}

func TestIntegrationTargetArchitecture(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	d := newTestDriver(t)

	arch, err := d.TargetArchitecture(ctx)
	if err != nil {
		t.Fatalf("TargetArchitecture: %v", err)
	}

	if arch == "" {
		t.Error("TargetArchitecture returned empty string")
	}

	// Should be a recognized architecture.
	validArchs := map[string]bool{
		"amd64": true, "x86_64": true,
		"arm64": true, "aarch64": true,
		"arm": true, "armv7l": true,
		"386": true, "i386": true, "i686": true,
		"ppc64le": true, "s390x": true, "riscv64": true,
	}
	if !validArchs[arch] {
		t.Logf("unexpected architecture %q (may still be valid)", arch)
	}
}
