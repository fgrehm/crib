package feature

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FeatureCache is a disk cache for resolved features stored under
// ~/.crib/feature-cache/ (or $CRIB_HOME/feature-cache/).
type FeatureCache struct {
	baseDir string
}

// NewFeatureCache creates a FeatureCache at the default location.
// The CRIB_HOME env var overrides the base directory: $CRIB_HOME/feature-cache.
func NewFeatureCache() (*FeatureCache, error) {
	var baseDir string
	if cribHome := os.Getenv("CRIB_HOME"); cribHome != "" {
		baseDir = filepath.Join(cribHome, "feature-cache")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".crib", "feature-cache")
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating feature cache directory: %w", err)
	}

	return &FeatureCache{baseDir: baseDir}, nil
}

// NewFeatureCacheAt creates a FeatureCache at a custom directory. Useful for testing.
func NewFeatureCacheAt(dir string) *FeatureCache {
	return &FeatureCache{baseDir: dir}
}

// Path returns the absolute path for a given cache key (may not exist).
func (c *FeatureCache) Path(key string) string {
	return filepath.Join(c.baseDir, filepath.FromSlash(key))
}

// Get checks if a cached entry exists for the given key.
// Returns the path and true if the entry exists, or empty string and false if not.
func (c *FeatureCache) Get(key string) (string, bool) {
	p := c.Path(key)
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return p, true
}

// Store creates the cache directory for the given key, calls populate to
// fill it, and rolls back by removing the directory on error.
func (c *FeatureCache) Store(key string, populate func(dir string) error) (string, error) {
	p := c.Path(key)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", fmt.Errorf("creating cache dir for %q: %w", key, err)
	}

	if err := populate(p); err != nil {
		_ = os.RemoveAll(p)
		return "", err
	}

	return p, nil
}

// ociCacheKey converts an OCI ref like "ghcr.io/org/repo:tag" to a safe
// filesystem path key like "ghcr.io/org/repo/tag".
func ociCacheKey(ref string) string {
	// Replace the last colon (tag separator) with a slash to avoid colons
	// in path components. Only the tag colon needs replacing — registry ports
	// are part of the hostname and appear before the first slash.
	idx := strings.LastIndex(ref, ":")
	if idx == -1 {
		return ref
	}
	// Ensure we are replacing the tag colon, not a port in the hostname.
	// The hostname is everything before the first '/'.
	firstSlash := strings.Index(ref, "/")
	if firstSlash != -1 && idx < firstSlash {
		// The colon is part of host:port, not a tag.
		return ref
	}
	return ref[:idx] + "/" + ref[idx+1:]
}
