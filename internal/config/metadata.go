package config

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
