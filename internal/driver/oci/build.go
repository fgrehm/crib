package oci

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/fgrehm/crib/internal/driver"
)

// BuildImage builds a container image from a Dockerfile.
// For Docker, it tries `docker buildx build --load` first, falling back to `docker build`.
// For Podman, it uses `podman build` directly.
func (d *OCIDriver) BuildImage(ctx context.Context, workspaceID string, opts *driver.BuildOptions) error {
	imageName := opts.Image
	if imageName == "" {
		tag := "latest"
		if opts.PrebuildHash != "" {
			tag = opts.PrebuildHash
		}
		imageName = ImageName(workspaceID, tag)
	}

	stdout, stderr := buildWriters(opts)

	if d.runtime == RuntimeDocker {
		// Try buildx first.
		args := d.buildBuildArgs(imageName, opts, true)
		if err := d.helper.Run(ctx, args, nil, stdout, stderr); err != nil {
			d.logger.Warn("buildx failed, falling back to docker build", "error", err)
			args = d.buildBuildArgs(imageName, opts, false)
			if err := d.helper.Run(ctx, args, nil, stdout, stderr); err != nil {
				return fmt.Errorf("building image for workspace %s: %w", workspaceID, err)
			}
		}
		return nil
	}

	// Podman always uses plain build.
	args := d.buildBuildArgs(imageName, opts, false)
	if err := d.helper.Run(ctx, args, nil, stdout, stderr); err != nil {
		return fmt.Errorf("building image for workspace %s: %w", workspaceID, err)
	}
	return nil
}

// buildWriters returns the stdout and stderr writers from opts, falling back to os.Stderr.
func buildWriters(opts *driver.BuildOptions) (io.Writer, io.Writer) {
	stdout := io.Writer(os.Stderr)
	stderr := io.Writer(os.Stderr)
	if opts.Stdout != nil {
		stdout = opts.Stdout
	}
	if opts.Stderr != nil {
		stderr = opts.Stderr
	}
	return stdout, stderr
}

// buildBuildArgs constructs the argument list for a build command.
// When useBuildx is true, it uses `buildx build --load` (Docker only).
func (d *OCIDriver) buildBuildArgs(imageName string, opts *driver.BuildOptions, useBuildx bool) []string {
	var args []string
	if useBuildx {
		args = []string{"buildx", "build", "--load"}
	} else {
		args = []string{"build"}
	}

	// Dockerfile.
	if opts.Dockerfile != "" {
		args = append(args, "-f", opts.Dockerfile)
	}

	// Tag.
	args = append(args, "-t", imageName)

	// Target.
	if opts.Target != "" {
		args = append(args, "--target", opts.Target)
	}

	// Build args (sorted for determinism).
	argKeys := make([]string, 0, len(opts.Args))
	for k := range opts.Args {
		argKeys = append(argKeys, k)
	}
	sort.Strings(argKeys)
	for _, k := range argKeys {
		args = append(args, "--build-arg", k+"="+opts.Args[k])
	}

	// Cache from.
	for _, c := range opts.CacheFrom {
		args = append(args, "--cache-from", c)
	}

	// Extra options from build.options (before context).
	args = append(args, opts.Options...)

	// Build context (required, must be last).
	buildCtx := opts.Context
	if buildCtx == "" {
		buildCtx = "."
	}
	args = append(args, buildCtx)

	return args
}
