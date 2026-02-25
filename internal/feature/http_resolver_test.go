package feature

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// buildFeatureTarGz creates an in-memory .tar.gz archive containing a single
// devcontainer-feature.json file with the given content.
func buildFeatureTarGz(t *testing.T, featureJSON string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	data := []byte(featureJSON)
	hdr := &tar.Header{
		Name: FeatureFileName,
		Mode: 0o644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestHTTPResolverDownload(t *testing.T) {
	const featureJSON = `{"id":"node","version":"1.0.0"}`
	archive := buildFeatureTarGz(t, featureJSON)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	}))
	t.Cleanup(srv.Close)

	cache := NewFeatureCacheAt(t.TempDir())
	resolver := &HTTPResolver{
		Cache:  cache,
		Client: srv.Client(), // pre-configured to trust the TLS test cert
	}

	url := srv.URL + "/features/node.tar.gz"
	path, err := resolver.Resolve(url, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(path, FeatureFileName))
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(got) != featureJSON {
		t.Errorf("content = %q, want %q", string(got), featureJSON)
	}
}

func TestHTTPResolverRejectsHTTP(t *testing.T) {
	cache := NewFeatureCacheAt(t.TempDir())
	resolver := &HTTPResolver{Cache: cache}

	_, err := resolver.Resolve("http://example.com/feature.tar.gz", "")
	if err == nil {
		t.Fatal("expected error for plain http:// URL")
	}
}

func TestHTTPResolverCacheHit(t *testing.T) {
	const featureJSON = `{"id":"go","version":"1.0.0"}`
	archive := buildFeatureTarGz(t, featureJSON)

	callCount := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	}))
	t.Cleanup(srv.Close)

	cache := NewFeatureCacheAt(t.TempDir())
	resolver := &HTTPResolver{
		Cache:  cache,
		Client: srv.Client(), // pre-configured to trust the TLS test cert
	}

	url := srv.URL + "/features/go.tar.gz"

	// First call — downloads.
	if _, err := resolver.Resolve(url, ""); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	// Second call — should use cache, not re-download.
	if _, err := resolver.Resolve(url, ""); err != nil {
		t.Fatalf("second Resolve: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 HTTP request, got %d", callCount)
	}
}
