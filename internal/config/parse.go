package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tidwall/jsonc"
)

// Find searches for a devcontainer.json starting from the given folder.
// Search order:
//  1. .devcontainer/devcontainer.json
//  2. .devcontainer.json
//  3. .devcontainer/{subfolder}/devcontainer.json (one level deep)
//
// Returns the absolute path to the config file, or empty string if not found.
func Find(folder string) (string, error) {
	absFolder, err := filepath.Abs(folder)
	if err != nil {
		return "", fmt.Errorf("resolving folder path: %w", err)
	}

	// 1. .devcontainer/devcontainer.json
	p := filepath.Join(absFolder, ".devcontainer", "devcontainer.json")
	if fileExists(p) {
		return p, nil
	}

	// 2. .devcontainer.json
	p = filepath.Join(absFolder, ".devcontainer.json")
	if fileExists(p) {
		return p, nil
	}

	// 3. .devcontainer/{subfolder}/devcontainer.json (one level deep)
	devcontainerDir := filepath.Join(absFolder, ".devcontainer")
	entries, err := os.ReadDir(devcontainerDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			p = filepath.Join(devcontainerDir, entry.Name(), "devcontainer.json")
			if fileExists(p) {
				return p, nil
			}
		}
	}

	return "", nil
}

// Parse reads and parses a devcontainer.json file at the given path.
// Supports JSONC (comments and trailing commas).
func Parse(path string) (*DevContainerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	config, err := ParseBytes(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	config.Origin = absPath

	return config, nil
}

// ParseBytes parses devcontainer.json content from bytes.
// Supports JSONC (comments and trailing commas).
func ParseBytes(data []byte) (*DevContainerConfig, error) {
	// Strip JSONC comments and trailing commas.
	cleaned := jsonc.ToJSON(data)

	var config DevContainerConfig
	if err := json.Unmarshal(cleaned, &config); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	replaceLegacy(&config)

	return &config, nil
}

// FindAndParse finds a devcontainer.json from the given folder and parses it.
// Returns ErrNotFound if no config file is found.
func FindAndParse(folder string) (*DevContainerConfig, error) {
	path, err := Find(folder)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, ErrNotFound
	}
	return Parse(path)
}

// replaceLegacy migrates deprecated fields to their modern equivalents.
// - extensions -> customizations.vscode.extensions
// - settings -> customizations.vscode.settings
func replaceLegacy(config *DevContainerConfig) {
	if len(config.Extensions) == 0 && len(config.Settings) == 0 {
		return
	}

	if config.Customizations == nil {
		config.Customizations = make(map[string]any)
	}

	vscode, ok := config.Customizations["vscode"].(map[string]any)
	if !ok {
		vscode = make(map[string]any)
	}

	if len(config.Extensions) > 0 {
		vscode["extensions"] = config.Extensions
		config.Extensions = nil
	}

	if len(config.Settings) > 0 {
		vscode["settings"] = config.Settings
		config.Settings = nil
	}

	config.Customizations["vscode"] = vscode
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
