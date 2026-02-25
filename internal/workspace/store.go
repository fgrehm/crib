package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	workspaceConfigFile = "workspace.json"
	workspaceResultFile = "result.json"
)

// ErrWorkspaceNotFound is returned when a workspace does not exist in the store.
var ErrWorkspaceNotFound = errors.New("workspace not found")

// Store manages workspace state on disk at a base directory.
type Store struct {
	baseDir string
}

// NewStore creates a Store at the default location (~/.crib/workspaces).
// The CRIB_HOME env var overrides the base directory: $CRIB_HOME/workspaces.
func NewStore() (*Store, error) {
	var baseDir string
	if cribHome := os.Getenv("CRIB_HOME"); cribHome != "" {
		baseDir = filepath.Join(cribHome, "workspaces")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".crib", "workspaces")
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating workspaces directory: %w", err)
	}

	return &Store{baseDir: baseDir}, nil
}

// NewStoreAt creates a Store with a custom base directory. Useful for testing.
func NewStoreAt(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// Save writes a workspace config to disk.
func (s *Store) Save(ws *Workspace) error {
	dir := s.workspaceDir(ws.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating workspace directory: %w", err)
	}

	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling workspace: %w", err)
	}

	path := filepath.Join(dir, workspaceConfigFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing workspace config: %w", err)
	}

	return nil
}

// Load reads a workspace config from disk.
func (s *Store) Load(id string) (*Workspace, error) {
	path := filepath.Join(s.workspaceDir(id), workspaceConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrWorkspaceNotFound
		}
		return nil, fmt.Errorf("reading workspace config: %w", err)
	}

	var ws Workspace
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("unmarshaling workspace: %w", err)
	}

	return &ws, nil
}

// Delete removes a workspace directory from disk.
func (s *Store) Delete(id string) error {
	dir := s.workspaceDir(id)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("deleting workspace: %w", err)
	}
	return nil
}

// List returns all known workspace IDs.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}

	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Only include directories that contain a workspace.json.
		path := filepath.Join(s.baseDir, entry.Name(), workspaceConfigFile)
		if _, err := os.Stat(path); err == nil {
			ids = append(ids, entry.Name())
		}
	}
	return ids, nil
}

// Exists checks if a workspace exists on disk.
func (s *Store) Exists(id string) bool {
	path := filepath.Join(s.workspaceDir(id), workspaceConfigFile)
	_, err := os.Stat(path)
	return err == nil
}

// SaveResult writes a build result to disk.
func (s *Store) SaveResult(id string, result *Result) error {
	dir := s.workspaceDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating workspace directory: %w", err)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}

	path := filepath.Join(dir, workspaceResultFile)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing result: %w", err)
	}

	return nil
}

// LoadResult reads a build result from disk. Returns nil, nil if not found.
func (s *Store) LoadResult(id string) (*Result, error) {
	path := filepath.Join(s.workspaceDir(id), workspaceResultFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading result: %w", err)
	}

	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshaling result: %w", err)
	}

	return &result, nil
}

// MarkHookDone records that a lifecycle hook has been executed for a workspace.
func (s *Store) MarkHookDone(id, hookName string) error {
	dir := filepath.Join(s.workspaceDir(id), "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}
	path := filepath.Join(dir, hookName+".done")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return fmt.Errorf("writing hook marker: %w", err)
	}
	return nil
}

// IsHookDone checks whether a lifecycle hook has already been executed.
func (s *Store) IsHookDone(id, hookName string) bool {
	path := filepath.Join(s.workspaceDir(id), "hooks", hookName+".done")
	_, err := os.Stat(path)
	return err == nil
}

// ClearHookMarkers removes all lifecycle hook markers for a workspace,
// allowing hooks to run again (used on recreate).
func (s *Store) ClearHookMarkers(id string) error {
	dir := filepath.Join(s.workspaceDir(id), "hooks")
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clearing hook markers: %w", err)
	}
	return nil
}

func (s *Store) workspaceDir(id string) string {
	return filepath.Join(s.baseDir, id)
}
