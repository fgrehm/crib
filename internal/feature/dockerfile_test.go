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

	content, prefix := GenerateDockerfile(features, "vscode", "vscode")

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

	content, _ := GenerateDockerfile(features, "root", "root")

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

	content, _ := GenerateDockerfile(features, "root", "root")

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

	_, prefix := GenerateDockerfile(features, "", "")

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

	content, _ := GenerateDockerfile(features, "vscode", "vscode")

	if !strings.Contains(content, "USER root") {
		t.Error("missing USER root for feature installation")
	}
	if !strings.Contains(content, "USER $_DEV_CONTAINERS_IMAGE_USER") {
		t.Error("missing user restore at end")
	}
}
