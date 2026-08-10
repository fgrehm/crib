---
name: crib-review
description: Review crib code and PRs against the project's architecture invariants and conventions. Use when reviewing changes to the crib devcontainer CLI (Go), including engine, plugin, driver, compose, config, and docs changes.
---

# crib Code Review

Review crib changes against the project's architecture invariants and conventions. crib is a Go CLI that reads `devcontainer.json`, builds a container, runs lifecycle hooks, and gets out of the way. No daemon, no in-container agent.

## Before reviewing

- Read `ARCHITECTURE.md` for the codemap and invariants.
- Load the per-area instructions for the code under review (see below).
- Verify the change: `make build`, `make test`, `make lint`, `make govulncheck`.

## Per-area instructions

Load the relevant reference file (in `references/`) based on what the change touches, and apply it alongside this skill:

| Area under review | Reference file |
|-------------------|------------------|
| Engine orchestration (`internal/engine/**`) | `references/engine.md` |
| Plugin system (`internal/engine/**`, `internal/plugin/**`) | `references/plugins.md` |
| Logging and output (`internal/**`, `cmd/**`) | `references/logging.md` |
| Docs, website, CHANGELOG, README | `references/docs.md` |

These files carry the authoritative per-area rules (backend abstraction, plugin dispatch parameter order, slog levels, naming conventions). The sections below summarize the review-relevant points; defer to the reference files when they conflict.

## Engine invariants (must hold)

- **No daemon agent inside the container.** Setup and teardown drive the container from the host via `docker exec` / `docker build`. No sidecar, no long-lived in-container service, no pre-baked crib binary.
- **One driver, two runtimes.** Docker and Podman go through `OCIDriver`, not separate implementations. Runtime detection: `CRIB_RUNTIME` env > podman > docker.
- **Both paths or neither.** Anything affecting container creation (mounts, env, labels, plugins, features) must be wired into both `singleBackend` and `composeBackend`. The split also exists in `restart.go`.
- **Config wins.** `remoteUser` / `containerUser` from devcontainer.json are consulted before any backend-specific or generic fallback. Enforced by `backend.pluginUser()` before plugin dispatch.
- **`remoteEnv` is injected, not baked.** Flows into the container via `docker exec -e`, not `ENV` in the image or `environment:` in compose. Baking it in defeats the userEnvProbe contract.
- **State lives under `~/.crib/`.** Per-workspace state in `~/.crib/workspaces/{id}/`, derived from the project path. Don't add parallel state stores.
- **Locked state-mutating commands.** `up`, `down`, `stop`, `rebuild`, `restart`, `remove` acquire `store.Lock(ctx, ws.ID)` before operating. Read-only commands (`exec`, `run`, `shell`, `logs`, `status`, `list`, `doctor`) do not lock.
- **No cycles in the dependency graph.** `cmd` is the composition root; leaf packages never import `engine`. If a change wants to import `engine` from a leaf package, the abstraction is wrong.

## Plugin system

- Plugins are **fail-open by design**. A broken plugin logs a warning and continues; it must never block container creation. This is intentional, not a bug.
- **Naming**: directory names are Go package names (no hyphens): `codingagents`, `shellhistory`. Display names use hyphens: `coding-agents`, `shell-history`. `Name()` returns the display name. Both forms are correct in their contexts.
- **Shell input validation**: plugins that construct shell commands use layered validation: `validAliasName` regex rejects characters unsafe for shell/paths (`;`, spaces, `..`, leading `-`); `plugin.ShellQuote()` wraps values in single quotes with escaping; generated scripts use positional parameters or `command -v` (not eval). If input passes the regex, it is safe for the generated script.
- **Dispatch**: `dispatchPlugins(ctx, ws, cfg, imageName, workspaceFolder, remoteUser)` — `remoteUser` is the 6th parameter. Both single and compose paths call it, then merge the response into their own target shape. Log a warning when dispatch fails.
- **Bind mount gotcha**: use directory mounts or `FileCopy` (exec-based injection) for files that undergo atomic renames inside the container. Docker/Podman hold the inode on single-file bind mounts, so `rename(tmp, target)` fails with EBUSY.

## Logging

- `internal/ui` for user-facing messages in `cmd/`; slog for diagnostics.
- slog levels: `Debug` for exec commands and internal decisions; `Warn` for non-fatal fallbacks; `Info` only for one-time startup events (runtime/compose detection).
- Guard expensive log argument evaluation (like `scrubArgs`) behind `logger.Enabled(ctx, slog.LevelDebug)`.

## Docs and naming

- `devcontainer` (one word) for files/configs; "dev container" (two words) for the concept; "DevContainer Features" / "DevContainer Spec" (PascalCase) for the spec; "Dev Containers" only for the VS Code product.
- `CHANGELOG.md` uses Keep a Changelog. Add an `[Unreleased]` entry for user-facing changes; internal refactors that preserve behavior need none.

## Go conventions

- Go 1.26+. Modern stdlib used freely: `for i := range n`, `strings.Cut`, `slices.*`, `maps.*`. Don't flag these as compatibility issues; backwards compatibility with older Go is not a concern.
- Logging via `log/slog`. Linting via golangci-lint v2 (`make lint`). Formatting via gofumpt/goimports (`make fmt`).

## Testing

- Unit tests: `go test ./internal/... -short`.
- **Integration tests required for engine features touching the container lifecycle** (hooks, env, user, features). Pattern: `newTestEngine(t)` with a real `OCIDriver` and temp-dir workspace store; temp project dir with `.devcontainer/devcontainer.json`; call `e.Up(ctx, ws, UpOptions{})`; verify side effects via `d.ExecContainer`. Test names start with `TestIntegration`. Use `alpine:3.20` as the base image.
- Parallel test safety: unique workspace IDs derived from `t.Name()` / `t.TempDir()`; scope assertions to the specific workspace ID or container name; don't query all containers/images globally.
