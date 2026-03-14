---
applyTo: "internal/engine/**,internal/plugin/**"
---

# Plugin System

Bundled plugins are in-process Go code under `internal/plugin/`. The engine
dispatches them via `dispatchPlugins()` which builds the request and returns the
response without merging into any target. Plugin dispatch is idempotent.

## Dispatch and wiring

Both code paths call `dispatchPlugins()`, then apply the response differently:

- **Single-container**: `runPreContainerRunPlugins()` merges into `RunOptions`
  (mounts, env, runArgs), then `execPluginCopies()` after container creation.
- **Compose**: response passed to `generateComposeOverride()` for mounts/env in
  the override YAML. `runArgs` are ignored (compose owns container config).
  `execPluginCopies()` runs after `compose up`.

## dispatchPlugins parameter order

```go
func (e *Engine) dispatchPlugins(ctx, ws, cfg, imageName, workspaceFolder, remoteUser string)
```

`remoteUser` is the 6th parameter. When empty, falls back to
`configRemoteUser(cfg)` which checks `cfg.RemoteUser` then `cfg.ContainerUser`.

When calling from different contexts:
- **Fresh creation**: pass the feature image name and empty remoteUser (config
  values are available).
- **Compose paths**: pass the resolved compose user (from `resolveComposeUser`).
- **Already-running paths**: pass `cfg.RemoteUser` or `cfg.ContainerUser`.
- **Restart paths**: pass `storedResult.RemoteUser` (detected at Up() time, may
  differ from config when compose auto-detects the user).

Log a warning when dispatch fails.

## Post-container-create dispatch

Plugins that need to run commands inside a running container implement the
optional `PostContainerCreator` interface (alongside `Plugin`). The manager's
`RunPostContainerCreate()` dispatches to these plugins after file copies and
volume chown in `finalize()`. Errors are logged and skipped (fail-open, same
as `PreContainerRun`).

Call site: `finalize.go` -> `dispatchPostContainerCreate()` in `single.go`.

The request carries `ExecFunc`, `ExecOutputFunc`, and `CopyFileFunc` closures
that wrap `driver.ExecContainer`, so plugins run commands without importing the
driver. `CopyFileFunc` pipes content via stdin (no base64 or shell quoting) and
is the preferred way to write files into the container.

Currently only the sandbox plugin implements this interface.

## Bind mount gotcha: file vs directory

Use directory mounts or `FileCopy` (exec-based injection) for files that undergo
atomic renames inside the container. Docker/Podman hold the inode on single-file
bind mounts, so `rename(tmp, target)` fails with EBUSY.
