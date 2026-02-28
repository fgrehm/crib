---
title: Remote Features Design
description: Design document for OCI and HTTP feature resolution.
---

## Problem

Currently, crib only supports **local features** (features defined as relative paths within the `.devcontainer/` directory). The feature resolver in `internal/feature/resolve.go` uses `LocalResolver` which rejects any non-relative paths.

The DevContainer specification supports fetching features from:
1. **Local paths** (`./<path>` or `../<path>`)
2. **OCI registries** (`ghcr.io/devcontainers/features/go:1`, `docker.io/my-org/features/custom:latest`)
3. **HTTP URLs** (`https://example.com/features/myfeature.tar.gz`)

This design outlines how to extend crib to support remote features from OCI registries and HTTP sources.

## Design Goals

1. **Fetch and cache** remote features locally without requiring Docker/Podman to pull them
2. **Support OCI registry authentication** (for private registries)
3. **Reuse existing infrastructure** (go-containerregistry is already a dependency)
4. **Maintain backwards compatibility** with local features
5. **Keep the dependency chain simple** (avoid adding OCI registry libraries if possible)

## Architecture

### Current Flow

```
feature ID (string)
    |
    v
LocalResolver.Resolve(featureID, configDir)
    |
    v
abs path to feature folder
    |
    v
Load devcontainer-feature.json + install.sh
    |
    v
Generate Dockerfile layers
```

### Proposed Flow

```
feature ID (string)
    |
    v
Route by pattern:
  - if "./" or "../" -> LocalResolver
  - if "ghcr.io/..." or "docker.io/..." -> OCIResolver
  - if "https://..." -> HTTPResolver
    |
    v
Fetch + extract to cache dir
    |
    v
abs path to feature folder (in cache)
    |
    v
Load devcontainer-feature.json + install.sh
    |
    v
Generate Dockerfile layers
```

## Implementation Plan

### Phase 1: OCI Registry Support

**File:** `internal/feature/resolve.go`

Add `OCIResolver` struct with these methods:

```go
type OCIResolver struct {
    Cache *FeatureCache // ~/.crib/feature-cache/
}

// Resolve(ref string, configDir string) -> (path to extracted feature, error)
// ref = "ghcr.io/devcontainers/features/go:1"
func (r *OCIResolver) Resolve(ref, configDir string) (string, error)
```

**Steps:**

1. Parse the ref using `go-containerregistry`'s image reference parser
2. Check cache: `~/.crib/feature-cache/{registry}/{org}/{repo}:{tag}/`
3. If cached and valid, return cache path
4. If not cached:
   - Authenticate if needed (`.docker/config.json` or env var)
   - Pull the image blob
   - Extract `devcontainer-feature.json` to verify it's a valid feature
   - Extract all files to cache
   - Return cache path
5. (Optional) Cache invalidation strategy: by tag (stable), or optional TTL for `latest`

**Dependencies:**

- `google/go-containerregistry` (already imported for OCI work, but may need additional imports)
- No new external dependencies required

### Phase 2: HTTP Support

**File:** `internal/feature/resolve.go`

Add `HTTPResolver`:

```go
type HTTPResolver struct {
    Cache *FeatureCache
}

func (r *HTTPResolver) Resolve(url, configDir string) (string, error)
```

**Steps:**

1. Calculate hash of URL: `hash(url)` -> cache key
2. Check cache: `~/.crib/feature-cache/http/{hash}/`
3. If cached (with Last-Modified / ETag), skip download
4. If not cached or stale:
   - Download via HTTP GET
   - Extract tarball (.tar.gz, .tgz, .tar)
   - Validate `devcontainer-feature.json` exists
   - Extract to cache
   - Return cache path

**Dependencies:**

- Standard `net/http`
- `archive/tar`, `compress/gzip` (standard library)

### Phase 3: Composite Resolver

**File:** `internal/feature/resolve.go`

Add router that dispatches to the right resolver:

```go
type FeatureResolver interface {
    Resolve(ref, configDir string) (string, error)
}

type CompositeResolver struct {
    Local *LocalResolver
    OCI   *OCIResolver
    HTTP  *HTTPResolver
}

func (r *CompositeResolver) Resolve(ref, configDir string) (string, error) {
    // Dispatch based on ref pattern
    switch {
    case strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../"):
        return r.Local.Resolve(ref, configDir)
    case isOCIRef(ref):
        return r.OCI.Resolve(ref, configDir)
    case strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://"):
        return r.HTTP.Resolve(ref, configDir)
    default:
        return "", fmt.Errorf("unknown feature ref format: %q", ref)
    }
}
```

### Phase 4: Cache Management

**File:** `internal/feature/cache.go`

```go
type FeatureCache struct {
    BaseDir string // ~/.crib/feature-cache/
}

func (c *FeatureCache) Get(key string) (string, bool) // returns path if exists
func (c *FeatureCache) Set(key string, files map[string][]byte) error // extract files
func (c *FeatureCache) Path(key string) string // returns abs path (whether exists or not)
func (c *FeatureCache) Clean() error // optional: remove stale entries
```

## Example: Remote Feature in devcontainer.json

Once implemented, users can do:

```json
{
  "name": "with-remote-features",
  "image": "docker.io/library/ubuntu:24.04",
  "features": {
    "ghcr.io/devcontainers/features/go:1": {
      "version": "1.21"
    },
    "ghcr.io/devcontainers/features/node:1": {
      "version": "20"
    },
    "https://example.com/features/custom-tool.tar.gz": {}
  }
}
```

And crib will:
1. Fetch and cache each feature
2. Extract `devcontainer-feature.json` + `install.sh` from each
3. Execute install scripts in dependency order (via `installsAfter`)
4. Generate merged Dockerfile with all feature layers

## Testing Strategy

### Unit Tests

- `TestOCIResolverLocalCache` - Verify cache hit skips download
- `TestOCIResolverRemotePull` - Mock OCI registry, verify feature extraction
- `TestHTTPResolverDownload` - Mock HTTP server, verify tarball extraction
- `TestCompositeResolverDispatch` - Verify routing to correct resolver
- `TestFeatureCacheGet/Set` - Basic cache operations

### Integration Tests

- `TestRemoteFeatureEndToEnd` - Set up test OCI registry, pull a real feature, build image
- `TestHTTPFeatureEndToEnd` - Start HTTP server with feature tarball, pull and build
- `TestAuthenticatedRegistry` - Pull from private registry (mocked)

## Backwards Compatibility

- Existing local feature examples (`with-local-features`) continue to work unchanged
- No breaking changes to `internal/feature/` public API
- `LocalResolver` remains the default for relative paths

## Security Considerations

1. **Registry Authentication:** Use `~/.docker/config.json` or `DOCKER_AUTH_*` env vars (standard Docker conventions)
2. **Feature Validation:** Always parse and validate `devcontainer-feature.json` before executing `install.sh`
3. **Cache Integrity:** Consider storing SHA256 hashes of downloaded files in cache metadata
4. **HTTP Only HTTPS:** Reject plain HTTP URLs (security best practice)
