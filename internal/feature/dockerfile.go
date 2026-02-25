package feature

import (
	"fmt"
	"strings"
)

const (
	dockerfileSyntax = "# syntax=docker.io/docker/dockerfile:1.4"
	baseImageArg     = "ARG _DEV_CONTAINERS_BASE_IMAGE=placeholder"
	baseStageName    = "dev_containers_base_stage"
	targetStageName  = "dev_containers_target_stage"
	builtinEnvFile   = "devcontainer-features.builtin.env"
	featureEnvFile   = "devcontainer-features.env"
)

// GenerateDockerfile produces the Dockerfile content and prefix for installing
// features into the container image.
//
// content is the main Dockerfile body (FROM, COPY, RUN layers).
// prefix is the syntax directive and base image ARG that must appear at the
// top of the final Dockerfile.
func GenerateDockerfile(features []*FeatureSet, containerUser, remoteUser string) (content, prefix string) {
	prefix = dockerfileSyntax + "\n" + baseImageArg + "\n"

	var b strings.Builder

	// Alias the base image as a named stage so the RUN --mount below can
	// reference it without a self-referential dependency on targetStageName.
	fmt.Fprintf(&b, "FROM $_DEV_CONTAINERS_BASE_IMAGE AS %s\n", baseStageName)
	b.WriteString("\n")

	// Feature installation stage builds on top of the base.
	fmt.Fprintf(&b, "FROM %s AS %s\n", baseStageName, targetStageName)
	b.WriteString("\n")

	// Switch to root for feature installation.
	b.WriteString("USER root\n")
	b.WriteString("\n")

	// Copy all feature files into the build context.
	fmt.Fprintf(&b, "COPY %s/ /tmp/build-features/\n", ContextFeatureFolder)
	b.WriteString("\n")

	// Source the builtin env file.
	fmt.Fprintf(&b, "RUN cat /tmp/build-features/%s >> /etc/environment 2>/dev/null || true\n", builtinEnvFile)
	b.WriteString("\n")

	// Per-feature ENV and RUN layers.
	for i, f := range features {
		// ContainerEnv as ENV instructions.
		for k, v := range f.Config.ContainerEnv {
			fmt.Fprintf(&b, "ENV %s=%q\n", k, v)
		}

		// RUN the feature installation wrapper script.
		// Mount from baseStageName (not the current stage) to avoid a
		// self-referential dependency that Podman and older BuildKit reject.
		fmt.Fprintf(&b, "RUN --mount=type=bind,from=%s,source=/,target=/build-context ", baseStageName)
		fmt.Fprintf(&b, "chmod +x /tmp/build-features/%d/devcontainer-features-install.sh ", i)
		fmt.Fprintf(&b, "&& /tmp/build-features/%d/devcontainer-features-install.sh\n", i)
		b.WriteString("\n")
	}

	// Restore the original user. The default must match the base image user
	// so the prebuild hash changes when the user changes.
	imageUser := containerUser
	if imageUser == "" {
		imageUser = "root"
	}
	fmt.Fprintf(&b, "ARG _DEV_CONTAINERS_IMAGE_USER=%s\n", imageUser)
	b.WriteString("USER $_DEV_CONTAINERS_IMAGE_USER\n")

	content = b.String()
	return content, prefix
}
