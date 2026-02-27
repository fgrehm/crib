# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# crib

Dev containers without the ceremony. crib reads `.devcontainer` configs, builds the container, and gets out of the way.

## Project Context

crib is a fresh implementation of a devcontainer CLI tool. It is not a fork of DevPod. The architecture uses DevPod's algorithms as reference (see `../devpod` local clone) but owns all its code.

## DevContainer Specification Reference

`docs/devcontainers-spec.md` is a quick-lookup companion to the [official Dev Container Specification](https://containers.dev/implementors/spec/). Use it when:

- Implementing or debugging config parsing (syntax, variable substitution, property details)
- Working with lifecycle hooks (hook types, execution order, timing)
- Understanding Features (structure, options, installation order, metadata)
- Determining how Docker Compose, image metadata, or workspace mounts work
- Checking port attributes, environment variables, or user/permissions semantics

The file is not a replacement for the official spec, just a distilled reference organized by topic with links to the authoritative sources.

## Implementation Notes

`docs/implementation-notes.md` documents quirks, workarounds, and spec compliance status. Consult it when:

- Debugging rootless Podman permission issues (userns_mode, UID sync, chown)
- Working with lifecycle hooks and environment probing (userEnvProbe)
- Understanding how container user detection works for compose containers
- Checking which spec features are implemented, partially implemented, or skipped

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

## Conventions

- Go module: `github.com/fgrehm/crib`
- All packages under `internal/`, this is a binary not a library.
- Logging via `log/slog`.
- CLI framework: `spf13/cobra`.
- Linting: golangci-lint v2 (errcheck, govet, staticcheck, unused, ineffassign).
- Pre-commit hooks: gofmt + golangci-lint on staged Go files.

## Implementation Status

All core packages and CLI commands are implemented. The tool supports image-based, Dockerfile-based, and Docker Compose-based devcontainers, including feature installation, lifecycle hooks, and workspace state management.
