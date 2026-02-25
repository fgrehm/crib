package feature

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Resolver resolves a feature ID to a local folder path containing the
// feature's files (devcontainer-feature.json, install.sh, etc.).
type Resolver interface {
	// Resolve returns the local folder path for the given feature ID.
	// configDir is the directory containing the devcontainer.json that
	// references this feature.
	Resolve(featureID, configDir string) (string, error)
}

// LocalResolver resolves features specified as relative paths (./ or ../).
type LocalResolver struct{}

// Resolve handles feature IDs that are relative paths. It resolves them
// relative to the configDir and verifies the directory exists.
func (r *LocalResolver) Resolve(featureID, configDir string) (string, error) {
	if !strings.HasPrefix(featureID, "./") && !strings.HasPrefix(featureID, "../") {
		return "", fmt.Errorf("LocalResolver only handles relative paths (./ or ../), got %q", featureID)
	}

	resolved := filepath.Join(configDir, featureID)
	resolved = filepath.Clean(resolved)

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("resolving feature %q: %w", featureID, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("feature path %q is not a directory", resolved)
	}

	return resolved, nil
}

// CompositeResolver dispatches to the appropriate resolver based on the
// feature reference format.
type CompositeResolver struct {
	Local *LocalResolver
	OCI   *OCIResolver
	HTTP  *HTTPResolver
}

// NewCompositeResolver creates a CompositeResolver backed by the given cache.
func NewCompositeResolver(cache *FeatureCache) *CompositeResolver {
	return &CompositeResolver{
		Local: &LocalResolver{},
		OCI:   &OCIResolver{Cache: cache},
		HTTP:  &HTTPResolver{Cache: cache},
	}
}

// Resolve dispatches to the correct resolver based on the ref format.
func (r *CompositeResolver) Resolve(ref, configDir string) (string, error) {
	switch {
	case strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../"):
		return r.Local.Resolve(ref, configDir)
	case isOCIRef(ref):
		return r.OCI.Resolve(ref, configDir)
	case strings.HasPrefix(ref, "https://"):
		return r.HTTP.Resolve(ref, configDir)
	case strings.HasPrefix(ref, "http://"):
		return "", fmt.Errorf("plain HTTP not supported, use HTTPS: %q", ref)
	default:
		return "", fmt.Errorf("unknown feature ref format: %q", ref)
	}
}

// isOCIRef returns true for refs like "ghcr.io/org/repo:tag".
// A ref is treated as OCI when it has no URL scheme, looks like host/path,
// and the first path segment contains a dot or colon (indicating a hostname).
func isOCIRef(ref string) bool {
	// Must not have a URL scheme.
	if strings.Contains(ref, "://") {
		return false
	}
	// Must not be a relative path.
	if strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") {
		return false
	}

	// Extract the first segment (everything before the first '/').
	firstSlash := strings.Index(ref, "/")
	if firstSlash == -1 {
		return false
	}
	host := ref[:firstSlash]

	// A hostname contains a dot (e.g. "ghcr.io") or a colon for a port
	// (e.g. "localhost:5000"). Plain names like "myfeature" are not OCI refs.
	for _, ch := range host {
		if ch == '.' || ch == ':' {
			return true
		}
		// Reject anything that looks like a filesystem path component.
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '-' && ch != '_' {
			return false
		}
	}
	return false
}
