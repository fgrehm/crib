# RFC: Engine Backend Abstraction

**Status:** Finalized

**Goal:** Extract the single-container and compose code paths into a
`containerBackend` interface, and consolidate all post-creation/post-restart
state management into a single `finalize` method. All plugin operations
(dispatch, file copies, volume chown, env wiring) live exclusively in the shared
orchestration layer, never inside the backend. This structurally prevents the bug
class from [ADR 001](../decisions/001-no-save-path-abstraction.md) and cuts
`upCompose` from cyclomatic complexity 38 to roughly 10-15.

Workstreams:

1. `containerBackend` interface and implementations
2. Unified `finalize` method
3. Simplified `Up()` and `Restart()` orchestration
4. Test migration

### TDD Approach

Every section follows test-first development. Task lists are ordered: write
tests, then implement to make them pass.

---

## 1. Current State

### 10 flows across 4 files

| # | Flow | File | Finalization pattern |
|---|------|------|---------------------|
| 1 | upSingle - existing (running/stopped) | single.go | setupAndReturn (full setup) |
| 2 | upSingle - fresh creation | single.go | finalizeSetup (full setup) |
| 3 | upSingleFromSnapshot | single.go | finalizeFromSnapshot |
| 4 | upCompose - existing (running/stopped) | compose.go | setupAndReturn (full setup) |
| 5 | upCompose - fresh creation | compose.go | finalizeSetup (full setup) |
| 6 | upComposeFromStored | compose.go | finalizeSetup or finalizeFromSnapshot |
| 7 | restartSimple - single | restart.go | inline env + resume hooks + save |
| 8 | restartSimple - compose | restart.go | inline env + resume hooks + save |
| 9 | restartRecreateSingle | restart.go | runRecreateLifecycle + save |
| 10 | restartRecreateCompose | restart.go | runRecreateLifecycle + save |

### Where plugins are wired today

Plugin dispatch and wiring is scattered across the flows:

| Operation | Single path | Compose path |
|-----------|------------|--------------|
| `dispatchPlugins()` | single.go, restart.go | compose.go, restart.go |
| Merge into RunOptions | `runPreContainerRunPlugins` in single.go | N/A |
| Merge into compose override | N/A | `generateComposeOverride` in compose.go |
| `execPluginCopies()` | single.go, restart.go | compose.go, restart.go |
| `chownPluginVolumes()` | single.go | compose.go (via finalizeSetup) |
| `envb.AddPluginResponse()` | single.go, restart.go | compose.go, restart.go |

Each flow calls these in slightly different order and with subtly different
arguments. The compose simple-restart path re-injects plugin copies; the single
simple-restart path does not (latent inconsistency).

### Save sites

6 save sites across engine.go, single.go, compose.go, restart.go. Each must
correctly dispatch plugins and build env via EnvBuilder. Missing any step
produces the bug class described in ADR 001.

---

## 2. Design

### Separation of concerns

```
Engine (shared orchestration)          containerBackend (type-specific)
---------------------------------      --------------------------------
parseAndSubstitute                     pluginUser()
initializeCommand                      start()
dispatchPlugins()        SHARED        buildImage()
execPluginCopies()       SHARED        createContainer()
chownPluginVolumes()     SHARED        deleteExisting()
EnvBuilder               SHARED        restart()
finalize()               SHARED        canResumeFromStored()
lifecycle hooks          SHARED
saveResult()             SHARED
```

The backend receives a `*plugin.PreContainerRunResponse` from the orchestrator
when it needs the response for container-creation-time wiring (single: merge
into RunOptions; compose: include in override YAML). The backend never calls
`dispatchPlugins` itself.

### containerBackend interface

```go
// containerBackend encapsulates the container-type-specific operations.
// Implementations handle container lifecycle only; plugin dispatch, file
// copies, env wiring, lifecycle hooks, and result saving live in the
// shared orchestration layer.
type containerBackend interface {
    // pluginUser returns the remote user for plugin dispatch.
    // Compose resolves from service config; single returns "" (config fallback).
    pluginUser(ctx context.Context) string

    // start brings up a stopped container (and dependent services for compose).
    // pluginResp is passed for compose override regeneration.
    // Returns the container ID (may differ from input for compose).
    start(ctx context.Context, containerID string, pluginResp *plugin.PreContainerRunResponse) (string, error)

    // buildImage builds the container image(s).
    // Single: Dockerfile build or image pull.
    // Compose: feature layer only (service build runs inside createContainer).
    buildImage(ctx context.Context) (*buildResult, error)

    // createContainer creates and starts a new container from the given image.
    // Single: build RunOptions, merge pluginResp, RunContainer.
    // Compose: generate override, compose build, compose up.
    createContainer(ctx context.Context, opts createOpts) (string, error)

    // deleteExisting removes all containers for the workspace.
    // Single: driver.DeleteContainer. Compose: composeDown.
    deleteExisting(ctx context.Context) error

    // restart restarts the container without recreation.
    // Single: driver.RestartContainer.
    // Compose: regenerate override, compose stop, compose start.
    // pluginResp is passed for compose override regeneration.
    restart(ctx context.Context, containerID string, pluginResp *plugin.PreContainerRunResponse) (string, error)

    // canResumeFromStored returns true if the backend can bring services up
    // from a stored result without rebuilding. Compose: true (images exist).
    // Single: false (Dockerfile-built images may have been pruned).
    canResumeFromStored() bool
}

// createOpts bundles parameters for createContainer.
type createOpts struct {
    imageName      string
    hasEntrypoints bool
    metadata       []*config.ImageMetadata // nil when creating from stored/snapshot
    pluginResp     *plugin.PreContainerRunResponse
    skipBuild      bool // true when resuming from stored result (images exist)
}

// Compile-time interface checks.
var _ containerBackend = (*singleBackend)(nil)
var _ containerBackend = (*composeBackend)(nil)
```

### Factory

```go
func (e *Engine) newBackend(ws *workspace.Workspace, cfg *config.DevContainerConfig,
    workspaceFolder string) containerBackend
```

Routes by `len(cfg.DockerComposeFile) > 0`. Both backend structs hold `*Engine`,
`*workspace.Workspace`, `*config.DevContainerConfig`, and `workspaceFolder`.
`composeBackend` additionally holds a `composeInvocation`.

### Backend implementations (summary)

**singleBackend:**

- `pluginUser` returns `""` (dispatch falls back to `configRemoteUser`).
- `start` calls `driver.StartContainer`; returns the same container ID.
- `buildImage` delegates to the existing `e.buildImage` helper.
- `createContainer` builds RunOptions, applies feature metadata, merges plugin
  response (mounts, env, runArgs), calls `driver.RunContainer`, then
  `driver.FindContainer` to get the container ID.
- `deleteExisting` finds and deletes the container via the driver.
- `restart` calls `driver.RestartContainer`; returns the same container ID.
- `canResumeFromStored` returns `false`.

**composeBackend:**

- `pluginUser` calls `e.resolveComposeUser` to read the user from service config.
- `start` regenerates the compose override (with current plugin state and
  snapshot/feature image), then calls `compose.Start`. Uses
  `findComposeContainer` to get the (possibly new) container ID.
- `buildImage` builds the feature layer only when `cfg.Features` is non-empty.
  Returns an empty `buildResult` otherwise (compose service build happens inside
  `createContainer`).
- `createContainer` generates the compose override (including plugin mounts, env,
  and feature capabilities). When `skipBuild` is false, runs `compose.Build`
  (skipping the primary service when feature-built). When `skipBuild` is true
  (stored resume), skips build entirely and goes straight to `compose.Up`. This
  matches the current `upComposeFromStored` behavior which never calls
  `compose.Build`. Uses `findComposeContainer` for the ID.
- `deleteExisting` calls `composeDown`.
- `restart` regenerates the override, then `compose.Stop` + `compose.Start`.
  Uses `findComposeContainer` for the ID.
- `canResumeFromStored` returns `true`.

### Unified finalize

Replaces `setupAndReturn`, `finalizeSetup`, `finalizeFromSnapshot`,
`runRecreateLifecycle`, and the inline finalization in `restartSimple`.

```go
type finalizeOpts struct {
    cc              containerContext
    imageName       string                              // original (not snapshot) for result
    hasEntrypoints  bool
    pluginResp      *plugin.PreContainerRunResponse
    storedResult    *workspace.Result                   // non-nil for snapshot/stored resume
    fromSnapshot    bool                                // true = restore env + resume hooks
    skipVolumeChown bool                                // true for restart (volumes exist)
}
```

**Flow:**

1. **Plugin post-creation (SHARED, always runs):** `execPluginCopies`, then
   `chownPluginVolumes` (skipped when `skipVolumeChown` is set, i.e. restarts
   where volumes already have correct ownership).

2. **Remote user resolution:** Skipped when `cc.remoteUser` is already set (e.g.
   `restartSimple` pre-sets it from stored result to avoid an unnecessary
   `whoami` exec).

3. **Env building:** Two modes:
   - `fromSnapshot`: Resolve `${containerEnv:*}` from stored env, build via
     `EnvBuilder.RestoreFrom` (probed + container PATH + plugin + config layers).
   - Fresh: Start `EnvBuilder` with config env, add plugin response.
     `setupContainer` will probe and finalize.

4. **Early save:** Build `UpResult` with all fields (container ID, image name,
   workspace folder, remote user, ports, feature entrypoints). Save before
   lifecycle hooks so `crib exec`/`crib shell` work during hook execution.

5. **Lifecycle:**
   - `fromSnapshot`: Run resume hooks only (`postStartCommand`,
     `postAttachCommand`). Create-time hook effects are already baked into the
     snapshot image.
   - Fresh: Run `setupContainer` (env probe, UID sync, chown, all lifecycle
     hooks), then `commitSnapshot`. The snapshot captures all filesystem changes
     from `onCreateCommand`, `updateContentCommand`, and `postCreateCommand`.

6. **Final save:** Persist the result again with probed env (fresh setup updates
   `cfg.RemoteEnv` during `setupContainer`).

### Snapshot and lifecycle hooks

The snapshot mechanism is unchanged. `commitSnapshot` runs after
`setupContainer` completes, which means all create-time hooks
(`onCreateCommand`, `updateContentCommand`, `postCreateCommand`) have finished
and their filesystem changes are captured in the committed image. On subsequent
`up` or `restart`, when a valid snapshot exists, `finalize` takes the
`fromSnapshot` path: it skips `setupContainer` entirely (no re-running of
create-time hooks) and only runs resume hooks (`postStartCommand`,
`postAttachCommand`). The stored env from the original setup is restored via
`EnvBuilder.RestoreFrom`.

Snapshot staleness detection (via hook hash comparison in `validSnapshot`) is
also unchanged. If hook definitions change, the snapshot is invalidated and
`finalize` takes the fresh path, re-running all hooks and committing a new
snapshot.

### Simplified Up()

`Up()` parses config, runs `initializeCommand`, creates the backend via
`newBackend`, then routes to one of three helpers:

- **`upExisting`** - container exists, no recreation requested. Loads stored
  result for image name. Dispatches plugins (shared). If stopped, calls
  `b.start(pluginResp)`. Calls `finalize`.

- **`upCreate`** - no container (or recreation requested). Checks for valid
  snapshot or stored result first. If an image is available, routes to
  `upFromImage`. Otherwise, calls `b.buildImage`, dispatches plugins,
  `b.createContainer`, `finalize`.

- **`upFromImage`** - creates container from a snapshot or stored image.
  Dispatches plugins, `b.createContainer` with `skipBuild: true` (images already
  exist), `finalize` with `fromSnapshot` flag when the image is a snapshot.

### Simplified Restart()

`Restart()` loads stored result, parses config, detects changes, then routes:

- **`restartSimple(b)`** - no config changes. Finds container, dispatches
  plugins (shared), calls `b.restart(pluginResp)`, `finalize` with
  `fromSnapshot: true` and `skipVolumeChown: true`. Pre-sets
  `cc.remoteUser` from stored result.

- **`restartRecreate(b)`** - safe config changes. Calls `Down`, checks for
  snapshot, dispatches plugins, creates container from snapshot/stored image or
  rebuilds if needed, `finalize` with appropriate `fromSnapshot` flag.

---

## 3. File Plan

### New files

| File | Contents |
|------|----------|
| `internal/engine/backend.go` | `containerBackend` interface, `createOpts`, `newBackend` factory, compile-time checks |
| `internal/engine/backend_single.go` | `singleBackend` struct and methods |
| `internal/engine/backend_compose.go` | `composeBackend` struct and methods |
| `internal/engine/finalize.go` | `finalizeOpts`, `finalize`, `toRestartResult` |

### Modified files

| File | Changes |
|------|---------|
| `engine.go` | `Up()` routes through `upExisting`/`upCreate`/`upFromImage`; `Restart()` routes through `restartSimple`/`restartRecreate`; remove top-level `saveResult` in `Up()` |
| `restart.go` | Replace `restartSimple`, `restartWithRecreate`, `restartRecreateCompose`, `restartRecreateSingle` with `restartSimple(b)` and `restartRecreate(b)` |

### Deleted code

| What | Where | Replaced by |
|------|-------|------------|
| `upSingle` | single.go | `singleBackend` methods + shared orchestration |
| `upSingleFromSnapshot` | single.go | `upFromImage` + `singleBackend.createContainer` |
| `upCompose` | compose.go | `composeBackend` methods + shared orchestration |
| `upComposeFromStored` | compose.go | `upFromImage` + `composeBackend.createContainer` |
| `finalizeSetup` | single.go | `finalize` |
| `finalizeFromSnapshot` | single.go | `finalize` with `fromSnapshot: true` |
| `setupAndReturn` | single.go | `finalize` |
| `runPreContainerRunPlugins` | single.go | inline in `singleBackend.createContainer` |
| `restartRecreateCompose` | restart.go | `restartRecreate` + `composeBackend` |
| `restartRecreateSingle` | restart.go | `restartRecreate` + `singleBackend` |
| `restartWithRecreate` | restart.go | `restartRecreate` |
| `runRecreateLifecycle` | restart.go | `finalize` |
| `recreateComposeServices` | engine.go | `composeBackend.createContainer` (with `composeDown` before) |

### Files unchanged

`setup.go`, `env.go`, `envbuilder.go`, `snapshot.go`, `build.go`, `lifecycle.go`,
`doctor.go`, `logs.go`, `change.go`, `initialize.go`. All `cmd/` files.

---

## 4. Test Strategy

### Test infrastructure additions

A `mockBackend` implementing `containerBackend` with function fields for each
method. This lets orchestration tests verify:

- `dispatchPlugins` is always called before backend methods
- `finalize` is always called after backend methods
- `pluginResp` from dispatch is passed through to the backend
- The backend never dispatches plugins itself

### Key test cases

**Orchestration tests** (new):

- [ ] `TestUp_ExistingRunning_DispatchesPluginsBeforeFinalize`
- [ ] `TestUp_ExistingStopped_DispatchesThenStartsThenFinalizes`
- [ ] `TestUp_FreshCreation_BuildThenDispatchThenCreateThenFinalize`
- [ ] `TestUp_Snapshot_DispatchWithSnapshotImage`
- [ ] `TestUp_ComposeFromStored_CanResumeFromStored`
- [ ] `TestRestart_Simple_DispatchThenRestartThenFinalize`
- [ ] `TestRestart_Recreate_WithSnapshot_FinalizeFromSnapshot`
- [ ] `TestRestart_Recreate_NoSnapshot_FinalizesFreshSetup`

**Finalize tests** (new):

- [ ] `TestFinalize_FreshSetup_RunsPluginCopiesAndChown`
- [ ] `TestFinalize_FreshSetup_CallsSetupContainerAndCommitsSnapshot`
- [ ] `TestFinalize_FromSnapshot_RestoresStoredEnvAndRunsResumeHooks`
- [ ] `TestFinalize_FromSnapshot_SkipsSetupContainerAndSnapshot`
- [ ] `TestFinalize_SkipVolumeChown_WhenFlagSet`
- [ ] `TestFinalize_EarlySave_BeforeLifecycleHooks`
- [ ] `TestFinalize_PreservesPathPrepend_FreshSetup`
- [ ] `TestFinalize_PreservesPathPrepend_FromSnapshot`

**Backend tests** (new):

- [ ] `TestSingleBackend_CreateContainer_MergesPluginResponseIntoRunOpts`
- [ ] `TestSingleBackend_Start_IgnoresPluginResp`
- [ ] `TestComposeBackend_CreateContainer_PassesPluginRespToOverride`
- [ ] `TestComposeBackend_Start_RegeneratesOverrideWithPlugins`
- [ ] `TestComposeBackend_Restart_RegeneratesOverrideThenStopStart`

**Regression tests** (migrated from existing):

- [ ] Migrate `TestUpSingle_AlreadyRunning_PreservesPathPrepend` -> `TestUp_ExistingRunning_PreservesPathPrepend`
- [ ] Migrate `TestRestartSimple_NonCompose_PreservesPathPrepend` -> `TestRestart_Simple_PreservesPathPrepend`
- [ ] Migrate `TestRestartSimple_NonCompose_PreservesProbedEnv` -> `TestRestart_Simple_PreservesProbedEnv`
- [ ] Migrate `TestRestartRecreateSingle_WithSnapshot_PreservesProbedEnv` -> `TestRestart_Recreate_WithSnapshot_PreservesProbedEnv`

**Integration tests** (unchanged):

Integration tests in `integration_test.go`, `compose_integration_test.go`, and
`restart_integration_test.go` test the public API (`Up`, `Down`, `Restart`) and
should pass without changes.

---

## 5. Behavioral Changes

### Intentional improvements

1. **Plugin copies on all paths.** Currently plugin file re-injection (SSH keys,
   credentials) only happens on some paths:
   - Single already-running: no copies
   - Compose already-running: no copies
   - Single simple restart: no copies
   - Compose simple restart: copies (re-injection after start)

   After this refactor, `finalize` always calls `execPluginCopies`, making all
   paths consistent. Host-side file changes (e.g. rotated SSH key, updated
   credentials) are picked up on every `up` and `restart`.

2. **Fewer redundant saves.** The current `Up()` does a final `saveResult` after
   `upSingle`/`upCompose` return, which is redundant with saves inside
   `finalizeSetup`. After refactoring, `finalize` handles all saves and `Up()`
   does not save again.

3. **Early save includes full metadata.** Currently the early save in
   `setupAndReturn` writes a result without `ImageName` or
   `HasFeatureEntrypoints` (those are set later in `finalizeSetup`). The
   proposed `finalize` sets all fields before the early save, so `crib exec`
   during hook execution sees the complete result.

### No behavioral change

- Env precedence order unchanged (probed < container PATH < plugin Env < config < PathPrepend).
- Snapshot commit timing unchanged (after create-time hooks complete).
- Early save timing unchanged (before lifecycle hooks, so exec/shell work).
- Compose override regeneration unchanged (on every up/restart).

---

## 6. Implementation Order

Each step is a self-contained commit that passes all tests.

1. **Add `containerBackend` interface and `newBackend` factory** (`backend.go`).
   No callers yet. Just the interface and the factory that routes by config type.

2. **Implement `singleBackend`** (`backend_single.go`). Extract methods from
   `single.go`. The existing `upSingle` code still works; the backend methods
   are unused until step 5.

3. **Implement `composeBackend`** (`backend_compose.go`). Extract methods from
   `compose.go`. Same as above.

4. **Add `finalize` method** (`finalize.go`). Implement as described.
   Add unit tests for all finalize modes. The existing finalization code still
   works; `finalize` is unused until step 5.

5. **Rewrite `Up()` orchestration** (`engine.go`). Replace the
   `upSingle`/`upCompose` routing with `upExisting`/`upCreate`/`upFromImage`
   that use the backend + finalize. Run all tests. This is the largest single
   step.

6. **Rewrite `Restart()` orchestration** (`restart.go`). Replace the 4 restart
   functions with `restartSimple(b)` and `restartRecreate(b)` that use the
   backend + finalize. Run all tests.

7. **Delete dead code.** Remove `upSingle`, `upCompose`, `upSingleFromSnapshot`,
   `upComposeFromStored`, `finalizeSetup`, `finalizeFromSnapshot`,
   `setupAndReturn`, `runPreContainerRunPlugins`, `restartRecreateCompose`,
   `restartRecreateSingle`, `restartWithRecreate`, `runRecreateLifecycle`,
   `recreateComposeServices`.

8. **Migrate and expand tests.** Migrate existing regression tests to use the
   new structure. Add orchestration and backend tests.

**Parallelizable:** Steps 2 and 3 can run in parallel. Step 4 can start after
step 1. Steps 5 and 6 are sequential (5 must finish first). Step 7 depends on
5+6. Step 8 can start alongside step 5.

---

## 7. Resolved Design Questions

### Compose `buildImage` + `createContainer` ordering


The current `upCompose` interleaves feature build, override generation, and
compose build. The proposed split puts feature build in `buildImage` and
override + compose build + compose up in `createContainer`. The override must be
generated before `compose build` because it sets `image: <featureImage>` on the
primary service, telling compose to skip building it. `createContainer`
generates the override first, then builds, then ups, preserving the correct
order. Test the override-before-build ordering explicitly.

### `start` and container ID stability

`compose start` starts existing containers without recreating (unlike `compose
up`). The container ID stays the same. `composeBackend.start` uses
`findComposeContainer` afterward as defense-in-depth for the podman compose
delegation edge case where labels may not be visible.

### Hook marker double-clear

`Up()` with `Recreate` clears markers before `b.deleteExisting(ctx)`.
`restartRecreate` calls `Down()` which also clears markers internally.
`ClearHookMarkers` is idempotent (removes `.done` files; second call is a
no-op). No issue.

### Compose build on stored resume

The current `upComposeFromStored` skips `compose.Build` entirely and goes
straight to `compose.Up` (images already exist). Without a `skipBuild` flag,
`createContainer` would always run `compose.Build`, regressing with unnecessary
network calls and possible failures on flaky connections. Resolved by adding
`skipBuild bool` to `createOpts`. When true (stored/snapshot resume), the
compose backend skips the build step. `singleBackend` ignores the field (it
builds the image in `buildImage`, not `createContainer`).

---

## 8. ADR 001 Revisit

This refactor addresses two of the three "revisit if" conditions from ADR 001:

> PathPrepend is joined by other plugin response fields that must survive saves

Done. EnvBuilder (v0.5.1) handles Env, PathPrepend, probed env, and container
PATH. This refactor ensures all paths go through `finalize` which uses
EnvBuilder consistently.

> The number of save sites grows beyond what tests can reasonably cover

Addressed. Save sites reduce from 6 (scattered across 4 files) to 2 (both
inside `finalize`: early save + final save). Every flow converges at `finalize`.

The third condition (a third orchestration path beyond single/compose) has not
occurred, but the `containerBackend` interface makes adding one straightforward.

ADR 001 should be updated to status "Superseded" with a reference to this
refactor once implementation is complete.

---

*Written in collaboration with Claude (Opus 4.6).*
