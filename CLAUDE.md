# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# crib

Dev containers without the ceremony. crib reads `.devcontainer` configs, builds the container, and gets out of the way.

## Project Context

crib is a fresh implementation of a devcontainer CLI tool. It is not a fork of DevPod. The architecture uses DevPod's algorithms as reference (see `../devpod` local clone) but owns all its code.

## Documentation

The [docs site](https://fgrehm.github.io/crib/) is the single source of truth for user-facing documentation. README.md is a GitHub landing page that links to the site.

Key reference pages (canonical files in `docs/`, symlinked into the website for publishing):

- **Devcontainer Spec Reference** (`docs/devcontainers-spec.md`) - quick-lookup companion to the [official spec](https://containers.dev/implementors/spec/). Use when implementing or debugging config parsing, lifecycle hooks, Features, Docker Compose, or workspace mounts.
- **Implementation Notes** (`docs/implementation-notes.md`) - quirks, workarounds, and spec compliance status. Use when debugging rootless Podman issues, lifecycle hooks, container user detection, or checking which spec features are implemented.

## Architecture

```
cmd/           -> CLI commands (cobra). Thin layer, delegates to engine/workspace.
internal/
  config/      -> devcontainer.json parsing, variable substitution, merging
  feature/     -> DevContainer Features (OCI resolution, ordering, Dockerfile generation)
  engine/      -> Core orchestration (up/down/remove flows, lifecycle hooks)
  driver/      -> Container runtime abstraction (Docker/Podman via single OCI driver)
  compose/     -> Docker Compose / Podman Compose helper
  workspace/   -> Workspace state management (~/.crib/workspaces/)
  dockerfile/  -> Dockerfile parsing and rewriting
```

Dependency flow: `cmd/ -> engine/ -> config/, feature/, driver/, compose/, dockerfile/, workspace/ -> config/`. No cycles. Leaves at the bottom, CLI at the top.

## Key Design Decisions

- No agent injection. All container setup via `docker exec` from the host.
- No SSH, no providers, no IDE integration. CLI only.
- Docker and Podman through a single `OCIDriver` (not separate implementations).
- Implicit workspace resolution from `cwd` (walk up to find `.devcontainer/`).
- Container naming: `crib-{workspace-id}` for human-readable `docker ps`.
- Container labels: `crib.workspace=<id>` for discovery.
- State stored in `~/.crib/workspaces/{id}/`.
- Runtime detection: `CRIB_RUNTIME` env var > podman > docker.

## Development Workflow

See the [Development](https://fgrehm.github.io/crib/contributing/development/) page on the docs site for branching model and build instructions.

- All work happens on `main`. No long-lived feature branches.
- Releases are tagged (e.g. `v0.3.0`) and the `stable` branch is updated to match.

## Build and Test

Requires Go 1.26+.

```
make build            # build bin/crib
make test             # unit tests (go test ./internal/... -short)
make lint             # golangci-lint (v2, managed as go tool dependency)
make test-integration # integration tests (requires Docker or Podman)
make setup-hooks      # configure git hooks (.githooks/pre-commit)
```

Run a single test:
```
go test ./internal/config/ -short -run TestParseFull
```

## Releasing

To cut a new release:

1. Run `make test` and `make lint` to verify everything is clean.
2. Update `CHANGELOG.md`: move `[Unreleased]` entries into a new `[X.Y.Z] - YYYY-MM-DD` section.
3. Update `VERSION` file to `X.Y.Z`.
4. Commit: `chore: release vX.Y.Z`.
5. Tag: `git tag vX.Y.Z`.
6. Update `stable` branch: `git branch -f stable vX.Y.Z`.
7. Push: `git push origin main vX.Y.Z stable`.

## Changelog

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) format. Update the `[Unreleased]` section when making user-facing changes (new features, bug fixes, breaking changes, command renames, etc.). Internal refactors that don't affect behavior don't need an entry.

When releasing, also mirror the new version section into `website/src/content/docs/reference/changelog.md`:
- The site changelog has no `[Unreleased]` section.
- Version headers are links: `## [0.3.1](https://github.com/fgrehm/crib/releases/tag/v0.3.1) - YYYY-MM-DD`

## Logging and Output

Four output mechanisms, each with a distinct purpose:

| Mechanism | Audience | Controlled by |
|-----------|----------|---------------|
| `internal/ui` (stdout) | User — results and errors | always visible; `cmd/` layer only |
| Engine progress callback | User — operation status (`  Creating container...`) | always visible |
| Engine stdout/stderr writers | User — subprocess output (hooks, build, compose) | `-V` / `--verbose` |
| `log/slog` (stderr) | Developer diagnostics | `--debug` |

**slog levels:** `Debug` for exec commands and internal decisions; `Warn` for non-fatal
fallbacks; `Info` for one-time startup events only (runtime/compose detection).

**`-V`** passes subprocess stdout through instead of discarding it. Does not change the slog
level. To echo a command in verbose mode, write it to the engine's stderr writer — not slog.

**`--debug`** sets slog to Debug and should also imply verbose.

Don't use slog in `cmd/` for user messages, don't promote exec logging above `Debug` to fake
verbose output, and don't hardcode `io.Discard` where the verbose flag should decide.

## Conventions

- Go module: `github.com/fgrehm/crib`
- All packages under `internal/`, this is a binary not a library.
- Logging via `log/slog`.
- CLI framework: `spf13/cobra`.
- Linting: golangci-lint v2 (errcheck, govet, staticcheck, unused, ineffassign).
- Pre-commit hooks: gofmt + golangci-lint on staged Go files.

## Implementation Status

All core packages and CLI commands are implemented. The tool supports image-based, Dockerfile-based, and Docker Compose-based devcontainers, including feature installation, lifecycle hooks, and workspace state management.
