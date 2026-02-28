# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Automatic port publishing for single-container workspaces. `forwardPorts` and
  `appPort` from `devcontainer.json` are now translated into `--publish` flags on
  `docker run`, so ports work without manual `runArgs` workarounds.
- Published ports shown on `crib ps`, `crib up`, `crib restart`, and `crib rebuild`.
  For compose workspaces, ports are parsed from `docker compose ps` output.
- Documentation website at [fgrehm.github.io/crib](https://fgrehm.github.io/crib/).
- `-V` / `--verbose` now prints each lifecycle hook command before running it
  (e.g. `  $ npm install`), making it easier to diagnose hook failures.
- `build.options` from `devcontainer.json` is now passed to `docker build` /
  `podman build` as extra CLI flags (e.g. `--network=host`, `--progress=plain`).
- Object-syntax lifecycle hooks now run their named entries in parallel, matching
  the devcontainer spec. String and array hooks are unchanged (sequential).
- `waitFor` is now respected: a "Container ready." progress message is emitted
  after the specified lifecycle stage completes (default: `updateContentCommand`).

### Changed

- `--debug` now implies `--verbose`: subprocess stdout (hooks, build, compose) is shown
  when debug logging is active.

### Fixed

- `crib restart` no longer fails with "cannot determine image name" on
  Dockerfile-based workspaces where the stored result had an empty image name.
  Falls back to rebuilding the image instead of erroring.

## [0.3.1] - 2026-02-28

### Fixed

- `down` / `stop` on rootless Podman no longer fails with "no pod with name or
  ID ... found". The `x-podman: { in_pod: false }` override was only passed
  during `up`, so `compose down` tried to remove a pod that never existed.
- `rebuild` now actually rebuilds images. Previously it passed `Recreate: false`
  to `Up`, which took the stored-result shortcut and skipped the image build.
- Environment probe now runs after lifecycle hooks (in addition to before), so
  the persisted environment for `shell`/`exec` includes tools installed by hooks
  (e.g. `mise install` in `bin/setup`).
- Filter mise internal state variables (`__MISE_*`, `MISE_SHELL`) from the probed
  environment. These are session-specific and confused mise when injected into a
  new shell via `crib shell`.

## [0.3.0] - 2026-02-27

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
  and `restart` commands. Dev builds include commit SHA and build timestamp.
- Suppress noisy compose stdout (container name listings) during up/down/restart.
  Use `--verbose` / `-V` to see full compose output.
- `status` / `ps` now shows all compose service statuses for compose workspaces.

### Fixed

- Lifecycle hooks (`onCreateCommand`, `updateContentCommand`, `postCreateCommand`)
  now run after `down` + `up` cycle. Previously, host-side hook markers persisted
  across container removal, causing hooks to be skipped.
- `restart` for compose workspaces now uses `compose up` instead of `compose restart`,
  fixing failures when dependency services (databases, sidecars) were stopped.
- `up` after `down` for compose workspaces no longer rebuilds images. When a
  stored result exists with a previously built image, the build is skipped and
  services are started directly.

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
