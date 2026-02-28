package oci

import (
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/driver"
)

func TestBuildBuildArgs_DockerBuildx(t *testing.T) {
	d := newTestDockerDriver()

	opts := &driver.BuildOptions{
		Dockerfile: "/tmp/Dockerfile",
		Context:    "/tmp/context",
		Target:     "dev",
		Args:       map[string]string{"VERSION": "1.0", "DEBUG": "true"},
		CacheFrom:  []string{"type=local,src=/cache"},
	}

	args := d.buildBuildArgs("myimage:latest", opts, true)
	got := strings.Join(args, " ")

	assertContains(t, got, "buildx build --load")
	assertContains(t, got, "-f /tmp/Dockerfile")
	assertContains(t, got, "-t myimage:latest")
	assertContains(t, got, "--target dev")
	// Build args should be sorted.
	assertContains(t, got, "--build-arg DEBUG=true")
	assertContains(t, got, "--build-arg VERSION=1.0")
	assertContains(t, got, "--cache-from type=local,src=/cache")

	// Context should be last.
	if !strings.HasSuffix(got, "/tmp/context") {
		t.Errorf("expected context at end of args, got: %s", got)
	}

	// Verify sorted order: DEBUG before VERSION.
	debugIdx := strings.Index(got, "DEBUG=true")
	versionIdx := strings.Index(got, "VERSION=1.0")
	if debugIdx > versionIdx {
		t.Errorf("build args not sorted: DEBUG at %d, VERSION at %d", debugIdx, versionIdx)
	}
}

func TestBuildBuildArgs_PlainBuild(t *testing.T) {
	d := newTestDockerDriver()

	opts := &driver.BuildOptions{
		Dockerfile: "Dockerfile",
		Context:    ".",
	}

	args := d.buildBuildArgs("test:latest", opts, false)
	got := strings.Join(args, " ")

	// Should start with "build", not "buildx build".
	if !strings.HasPrefix(got, "build ") {
		t.Errorf("expected plain build, got: %s", got)
	}
	assertContains(t, got, "-f Dockerfile")
	assertContains(t, got, "-t test:latest")
}

func TestBuildBuildArgs_PodmanBuild(t *testing.T) {
	d := newTestPodmanDriver()

	opts := &driver.BuildOptions{
		Dockerfile: "Dockerfile",
		Context:    "/src",
	}

	args := d.buildBuildArgs("myimg:v1", opts, false)
	got := strings.Join(args, " ")

	if !strings.HasPrefix(got, "build ") {
		t.Errorf("podman should use plain build, got: %s", got)
	}
	assertContains(t, got, "-t myimg:v1")
}

func TestBuildBuildArgs_Minimal(t *testing.T) {
	d := newTestDockerDriver()

	opts := &driver.BuildOptions{}
	args := d.buildBuildArgs("img:latest", opts, false)
	got := strings.Join(args, " ")

	// Should have build, tag, and default context.
	if got != "build -t img:latest ." {
		t.Errorf("unexpected minimal args: %s", got)
	}
}

func TestBuildBuildArgs_WithOptions(t *testing.T) {
	d := newTestDockerDriver()

	opts := &driver.BuildOptions{
		Context: "/ctx",
		Options: []string{"--network=host", "--progress=plain"},
	}

	args := d.buildBuildArgs("img:latest", opts, false)
	got := strings.Join(args, " ")

	assertContains(t, got, "--network=host")
	assertContains(t, got, "--progress=plain")

	// Context must still be last.
	if !strings.HasSuffix(got, "/ctx") {
		t.Errorf("expected context at end, got: %s", got)
	}
}

func TestBuildBuildArgs_NoBuildArgsNoTarget(t *testing.T) {
	d := newTestDockerDriver()

	opts := &driver.BuildOptions{
		Context: "/myctx",
	}

	args := d.buildBuildArgs("img:v1", opts, false)
	got := strings.Join(args, " ")

	for _, flag := range []string{"--target", "--build-arg", "--cache-from"} {
		if strings.Contains(got, flag) {
			t.Errorf("unexpected flag %q in args: %s", flag, got)
		}
	}
}
