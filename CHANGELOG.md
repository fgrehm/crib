# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-02-25

### Added

- Core `crib` CLI for managing dev containers
- Support for Docker and Podman via single OCI driver
- `.devcontainer` configuration parsing, variable substitution, and merging
- All three configuration scenarios: image-based, Dockerfile-based, Docker Compose-based
- DevContainer Features support with OCI image resolution and ordering
- Workspace state management in `~/.crib/workspaces/`
- Implicit workspace resolution from current working directory
- Commands: `up`, `stop`, `delete`, `status`, `list`, `exec`, `shell`, `rebuild`, `version`
- All lifecycle hooks: `initializeCommand`, `onCreateCommand`, `updateContentCommand`,
  `postCreateCommand`, `postStartCommand`, `postAttachCommand`
- `userEnvProbe` support for probing user environment (mise, rbenv, nvm, etc.)
- Image metadata parsing (`devcontainer.metadata` label) with spec-compliant merge rules
- `updateRemoteUserUID` with UID/GID sync and conflict resolution
- Auto-injection of `--userns=keep-id` / `userns_mode: "keep-id"` for rootless Podman
- Container user auto-detection via `whoami` for compose containers
- Early result persistence so `exec`/`shell` work while lifecycle hooks are still running
- Version info on error output for debugging
- Container naming with `crib-{workspace-id}` convention
- Container labeling with `crib.workspace={id}` for discovery
- Build and test tooling (Makefile, golangci-lint v2, pre-commit hooks)
- Debug logging via `--debug` flag
