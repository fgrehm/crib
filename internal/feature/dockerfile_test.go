package feature

import (
	"strings"
	"testing"
)

func TestGenerateDockerfileSingle(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "my-feature",
			Config: &FeatureConfig{
				ID: "my-feature",
			},
		},
	}

	content, prefix := GenerateDockerfile(features, "vscode", "vscode", nil)

	// Prefix checks.
	if !strings.Contains(prefix, "# syntax=docker.io/docker/dockerfile:1.4") {
		t.Error("prefix missing syntax directive")
	}
	if !strings.Contains(prefix, "ARG _DEV_CONTAINERS_BASE_IMAGE=placeholder") {
		t.Error("prefix missing base image ARG")
	}

	// Content checks.
	if !strings.Contains(content, "FROM $_DEV_CONTAINERS_BASE_IMAGE AS dev_containers_base_stage") {
		t.Error("content missing FROM with base stage")
	}
	if !strings.Contains(content, "FROM dev_containers_base_stage AS dev_containers_target_stage") {
		t.Error("content missing FROM with target stage")
	}
	if !strings.Contains(content, "USER root") {
		t.Error("content missing USER root")
	}
	if !strings.Contains(content, "COPY .crib-features/ /tmp/build-features/") {
		t.Error("content missing COPY features")
	}
	if !strings.Contains(content, "devcontainer-features-install.sh") {
		t.Error("content missing install script reference")
	}
	if !strings.Contains(content, "ARG _DEV_CONTAINERS_IMAGE_USER=vscode") {
		t.Error("content missing user restore ARG")
	}
	if !strings.Contains(content, "USER $_DEV_CONTAINERS_IMAGE_USER") {
		t.Error("content missing user restore")
	}
}

func TestGenerateDockerfileMultiple(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "feature-a",
			Config:   &FeatureConfig{ID: "feature-a"},
		},
		{
			ConfigID: "feature-b",
			Config:   &FeatureConfig{ID: "feature-b"},
		},
	}

	content, _ := GenerateDockerfile(features, "root", "root", nil)

	// Both features should have numbered install scripts.
	if !strings.Contains(content, "/tmp/build-features/0/devcontainer-features-install.sh") {
		t.Error("missing feature 0 install script")
	}
	if !strings.Contains(content, "/tmp/build-features/1/devcontainer-features-install.sh") {
		t.Error("missing feature 1 install script")
	}
}

func TestGenerateDockerfileContainerEnv(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "my-feature",
			Config: &FeatureConfig{
				ID: "my-feature",
				ContainerEnv: map[string]string{
					"MY_VAR": "my_value",
				},
			},
		},
	}

	content, _ := GenerateDockerfile(features, "root", "root", nil)

	if !strings.Contains(content, `ENV MY_VAR="my_value"`) {
		t.Errorf("content missing ENV instruction, got:\n%s", content)
	}
}

func TestGenerateDockerfilePrefix(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "test",
			Config:   &FeatureConfig{ID: "test"},
		},
	}

	_, prefix := GenerateDockerfile(features, "", "", nil)

	lines := strings.Split(strings.TrimSpace(prefix), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 prefix lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "# syntax=docker.io/docker/dockerfile:1.4" {
		t.Errorf("line 0 = %q", lines[0])
	}
	if lines[1] != "ARG _DEV_CONTAINERS_BASE_IMAGE=placeholder" {
		t.Errorf("line 1 = %q", lines[1])
	}
}

func TestGenerateDockerfileUserVariables(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "test",
			Config:   &FeatureConfig{ID: "test"},
		},
	}

	content, _ := GenerateDockerfile(features, "vscode", "vscode", nil)

	if !strings.Contains(content, "USER root") {
		t.Error("missing USER root for feature installation")
	}
	if !strings.Contains(content, "USER $_DEV_CONTAINERS_IMAGE_USER") {
		t.Error("missing user restore at end")
	}
}

func TestGenerateDockerfileCacheMounts(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "test",
			Config:   &FeatureConfig{ID: "test"},
		},
	}

	mounts := []string{"/var/cache/apt", "/var/lib/apt/lists", "/root/.npm"}
	content, _ := GenerateDockerfile(features, "root", "root", mounts)

	// Each cache mount should appear on the RUN line.
	for _, m := range mounts {
		want := "--mount=type=cache,target=" + m
		if !strings.Contains(content, want) {
			t.Errorf("missing cache mount %q in:\n%s", want, content)
		}
	}

	// The bind mount should still be there.
	if !strings.Contains(content, "--mount=type=bind,from=dev_containers_base_stage") {
		t.Error("missing bind mount")
	}
}

func TestGenerateDockerfileCacheMountsAptDisablesDockerClean(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "test",
			Config:   &FeatureConfig{ID: "test"},
		},
	}

	// With apt cache: should disable docker-clean.
	content, _ := GenerateDockerfile(features, "root", "root", []string{"/var/cache/apt"})
	if !strings.Contains(content, "rm -f /etc/apt/apt.conf.d/docker-clean") {
		t.Error("expected docker-clean removal with apt cache")
	}

	// Without apt: no docker-clean removal.
	content, _ = GenerateDockerfile(features, "root", "root", []string{"/root/.npm"})
	if strings.Contains(content, "docker-clean") {
		t.Error("unexpected docker-clean removal without apt cache")
	}

	// No cache mounts: no docker-clean removal.
	content, _ = GenerateDockerfile(features, "root", "root", nil)
	if strings.Contains(content, "docker-clean") {
		t.Error("unexpected docker-clean removal with nil cache mounts")
	}
}

func TestGenerateDockerfileSingleEntrypoint(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "docker-in-docker",
			Config: &FeatureConfig{
				ID:         "docker-in-docker",
				Entrypoint: "/usr/local/share/docker-init.sh",
			},
		},
	}

	content, _ := GenerateDockerfile(features, "root", "root", nil)

	if !strings.Contains(content, `ENTRYPOINT ["/usr/local/share/docker-init.sh"]`) {
		t.Errorf("missing single ENTRYPOINT instruction in:\n%s", content)
	}
}

func TestGenerateDockerfileMultipleEntrypoints(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "feature-a",
			Config: &FeatureConfig{
				ID:         "feature-a",
				Entrypoint: "/entry-a.sh",
			},
		},
		{
			ConfigID: "feature-b",
			Config: &FeatureConfig{
				ID:         "feature-b",
				Entrypoint: "/entry-b.sh",
			},
		},
	}

	content, _ := GenerateDockerfile(features, "root", "root", nil)

	// Later features wrap earlier ones (outermost runs first).
	if !strings.Contains(content, "crib-entrypoint.sh") {
		t.Errorf("missing wrapper entrypoint script in:\n%s", content)
	}
	// The wrapper should chain: exec /entry-b.sh /entry-a.sh "$@"
	if !strings.Contains(content, "/entry-b.sh /entry-a.sh") {
		t.Errorf("expected /entry-b.sh to wrap /entry-a.sh in:\n%s", content)
	}
	if !strings.Contains(content, `ENTRYPOINT ["/usr/local/share/crib-entrypoint.sh"]`) {
		t.Errorf("missing wrapper ENTRYPOINT in:\n%s", content)
	}
}

func TestGenerateDockerfileNoEntrypoint(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "plain",
			Config:   &FeatureConfig{ID: "plain"},
		},
	}

	content, _ := GenerateDockerfile(features, "root", "root", nil)

	if strings.Contains(content, "ENTRYPOINT") {
		t.Errorf("unexpected ENTRYPOINT for features without entrypoints:\n%s", content)
	}
}

func TestGenerateDockerfileNoCacheMountsWithoutProviders(t *testing.T) {
	features := []*FeatureSet{
		{
			ConfigID: "test",
			Config:   &FeatureConfig{ID: "test"},
		},
	}

	content, _ := GenerateDockerfile(features, "root", "root", nil)

	// Should not have any cache mounts.
	if strings.Contains(content, "type=cache") {
		t.Errorf("unexpected cache mount in:\n%s", content)
	}
}
