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
  plugin/      -> Plugin system (codingagents, packagecache, sandbox, shellhistory, ssh)
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

## Plugin System

Bundled plugins live in `internal/plugin/{name}/`. The engine dispatches them
at two lifecycle points:

- **PreContainerRun**: before container creation. Returns mounts, env, copies.
- **PostContainerCreate**: after container creation. Runs commands inside the
  container via `ExecFunc`/`CopyFileFunc` closures (no driver import needed).

### Error handling

Plugins are **fail-open by design**. The plugin manager logs errors as warnings
and continues. One broken plugin must never block container creation. This is
intentional, not a bug. Within a plugin, some steps can be independently
non-fatal (e.g. network blocking fails but filesystem sandboxing still works).

### remoteUser in PostContainerCreate

`PostContainerCreateRequest.RemoteUser` comes from `configRemoteUser(cfg)`
(devcontainer.json's `remoteUser` or `containerUser`), not from the resolved
container user. This is correct: the container user hasn't been probed yet at
this point in the `finalize()` flow. When no user is configured, it defaults to
`""`, and `InferRemoteHome("")` returns `/root` (containers without an explicit
user run as root).

### Plugin naming convention

Plugin directory names are Go package names (no hyphens): `codingagents`,
`shellhistory`. Display names use hyphens: `coding-agents`, `shell-history`.
The `Name()` method returns the display name. Both forms are correct in their
respective contexts.

### Shell input validation

Plugins that construct shell commands use layered validation:

- `validAliasName` regex rejects characters unsafe for shell/paths (`;`, spaces,
  `..`, leading `-`).
- `plugin.ShellQuote()` wraps values in single quotes with proper escaping.
- Generated scripts use positional parameters or `command -v` (not eval).

If input passes the regex, it is safe for use in the generated script. Review
the regex definition before flagging injection concerns.

## Conventions

- Go module: `github.com/fgrehm/crib`
- Logging: `log/slog`. `Debug` for exec and decisions, `Warn` for non-fatal
  fallbacks, `Info` only for one-time startup events.
- Naming: `devcontainer` (one word) for files/configs, "dev container" (two
  words) for the concept, "DevContainer Features" (PascalCase) for the spec.

## Testing

- `go test ./internal/... -short` for unit tests.
- Plugin tests use fake `ExecFunc`/`CopyFileFunc`/`ExecOutputFunc` closures
  (no real containers). Assert on captured commands and file contents.
- `mockDriver` in engine tests uses `sync.Mutex` on `execCalls` because
  parallel lifecycle hooks call `ExecContainer` concurrently.

## Releasing

`CHANGELOG.md` uses Keep a Changelog format. The `[Unreleased]` section
accumulates entries during development. At release time, entries move to a
versioned section. The CI release workflow intentionally fails if no release
notes exist for the tagged version; this is by design, not a bug.
