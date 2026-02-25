package config

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moby/patternmatcher"
)

// PrebuildHashParams holds all inputs for computing the prebuild hash.
type PrebuildHashParams struct {
	// Config is the devcontainer configuration.
	Config *DevContainerConfig

	// Platform is the target platform (e.g., "linux/amd64").
	Platform string

	// ContextPath is the absolute path to the build context directory.
	ContextPath string

	// DockerfileContent is the raw Dockerfile content.
	DockerfileContent string

	// IncludeFiles limits the context hash to only these files (relative paths).
	// If nil, all files in the context are included (minus .dockerignore exclusions).
	IncludeFiles []string
}

// CalculatePrebuildHash computes a deterministic hash for caching built images.
// Returns a hash in format "crib-{32 hex chars}".
func CalculatePrebuildHash(params PrebuildHashParams) (string, error) {
	arch := normalizeArchitecture(params.Platform)

	configJSON, err := normalizeConfigForHash(params.Config)
	if err != nil {
		return "", fmt.Errorf("normalizing config: %w", err)
	}

	contextHash, err := directoryHash(params.ContextPath, params.IncludeFiles)
	if err != nil {
		return "", fmt.Errorf("hashing context: %w", err)
	}

	h := sha256.New()
	h.Write([]byte(arch))
	h.Write(configJSON)
	h.Write([]byte(params.DockerfileContent))
	h.Write([]byte(contextHash))

	hash := fmt.Sprintf("%x", h.Sum(nil))
	if len(hash) > 32 {
		hash = hash[:32]
	}

	return "crib-" + hash, nil
}

func normalizeArchitecture(platform string) string {
	if platform == "" {
		return "amd64"
	}
	parts := strings.Split(platform, "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return platform
}

// normalizeConfigForHash keeps only build-relevant fields from the config.
func normalizeConfigForHash(config *DevContainerConfig) ([]byte, error) {
	normalized := struct {
		Name       string              `json:"name,omitempty"`
		Image      string              `json:"image,omitempty"`
		Dockerfile string              `json:"dockerfile,omitempty"`
		Context    string              `json:"context,omitempty"`
		Build      *ConfigBuildOptions `json:"build,omitempty"`
		Features   map[string]any      `json:"features,omitempty"`
	}{
		Name:       config.Name,
		Image:      config.Image,
		Dockerfile: config.Dockerfile,
		Context:    config.Context,
		Build:      config.Build,
		Features:   config.Features,
	}

	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// directoryHash computes a hash of all files in a directory,
// respecting .dockerignore patterns.
func directoryHash(contextDir string, includeFiles []string) (string, error) {
	if contextDir == "" {
		return "", nil
	}

	excludes, err := readDockerignore(contextDir)
	if err != nil {
		return "", err
	}

	var matcher *patternmatcher.PatternMatcher
	if len(excludes) > 0 {
		matcher, err = patternmatcher.New(excludes)
		if err != nil {
			return "", fmt.Errorf("creating pattern matcher: %w", err)
		}
	}

	// If includeFiles is specified, only hash those files.
	if len(includeFiles) > 0 {
		return hashSpecificFiles(contextDir, includeFiles)
	}

	// Otherwise, hash all files minus .dockerignore exclusions.
	return hashAllFiles(contextDir, matcher)
}

func hashAllFiles(contextDir string, matcher *patternmatcher.PatternMatcher) (string, error) {
	h := sha256.New()
	var files []string

	err := filepath.WalkDir(contextDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(contextDir, path)
		if err != nil {
			return err
		}

		if matcher != nil {
			match, err := matcher.MatchesOrParentMatches(rel)
			if err != nil {
				return err
			}
			if match {
				return nil
			}
		}

		files = append(files, rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking context: %w", err)
	}

	sort.Strings(files)

	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(contextDir, f))
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", f, err)
		}
		_, _ = fmt.Fprintf(h, "%s\n", f)
		h.Write(data)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func hashSpecificFiles(contextDir string, files []string) (string, error) {
	h := sha256.New()

	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)

	for _, f := range sorted {
		path := filepath.Join(contextDir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("reading %s: %w", f, err)
		}
		_, _ = fmt.Fprintf(h, "%s\n", f)
		h.Write(data)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func readDockerignore(contextDir string) ([]string, error) {
	path := filepath.Join(contextDir, ".dockerignore")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading .dockerignore: %w", err)
	}
	defer func() { _ = f.Close() }()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}
