package feature

import (
	"archive/tar"
	"bytes"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// buildFeatureImage creates a minimal OCI image containing a
// devcontainer-feature.json at the root of a single layer.
func buildFeatureImage(t *testing.T, featureJSON string) v1.Image {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	data := []byte(featureJSON)
	hdr := &tar.Header{
		Name: FeatureFileName,
		Mode: 0o644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("writing tar header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("writing tar content: %v", err)
	}
	_ = tw.Close()

	content := buf.Bytes()
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content)), nil
	})
	if err != nil {
		t.Fatalf("creating layer: %v", err)
	}

	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		t.Fatalf("appending layer: %v", err)
	}
	return img
}

func TestOCIResolverCacheHit(t *testing.T) {
	cacheDir := t.TempDir()
	cache := NewFeatureCacheAt(cacheDir)

	const key = "registry.example.com/features/go/1"
	// Pre-populate cache.
	if _, err := cache.Store(key, func(d string) error {
		return os.WriteFile(filepath.Join(d, FeatureFileName), []byte(`{"id":"go"}`), 0o644)
	}); err != nil {
		t.Fatal(err)
	}

	resolver := &OCIResolver{Cache: cache}
	// Resolve with a ref that maps to the cached key.
	path, err := resolver.Resolve("registry.example.com/features/go:1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != cache.Path(key) {
		t.Errorf("got path %q, want %q", path, cache.Path(key))
	}
}

func TestOCIResolverDownload(t *testing.T) {
	// Start a local OCI registry.
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)

	const featureJSON = `{"id":"go","version":"1.0.0"}`
	img := buildFeatureImage(t, featureJSON)

	// Push the image to the local registry.
	host := srv.Listener.Addr().String()
	ref := host + "/features/go:1"
	parsed, err := name.ParseReference(ref, name.Insecure)
	if err != nil {
		t.Fatalf("parsing ref: %v", err)
	}
	if err := remote.Write(parsed, img,
		remote.WithTransport(srv.Client().Transport),
		remote.WithAuth(authn.Anonymous),
	); err != nil {
		t.Fatalf("pushing image: %v", err)
	}

	cacheDir := t.TempDir()
	cache := NewFeatureCacheAt(cacheDir)
	resolver := &OCIResolver{Cache: cache}

	path, err := resolver.resolveWithOptions(ref, "",
		remote.WithTransport(srv.Client().Transport),
		remote.WithAuth(authn.Anonymous),
	)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Verify devcontainer-feature.json was extracted.
	featureFile := filepath.Join(path, FeatureFileName)
	got, err := os.ReadFile(featureFile)
	if err != nil {
		t.Fatalf("reading extracted feature file: %v", err)
	}
	if string(got) != featureJSON {
		t.Errorf("feature file content = %q, want %q", string(got), featureJSON)
	}
}
