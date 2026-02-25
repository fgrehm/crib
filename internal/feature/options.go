package feature

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var nonWordChars = regexp.MustCompile(`[^\w]`)

// FeatureEnvVars converts feature options to sorted environment variable
// lines in the form SAFE_ID="value". User-provided options override
// defaults from the feature config.
func FeatureEnvVars(fc *FeatureConfig, userOptions any) []string {
	merged := mergeOptions(fc, userOptions)

	var lines []string
	for id, value := range merged {
		safe := safeID(id)
		lines = append(lines, fmt.Sprintf("%s=%q", safe, value))
	}
	sort.Strings(lines)
	return lines
}

// safeID converts an option ID to an environment-safe name:
// uppercase and replace non-word characters with underscores.
func safeID(id string) string {
	s := strings.ToUpper(id)
	return nonWordChars.ReplaceAllString(s, "_")
}

// mergeOptions combines feature option defaults with user-provided overrides.
// userOptions can be:
//   - map[string]any: per-option overrides
//   - string: treated as {"version": value}
//   - nil: defaults only
func mergeOptions(fc *FeatureConfig, userOptions any) map[string]string {
	result := make(map[string]string)

	// Apply defaults from feature config.
	for id, opt := range fc.Options {
		if def := string(opt.Default); def != "" {
			result[id] = def
		}
	}

	// Apply user overrides.
	switch opts := userOptions.(type) {
	case map[string]any:
		for k, v := range opts {
			result[k] = fmt.Sprintf("%v", v)
		}
	case string:
		result["version"] = opts
	}

	return result
}
