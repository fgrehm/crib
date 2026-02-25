package workspace

import "time"

// Workspace represents a crib workspace, which maps a local project
// directory to a devcontainer.
type Workspace struct {
	// ID is the workspace identifier, derived from the project directory name.
	ID string `json:"id"`

	// Source is the absolute path to the project root directory.
	Source string `json:"source"`

	// DevContainerPath is the relative path to the devcontainer config
	// from the project root (e.g., ".devcontainer/devcontainer.json").
	DevContainerPath string `json:"devContainerPath,omitempty"`

	// CreatedAt is when this workspace was first created.
	CreatedAt time.Time `json:"createdAt"`

	// LastUsedAt is when this workspace was last accessed.
	LastUsedAt time.Time `json:"lastUsedAt"`
}
