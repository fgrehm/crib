package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fgrehm/crib/internal/driver"
)

// volumeEntry matches the JSON output of `docker/podman volume ls --format json`.
type volumeEntry struct {
	Name string `json:"Name"`
}

// ListVolumes returns volumes whose names start with nameFilter.
func (d *OCIDriver) ListVolumes(ctx context.Context, nameFilter string) ([]driver.VolumeInfo, error) {
	out, err := d.helper.Output(ctx, "volume", "ls", "--filter", "name="+nameFilter, "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	entries, err := parseVolumeJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing volume list: %w", err)
	}

	volumes := make([]driver.VolumeInfo, len(entries))
	for i, e := range entries {
		volumes[i] = driver.VolumeInfo{Name: e.Name}
	}

	// Best-effort size enrichment with a short timeout. `docker system df -v`
	// can block for a long time when the daemon is busy (e.g. during builds),
	// so we cap it to keep `cache list` responsive.
	sizeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sizes := d.volumeSizes(sizeCtx)
	for i := range volumes {
		if s, ok := sizes[volumes[i].Name]; ok {
			volumes[i].Size = s
		}
	}

	return volumes, nil
}

// parseVolumeJSON handles both Docker (one JSON object per line) and
// Podman (JSON array) output formats from `volume ls --format json`.
func parseVolumeJSON(raw string) ([]volumeEntry, error) {
	// Try JSON array first (Podman format).
	if strings.HasPrefix(raw, "[") {
		var entries []volumeEntry
		if err := json.Unmarshal([]byte(raw), &entries); err == nil {
			return entries, nil
		}
	}

	// Fall back to newline-delimited JSON objects (Docker format).
	var entries []volumeEntry
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry volumeEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Name != "" {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// RemoveVolume removes a named volume.
func (d *OCIDriver) RemoveVolume(ctx context.Context, name string) error {
	_, err := d.helper.Output(ctx, "volume", "rm", name)
	if err != nil {
		return fmt.Errorf("removing volume %s: %w", name, err)
	}
	return nil
}

// dfVolume matches entries in the Volumes array of `docker system df -v --format json`.
type dfVolume struct {
	Name string `json:"Name"`
	Size string `json:"Size"`
}

// dfOutput matches the top-level JSON from `docker system df -v --format json`.
type dfOutput struct {
	Volumes []dfVolume `json:"Volumes"`
}

// volumeSizes returns a map of volume name to human-readable size.
// Best-effort: returns an empty map on any failure.
//
// Docker supports `system df -v --format json`. Podman does not (it rejects
// combining --format with -v), so we fall back to parsing the text table from
// `system df -v`.
func (d *OCIDriver) volumeSizes(ctx context.Context) map[string]string {
	// Try JSON first (Docker).
	out, err := d.helper.Output(ctx, "system", "df", "-v", "--format", "json")
	if err == nil {
		if sizes := parseVolumeSizesJSON(strings.TrimSpace(string(out))); sizes != nil {
			return sizes
		}
	}

	// Fall back to text table (Podman).
	out, err = d.helper.Output(ctx, "system", "df", "-v")
	if err != nil {
		d.logger.Debug("system df failed, skipping volume sizes", "error", err)
		return nil
	}
	return parseVolumeSizesText(string(out))
}

// parseVolumeSizesJSON parses the JSON output of `docker system df -v --format json`.
func parseVolumeSizesJSON(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	var df dfOutput
	if err := json.Unmarshal([]byte(raw), &df); err != nil {
		return nil
	}
	sizes := make(map[string]string, len(df.Volumes))
	for _, v := range df.Volumes {
		sizes[v.Name] = v.Size
	}
	return sizes
}

// parseVolumeSizesText parses the text table output of `podman system df -v`.
// It looks for the "Local Volumes space usage:" section and extracts name/size
// pairs from the table rows.
func parseVolumeSizesText(raw string) map[string]string {
	lines := strings.Split(raw, "\n")

	// Find the volume section header.
	volSection := -1
	for i, line := range lines {
		if strings.Contains(line, "Local Volumes space usage") {
			volSection = i
			break
		}
	}
	if volSection < 0 {
		return nil
	}

	// Find the header row to determine column positions.
	headerIdx := -1
	for i := volSection + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "VOLUME NAME") {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return nil
	}

	// Determine column start positions from the header.
	header := lines[headerIdx]
	sizeCol := strings.Index(header, "SIZE")
	if sizeCol < 0 {
		return nil
	}

	sizes := make(map[string]string)
	for i := headerIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			break // end of volume section
		}
		// Name is the first whitespace-delimited field.
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		name := fields[0]
		// Size is the last field (column position can vary with name length).
		var size string
		if sizeCol < len(line) {
			size = strings.TrimSpace(line[sizeCol:])
		} else {
			size = fields[len(fields)-1]
		}
		sizes[name] = size
	}
	return sizes
}
