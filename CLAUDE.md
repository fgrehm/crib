# CLAUDE.md

Guidance for Claude Code when working in this repository.

Detailed instructions for specific areas are in `docs/ai-instructions/`. Read
them when working on the relevant code:

- `engine.instructions.md` - dual code paths, remoteEnv invariants, save sites
- `plugins.instructions.md` - plugin dispatch, wiring, parameter order
- `logging.instructions.md` - output mechanisms, slog rules, verbose/debug
- `docs.instructions.md` - naming conventions, docs workflow, changelog

## What is crib

Dev containers without the ceremony. crib reads `.devcontainer` configs, builds
the container, and gets out of the way. CLI only, no IDE integration.

## Architecture

```
cmd/           -> CLI (cobra). Thin layer, delegates to engine/.
internal/
  config/      -> devcontainer.json parsing, variable substitution, merging
  feature/     -> DevContainer Features (OCI resolution, ordering, Dockerfile generation)
  engine/      -> Core orchestration (up/down/remove flows, lifecycle hooks)
  driver/      -> Container runtime abstraction (Docker/Podman via single OCI driver)
  compose/     -> Docker Compose / Podman Compose helper
  plugin/      -> Plugin system (codingagents, packagecache, shellhistory, ssh)
  workspace/   -> Workspace state management (~/.crib/workspaces/)
  dockerfile/  -> Dockerfile parsing and rewriting
```

Dependency flow: `cmd/ -> engine/ -> {config/, feature/, driver/, compose/,
dockerfile/, workspace/}`. No cycles.

## Key Design Decisions

- No agent injection. All container setup via `docker exec` from the host.
- Docker and Podman through a single `OCIDriver` (not separate implementations).
- Implicit workspace resolution from `cwd` (walk up to find `.devcontainer/`).
- Container naming: `crib-{workspace-id}`, labels: `crib.workspace=<id>`.
- State stored in `~/.crib/workspaces/{id}/`.
- Runtime detection: `CRIB_RUNTIME` env var > podman > docker.

## Build and Test

Requires Go 1.26+.

```
make build            # build bin/crib
make test             # unit tests (go test ./internal/... -short)
make lint             # golangci-lint v2 (managed as go tool dependency)
make test-integration # integration tests (requires Docker or Podman)
```

Run a single test: `go test ./internal/config/ -short -run TestParseFull`

## Conventions

- Go module: `github.com/fgrehm/crib`
- All packages under `internal/`; this is a binary, not a library.
- CLI: `spf13/cobra`. Logging: `log/slog`.
- Linting: golangci-lint v2 (errcheck, govet, staticcheck, unused, ineffassign).
- Pre-commit hooks: gofmt + golangci-lint on staged Go files.

## Key Reference Pages

- `docs/devcontainers-spec.md` - quick-lookup companion to the [official spec](https://containers.dev/implementors/spec/)
- `docs/implementation-notes.md` - quirks, workarounds, spec compliance status
- `docs/plugin-development.md` - plugin interface, response types, merge rules
- `docs/decisions/` - architecture decision records

## Releasing

1. `make test && make lint`
2. Move `CHANGELOG.md` `[Unreleased]` entries into `[X.Y.Z] - YYYY-MM-DD`.
3. Update `VERSION` file.
4. Commit: `chore: release vX.Y.Z`.
5. Tag and push: `git tag vX.Y.Z && git push origin main vX.Y.Z`
   CI updates the `stable` branch automatically after the tag is pushed.
