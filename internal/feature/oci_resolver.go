package feature

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// OCIResolver resolves features from OCI registries.
// It caches resolved features to avoid redundant pulls.
type OCIResolver struct {
	Cache *FeatureCache
}

// Resolve downloads and caches the feature at the given OCI ref.
// configDir is unused for OCI refs but kept for interface compatibility.
func (r *OCIResolver) Resolve(ref, configDir string) (string, error) {
	return r.resolveWithOptions(ref, configDir, remote.WithAuthFromKeychain(authn.DefaultKeychain))
}

// resolveWithOptions is the internal implementation that accepts custom remote
// options. Used directly in tests to inject a local registry transport.
func (r *OCIResolver) resolveWithOptions(ref, _ string, opts ...remote.Option) (string, error) {
	key := ociCacheKey(ref)

	if path, ok := r.Cache.Get(key); ok {
		return path, nil
	}

	parsed, err := name.ParseReference(ref, name.Insecure)
	if err != nil {
		return "", fmt.Errorf("parsing OCI ref %q: %w", ref, err)
	}

	img, err := remote.Image(parsed, opts...)
	if err != nil {
		return "", fmt.Errorf("pulling OCI image %q: %w", ref, err)
	}

	path, err := r.Cache.Store(key, func(dir string) error {
		return extractOCIImage(img, dir)
	})
	if err != nil {
		return "", fmt.Errorf("caching OCI feature %q: %w", ref, err)
	}

	// Validate the extracted feature.
	featureFile := filepath.Join(path, FeatureFileName)
	if _, err := os.Stat(featureFile); err != nil {
		return "", fmt.Errorf("OCI feature %q missing %s after extraction", ref, FeatureFileName)
	}

	return path, nil
}

// extractOCIImage extracts all layers of img into dir, merging their contents.
func extractOCIImage(img v1.Image, dir string) error {
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("getting image layers: %w", err)
	}

	for _, layer := range layers {
		rc, err := layer.Uncompressed()
		if err != nil {
			return fmt.Errorf("getting uncompressed layer: %w", err)
		}
		if err := extractTar(rc, dir); err != nil {
			_ = rc.Close()
			return err
		}
		_ = rc.Close()
	}
	return nil
}

// extractTar extracts a tar archive from r into dir.
// Entries outside dir (via path traversal) are silently skipped.
func extractTar(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		// Strip leading "./" and "/" from the path.
		entryPath := strings.TrimPrefix(filepath.Clean("/"+hdr.Name), "/")
		if entryPath == "" || entryPath == "." {
			continue
		}

		target := filepath.Join(dir, entryPath)

		// Guard against path traversal.
		if !strings.HasPrefix(target, filepath.Clean(dir)+string(os.PathSeparator)) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("creating parent dir for %s: %w", target, err)
			}
			//nolint:gosec // file mode comes from the tar header
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("creating file %s: %w", target, err)
			}
			if _, err := io.Copy(f, tr); err != nil { //nolint:gosec
				_ = f.Close()
				return fmt.Errorf("writing file %s: %w", target, err)
			}
			_ = f.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("creating parent dir for symlink %s: %w", target, err)
			}
			_ = os.Remove(target) // ignore error if target doesn't exist
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("creating symlink %s -> %s: %w", target, hdr.Linkname, err)
			}
		}
	}
	return nil
}

// extractTarGz extracts a gzip-compressed tar archive from r into dir.
func extractTarGz(r io.Reader, dir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()
	return extractTar(gz, dir)
}
