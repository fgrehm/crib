package oci

import (
	"testing"

	"github.com/fgrehm/crib/internal/driver"
)

func TestParseImageList_Basic(t *testing.T) {
	output := "crib-myws\tcrib-abc123\tsha256:123\t52428800\tmyws\n"
	images := parseImageList(output)

	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}

	want := driver.ImageInfo{
		Reference:   "crib-myws:crib-abc123",
		ID:          "sha256:123",
		Size:        52428800,
		WorkspaceID: "myws",
	}
	if images[0] != want {
		t.Errorf("got %+v, want %+v", images[0], want)
	}
}

func TestParseImageList_LocalhostPrefix(t *testing.T) {
	output := "localhost/crib-myws\tsnapshot\tsha256:456\t100\tmyws\n"
	images := parseImageList(output)

	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].Reference != "crib-myws:snapshot" {
		t.Errorf("Reference = %q, want crib-myws:snapshot (localhost/ stripped)", images[0].Reference)
	}
}

func TestParseImageList_MultipleImages(t *testing.T) {
	output := "crib-ws1\tcrib-aaa\tsha256:1\t100\tws1\n" +
		"crib-ws1\tsnapshot\tsha256:2\t200\tws1\n" +
		"crib-ws2\tcrib-bbb\tsha256:3\t300\tws2\n"
	images := parseImageList(output)

	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}
}

func TestParseImageList_EmptyOutput(t *testing.T) {
	images := parseImageList("")
	if len(images) != 0 {
		t.Errorf("expected 0 images, got %d", len(images))
	}
}

func TestParseImageList_WhitespaceOnly(t *testing.T) {
	images := parseImageList("  \n  \n")
	if len(images) != 0 {
		t.Errorf("expected 0 images, got %d", len(images))
	}
}

func TestParseImageList_InvalidSize(t *testing.T) {
	output := "crib-ws\ttag\tsha256:1\tnot-a-number\tws\n"
	images := parseImageList(output)

	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].Size != 0 {
		t.Errorf("Size = %d, want 0 for invalid size", images[0].Size)
	}
}

func TestParseImageList_NoneTag(t *testing.T) {
	output := "crib-ws\t<none>\tsha256:1\t100\tws\n"
	images := parseImageList(output)

	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].Reference != "crib-ws" {
		t.Errorf("Reference = %q, want crib-ws (no tag)", images[0].Reference)
	}
}
