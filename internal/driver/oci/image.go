package oci

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/fgrehm/crib/internal/driver"
)

// InspectImage returns details about a container image.
func (d *OCIDriver) InspectImage(ctx context.Context, imageName string) (*driver.ImageDetails, error) {
	var images []driver.ImageDetails
	if err := d.helper.Inspect(ctx, []string{imageName}, "image", &images); err != nil {
		return nil, fmt.Errorf("inspecting image %s: %w", imageName, err)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("image %s not found", imageName)
	}
	return &images[0], nil
}

// RemoveImage removes a container image.
func (d *OCIDriver) RemoveImage(ctx context.Context, imageName string) error {
	_, err := d.helper.Output(ctx, "rmi", imageName)
	if err != nil {
		return fmt.Errorf("removing image %s: %w", imageName, err)
	}
	return nil
}

// ListImages returns images matching the given label filter.
func (d *OCIDriver) ListImages(ctx context.Context, label string) ([]driver.ImageInfo, error) {
	// Use Go template for consistent output across Docker and Podman.
	// Fields: Repository, Tag, ID, Size (bytes), and the crib.workspace label value.
	format := `{{.Repository}}	{{.Tag}}	{{.ID}}	{{.Size}}	{{index .Labels "crib.workspace"}}`
	out, err := d.helper.Output(ctx, "images",
		"--filter", "label="+label,
		"--format", format,
		"--no-trunc",
	)
	if err != nil {
		return nil, fmt.Errorf("listing images with label %s: %w", label, err)
	}
	return parseImageList(string(out)), nil
}

// parseImageList parses tab-separated image output into ImageInfo values.
func parseImageList(output string) []driver.ImageInfo {
	var images []driver.ImageInfo
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}
		repo := strings.TrimPrefix(parts[0], "localhost/")
		tag := parts[1]
		ref := repo
		if tag != "" && tag != "<none>" {
			ref = repo + ":" + tag
		}
		size, _ := strconv.ParseInt(parts[3], 10, 64)
		images = append(images, driver.ImageInfo{
			Reference:   ref,
			ID:          parts[2],
			Size:        size,
			WorkspaceID: parts[4],
		})
	}
	return images
}
