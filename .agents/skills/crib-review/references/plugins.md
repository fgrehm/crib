# Plugin System

Bundled plugins are in-process Go code under `internal/plugin/`. The engine dispatches them via `dispatchPlugins()` which builds the request and returns the response without merging into any target. Plugin dispatch is idempotent.

## Dispatch and wiring

Both code paths call `dispatchPlugins()`, then apply the response differently:

- **Single-container**: `runPreContainerRunPlugins()` merges into `RunOptions` (mounts, env, runArgs), then `execPluginCopies()` after container creation.
- **Compose**: response passed to `generateComposeOverride()` for mounts/env in the override YAML. `runArgs` are ignored (compose owns container config). `execPluginCopies()` runs after `compose up`.

## dispatchPlugins parameter order

```go
func (e *Engine) dispatchPlugins(ctx, ws, cfg, imageName, workspaceFolder, remoteUser string)
```

`remoteUser` is the 6th parameter. When empty, falls back to `configRemoteUser(cfg)` which checks `cfg.RemoteUser` then `cfg.ContainerUser`.

When calling from different contexts:
- **Fresh creation**: pass the feature image name and empty remoteUser (config values are available).
- **Compose paths**: pass the resolved compose user (from `resolveComposeUser`).
- **Already-running paths**: pass `cfg.RemoteUser` or `cfg.ContainerUser`.
- **Restart paths**: pass `storedResult.RemoteUser` (detected at Up() time, may differ from config when compose auto-detects the user).

Log a warning when dispatch fails.

## Error handling

Plugins are **fail-open by design**. The plugin manager logs errors as warnings and continues. One broken plugin must never block container creation. This is intentional, not a bug.

## Naming convention

Plugin directory names are Go package names (no hyphens): `codingagents`, `shellhistory`. Display names use hyphens: `coding-agents`, `shell-history`. The `Name()` method returns the display name. Both forms are correct in their respective contexts.

## Shell input validation

Plugins that construct shell commands use layered validation:

- `validAliasName` regex rejects characters unsafe for shell/paths (`;`, spaces, `..`, leading `-`).
- `plugin.ShellQuote()` wraps values in single quotes with proper escaping.
- Generated scripts use positional parameters or `command -v` (not eval).

If input passes the regex, it is safe for use in the generated script. Review the regex definition before flagging injection concerns.

## Bind mount gotcha: file vs directory

Use directory mounts or `FileCopy` (exec-based injection) for files that undergo atomic renames inside the container. Docker/Podman hold the inode on single-file bind mounts, so `rename(tmp, target)` fails with EBUSY.
