# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Rename `stop` to `down` (alias: `stop`). Now stops and removes the container
  instead of just stopping it, clearing lifecycle hook markers so the next `up`
  runs all hooks from scratch.
- Rename `delete` to `remove` (aliases: `rm`, `delete`). Removes container and
  workspace state.
- Add short aliases: `list` (`ls`), `status` (`ps`), `shell` (`sh`).
- `rebuild` no longer needs to re-save workspace state after removing the
  container (uses `down` instead of the old `delete`).
- Display crib version at the start of `up`, `down`, `remove`, `rebuild`,
  and `restart` commands.

### Fixed

- Lifecycle hooks (`onCreateCommand`, `updateContentCommand`, `postCreateCommand`)
  now run after `down` + `up` cycle. Previously, host-side hook markers persisted
  across container removal, causing hooks to be skipped.
- `restart` for compose workspaces now uses `compose up` instead of `compose restart`,
  fixing failures when dependency services (databases, sidecars) were stopped.

## [0.2.0] - 2026-02-26

### Added

- `crib restart` command with smart change detection
  - No config changes: simple container restart, runs only resume-flow hooks
    (`postStartCommand`, `postAttachCommand`)
  - Safe changes (volumes, mounts, ports, env, runArgs, user): recreates container
    without rebuilding the image, runs only resume-flow hooks
  - Image-affecting changes (image, Dockerfile, features, build args): reports error
    and suggests `crib rebuild`
- `RestartContainer` method in container driver interface
- `Restart` method in compose helper
- Smart Restart section in README
- New project logo

### Changed

- Refactor engine package: extract `change.go`, `restart.go` from `engine.go`
- Deduplicate config parsing (`parseAndSubstitute`) and user resolution (`resolveRemoteUser`)

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
