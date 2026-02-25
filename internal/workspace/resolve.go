package workspace

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fgrehm/crib/internal/config"
)

// ErrNoDevContainer is returned when no devcontainer configuration is found
// walking up from the start directory.
var ErrNoDevContainer = errors.New("no devcontainer configuration found")

// ResolveResult holds the outcome of workspace resolution.
type ResolveResult struct {
	// ProjectRoot is the absolute path to the project root directory.
	ProjectRoot string

	// ConfigPath is the absolute path to the devcontainer.json file.
	ConfigPath string

	// RelativeConfigPath is the config path relative to ProjectRoot.
	RelativeConfigPath string

	// WorkspaceID is the derived workspace identifier.
	WorkspaceID string
}

// Resolve walks up from startDir looking for a .devcontainer/ directory
// or .devcontainer.json file. Returns the project root, config path, and
// derived workspace ID.
func Resolve(startDir string) (*ResolveResult, error) {
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("resolving start directory: %w", err)
	}

	dir := absDir
	for {
		configPath, err := config.Find(dir)
		if err != nil {
			return nil, fmt.Errorf("searching for devcontainer config: %w", err)
		}
		if configPath != "" {
			relPath, err := filepath.Rel(dir, configPath)
			if err != nil {
				return nil, fmt.Errorf("computing relative config path: %w", err)
			}
			return &ResolveResult{
				ProjectRoot:        dir,
				ConfigPath:         configPath,
				RelativeConfigPath: relPath,
				WorkspaceID:        Slugify(filepath.Base(dir)),
			}, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root.
			return nil, ErrNoDevContainer
		}
		dir = parent
	}
}

// ResolveConfigDir resolves workspace info when the devcontainer config directory
// is explicitly given (bypasses the walk-up). The config directory must contain
// a devcontainer.json directly. The project root is the parent of configDir.
func ResolveConfigDir(configDir string) (*ResolveResult, error) {
	absDir, err := filepath.Abs(configDir)
	if err != nil {
		return nil, fmt.Errorf("resolving config dir: %w", err)
	}

	configPath := filepath.Join(absDir, "devcontainer.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no devcontainer.json found in %s", absDir)
	} else if err != nil {
		return nil, fmt.Errorf("checking devcontainer.json: %w", err)
	}

	projectRoot := filepath.Dir(absDir)
	relPath, err := filepath.Rel(projectRoot, configPath)
	if err != nil {
		return nil, fmt.Errorf("computing relative config path: %w", err)
	}

	return &ResolveResult{
		ProjectRoot:        projectRoot,
		ConfigPath:         configPath,
		RelativeConfigPath: relPath,
		WorkspaceID:        Slugify(filepath.Base(projectRoot)),
	}, nil
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9-]+`)

// Slugify converts a project directory name into a valid workspace ID.
// Rules: lowercase, replace non-alphanumeric with hyphens, trim hyphens,
// truncate to 48 chars with hash suffix if longer.
func Slugify(name string) string {
	slug := strings.ToLower(name)
	slug = nonAlphanumeric.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")

	if slug == "" {
		slug = "workspace"
	}

	const maxLen = 48
	if len(slug) > maxLen {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(name)))
		slug = slug[:40] + "-" + hash[:7]
	}

	return slug
}
