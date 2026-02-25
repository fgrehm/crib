package feature

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalResolverRelativePath(t *testing.T) {
	base := t.TempDir()
	featureDir := filepath.Join(base, "my-feature")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}

	r := &LocalResolver{}
	result, err := r.Resolve("./my-feature", base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != featureDir {
		t.Errorf("got %q, want %q", result, featureDir)
	}
}

func TestLocalResolverParentPath(t *testing.T) {
	base := t.TempDir()
	featureDir := filepath.Join(base, "features", "my-feature")
	configDir := filepath.Join(base, "config")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	r := &LocalResolver{}
	result, err := r.Resolve("../features/my-feature", configDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != featureDir {
		t.Errorf("got %q, want %q", result, featureDir)
	}
}

func TestLocalResolverMissingFolder(t *testing.T) {
	base := t.TempDir()

	r := &LocalResolver{}
	_, err := r.Resolve("./nonexistent", base)
	if err == nil {
		t.Fatal("expected error for missing folder")
	}
}

func TestLocalResolverNonRelativePath(t *testing.T) {
	r := &LocalResolver{}
	_, err := r.Resolve("ghcr.io/devcontainers/features/node:1", "/some/dir")
	if err == nil {
		t.Fatal("expected error for non-relative path")
	}
}

func TestLocalResolverHTTPPath(t *testing.T) {
	r := &LocalResolver{}
	_, err := r.Resolve("https://example.com/feature.tgz", "/some/dir")
	if err == nil {
		t.Fatal("expected error for HTTP path")
	}
}

func TestLocalResolverFileNotDir(t *testing.T) {
	base := t.TempDir()
	filePath := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &LocalResolver{}
	_, err := r.Resolve("./not-a-dir", base)
	if err == nil {
		t.Fatal("expected error when path is a file, not a directory")
	}
}

func TestIsOCIRef(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"ghcr.io/devcontainers/features/go:1", true},
		{"registry.example.com/org/repo:latest", true},
		{"localhost:5000/features/go:1", true},
		{"./features/node", false},
		{"../features/node", false},
		{"https://example.com/feature.tar.gz", false},
		{"http://example.com/feature.tar.gz", false},
		{"just-a-name", false},
		{"no-dot/path:tag", false},
	}
	for _, tc := range tests {
		got := isOCIRef(tc.ref)
		if got != tc.want {
			t.Errorf("isOCIRef(%q) = %v, want %v", tc.ref, got, tc.want)
		}
	}
}

func TestCompositeResolverDispatch(t *testing.T) {
	base := t.TempDir()

	// Create a local feature directory for the local case.
	featureDir := filepath.Join(base, "my-feature")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cache := NewFeatureCacheAt(t.TempDir())
	resolver := NewCompositeResolver(cache)

	t.Run("local", func(t *testing.T) {
		path, err := resolver.Resolve("./my-feature", base)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != featureDir {
			t.Errorf("got %q, want %q", path, featureDir)
		}
	})

	t.Run("plain http rejected", func(t *testing.T) {
		_, err := resolver.Resolve("http://example.com/feature.tar.gz", base)
		if err == nil {
			t.Fatal("expected error for plain http://")
		}
	})

	t.Run("unknown format rejected", func(t *testing.T) {
		_, err := resolver.Resolve("just-a-name", base)
		if err == nil {
			t.Fatal("expected error for unknown ref format")
		}
	})
}
