package workspace

import "encoding/json"

// Result stores the outcome of a successful `crib up` run.
type Result struct {
	// ContainerID is the Docker/Podman container ID.
	ContainerID string `json:"containerID"`

	// ImageName is the name of the built/pulled image.
	ImageName string `json:"imageName"`

	// MergedConfig is the devcontainer config after merging with image metadata.
	// Stored as raw JSON to avoid a dependency on the config package.
	MergedConfig json.RawMessage `json:"mergedConfig"`

	// WorkspaceFolder is the path inside the container where the project is mounted.
	WorkspaceFolder string `json:"workspaceFolder"`

	// RemoteEnv holds the resolved remoteEnv variables from devcontainer.json.
	// ${containerEnv:VAR} references are already substituted.
	// These should be injected via -e flags when running docker/podman exec.
	RemoteEnv map[string]string `json:"remoteEnv,omitempty"`

	// RemoteUser is the user to run commands as inside the container.
	// Passed as -u to docker/podman exec.
	RemoteUser string `json:"remoteUser,omitempty"`
}
