package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCalculatePrebuildHash_StableForSameInputs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM ubuntu:22.04\nRUN echo hello")
	writeFile(t, filepath.Join(dir, "app.go"), "package main")

	params := PrebuildHashParams{
		Config:            &DevContainerConfig{ImageContainer: ImageContainer{Image: "ubuntu"}},
		Platform:          "linux/amd64",
		ContextPath:       dir,
		DockerfileContent: "FROM ubuntu:22.04\nRUN echo hello",
	}

	hash1, err := CalculatePrebuildHash(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := CalculatePrebuildHash(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("hashes should be stable: %q != %q", hash1, hash2)
	}
}

func TestCalculatePrebuildHash_Prefix(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "file.txt"), "content")

	hash, err := CalculatePrebuildHash(PrebuildHashParams{
		Config:      &DevContainerConfig{},
		ContextPath: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(hash, "crib-") {
		t.Errorf("hash should start with 'crib-', got %q", hash)
	}
}

func TestCalculatePrebuildHash_Length(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "file.txt"), "content")

	hash, err := CalculatePrebuildHash(PrebuildHashParams{
		Config:      &DevContainerConfig{},
		ContextPath: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// "crib-" (5) + 32 hex chars = 37
	if len(hash) != 37 {
		t.Errorf("hash length = %d, want 37 (got %q)", len(hash), hash)
	}
}

func TestCalculatePrebuildHash_ChangesWithDockerfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "file.txt"), "content")

	hash1, err := CalculatePrebuildHash(PrebuildHashParams{
		Config:            &DevContainerConfig{},
		ContextPath:       dir,
		DockerfileContent: "FROM ubuntu:22.04",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := CalculatePrebuildHash(PrebuildHashParams{
		Config:            &DevContainerConfig{},
		ContextPath:       dir,
		DockerfileContent: "FROM ubuntu:24.04",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("hash should change when Dockerfile content changes")
	}
}

func TestCalculatePrebuildHash_ChangesWithBuildArgs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "file.txt"), "content")

	s1 := "v1"
	hash1, err := CalculatePrebuildHash(PrebuildHashParams{
		Config: &DevContainerConfig{
			DockerfileContainer: DockerfileContainer{
				Build: &ConfigBuildOptions{Args: map[string]*string{"VERSION": &s1}},
			},
		},
		ContextPath: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s2 := "v2"
	hash2, err := CalculatePrebuildHash(PrebuildHashParams{
		Config: &DevContainerConfig{
			DockerfileContainer: DockerfileContainer{
				Build: &ConfigBuildOptions{Args: map[string]*string{"VERSION": &s2}},
			},
		},
		ContextPath: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("hash should change when build args change")
	}
}

func TestNormalizeArchitecture(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"linux/amd64", "amd64"},
		{"linux/arm64", "arm64"},
		{"amd64", "amd64"},
		{"", "amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeArchitecture(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDockerignoreRespected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.go"), "package main")
	writeFile(t, filepath.Join(dir, "temp.log"), "some log data")
	writeFile(t, filepath.Join(dir, ".dockerignore"), "*.log\n")

	// Hash with .dockerignore
	hash1, err := CalculatePrebuildHash(PrebuildHashParams{
		Config:      &DevContainerConfig{},
		ContextPath: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that changing the excluded file doesn't change the hash.
	writeFile(t, filepath.Join(dir, ".dockerignore"), "*.log\n")
	writeFile(t, filepath.Join(dir, "temp.log"), "different log data")

	hash3, err := CalculatePrebuildHash(PrebuildHashParams{
		Config:      &DevContainerConfig{},
		ContextPath: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 != hash3 {
		t.Error("hash should not change when excluded file changes")
	}
}

func TestCalculatePrebuildHash_IncludeFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.go"), "package main")
	writeFile(t, filepath.Join(dir, "other.go"), "package other")

	// Hash with only app.go.
	hash1, err := CalculatePrebuildHash(PrebuildHashParams{
		Config:       &DevContainerConfig{},
		ContextPath:  dir,
		IncludeFiles: []string{"app.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Change other.go, hash should be the same.
	writeFile(t, filepath.Join(dir, "other.go"), "package changed")

	hash2, err := CalculatePrebuildHash(PrebuildHashParams{
		Config:       &DevContainerConfig{},
		ContextPath:  dir,
		IncludeFiles: []string{"app.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 != hash2 {
		t.Error("hash should not change when non-included file changes")
	}

	// Change app.go, hash should change.
	writeFile(t, filepath.Join(dir, "app.go"), "package updated")

	hash3, err := CalculatePrebuildHash(PrebuildHashParams{
		Config:       &DevContainerConfig{},
		ContextPath:  dir,
		IncludeFiles: []string{"app.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 == hash3 {
		t.Error("hash should change when included file changes")
	}
}

func TestCalculatePrebuildHash_EmptyContextPath(t *testing.T) {
	hash, err := CalculatePrebuildHash(PrebuildHashParams{
		Config:            &DevContainerConfig{ImageContainer: ImageContainer{Image: "ubuntu"}},
		DockerfileContent: "FROM ubuntu",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(hash, "crib-") {
		t.Errorf("hash should start with 'crib-', got %q", hash)
	}
}
