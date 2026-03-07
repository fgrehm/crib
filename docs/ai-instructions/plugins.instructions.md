---
applyTo: "internal/engine/**,internal/plugin/**"
---

# Plugin System

Bundled plugins are in-process Go code under `internal/plugin/`. The engine
dispatches them via `dispatchPlugins()` which builds the request and returns the
response without merging into any target.

## Dispatch and wiring

Both code paths call `dispatchPlugins()`, then apply the response differently:

- **Single-container**: `runPreContainerRunPlugins()` merges into `RunOptions`
  (mounts, env, runArgs), then `execPluginCopies()` after container creation.
- **Compose**: response passed to `generateComposeOverride()` for mounts/env in
  the override YAML. `runArgs` are ignored (compose owns container config).
  `execPluginCopies()` runs after `compose up`.

Plugin dispatch is idempotent. Bundled plugins are in-process Go code with no
I/O side effects.

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

Always log a warning when dispatch fails instead of silently ignoring the error.

## Bind mount gotcha: file vs directory

Never bind-mount a single file if anything inside the container does atomic
renames on it. Docker/Podman hold the inode, so `rename(tmp, target)` fails with
EBUSY. Mount the parent directory instead, or use `FileCopy` (exec-based
injection).
