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

Any feature affecting container creation (plugins, mounts, env vars, labels) must
be wired into both paths. The compose path cannot use `RunOptions` directly;
inject config via `generateComposeOverride()` (for mounts, env, labels) and
`execPluginCopies()` (for file copies after container start).

## remoteEnv is injected at exec time, not baked in

`remoteEnv` (including plugin `PathPrepend` entries) is injected via
`docker exec -e`, NOT written to the container's native environment.
`probeUserEnv` runs without injecting remoteEnv, so it cannot recapture
PathPrepend entries.

Every code path that calls `saveResult` must explicitly dispatch plugins and
apply PathPrepend beforehand. There are 6 save sites total:

| # | Location | Flow |
|---|----------|------|
| 1 | `setupAndReturn` | Early save during Up |
| 2 | `finalizeSetup` | Final save for fresh creation |
| 3 | `Up()` in engine.go | Top-level final save |
| 4 | `restartSimple` | Simple restart save |
| 5 | `restartRecreateCompose` | Compose recreate save |
| 6 | `restartRecreateSingle` | Single recreate save |

Tests in `single_test.go` and `restart_test.go` cover this invariant. See
`docs/decisions/001-no-save-path-abstraction.md` for the ADR on why these are
explicit rather than abstracted.

## Compose override is ephemeral

For single-container workspaces, env vars and mounts are baked into the container
config at creation time and survive `docker restart`. For compose workspaces, the
override YAML is a temp file regenerated on every `compose up`.

This means every code path that calls `generateComposeOverride()` must dispatch
plugins and pass the response, otherwise plugin env vars and mounts silently
disappear. This includes:

- `upCompose` (fresh up, stopped container restart, stored result)
- `restartSimple` compose path
- `restartRecreateCompose`

Never use a stored container ID for compose operations after `compose up`. The
override may have changed, causing compose to recreate the container with a new
ID. Always call `findComposeContainer()` after `compose up` returns.
