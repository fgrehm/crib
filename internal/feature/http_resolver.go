package feature

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// HTTPResolver resolves features from HTTPS URLs pointing to tar.gz archives.
type HTTPResolver struct {
	Cache  *FeatureCache
	Client *http.Client // nil uses http.DefaultClient
}

// Resolve downloads and caches the feature tarball at the given HTTPS URL.
// configDir is unused for HTTP refs but kept for interface compatibility.
func (r *HTTPResolver) Resolve(url, _ string) (string, error) {
	if strings.HasPrefix(url, "http://") {
		return "", fmt.Errorf("plain HTTP not supported, use HTTPS: %q", url)
	}
	if !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("HTTPResolver requires an https:// URL, got %q", url)
	}

	key := httpCacheKey(url)

	if path, ok := r.Cache.Get(key); ok {
		return path, nil
	}

	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return "", fmt.Errorf("downloading feature from %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading feature from %q: HTTP %d", url, resp.StatusCode)
	}

	path, err := r.Cache.Store(key, func(dir string) error {
		return extractHTTPArchive(resp.Body, url, resp.Header.Get("Content-Type"), dir)
	})
	if err != nil {
		return "", fmt.Errorf("caching HTTP feature %q: %w", url, err)
	}

	// Validate the extracted feature.
	featureFile := filepath.Join(path, FeatureFileName)
	if _, err := os.Stat(featureFile); err != nil {
		return "", fmt.Errorf("HTTP feature %q missing %s after extraction", url, FeatureFileName)
	}

	return path, nil
}

// extractHTTPArchive extracts the archive from r into dir.
// The format is inferred from the URL suffix or Content-Type header.
func extractHTTPArchive(r io.Reader, url, contentType string, dir string) error {
	if isTarGz(url, contentType) {
		return extractTarGz(r, dir)
	}
	if strings.HasSuffix(url, ".tar") {
		return extractTar(r, dir)
	}
	// Default to tar.gz.
	return extractTarGz(r, dir)
}

// isTarGz returns true if the URL or Content-Type indicates a gzip-compressed tar.
func isTarGz(url, contentType string) bool {
	return strings.HasSuffix(url, ".tar.gz") ||
		strings.HasSuffix(url, ".tgz") ||
		strings.Contains(contentType, "gzip") ||
		strings.Contains(contentType, "x-tar")
}

// httpCacheKey produces a filesystem-safe cache key from an HTTPS URL.
// Uses the first 16 hex characters of the URL's SHA-256 as a compact key.
func httpCacheKey(url string) string {
	sum := sha256.Sum256([]byte(url))
	return "http/" + fmt.Sprintf("%x", sum[:8])
}
