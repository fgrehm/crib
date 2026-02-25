package feature

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFeatureCacheGetMiss(t *testing.T) {
	cache := NewFeatureCacheAt(t.TempDir())
	_, ok := cache.Get("does/not/exist")
	if ok {
		t.Fatal("expected Get to return false for missing key")
	}
}

func TestFeatureCacheGetHit(t *testing.T) {
	dir := t.TempDir()
	cache := NewFeatureCacheAt(dir)

	const key = "ghcr.io/devcontainers/features/go/1"
	_, err := cache.Store(key, func(d string) error {
		return os.WriteFile(filepath.Join(d, FeatureFileName), []byte(`{"id":"go"}`), 0o644)
	})
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	got, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected Get to return true after Store")
	}
	want := cache.Path(key)
	if got != want {
		t.Errorf("got path %q, want %q", got, want)
	}
}

func TestFeatureCacheStoreRollback(t *testing.T) {
	dir := t.TempDir()
	cache := NewFeatureCacheAt(dir)

	const key = "some/feature"
	populateErr := errors.New("populate failed")

	_, err := cache.Store(key, func(d string) error {
		return populateErr
	})
	if !errors.Is(err, populateErr) {
		t.Fatalf("expected populate error, got: %v", err)
	}

	// Directory should have been removed.
	if _, statErr := os.Stat(cache.Path(key)); !os.IsNotExist(statErr) {
		t.Error("expected cache dir to be removed after rollback")
	}
}

func TestOCICacheKey(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{
			ref:  "ghcr.io/devcontainers/features/go:1",
			want: "ghcr.io/devcontainers/features/go/1",
		},
		{
			ref:  "ghcr.io/devcontainers/features/node:latest",
			want: "ghcr.io/devcontainers/features/node/latest",
		},
		{
			ref:  "ghcr.io/devcontainers/features/go",
			want: "ghcr.io/devcontainers/features/go",
		},
		{
			ref:  "registry.example.com:5000/features/go:1",
			want: "registry.example.com:5000/features/go/1",
		},
	}
	for _, tc := range tests {
		got := ociCacheKey(tc.ref)
		if got != tc.want {
			t.Errorf("ociCacheKey(%q) = %q, want %q", tc.ref, got, tc.want)
		}
	}
}
