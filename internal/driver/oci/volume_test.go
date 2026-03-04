package oci

import (
	"testing"
)

func TestParseVolumeJSON_DockerFormat(t *testing.T) {
	// Docker outputs one JSON object per line.
	raw := `{"CreatedAt":"2026-03-04T12:00:00Z","Driver":"local","Labels":"","Links":"N/A","Mountpoint":"/var/lib/docker/volumes/crib-cache-myws-npm/_data","Name":"crib-cache-myws-npm","Scope":"local","Size":"15.2MB"}
{"CreatedAt":"2026-03-04T12:00:00Z","Driver":"local","Labels":"","Links":"N/A","Mountpoint":"/var/lib/docker/volumes/crib-cache-myws-go/_data","Name":"crib-cache-myws-go","Scope":"local","Size":"216.6MB"}`

	entries, err := parseVolumeJSON(raw)
	if err != nil {
		t.Fatalf("parseVolumeJSON: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "crib-cache-myws-npm" {
		t.Errorf("entries[0].Name = %q, want crib-cache-myws-npm", entries[0].Name)
	}
	if entries[1].Name != "crib-cache-myws-go" {
		t.Errorf("entries[1].Name = %q, want crib-cache-myws-go", entries[1].Name)
	}
}

func TestParseVolumeJSON_PodmanFormat(t *testing.T) {
	// Podman outputs a pretty-printed JSON array.
	raw := `[
    {
        "Name": "crib-cache-myws-npm",
        "Driver": "local",
        "Mountpoint": "/home/user/.local/share/containers/storage/volumes/crib-cache-myws-npm/_data",
        "CreatedAt": "2026-03-04T12:00:00-03:00",
        "Labels": {},
        "Scope": "local",
        "Options": {},
        "MountCount": 0,
        "NeedsCopyUp": true,
        "NeedsChown": true
    },
    {
        "Name": "crib-cache-myws-go",
        "Driver": "local",
        "Mountpoint": "/home/user/.local/share/containers/storage/volumes/crib-cache-myws-go/_data",
        "CreatedAt": "2026-03-04T12:00:00-03:00",
        "Labels": {},
        "Scope": "local",
        "Options": {},
        "MountCount": 1,
        "NeedsCopyUp": true,
        "NeedsChown": true
    }
]`

	entries, err := parseVolumeJSON(raw)
	if err != nil {
		t.Fatalf("parseVolumeJSON: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "crib-cache-myws-npm" {
		t.Errorf("entries[0].Name = %q, want crib-cache-myws-npm", entries[0].Name)
	}
	if entries[1].Name != "crib-cache-myws-go" {
		t.Errorf("entries[1].Name = %q, want crib-cache-myws-go", entries[1].Name)
	}
}

func TestParseVolumeJSON_Empty(t *testing.T) {
	entries, err := parseVolumeJSON("")
	if err != nil {
		t.Fatalf("parseVolumeJSON: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseVolumeJSON_EmptyArray(t *testing.T) {
	entries, err := parseVolumeJSON("[]")
	if err != nil {
		t.Fatalf("parseVolumeJSON: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseVolumeSizesJSON(t *testing.T) {
	raw := `{
  "Images": [],
  "Containers": [],
  "Volumes": [
    {"Name": "crib-cache-myws-npm", "Links": 1, "Size": "15.2MB", "Reclaimable": "15.2MB"},
    {"Name": "crib-cache-myws-go", "Links": 0, "Size": "216.6MB", "Reclaimable": "216.6MB"}
  ],
  "BuildCache": []
}`

	sizes := parseVolumeSizesJSON(raw)
	if sizes == nil {
		t.Fatal("expected non-nil sizes map")
	}
	if sizes["crib-cache-myws-npm"] != "15.2MB" {
		t.Errorf("npm size = %q, want 15.2MB", sizes["crib-cache-myws-npm"])
	}
	if sizes["crib-cache-myws-go"] != "216.6MB" {
		t.Errorf("go size = %q, want 216.6MB", sizes["crib-cache-myws-go"])
	}
}

func TestParseVolumeSizesJSON_Empty(t *testing.T) {
	if sizes := parseVolumeSizesJSON(""); sizes != nil {
		t.Errorf("expected nil for empty input, got %v", sizes)
	}
}

func TestParseVolumeSizesJSON_InvalidJSON(t *testing.T) {
	if sizes := parseVolumeSizesJSON("not json"); sizes != nil {
		t.Errorf("expected nil for invalid JSON, got %v", sizes)
	}
}

func TestParseVolumeSizesText(t *testing.T) {
	raw := `Images space usage:

REPOSITORY    TAG       IMAGE ID      CREATED       SIZE        SHARED SIZE  UNIQUE SIZE  CONTAINERS
ubuntu        latest    abc123        2 weeks       77.8MB      0B           77.8MB       0

Containers space usage:

CONTAINER ID  IMAGE     COMMAND     LOCAL VOLUMES  SIZE        CREATED       STATUS       NAMES

Local Volumes space usage:

VOLUME NAME              LINKS     SIZE
crib-cache-myws-npm      1         15.2MB
crib-cache-myws-go       0         216.6MB
crib-cache-myws-cargo    1         0B
`

	sizes := parseVolumeSizesText(raw)
	if sizes == nil {
		t.Fatal("expected non-nil sizes map")
	}
	if len(sizes) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(sizes))
	}
	if sizes["crib-cache-myws-npm"] != "15.2MB" {
		t.Errorf("npm size = %q, want 15.2MB", sizes["crib-cache-myws-npm"])
	}
	if sizes["crib-cache-myws-go"] != "216.6MB" {
		t.Errorf("go size = %q, want 216.6MB", sizes["crib-cache-myws-go"])
	}
	if sizes["crib-cache-myws-cargo"] != "0B" {
		t.Errorf("cargo size = %q, want 0B", sizes["crib-cache-myws-cargo"])
	}
}

func TestParseVolumeSizesText_NoVolumeSection(t *testing.T) {
	raw := `Images space usage:

REPOSITORY    TAG       IMAGE ID
`
	if sizes := parseVolumeSizesText(raw); sizes != nil {
		t.Errorf("expected nil for no volume section, got %v", sizes)
	}
}

func TestParseVolumeSizesText_EmptyVolumeSection(t *testing.T) {
	raw := `Local Volumes space usage:

VOLUME NAME              LINKS     SIZE
`
	sizes := parseVolumeSizesText(raw)
	if sizes == nil {
		t.Fatal("expected non-nil sizes map")
	}
	if len(sizes) != 0 {
		t.Errorf("expected 0 entries, got %d", len(sizes))
	}
}
