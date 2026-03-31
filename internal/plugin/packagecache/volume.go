package packagecache

import "strings"

// ParseVolumeName extracts workspace ID and provider from a cache volume name.
// Volume names follow the pattern "crib-cache-{wsID}-{provider}", but compose
// workspaces may have a "{project}_" prefix (e.g. "crib-web_crib-cache-web-apt").
// All provider names are single words (no hyphens), so the provider is the
// segment after the last hyphen in the suffix after "crib-cache-".
func ParseVolumeName(name string) (workspaceID, provider string) {
	// Strip compose project prefix if present (everything before "crib-cache-").
	if i := strings.Index(name, GlobalVolumePrefix); i > 0 {
		name = name[i:]
	}
	suffix := strings.TrimPrefix(name, GlobalVolumePrefix)
	if i := strings.LastIndex(suffix, "-"); i >= 0 {
		return suffix[:i], suffix[i+1:]
	}
	return suffix, ""
}
