package oci

import (
	"context"
	"fmt"

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
