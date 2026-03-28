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

## Plugin System

Bundled plugins live in `internal/plugin/{name}/`. The engine dispatches them
at the **PreContainerRun** lifecycle point: before container creation. Returns
mounts, env, copies.

### Error handling

Plugins are **fail-open by design**. The plugin manager logs errors as warnings
and continues. One broken plugin must never block container creation. This is
intentional, not a bug.

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

## Go Version

This project requires Go 1.26+ (see `go.mod`). Modern Go features are used
throughout the codebase:

- `for i := range n` (integer range) is valid and idiomatic. Do not flag it
  as a compilation error or suggest rewriting to `for i := 0; i < n; i++`.
- `range` over function iterators may also appear.

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

## Tooling

- Go version: see `go.mod`.
- Linter: golangci-lint v2, managed as a Go tool dependency. Run `make lint` or
  `go tool golangci-lint run ./...`. Config in `.golangci.yml`.
- Formatting: `make fmt` runs gofumpt + goimports via `go tool golangci-lint fmt`.
- Dead code: `make deadcode` runs `go tool deadcode ./...` (hard gate in CI).
- Complexity: `make audit` runs gocyclo (informational at 15, hard gate at 30 in CI).
- Tests: `make test` runs with `-race -shuffle=on -short`.
- Pre-commit hook: `.githooks/pre-commit` auto-formats and lints staged files.
  Run `make setup-hooks` to activate.
- Release: tag-triggered via GoReleaser. Release notes extracted from `CHANGELOG.md`.
  See the Releasing section in CLAUDE.md.

## CHANGELOG

When reviewing PRs, verify that `CHANGELOG.md` has an `[Unreleased]` entry for any
user-facing change (features, fixes, breaking changes). Use
[Keep a Changelog](https://keepachangelog.com/) format.

`CHANGELOG.md` uses Keep a Changelog format. The `[Unreleased]` section
accumulates entries during development. At release time, entries move to a
versioned section. The CI release workflow intentionally fails if no release
notes exist for the tagged version; this is by design, not a bug.
