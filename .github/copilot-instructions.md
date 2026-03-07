# Copilot Instructions for crib

crib is a devcontainer CLI tool written in Go. It reads `.devcontainer` configs,
builds the container, and gets out of the way. Docker and Podman are supported
through a single `OCIDriver`.

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

## Critical: Dual Code Paths

The engine has two code paths that diverge at `Up()`:

- **Single-container** (`engine/single.go`): uses `driver.RunOptions`.
- **Compose** (`engine/compose.go`): generates an override YAML for `compose up`.

Both converge at `setupAndReturn()` for lifecycle hooks and result saving.
`restart.go` also has separate methods for each path.

Any feature affecting container creation (plugins, mounts, env, labels) must be
wired into both paths. The compose path uses `generateComposeOverride()` for
mounts/env and `execPluginCopies()` for file copies after start.

### remoteEnv is injected at exec time, not baked in

`remoteEnv` (including plugin `PathPrepend` entries) is injected via
`docker exec -e`, not written to the container's native environment.
`probeUserEnv` cannot recapture these entries. Every code path that calls
`saveResult` must explicitly dispatch plugins and apply `PathPrepend` beforehand.
There are 6 save sites across `single.go`, `compose.go`, `engine.go`, and
`restart.go`. Tests in `single_test.go` and `restart_test.go` cover this
invariant.

### Compose override is ephemeral

For compose workspaces, the override YAML is a temp file regenerated on every
`compose up`. Every code path that calls `generateComposeOverride()` must
dispatch plugins and pass the response. Never use a stored container ID after
`compose up`; always call `findComposeContainer()`.

## Plugin System

Bundled plugins are in-process Go code dispatched via `dispatchPlugins()`.
The response is applied differently per path: single-container merges into
`RunOptions`; compose passes to `generateComposeOverride()`. Plugin dispatch is
idempotent.

`dispatchPlugins(ctx, ws, cfg, imageName, workspaceFolder, remoteUser)` takes
`remoteUser` as the 6th parameter. When empty, it falls back to
`configRemoteUser(cfg)`. For compose, pass the resolved compose user. For
restart paths, pass `storedResult.RemoteUser` (detected at Up() time).

## Conventions

- Go module: `github.com/fgrehm/crib`
- Logging: `log/slog`. `Debug` for exec and decisions, `Warn` for non-fatal
  fallbacks, `Info` only for one-time startup events.
- Error handling: `inferWorkspaceID` only falls back on `ErrNoDevContainer`,
  surfacing real errors (permissions, I/O).
- Naming: `devcontainer` (one word) for files/configs, "dev container" (two
  words) for the concept, "DevContainer Features" (PascalCase) for the spec.

## Releasing

`CHANGELOG.md` uses Keep a Changelog format. The `[Unreleased]` section
accumulates entries during development. At release time, entries move to a
versioned section. The CI release workflow intentionally fails if no release
notes exist for the tagged version; this is by design, not a bug.
