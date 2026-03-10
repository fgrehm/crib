# Plan: Engine Backend Abstraction

## IMPORTANT

Make sure to commit as major milestones are achieved, make sure that tests pass before commits

## Context

The engine has 10 code flows across 4 files (single.go, compose.go, restart.go,
engine.go) with 6 scattered save sites and duplicated plugin wiring. The
finalized RFC at `docs/rfcs/engine-backend-abstraction.md` specifies a
`containerBackend` interface to unify these paths, with a shared `finalize`
method that structurally prevents the ADR 001 bug class.

This plan implements the RFC in 6 commits following red-green-refactor.

## Verification Commands

- **Build:** `make build`
- **Test:** `make test`
- **Lint:** `make lint`

## Step 1: Interface + factory + backend stubs

**New file: `internal/engine/backend.go`**

- `containerBackend` interface (7 methods: `pluginUser`, `start`, `buildImage`,
  `createContainer`, `deleteExisting`, `restart`, `canResumeFromStored`)
- `createOpts` struct (`imageName`, `hasEntrypoints`, `metadata`, `pluginResp`, `skipBuild`)
- `singleBackend` and `composeBackend` struct definitions (fields only, no methods yet)
- `newBackend` factory routing by `len(cfg.DockerComposeFile) > 0`
- Compile-time checks: `var _ containerBackend = (*singleBackend)(nil)` etc.
- All interface methods as stub `panic("not implemented")` on both structs

No callers, no test changes. Compile-time checks are the verification.

**Commit:** `refactor(engine): add containerBackend interface and factory`

## Step 2: singleBackend implementation

**New file: `internal/engine/backend_single.go`**

Move struct definition from backend.go. Implement all 7 methods:

| Method | Source | Logic |
|--------|--------|-------|
| `pluginUser` | new | returns `""` |
| `start` | `upSingle` L40-44 | `driver.StartContainer`, returns same ID |
| `buildImage` | thin wrapper | delegates to `e.buildImage(ctx, ws, cfg)` |
| `createContainer` | `upSingle` L98-135 + `runPreContainerRunPlugins` | buildRunOptions + applyFeatureMetadata + merge pluginResp into RunOptions + RunContainer + FindContainer |
| `deleteExisting` | `upSingle` L71-78 | FindContainer + DeleteContainer |
| `restart` | `restartSimple` single branch L155-166 | `driver.RestartContainer`, returns same ID |
| `canResumeFromStored` | new | returns `false` |

Key: `createContainer` inlines the `runPreContainerRunPlugins` merge logic
(mounts append, env append, runArgs append) since only single-container needs it.

**Tests:** `backend_single_test.go`
- `TestSingleBackend_CreateContainer_MergesPluginResponse` (use `restartMockDriver` pattern from restart_test.go)
- `TestSingleBackend_PluginUser_ReturnsEmpty`
- `TestSingleBackend_CanResumeFromStored_ReturnsFalse`

**Commit:** `refactor(engine): implement singleBackend`

## Step 3: composeBackend implementation

**New file: `internal/engine/backend_compose.go`**

Move struct definition from backend.go. Implement all 7 methods:

| Method | Source | Logic |
|--------|--------|-------|
| `pluginUser` | compose.go L60,99,148 | `e.resolveComposeUser(ctx, cfg, inv.files)` |
| `start` | `upCompose` L54-91 | load stored image, check snapshot, regenerate override, `compose.Start`, `findComposeContainer` |
| `buildImage` | `upCompose` L134-144 | feature build only when `len(cfg.Features) > 0`, else empty `buildResult` |
| `createContainer` | `upCompose` L156-207 + `upComposeFromStored` L237-257 | generate override, optionally `compose.Build` (skip when `opts.skipBuild`), `compose.Up`, `findComposeContainer` |
| `deleteExisting` | compose.go | `e.composeDown(ctx, inv, false)` |
| `restart` | `restartSimple` compose branch L96-147 | regenerate override, `compose.Stop` + `compose.Start`, `findComposeContainer` |
| `canResumeFromStored` | new | returns `true` |

Key: `createContainer` respects `opts.skipBuild` to match current
`upComposeFromStored` behavior (no compose build on stored resume).

**Tests:** `backend_compose_test.go`
- `TestComposeBackend_PluginUser_Delegates`
- `TestComposeBackend_CanResumeFromStored_ReturnsTrue`
- `TestComposeBackend_BuildImage_SkipsWhenNoFeatures`

**Commit:** `refactor(engine): implement composeBackend`

## Step 4: Unified finalize method

**New file: `internal/engine/finalize.go`**

- `finalizeOpts` struct (cc, imageName, hasEntrypoints, pluginResp, storedResult, fromSnapshot, skipVolumeChown)
- `finalize` method on `*Engine`:
  1. `execPluginCopies` (always)
  2. `chownPluginVolumes` (unless `skipVolumeChown`)
  3. `resolveRemoteUser` (unless `cc.remoteUser` already set)
  4. Env building: `fromSnapshot` uses `resolveConfigEnvFromStored` + `RestoreFrom`; fresh uses `NewEnvBuilder(cfg.RemoteEnv)`
  5. Early save with full metadata (imageName, hasEntrypoints)
  6. Lifecycle: `fromSnapshot` -> `runResumeHooks`; fresh -> `setupContainer` + `commitSnapshot`
  7. Final save
- `toRestartResult` helper converting `*UpResult` to `*RestartResult`

**Reuse:** `resolveConfigEnvFromStored` (restart.go), `runResumeHooks` (restart.go),
`setupContainer` (setup.go), `commitSnapshot` (snapshot.go), `execPluginCopies` (single.go),
`chownPluginVolumes` (single.go), `portSpecToBindings`/`collectPorts` (single.go)

**Tests:** `finalize_test.go`
- `TestFinalize_FreshSetup_RunsPluginCopiesAndChown`
- `TestFinalize_FreshSetup_CallsSetupContainerAndCommitsSnapshot`
- `TestFinalize_FromSnapshot_RestoresStoredEnvAndRunsResumeHooks`
- `TestFinalize_FromSnapshot_SkipsSetupAndSnapshot`
- `TestFinalize_SkipVolumeChown`
- `TestFinalize_EarlySave_BeforeLifecycleHooks`
- `TestFinalize_PreservesPathPrepend_FreshSetup`
- `TestFinalize_PreservesPathPrepend_FromSnapshot`
- `TestFinalize_RemoteUserSkippedWhenPreset`

Uses existing `mockDriver`/`fixedFindContainerDriver` from setup_test.go.

**Commit:** `refactor(engine): add unified finalize method`

## Step 5: Rewrite Up() + delete old up code + migrate tests

This is the largest step. Rewrite `Up()` in engine.go, delete old functions,
migrate affected tests.

**Modify: `internal/engine/engine.go`**

Rewrite `Up()`:
1. parseAndSubstitute, initializeCommand (unchanged)
2. Compose guards: check `e.compose == nil` and `cfg.Service == ""`
3. `b := e.newBackend(ws, cfg, workspaceFolder)`
4. `driver.FindContainer` for existing container check
5. Route to `upExisting` / `upCreate` / `upFromImage`
6. Remove redundant `saveResult` after return (finalize handles it)

New private methods in engine.go:
- `upExisting(ctx, ws, cfg, workspaceFolder, b, container)` - load stored for imageName, dispatch plugins, b.start if stopped, finalize
- `upCreate(ctx, ws, cfg, workspaceFolder, b, isRecreate)` - check snapshot/stored, route to upFromImage or fresh build path
- `upFromImage(ctx, ws, cfg, workspaceFolder, b, imageName, stored, isSnapshot)` - dispatch plugins, b.createContainer(skipBuild:true), finalize

**Delete from single.go:**
- `upSingle`, `upSingleFromSnapshot`, `finalizeSetup`, `finalizeFromSnapshot`, `setupAndReturn`, `runPreContainerRunPlugins`

**Delete from compose.go:**
- `upCompose`, `upComposeFromStored`

**Delete from engine.go:**
- `recreateComposeServices`

**Test migration (32 tests affected):**

Tests calling `upSingle` (11 tests in single_test.go + up_snapshot_test.go):
- 3 in single_test.go (`TestUpSingle_AlreadyRunning_*`) -> migrate to call `Up()` or rewrite as finalize/backend tests
- 8 in up_snapshot_test.go (`TestUpSingle_FromSnapshot_*`) -> migrate to call `Up()` or finalize

Tests calling `finalizeFromSnapshot` (3 in up_snapshot_test.go):
- `TestFinalizeFromSnapshot_*` -> migrate to call `finalize()` directly

Tests calling `runPreContainerRunPlugins` (6 in plugin_test.go):
- `TestRunPreContainerRunPlugins_*` -> migrate to test `singleBackend.createContainer` or `dispatchPlugins` (the merge logic moves into `singleBackend.createContainer`)

**Commit:** `refactor(engine): rewrite Up() with containerBackend + finalize`

## Step 6: Rewrite Restart() + delete old restart code + migrate tests + docs

**Modify: `internal/engine/restart.go`**

Rewrite `Restart()` body to create backend and route to:
- `restartSimple(ctx, ws, cfg, workspaceFolder, b, storedResult)` - find container, dispatch plugins, `b.restart(pluginResp)`, finalize with `fromSnapshot:true`, `skipVolumeChown:true`, pre-set `cc.remoteUser`
- `restartRecreate(ctx, ws, cfg, workspaceFolder, b)` - load stored, `Down()`, check snapshot, dispatch plugins, `b.createContainer`, finalize

**Delete from restart.go:**
- Old `restartSimple` (name collision, different signature)
- `restartWithRecreate`, `restartRecreateCompose`, `restartRecreateSingle`, `runRecreateLifecycle`

**Test migration (12 tests affected):**

Tests calling `restartSimple` (7 tests):
- `TestRestartSimple_NonCompose_*` -> migrate to call `Restart()` through public API or rewrite as backend + finalize tests

Tests calling `restartRecreateSingle` (5 tests):
- `TestRestartRecreateSingle_*` -> migrate to call `Restart()` or rewrite

Safe tests (no changes needed):
- `TestDetectConfigChange_*` (14 tests) - test pure function
- `TestRunResumeHooks_*` (1 test) - test stays
- `TestResolveConfigEnvFromStored*` (5 tests) - test pure function

**Update docs:**
- `docs/ai-instructions/engine.instructions.md` - update save sites (6->2), code paths
- `docs/decisions/001-no-save-path-abstraction.md` - set status "Superseded"

**Commit:** `refactor(engine): rewrite Restart() with containerBackend + finalize`

## File Summary

| File | Action |
|------|--------|
| `internal/engine/backend.go` | **New** - interface, createOpts, factory |
| `internal/engine/backend_single.go` | **New** - singleBackend |
| `internal/engine/backend_compose.go` | **New** - composeBackend |
| `internal/engine/finalize.go` | **New** - finalizeOpts, finalize, toRestartResult |
| `internal/engine/engine.go` | **Modify** - rewrite Up(), add upExisting/upCreate/upFromImage, delete recreateComposeServices |
| `internal/engine/restart.go` | **Modify** - rewrite Restart(), replace 5 functions with 2 |
| `internal/engine/single.go` | **Modify** - delete 6 functions (keep buildRunOptions, applyFeatureMetadata, dispatchPlugins, execPluginCopies, chownPluginVolumes, collectPorts, portSpecToBindings, etc.) |
| `internal/engine/compose.go` | **Modify** - delete 2 functions (keep generateComposeOverride, buildComposeFeatures, resolveComposeUser, composeDown, findComposeContainer, etc.) |
| `internal/engine/backend_single_test.go` | **New** |
| `internal/engine/backend_compose_test.go` | **New** |
| `internal/engine/finalize_test.go` | **New** |
| `internal/engine/single_test.go` | **Modify** - migrate 3 tests |
| `internal/engine/up_snapshot_test.go` | **Modify** - migrate 11 tests |
| `internal/engine/plugin_test.go` | **Modify** - migrate 6 tests |
| `internal/engine/restart_test.go` | **Modify** - migrate 12 tests |
| `docs/ai-instructions/engine.instructions.md` | **Modify** |
| `docs/decisions/001-no-save-path-abstraction.md` | **Modify** |

## Key Reusable Functions (not moved, called by backends)

| Function | File | Used by |
|----------|------|---------|
| `buildRunOptions` | single.go:182 | singleBackend.createContainer |
| `applyFeatureMetadata` | single.go:263 | singleBackend.createContainer |
| `dispatchPlugins` | single.go:488 | shared orchestration (Up, Restart) |
| `execPluginCopies` | single.go:555 | finalize |
| `chownPluginVolumes` | single.go:368 | finalize |
| `collectPorts` | single.go:429 | finalize |
| `portSpecToBindings` | single.go:451 | finalize |
| `buildImage` | build.go:27 | singleBackend.buildImage |
| `buildComposeFeatures` | compose.go:273 | composeBackend.buildImage |
| `generateComposeOverride` | compose.go:382 | composeBackend.createContainer/start/restart |
| `resolveComposeUser` | compose.go:309 | composeBackend.pluginUser |
| `findComposeContainer` | compose.go:678 | composeBackend.start/createContainer/restart |
| `ensureContainerRunning` | compose.go:638 | composeBackend.start/createContainer |
| `composeDown` | compose.go:579 | composeBackend.deleteExisting |
| `resolveFeatureMetadata` | build.go:343 | composeBackend.createContainer |
| `resolveConfigEnvFromStored` | restart.go:437 | finalize |
| `runResumeHooks` | restart.go:474 | finalize |
| `setupContainer` | setup.go:28 | finalize |
| `commitSnapshot` | snapshot.go:46 | finalize |
| `validSnapshot` | snapshot.go:99 | upCreate, restartRecreate, composeBackend.start/restart |
| `saveResult` | engine.go:218 | finalize |
| `resolveRemoteUser` | engine.go:398 | finalize |
| `configRemoteUser` | engine.go:388 | finalize |
