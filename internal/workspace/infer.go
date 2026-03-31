package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// InferID derives a workspace ID from the given directories without requiring
// workspace state to exist. It first tries normal devcontainer resolution (which
// walks up to find .devcontainer/), and falls back to slugifying the directory
// name if no devcontainer config is found. This allows cache commands to work
// even if the project was deleted or was never set up with crib.
//
// Precedence: configDir > dir > cwd. When all three are empty, os.Getwd() is
// used as a fallback. Pass the working directory explicitly to avoid that call.
func InferID(configDir, dir, cwd string) (string, error) {
	switch {
	case configDir != "":
		rr, err := ResolveConfigDir(configDir)
		if err == nil {
			return rr.WorkspaceID, nil
		}
		// Only fall back when the config doesn't exist (project deleted
		// or never set up). Surface real errors (permissions, I/O).
		if !errors.Is(err, ErrNoDevContainer) {
			return "", err
		}
		absDir, err := filepath.Abs(configDir)
		if err != nil {
			return "", fmt.Errorf("resolving config dir: %w", err)
		}
		return GenerateID(filepath.Dir(absDir)), nil

	case dir != "":
		rr, err := Resolve(dir)
		if err == nil {
			return rr.WorkspaceID, nil
		}
		if !errors.Is(err, ErrNoDevContainer) {
			return "", err
		}
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("resolving dir: %w", err)
		}
		return GenerateID(absDir), nil

	default:
		if cwd == "" {
			var err error
			cwd, err = os.Getwd()
			if err != nil {
				return "", fmt.Errorf("getting working directory: %w", err)
			}
		}
		rr, err := Resolve(cwd)
		if err == nil {
			return rr.WorkspaceID, nil
		}
		if !errors.Is(err, ErrNoDevContainer) {
			return "", err
		}
		return GenerateID(cwd), nil
	}
}
