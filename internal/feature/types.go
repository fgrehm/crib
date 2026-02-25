package feature

import (
	"encoding/json"
	"fmt"

	"github.com/fgrehm/crib/internal/config"
)

const (
	// FeatureFileName is the expected name for feature metadata files.
	FeatureFileName = "devcontainer-feature.json"

	// ContextFeatureFolder is the directory name used within the build context
	// for feature installation files.
	ContextFeatureFolder = ".crib-features"
)

// FeatureSet pairs a feature's configuration with its resolved location and
// user-provided options from devcontainer.json.
type FeatureSet struct {
	ConfigID string
	Folder   string
	Config   *FeatureConfig
	Options  any
}

// FeatureConfig represents a parsed devcontainer-feature.json file.
type FeatureConfig struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name,omitempty"`
	Version       string                   `json:"version,omitempty"`
	Description   string                   `json:"description,omitempty"`
	Entrypoint    string                   `json:"entrypoint,omitempty"`
	Deprecated    bool                     `json:"deprecated,omitempty"`
	Options       map[string]FeatureOption `json:"options,omitempty"`
	DependsOn     DependsOn                `json:"dependsOn,omitempty"`
	InstallsAfter []string                 `json:"installsAfter,omitempty"`
	CapAdd        []string                 `json:"capAdd,omitempty"`
	Init          *bool                    `json:"init,omitempty"`
	Privileged    *bool                    `json:"privileged,omitempty"`
	SecurityOpt   []string                 `json:"securityOpt,omitempty"`
	Mounts        []config.Mount           `json:"mounts,omitempty"`
	ContainerEnv  map[string]string        `json:"containerEnv,omitempty"`

	// Lifecycle hooks.
	OnCreateCommand   config.LifecycleHook `json:"onCreateCommand,omitempty"`
	PostCreateCommand config.LifecycleHook `json:"postCreateCommand,omitempty"`
	PostStartCommand  config.LifecycleHook `json:"postStartCommand,omitempty"`
	PostAttachCommand config.LifecycleHook `json:"postAttachCommand,omitempty"`
}

// FeatureOption describes a single option that a feature accepts.
type FeatureOption struct {
	Default     config.StrBool `json:"default,omitempty"`
	Description string         `json:"description,omitempty"`
	Type        string         `json:"type,omitempty"`
	Enum        []string       `json:"enum,omitempty"`
	Proposals   []string       `json:"proposals,omitempty"`
}

// DependsOn holds hard dependencies as a map of feature IDs to their options.
// It rejects JSON arrays and strings, accepting only objects.
type DependsOn map[string]any

// UnmarshalJSON implements json.Unmarshaler.
// It accepts only JSON objects (or null), rejecting arrays and other types.
func (d *DependsOn) UnmarshalJSON(data []byte) error {
	// Accept null.
	if string(data) == "null" {
		return nil
	}

	// Reject arrays.
	var arr []json.RawMessage
	if json.Unmarshal(data, &arr) == nil {
		return fmt.Errorf("dependsOn must be an object, not an array")
	}

	// Reject plain strings.
	var s string
	if json.Unmarshal(data, &s) == nil {
		return fmt.Errorf("dependsOn must be an object, not a string")
	}

	// Parse as object.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("dependsOn must be an object: %w", err)
	}
	*d = m
	return nil
}

// BuildInfo holds the generated Dockerfile content and metadata for feature
// installation during the image build.
type BuildInfo struct {
	FeaturesFolder          string
	DockerfileContent       string
	DockerfilePrefixContent string
	OverrideTarget          string
	BuildArgs               map[string]string
}
