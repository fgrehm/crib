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

	// SnapshotImage is the name of the snapshot image created after create-time
	// hooks completed. Used by restart to skip re-running those hooks.
	SnapshotImage string `json:"snapshotImage,omitempty"`

	// SnapshotHookHash is a hash of the create-time hook definitions at the time
	// the snapshot was taken. If hooks change, the snapshot is stale.
	SnapshotHookHash string `json:"snapshotHookHash,omitempty"`

	// HasFeatureEntrypoints is true when the image was built with features
	// that declare entrypoints (e.g. docker-in-docker). Used by restart
	// paths to know whether to override the container entrypoint.
	HasFeatureEntrypoints bool `json:"hasFeatureEntrypoints,omitempty"`

	// ComposeFilesHash is a hash of the compose file contents at the time
	// the result was saved. Used by restart to detect changes inside compose
	// files (volumes, ports, etc.) that are invisible to devcontainer.json
	// config comparison.
	ComposeFilesHash string `json:"composeFilesHash,omitempty"`
}
