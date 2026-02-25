package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrNotFound is returned when no devcontainer.json is found.
var ErrNotFound = errors.New("devcontainer.json not found")

// DevContainerConfig is the top-level parsed devcontainer.json.
type DevContainerConfig struct {
	DevContainerConfigBase `json:",inline"`
	DevContainerActions    `json:",inline"`
	NonComposeBase         `json:",inline"`
	ImageContainer         `json:",inline"`
	ComposeContainer       `json:",inline"`
	DockerfileContainer    `json:",inline"`

	// Origin is the absolute path to the devcontainer.json file (not serialized).
	Origin string `json:"-"`
}

// MergedDevContainerConfig is the result of merging a base config with
// image metadata entries from features and base images.
type MergedDevContainerConfig struct {
	DevContainerConfigBase `json:",inline"`
	MergedConfigProperties `json:",inline"`
	NonComposeBase         `json:",inline"`
	ImageContainer         `json:",inline"`
	ComposeContainer       `json:",inline"`
	DockerfileContainer    `json:",inline"`

	Origin string `json:"-"`
}

// DevContainerConfigBase holds common configuration fields.
type DevContainerConfigBase struct {
	Name                        string                   `json:"name,omitempty"`
	Features                    map[string]any           `json:"features,omitempty"`
	OverrideFeatureInstallOrder []string                 `json:"overrideFeatureInstallOrder,omitempty"`
	ForwardPorts                StrIntArray              `json:"forwardPorts,omitempty"`
	PortsAttributes             map[string]PortAttribute `json:"portsAttributes,omitempty"`
	OtherPortsAttributes        *PortAttribute           `json:"otherPortsAttributes,omitempty"`
	UpdateRemoteUserUID         *bool                    `json:"updateRemoteUserUID,omitempty"`
	RemoteEnv                   map[string]string        `json:"remoteEnv,omitempty"`
	RemoteUser                  string                   `json:"remoteUser,omitempty"`
	InitializeCommand           LifecycleHook            `json:"initializeCommand,omitempty"`
	ShutdownAction              string                   `json:"shutdownAction,omitempty"`
	WaitFor                     string                   `json:"waitFor,omitempty"`
	UserEnvProbe                string                   `json:"userEnvProbe,omitempty"`
	HostRequirements            *HostRequirements        `json:"hostRequirements,omitempty"`
	OverrideCommand             *bool                    `json:"overrideCommand,omitempty"`
	WorkspaceFolder             string                   `json:"workspaceFolder,omitempty"`

	// Deprecated fields (kept for legacy replacement).
	Settings   map[string]any `json:"settings,omitempty"`
	Extensions []string       `json:"extensions,omitempty"`
	DevPort    int            `json:"devPort,omitempty"`
}

// DevContainerActions holds lifecycle hook definitions.
type DevContainerActions struct {
	OnCreateCommand      LifecycleHook  `json:"onCreateCommand,omitempty"`
	UpdateContentCommand LifecycleHook  `json:"updateContentCommand,omitempty"`
	PostCreateCommand    LifecycleHook  `json:"postCreateCommand,omitempty"`
	PostStartCommand     LifecycleHook  `json:"postStartCommand,omitempty"`
	PostAttachCommand    LifecycleHook  `json:"postAttachCommand,omitempty"`
	Customizations       map[string]any `json:"customizations,omitempty"`
}

// NonComposeBase holds container runtime configuration for non-compose scenarios.
type NonComposeBase struct {
	AppPort        StrIntArray       `json:"appPort,omitempty"`
	ContainerEnv   map[string]string `json:"containerEnv,omitempty"`
	ContainerUser  string            `json:"containerUser,omitempty"`
	Mounts         []Mount           `json:"mounts,omitempty"`
	Init           *bool             `json:"init,omitempty"`
	Privileged     *bool             `json:"privileged,omitempty"`
	CapAdd         []string          `json:"capAdd,omitempty"`
	SecurityOpt    []string          `json:"securityOpt,omitempty"`
	RunArgs        []string          `json:"runArgs,omitempty"`
	WorkspaceMount string            `json:"workspaceMount,omitempty"`
}

// ImageContainer holds the image reference for image-based devcontainers.
type ImageContainer struct {
	Image string `json:"image,omitempty"`
}

// ComposeContainer holds Docker Compose configuration.
type ComposeContainer struct {
	DockerComposeFile StrArray `json:"dockerComposeFile,omitempty"`
	Service           string   `json:"service,omitempty"`
	RunServices       []string `json:"runServices,omitempty"`
}

// DockerfileContainer holds Dockerfile-based build configuration.
type DockerfileContainer struct {
	Dockerfile string              `json:"dockerfile,omitempty"`
	Context    string              `json:"context,omitempty"`
	Build      *ConfigBuildOptions `json:"build,omitempty"`
}

// ConfigBuildOptions holds build arguments and options.
type ConfigBuildOptions struct {
	Dockerfile string             `json:"dockerfile,omitempty"`
	Context    string             `json:"context,omitempty"`
	Args       map[string]*string `json:"args,omitempty"`
	Target     string             `json:"target,omitempty"`
	CacheFrom  StrArray           `json:"cacheFrom,omitempty"`
	Options    []string           `json:"options,omitempty"`
}

// MergedConfigProperties holds accumulated lifecycle hooks and
// customizations from multiple image metadata entries.
type MergedConfigProperties struct {
	Entrypoints           []string        `json:"entrypoints,omitempty"`
	OnCreateCommands      []LifecycleHook `json:"onCreateCommands,omitempty"`
	UpdateContentCommands []LifecycleHook `json:"updateContentCommands,omitempty"`
	PostCreateCommands    []LifecycleHook `json:"postCreateCommands,omitempty"`
	PostStartCommands     []LifecycleHook `json:"postStartCommands,omitempty"`
	PostAttachCommands    []LifecycleHook `json:"postAttachCommands,omitempty"`
}

// HostRequirements defines minimum host resource requirements.
type HostRequirements struct {
	CPUs    int    `json:"cpus,omitempty"`
	Memory  string `json:"memory,omitempty"`
	Storage string `json:"storage,omitempty"`
	GPU     any    `json:"gpu,omitempty"`
}

// PortAttribute describes port forwarding metadata.
type PortAttribute struct {
	Label            string `json:"label,omitempty"`
	Protocol         string `json:"protocol,omitempty"`
	OnAutoForward    string `json:"onAutoForward,omitempty"`
	RequireLocalPort bool   `json:"requireLocalPort,omitempty"`
	ElevateIfNeeded  bool   `json:"elevateIfNeeded,omitempty"`
}

// Mount represents a volume or bind mount. It supports both string format
// ("type=bind,src=/a,dst=/b") and object format in JSON.
type Mount struct {
	Type     string `json:"type,omitempty"`
	Source   string `json:"source,omitempty"`
	Target   string `json:"target,omitempty"`
	External bool   `json:"external,omitempty"`
}

// ParseMount parses a mount string in Docker mount format.
// Example: "type=bind,src=/tmp,dst=/tmp" or "type=volume,source=mydata,target=/data".
func ParseMount(s string) Mount {
	m := Mount{}
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch k {
		case "type":
			m.Type = v
		case "src", "source":
			m.Source = v
		case "dst", "destination", "target":
			m.Target = v
		}
	}
	return m
}

// String returns the mount in Docker mount string format.
func (m Mount) String() string {
	parts := make([]string, 0, 3)
	if m.Type != "" {
		parts = append(parts, "type="+m.Type)
	}
	if m.Source != "" {
		parts = append(parts, "src="+m.Source)
	}
	if m.Target != "" {
		parts = append(parts, "dst="+m.Target)
	}
	return strings.Join(parts, ",")
}

// UnmarshalJSON handles both string format and object format for mounts.
func (m *Mount) UnmarshalJSON(data []byte) error {
	// Try string format first.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*m = ParseMount(s)
		return nil
	}

	// Fall back to object format.
	type mountAlias Mount
	var alias mountAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("mount must be a string or object: %w", err)
	}
	*m = Mount(alias)
	return nil
}

// GetContextPath returns the resolved build context path relative to
// the devcontainer.json directory.
func GetContextPath(config *DevContainerConfig) string {
	configDir := filepath.Dir(config.Origin)

	// Explicit context in build options takes priority.
	if config.Build != nil && config.Build.Context != "" {
		return filepath.Join(configDir, config.Build.Context)
	}

	// Legacy context field.
	if config.Context != "" {
		return filepath.Join(configDir, config.Context)
	}

	// Default: the directory containing devcontainer.json.
	return configDir
}

// GetDockerfilePath returns the resolved Dockerfile path.
func GetDockerfilePath(config *DevContainerConfig) string {
	configDir := filepath.Dir(config.Origin)

	if config.Build != nil && config.Build.Dockerfile != "" {
		return filepath.Join(configDir, config.Build.Dockerfile)
	}
	if config.Dockerfile != "" {
		return filepath.Join(configDir, config.Dockerfile)
	}
	return ""
}

// --- Custom JSON types ---

// StrArray accepts either a single string or an array of strings in JSON.
type StrArray []string

// UnmarshalJSON implements json.Unmarshaler.
func (sa *StrArray) UnmarshalJSON(data []byte) error {
	// Try single string first.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*sa = StrArray{s}
		return nil
	}

	// Try array of strings.
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("expected string or []string: %w", err)
	}
	*sa = arr
	return nil
}

// StrIntArray accepts a string, number, or an array of mixed string/number values.
// All values are stored as strings.
type StrIntArray []string

// UnmarshalJSON implements json.Unmarshaler.
func (sa *StrIntArray) UnmarshalJSON(data []byte) error {
	// Try single string.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*sa = StrIntArray{s}
		return nil
	}

	// Try single number.
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*sa = StrIntArray{n.String()}
		return nil
	}

	// Try array of mixed types.
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("expected string, number, or array: %w", err)
	}

	result := make(StrIntArray, 0, len(arr))
	for _, raw := range arr {
		var sv string
		if err := json.Unmarshal(raw, &sv); err == nil {
			result = append(result, sv)
			continue
		}
		var nv json.Number
		if err := json.Unmarshal(raw, &nv); err == nil {
			result = append(result, nv.String())
			continue
		}
		return fmt.Errorf("array element must be string or number: %s", string(raw))
	}
	*sa = result
	return nil
}

// LifecycleHook accepts multiple JSON formats for lifecycle commands:
//   - A string: "echo hello"
//   - An array: ["echo", "hello"]
//   - An object: {"name": "echo hello"} or {"name": ["echo", "hello"]}
//
// All forms are normalized to map[string][]string.
// For string and array forms, the key is empty string.
type LifecycleHook map[string][]string

// UnmarshalJSON implements json.Unmarshaler.
func (l *LifecycleHook) UnmarshalJSON(data []byte) error {
	// Try single string: "command"
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*l = LifecycleHook{"": {s}}
		return nil
	}

	// Try array of strings: ["cmd", "arg1"]
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*l = LifecycleHook{"": arr}
		return nil
	}

	// Try object: {"name": "cmd"} or {"name": ["cmd", "arg1"]}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("lifecycle hook must be a string, array, or object: %w", err)
	}

	result := make(LifecycleHook, len(obj))
	for k, v := range obj {
		// Try string value.
		var sv string
		if err := json.Unmarshal(v, &sv); err == nil {
			result[k] = []string{sv}
			continue
		}
		// Try array value.
		var av []string
		if err := json.Unmarshal(v, &av); err == nil {
			result[k] = av
			continue
		}
		return fmt.Errorf("lifecycle hook value for %q must be a string or array: %s", k, string(v))
	}
	*l = result
	return nil
}

// StrBool accepts either a string or bool in JSON, storing as string.
type StrBool string

// UnmarshalJSON implements json.Unmarshaler.
func (s *StrBool) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		if b {
			*s = "true"
		} else {
			*s = "false"
		}
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return fmt.Errorf("expected string or bool: %w", err)
	}
	*s = StrBool(str)
	return nil
}

// IsTrue returns true if the value is "true" (case-insensitive).
func (s StrBool) IsTrue() bool {
	return strings.EqualFold(string(s), "true")
}
