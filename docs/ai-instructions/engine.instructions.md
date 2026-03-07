---
applyTo: "internal/engine/**"
---

# Engine: Dual Code Paths

The engine has two separate code paths that diverge at `Up()`:

- **Single-container** (`single.go`): image-based and Dockerfile-based
  devcontainers. Uses `driver.RunOptions` + `driver.RunContainer()`.
- **Compose** (`compose.go`): Docker Compose devcontainers. Generates a compose
  override YAML and delegates to `compose up`.

Both paths converge at `setupAndReturn()` for lifecycle hooks and result saving.
`restart.go` also has separate methods: `restartRecreateSingle` vs
`restartRecreateCompose`.

Wire any feature affecting container creation (plugins, mounts, env vars, labels)
into both paths. The compose path uses `generateComposeOverride()` for mounts,
env, and labels, and `execPluginCopies()` for file copies after container start.

## remoteEnv is injected at exec time, not baked in

`remoteEnv` (including plugin `PathPrepend` entries) is injected via
`docker exec -e`, NOT written to the container's native environment.
`probeUserEnv` runs without injecting remoteEnv, so it cannot recapture
PathPrepend entries.

Every code path that calls `saveResult` must dispatch plugins and apply
PathPrepend beforehand. The 6 save sites:

| # | Location | Flow |
|---|----------|------|
| 1 | `setupAndReturn` | Early save during Up |
| 2 | `finalizeSetup` | Final save for fresh creation |
| 3 | `Up()` in engine.go | Top-level final save |
| 4 | `restartSimple` | Simple restart save |
| 5 | `restartRecreateCompose` | Compose recreate save |
| 6 | `restartRecreateSingle` | Single recreate save |

Tests in `single_test.go` and `restart_test.go` cover this invariant. See
`docs/decisions/001-no-save-path-abstraction.md` for the ADR.

## Compose override is ephemeral

The compose override YAML is a temp file regenerated on every `compose up`.
Every code path that calls `generateComposeOverride()` must dispatch plugins and
pass the response; otherwise plugin env vars and mounts silently disappear.
Paths that generate overrides:

- `upCompose` (fresh up, stopped container restart, stored result)
- `restartSimple` compose path
- `restartRecreateCompose`

Always call `findComposeContainer()` after `compose up` to get the current
container ID. The override may have caused compose to recreate the container.
