package engine

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/globalconfig"
	"github.com/fgrehm/crib/internal/plugin"
	"github.com/fgrehm/crib/internal/plugin/dotfiles"
	"github.com/fgrehm/crib/internal/workspace"
)

// mountPath is the path inside the container where the local dotfiles repo is mounted.
const dotfilesSourceMount = "/dotfiles-source"

// setupLocalDotfilesRepo creates a local git repo with an install.sh that
// touches a marker file inside the container when executed. Returns the repo
// directory, which callers should mount at dotfilesSourceMount.
func setupLocalDotfilesRepo(t *testing.T, installMarker string) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("skipping: git not available on host")
	}

	repoDir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@crib.test")
	run("config", "user.name", "Crib Test")

	installSh := fmt.Sprintf("#!/bin/sh\ntouch %s\n", installMarker)
	if err := os.WriteFile(filepath.Join(repoDir, "install.sh"), []byte(installSh), 0o755); err != nil {
		t.Fatal(err)
	}

	run("add", "install.sh")
	run("commit", "-m", "init")

	return repoDir
}

// dotfilesDevcontainerConfig returns a devcontainer.json that builds a git-enabled
// image and mounts repoDir at dotfilesSourceMount.
func dotfilesDevcontainerConfig(repoDir string) string {
	return fmt.Sprintf(`{
		"build": {"dockerfile": "Dockerfile"},
		"remoteUser": "root",
		"overrideCommand": true,
		"mounts": ["source=%s,target=%s,type=bind"]
	}`, repoDir, dotfilesSourceMount)
}

// dotfilesDockerfile builds a minimal image with git from alpine:3.20.
// Avoids alpine/git which declares /git as a VOLUME (changes there are lost
// on commit/recreate) and carries ENTRYPOINT/WorkingDir baggage.
// Configures safe.directory='*' in the image so git trusts the bind-mounted
// dotfiles source regardless of ownership (host vs container UID mismatch).
const dotfilesDockerfile = "FROM alpine:3.20\nRUN apk add --no-cache git && git config --global --add safe.directory '*'\n"

func TestIntegrationDotfilesPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	ctx := context.Background()
	e, d, _ := newTestEngine(t)
	e.SetOutput(os.Stdout, os.Stderr)

	repoDir := setupLocalDotfilesRepo(t, "/tmp/dotfiles-installed")

	mgr := plugin.NewManager(slog.Default())
	mgr.Register(dotfiles.New(globalconfig.DotfilesConfig{
		Repository: dotfilesSourceMount,
	}))
	e.SetPlugins(mgr)
	e.SetRuntime(d.Runtime().String())

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "Dockerfile"), []byte(dotfilesDockerfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(dotfilesDevcontainerConfig(repoDir)), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-dotfiles"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:           projectDir,
		DevContainerPath: ".devcontainer/devcontainer.json",
		CreatedAt:        time.Now(),
		LastUsedAt:       time.Now(),
	}

	_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
		cleanupWorkspaceImages(t, d, wsID)
	})

	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Verify the repo was cloned to the default target path.
	// Use "root" user to match remoteUser (Podman rootless defaults to a non-root user).
	var stdout bytes.Buffer
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"test", "-d", "/root/dotfiles"}, nil, &stdout, nil, nil, "root"); err != nil {
		t.Error("dotfiles not cloned: /root/dotfiles not found")
	}

	// Verify install.sh was auto-detected and executed.
	stdout.Reset()
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"test", "-f", "/tmp/dotfiles-installed"}, nil, &stdout, nil, nil, "root"); err != nil {
		t.Error("install.sh did not run: /tmp/dotfiles-installed not found")
	}
}

func TestIntegrationDotfilesPluginInstallCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	ctx := context.Background()
	e, d, _ := newTestEngine(t)
	e.SetOutput(os.Stdout, os.Stderr)

	// The repo has an install.sh, but installCommand overrides it.
	repoDir := setupLocalDotfilesRepo(t, "/tmp/autodetect-ran")

	mgr := plugin.NewManager(slog.Default())
	mgr.Register(dotfiles.New(globalconfig.DotfilesConfig{
		Repository:     dotfilesSourceMount,
		InstallCommand: "touch /tmp/custom-install-ran",
	}))
	e.SetPlugins(mgr)
	e.SetRuntime(d.Runtime().String())

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "Dockerfile"), []byte(dotfilesDockerfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(dotfilesDevcontainerConfig(repoDir)), 0o644); err != nil {
		t.Fatal(err)
	}

	wsID := "test-engine-dotfiles-cmd"
	ws := &workspace.Workspace{
		ID:               wsID,
		Source:           projectDir,
		DevContainerPath: ".devcontainer/devcontainer.json",
		CreatedAt:        time.Now(),
		LastUsedAt:       time.Now(),
	}

	_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
	t.Cleanup(func() {
		_ = d.DeleteContainer(ctx, wsID, oci.ContainerName(wsID))
		cleanupWorkspaceImages(t, d, wsID)
	})

	result, err := e.Up(ctx, ws, UpOptions{})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Verify installCommand ran.
	var stdout bytes.Buffer
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"test", "-f", "/tmp/custom-install-ran"}, nil, &stdout, nil, nil, "root"); err != nil {
		t.Error("installCommand did not run: /tmp/custom-install-ran not found")
	}

	// Verify install.sh was NOT auto-detected (installCommand takes precedence).
	stdout.Reset()
	if err := d.ExecContainer(ctx, wsID, result.ContainerID, []string{"test", "-f", "/tmp/autodetect-ran"}, nil, &stdout, nil, nil, "root"); err == nil {
		t.Error("install.sh should not have run when installCommand is set")
	}
}
