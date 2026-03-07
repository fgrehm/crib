# ADR 001: No abstraction layer for dual-path plugin dispatch

**Status:** Accepted
**Date:** 2026-03-07

## Context

The engine has two separate code paths (single-container and compose) that
diverge at `Up()` and again at `Restart()`. Both must dispatch plugins and apply
`PathPrepend` to `cfg.RemoteEnv` before saving the workspace result. There are 6
distinct save sites across `single.go`, `compose.go`, `engine.go`, and
`restart.go`.

In v0.5.1, we discovered that 3 of the 6 save sites silently dropped plugin PATH
entries because they skipped plugin dispatch. The root cause: `remoteEnv` is
injected at exec time (`docker exec -e`), not written to the container's native
environment. `probeUserEnv` cannot recapture PathPrepend entries, so any save
path that skips dispatch overwrites the stored RemoteEnv without them.

After fixing the bugs, we evaluated whether an abstraction layer could
structurally prevent this class of bug in the future.

## Options considered

### A. Result builder pattern

A builder struct that enforces plugin dispatch as a required step before
producing the final result (`.WithPlugins().Build()`). Rejected because it just
moves "remember to call `dispatchPlugins`" to "remember to call
`builder.WithPlugins()`". Same class of bug, more indirection.

### B. `ensurePluginState` helper

A single function that dispatches plugins if not already done and returns
PathPrepend. This is essentially what `applyPathPrepend` already is. It does not
enforce anything structurally; the caller still has to remember to call it.

### C. Make `setupAndReturn` always dispatch plugins internally

Remove the `pathPrepend` parameter so callers cannot forget. This only covers 3
of the 6 save sites (the `Up()` paths). The restart paths bypass
`setupAndReturn` entirely, calling `runResumeHooks` or `runRecreateLifecycle`
plus `saveResult` directly. Partial solution at best.

We also considered variants that push dispatch into `setupContainer`,
`runResumeHooks`, or `saveResult` itself. Each either mixed concerns (hook
runners dispatching plugins), required threading extra parameters through
unrelated functions, or introduced double-dispatch for paths that already handle
plugins correctly.

## Decision

Keep the explicit dispatch in each path. Do not add an abstraction layer.

## Rationale

1. **The 6 paths are genuinely different.** Compose needs `composeUser` for
   correct home directory resolution. Fresh creation needs `featureImage`. Simple
   restart just needs PathPrepend, not full plugin copies. Forcing these through
   a single abstraction means passing lots of optional parameters, adding
   complexity without reducing bugs.

2. **Tests are the real safety net.** `TestUpSingle_AlreadyRunning_PreservesPathPrepend`
   and `TestRestartSimple_NonCompose_PreservesPathPrepend` catch this exact bug
   pattern. Any new code path that saves without PathPrepend will fail the same
   way and should have a corresponding test.

3. **The surface area is bounded.** Only 3 entry methods can add new save paths:
   `upSingle`, `upCompose`, and `Restart`. These change rarely.

4. **Plugin dispatch is idempotent.** Bundled plugins are in-process Go code with
   no I/O side effects, so double-dispatch is safe but unnecessary.

## Revisit if

- A third container orchestration path is added (beyond single and compose).
- The number of save sites grows beyond what tests can reasonably cover.
- `PathPrepend` is joined by other plugin response fields that must survive saves
  (at that point, storing plugin state in `workspace.Result` becomes worthwhile).
