package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/driver"
	"github.com/fgrehm/crib/internal/feature"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestFeatureToMetadata(t *testing.T) {
	priv := true
	init := true
	f := &feature.FeatureSet{
		Config: &feature.FeatureConfig{
			ID:          "docker-in-docker",
			Entrypoint:  "/usr/local/share/docker-init.sh",
			CapAdd:      []string{"SYS_PTRACE", "NET_ADMIN"},
			SecurityOpt: []string{"seccomp=unconfined"},
			Init:        &init,
			Privileged:  &priv,
			Mounts: []config.Mount{
				{Type: "volume", Source: "dind-var-lib-docker-${devcontainerId}", Target: "/var/lib/docker"},
			},
			ContainerEnv: map[string]string{
				"DOCKER_HOST": "unix:///var/run/docker.sock",
			},
		},
	}

	m := featureToMetadata(f)

	if m.ID != "docker-in-docker" {
		t.Errorf("ID = %q, want docker-in-docker", m.ID)
	}
	if m.Entrypoint != "/usr/local/share/docker-init.sh" {
		t.Errorf("Entrypoint = %q, want /usr/local/share/docker-init.sh", m.Entrypoint)
	}
	if m.Privileged == nil || !*m.Privileged {
		t.Error("Privileged should be true")
	}
	if m.Init == nil || !*m.Init {
		t.Error("Init should be true")
	}
	if len(m.CapAdd) != 2 || m.CapAdd[0] != "SYS_PTRACE" {
		t.Errorf("CapAdd = %v, want [SYS_PTRACE NET_ADMIN]", m.CapAdd)
	}
	if len(m.SecurityOpt) != 1 || m.SecurityOpt[0] != "seccomp=unconfined" {
		t.Errorf("SecurityOpt = %v, want [seccomp=unconfined]", m.SecurityOpt)
	}
	if len(m.Mounts) != 1 || m.Mounts[0].Source != "dind-var-lib-docker-${devcontainerId}" {
		t.Errorf("Mounts = %v, want [{volume dind-var-lib-docker-${devcontainerId} /var/lib/docker}]", m.Mounts)
	}
	if len(m.ContainerEnv) != 0 {
		t.Errorf("ContainerEnv should be empty (baked into image, not runtime metadata), got %v", m.ContainerEnv)
	}
}

// Regression: feature containerEnv like PATH=/nvm/bin:${PATH} is baked into
// the image as a Dockerfile ENV instruction. featureToMetadata must NOT copy
// it to ImageMetadata, because metadata containerEnv gets passed as runtime
// -e flags (single) or compose environment (compose), which would override
// the image's correctly-expanded PATH with an unexpanded literal.
func TestFeatureToMetadata_ContainerEnvExcluded(t *testing.T) {
	f := &feature.FeatureSet{
		Config: &feature.FeatureConfig{
			ID: "node",
			ContainerEnv: map[string]string{
				"PATH": "/usr/local/share/nvm/versions/node/v22/bin:${PATH}",
			},
		},
	}

	m := featureToMetadata(f)

	if len(m.ContainerEnv) != 0 {
		t.Errorf("featureToMetadata should exclude ContainerEnv (baked into image), got %v", m.ContainerEnv)
	}

	// Verify the metadata doesn't leak into runtime opts.
	opts := &driver.RunOptions{}
	applyFeatureMetadata(opts, []*config.ImageMetadata{m}, nil)

	for _, env := range opts.Env {
		if strings.HasPrefix(env, "PATH=") {
			t.Errorf("feature containerEnv PATH leaked into runtime opts: %s", env)
		}
	}
}

func TestFeatureToMetadata_Minimal(t *testing.T) {
	f := &feature.FeatureSet{
		Config: &feature.FeatureConfig{
			ID: "go",
		},
	}

	m := featureToMetadata(f)

	if m.ID != "go" {
		t.Errorf("ID = %q, want go", m.ID)
	}
	if m.Entrypoint != "" {
		t.Errorf("Entrypoint should be empty, got %q", m.Entrypoint)
	}
	if m.Privileged != nil {
		t.Error("Privileged should be nil")
	}
	if m.Init != nil {
		t.Error("Init should be nil")
	}
	if len(m.CapAdd) != 0 {
		t.Errorf("CapAdd should be empty, got %v", m.CapAdd)
	}
	if len(m.Mounts) != 0 {
		t.Errorf("Mounts should be empty, got %v", m.Mounts)
	}
}

func TestFeatureToMetadata_LifecycleHooks(t *testing.T) {
	f := &feature.FeatureSet{
		Config: &feature.FeatureConfig{
			ID:                "test-feature",
			OnCreateCommand:   config.LifecycleHook{"": {"echo oncreate"}},
			PostCreateCommand: config.LifecycleHook{"": {"echo postcreate"}},
			PostStartCommand:  config.LifecycleHook{"": {"echo poststart"}},
			PostAttachCommand: config.LifecycleHook{"": {"echo postattach"}},
		},
	}

	m := featureToMetadata(f)

	if len(m.OnCreateCommand) == 0 {
		t.Error("OnCreateCommand should be set")
	}
	if len(m.PostCreateCommand) == 0 {
		t.Error("PostCreateCommand should be set")
	}
	if len(m.PostStartCommand) == 0 {
		t.Error("PostStartCommand should be set")
	}
	if len(m.PostAttachCommand) == 0 {
		t.Error("PostAttachCommand should be set")
	}
}

func TestResolveFeatureMetadata_NoFeatures(t *testing.T) {
	eng := &Engine{logger: slog.Default()}
	cfg := &config.DevContainerConfig{}

	got := eng.resolveFeatureMetadata(cfg)
	if got != nil {
		t.Errorf("resolveFeatureMetadata with no features should return nil, got %v", got)
	}
}

func TestCleanupPreviousBuildImage_NewHashReplacesOld(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	md := &imageTrackingDriver{}
	eng := &Engine{driver: md, store: store, logger: slog.Default()}

	oldImage := "crib-myws:crib-oldhash"
	if err := store.SaveResult("myws", &workspace.Result{ImageName: oldImage}); err != nil {
		t.Fatal(err)
	}

	eng.cleanupPreviousBuildImage(context.Background(), "myws", "crib-myws:crib-newhash")

	if len(md.removedImages) != 1 || md.removedImages[0] != oldImage {
		t.Errorf("removedImages = %v, want [%s]", md.removedImages, oldImage)
	}
}

func TestCleanupPreviousBuildImage_CacheHit_NoRemoval(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	md := &imageTrackingDriver{}
	eng := &Engine{driver: md, store: store, logger: slog.Default()}

	sameImage := "crib-myws:crib-samehash"
	if err := store.SaveResult("myws", &workspace.Result{ImageName: sameImage}); err != nil {
		t.Fatal(err)
	}

	eng.cleanupPreviousBuildImage(context.Background(), "myws", sameImage)

	if len(md.removedImages) != 0 {
		t.Errorf("removedImages = %v, want none (cache hit)", md.removedImages)
	}
}

func TestCleanupPreviousBuildImage_BaseImage_NotRemoved(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	md := &imageTrackingDriver{}
	eng := &Engine{driver: md, store: store, logger: slog.Default()}

	if err := store.SaveResult("myws", &workspace.Result{ImageName: "ubuntu:22.04"}); err != nil {
		t.Fatal(err)
	}

	eng.cleanupPreviousBuildImage(context.Background(), "myws", "crib-myws:crib-newhash")

	if len(md.removedImages) != 0 {
		t.Errorf("removedImages = %v, want none (base image)", md.removedImages)
	}
}

func TestCleanupPreviousBuildImage_RemoveFailure_NoError(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	md := &imageTrackingDriver{removeErr: fmt.Errorf("image in use")}
	eng := &Engine{driver: md, store: store, logger: slog.Default()}

	oldImage := "crib-myws:crib-oldhash"
	if err := store.SaveResult("myws", &workspace.Result{ImageName: oldImage}); err != nil {
		t.Fatal(err)
	}

	// Should not panic or propagate the error.
	eng.cleanupPreviousBuildImage(context.Background(), "myws", "crib-myws:crib-newhash")

	if len(md.removedImages) != 1 {
		t.Errorf("removedImages = %v, want 1 attempt even on failure", md.removedImages)
	}
}

func TestCleanupPreviousBuildImage_FirstBuild_NoRemoval(t *testing.T) {
	store := workspace.NewStoreAt(t.TempDir())
	md := &imageTrackingDriver{}
	eng := &Engine{driver: md, store: store, logger: slog.Default()}

	// No stored result exists.
	eng.cleanupPreviousBuildImage(context.Background(), "myws", "crib-myws:crib-first")

	if len(md.removedImages) != 0 {
		t.Errorf("removedImages = %v, want none (first build)", md.removedImages)
	}
}

func TestResolveComposeContainerUser(t *testing.T) {
	tests := []struct {
		name          string
		containerUser string
		remoteUser    string
		serviceUser   string
		want          string
	}{
		{"containerUser wins", "vscode", "dev", "svc", "vscode"},
		{"remoteUser second", "", "dev", "svc", "dev"},
		{"serviceUser third", "", "", "svc", "svc"},
		{"defaults to root", "", "", "", "root"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := &Engine{driver: &mockDriver{}, logger: slog.Default()}
			cfg := &config.DevContainerConfig{}
			cfg.ContainerUser = tt.containerUser
			cfg.RemoteUser = tt.remoteUser

			got := eng.resolveComposeContainerUser(context.Background(), cfg, tt.serviceUser, "")
			if got != tt.want {
				t.Errorf("resolveComposeContainerUser = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveContainerUser_FromConfig(t *testing.T) {
	tests := []struct {
		name          string
		containerUser string
		remoteUser    string
		want          string
	}{
		{"containerUser set", "vscode", "", "vscode"},
		{"remoteUser only", "", "dev", "dev"},
		{"both set - containerUser wins", "vscode", "dev", "vscode"},
		{"neither set - defaults to root", "", "", "root"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.DevContainerConfig{}
			cfg.ContainerUser = tt.containerUser
			cfg.RemoteUser = tt.remoteUser

			got := resolveContainerUser(cfg)
			if got != tt.want {
				t.Errorf("resolveContainerUser = %q, want %q", got, tt.want)
			}
		})
	}
}
