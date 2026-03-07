# Copilot Instructions for crib

crib is a devcontainer CLI tool written in Go. It reads `.devcontainer` configs,
builds the container, and gets out of the way. Docker and Podman are supported
through a single `OCIDriver`. CLI only, no IDE integration.

## Architecture

```
cmd/           -> CLI (cobra). Thin layer, delegates to engine/.
internal/
  config/      -> devcontainer.json parsing, variable substitution
  feature/     -> DevContainer Features (OCI resolution, ordering)
  engine/      -> Core orchestration (up/down/restart, lifecycle hooks)
  driver/      -> Container runtime abstraction (Docker/Podman)
  compose/     -> Docker Compose / Podman Compose helper
  plugin/      -> Plugin system (codingagents, packagecache, shellhistory, ssh)
  workspace/   -> Workspace state (~/.crib/workspaces/)
  dockerfile/  -> Dockerfile parsing and rewriting
```

All packages are under `internal/`; this is a binary, not a library.
Dependency flow: `cmd/ -> engine/ -> {config/, feature/, driver/, compose/,
dockerfile/, workspace/}`. No cycles.

## Key Design Decisions

- No agent injection. All container setup via `docker exec` from the host.
- Docker and Podman through a single `OCIDriver` (not separate implementations).
- Implicit workspace resolution from `cwd` (walk up to find `.devcontainer/`).
- Container naming: `crib-{workspace-id}`, labels: `crib.workspace=<id>`.
- State stored in `~/.crib/workspaces/{id}/`.

## Conventions

- Go module: `github.com/fgrehm/crib`
- Logging: `log/slog`. `Debug` for exec and decisions, `Warn` for non-fatal
  fallbacks, `Info` only for one-time startup events.
- Naming: `devcontainer` (one word) for files/configs, "dev container" (two
  words) for the concept, "DevContainer Features" (PascalCase) for the spec.

## Releasing

`CHANGELOG.md` uses Keep a Changelog format. The `[Unreleased]` section
accumulates entries during development. At release time, entries move to a
versioned section. The CI release workflow intentionally fails if no release
notes exist for the tagged version; this is by design, not a bug.
