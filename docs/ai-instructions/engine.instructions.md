---
applyTo: "internal/engine/**"
---

# Engine: containerBackend + finalize Architecture

The engine uses a `containerBackend` interface to abstract single-container vs
compose differences. All post-creation/post-restart steps converge in a single
`finalize` method.

## Backend abstraction

- **`containerBackend`** (`backend.go`): interface with 7 methods:
  `pluginUser`, `start`, `buildImage`, `createContainer`, `deleteExisting`,
  `restart`, `canResumeFromStored`.
- **`singleBackend`** (`backend_single.go`): single-container implementation.
  Uses `driver.RunOptions` + `driver.RunContainer()`.
- **`composeBackend`** (`backend_compose.go`): compose implementation.
  Generates compose override YAML and delegates to `compose up`.
- **`newBackend`** factory routes by `len(cfg.DockerComposeFile) > 0`.

Plugin dispatch, file copies, env wiring, lifecycle hooks, and result saving
live in the shared orchestration layer, never inside the backend.

## Up() orchestration

`Up()` in `engine.go` parses config, runs `initializeCommand`, creates a backend,
then routes to one of three helpers:

- **`upExisting`**: container exists, no recreation. Dispatches plugins, calls
  `b.start()` if stopped, then `finalize`.
- **`upCreate`**: no container (or recreation). Checks snapshot/stored result,
  routes to `upFromImage` or does fresh build + create + finalize.
- **`upFromImage`**: creates container from snapshot or stored image. Dispatches
  plugins, calls `b.createContainer(skipBuild: true)`, then `finalize`.

## Restart() orchestration

`Restart()` in `restart.go` loads stored result, detects config changes, then
routes to:

- **`restartSimple`**: no config changes. Finds container, dispatches plugins,
  calls `b.restart()`, then `finalize` with `fromSnapshot: true` and
  `skipVolumeChown: true`.
- **`restartRecreate`**: safe config changes. Calls `Down()`, checks snapshot,
  dispatches plugins, creates container, then `finalize`.

## Unified finalize

`finalize()` in `finalize.go` replaces the old `setupAndReturn`, `finalizeSetup`,
`finalizeFromSnapshot`, and `runRecreateLifecycle`. Every flow converges here.

Steps in order:
1. Plugin file copies (`execPluginCopies`)
2. Volume chown (if needed)
3. Remote user resolution
4. Early save before lifecycle hooks (so `crib exec`/`crib shell` work)
5. Lifecycle hooks or snapshot restore
6. Final save after setup completes (with probed env)

## remoteEnv is injected at exec time, not baked in

`remoteEnv` (including plugin `PathPrepend` entries) is injected via
`docker exec -e`, NOT written to the container's native environment.

## Compose override is regenerated on start/create/restart

The compose override YAML is written to
`~/.crib/workspaces/{id}/compose-override.yml` and regenerated whenever
compose services are started, created, or restarted. The `composeBackend`
methods (`start`, `createContainer`, `restart`) handle override generation
internally. When a container is already running, `upExisting` skips
regeneration (no backend call needed).

## Persisted build artifacts

The workspace state directory (`~/.crib/workspaces/{id}/`) stores artifacts for
troubleshooting:

| File | Written by | Contents |
|------|-----------|----------|
| `compose-override.yml` | `generateComposeOverride()` | Last compose override YAML (compose only) |
| `Dockerfile` | `doBuild()` | Generated Dockerfile used for the last image build |
