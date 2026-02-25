package config

import (
	"encoding/json"
	"fmt"
)

// ImageMetadata represents metadata embedded in an image label or
// extracted from a feature's devcontainer-feature.json.
type ImageMetadata struct {
	ID         string `json:"id,omitempty"`
	Entrypoint string `json:"entrypoint,omitempty"`

	DevContainerConfigBase `json:",inline"`
	DevContainerActions    `json:",inline"`
	NonComposeBase         `json:",inline"`
}

// ImageMetadataConfig holds both raw and processed image metadata entries.
type ImageMetadataConfig struct {
	Raw    []*ImageMetadata
	Config []*ImageMetadata
}

// ImageMetadataFromConfig creates an ImageMetadata entry from a DevContainerConfig.
func ImageMetadataFromConfig(config *DevContainerConfig) *ImageMetadata {
	return &ImageMetadata{
		DevContainerConfigBase: config.DevContainerConfigBase,
		DevContainerActions:    config.DevContainerActions,
		NonComposeBase:         config.NonComposeBase,
	}
}

// ParseImageMetadata parses the devcontainer.metadata image label value.
// The label contains a JSON array of ImageMetadata entries.
func ParseImageMetadata(label string) ([]*ImageMetadata, error) {
	if label == "" {
		return nil, nil
	}

	var entries []*ImageMetadata
	if err := json.Unmarshal([]byte(label), &entries); err != nil {
		// Try single object (not array).
		var single ImageMetadata
		if err2 := json.Unmarshal([]byte(label), &single); err2 != nil {
			return nil, fmt.Errorf("parsing image metadata: %w", err)
		}
		entries = []*ImageMetadata{&single}
	}
	return entries, nil
}
