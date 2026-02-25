package feature

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tidwall/jsonc"
)

// ParseFeatureConfig reads and parses a devcontainer-feature.json from the
// given folder. The file is parsed as JSONC (JSON with comments).
func ParseFeatureConfig(folder string) (*FeatureConfig, error) {
	path := filepath.Join(folder, FeatureFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	data = jsonc.ToJSON(data)

	var fc FeatureConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &fc, nil
}
