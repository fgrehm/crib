# AGENTS.md

Guidance for coding agents when working in this repository.

Detailed instructions for specific areas are in `docs/ai-instructions/`. Read them when working on the relevant code:

- `engine.instructions.md` - dual code paths, remoteEnv invariants, save sites
- `plugins.instructions.md` - plugin dispatch, wiring, parameter order
- `logging.instructions.md` - output mechanisms, slog rules, verbose/debug
- `docs.instructions.md` - naming conventions, docs workflow, changelog

## What is crib

Dev containers without the ceremony. crib reads `.devcontainer` configs, builds the container, and gets out of the way. CLI only, no IDE integration.

## Architecture

See `ARCHITECTURE.md` at the repo root for the codemap, module relationships, and invariants. Read it before making structural changes.

## Operational notes for agents

These supplement `ARCHITECTURE.md` with things relevant when making changes:

- Workspace state tracks `CribVersion` (refreshed on every access via `currentWorkspace()` in `cmd/root.go`). The field is recorded but no version-dependent logic exists yet. When adding migrations, snapshot invalidation, or breaking state changes, use `ws.CribVersion` to gate behavior by the version that last touched the workspace.
- Container naming: `crib-{workspace-id}`. Labels: `crib.workspace=<id>`, `crib.home=<store-base-dir>` (the second enables multi-store isolation in tests and CI, never assume a single global store).
- Runtime detection: `CRIB_RUNTIME` env > podman > docker. Don't hardcode either runtime in new code.

## Commands

Requires Go 1.26+.

```bash
make build            # compile to dist/crib (injects version via ldflags)
make test             # run unit tests (-race -shuffle=on -short)
make lint             # golangci-lint v2 (go tool)
make fmt              # format with gofumpt/goimports (go tool)
make deadcode         # check for unreachable functions
make audit            # cyclomatic complexity check (gocyclo, informational)
make govulncheck      # run vulnerability check
make coverage         # generate HTML coverage report
make vendor           # tidy and vendor dependencies
make install          # build and install to ~/.local/bin
make setup-hooks      # configure .githooks/ pre-commit hook
make clean            # remove build artifacts
make test-integration # integration tests (requires Docker or Podman)
make test-e2e         # end-to-end tests against the crib binary
make docs             # serve documentation at http://localhost:4321/crib
```

Run a single test: `go test ./internal/config/ -short -run TestParseFull`

### Integration tests

Integration tests live alongside unit tests in `*_integration_test.go` files (primarily in `internal/engine/`). They require Docker or Podman and are skipped by `-short`. Run them with `make test-integration`.

**Pattern**: `newTestEngine(t)` creates an engine with a real `OCIDriver` and a temp-dir workspace store. Tests create a temp project dir, write `.devcontainer/devcontainer.json`, build a `workspace.Workspace` struct, call `e.Up(ctx, ws, UpOptions{})`, then verify side effects via `d.ExecContainer(ctx, ...)`. Cleanup with `t.Cleanup` deletes containers and images via `cleanupWorkspaceImages(t, d, wsID)`.

**Convention**: Test function names start with `TestIntegration`. Workspace IDs use `test-engine-<suffix>` to avoid collisions. Use `alpine:3.20` as the base image (small, fast to pull). Local features go in the temp project's `.devcontainer/` directory.

**Requirement**: Always write integration tests for new engine features that touch the container lifecycle (hooks, env, user, features). Unit tests with mock drivers are good for logic but miss real Docker/Podman behavior.

### Parallel test safety

E2E tests use `t.Parallel()`. Integration tests do not (container runtime contention on 2-core CI runners). When adding or modifying parallel tests that touch real containers, check:

- **Unique workspace IDs**: directory basenames must differ across parallel tests. Avoid fixed names like `"my-project"`. Derive from `t.Name()` or use `t.TempDir()` basenames.
- **Scoped assertions**: never query all containers/images globally (e.g., `docker images --filter reference=crib-*`). Always scope to the specific workspace ID or container name.
- **Log noise tolerance**: parallel tests cause transient warnings (containers vanishing between list and inspect). Assert on structured output (summary lines, exit codes) rather than scanning for words like "error" in raw output.
- **Subtest sequencing**: when subtests share container state, document that they must not call `t.Parallel()`. The parent test can be parallel.

## Conventions

- Go module: `github.com/fgrehm/crib`
- All packages under `internal/`; this is a binary, not a library.
- CLI: `spf13/cobra`. Logging: `log/slog`.
- Linting: golangci-lint v2 (errcheck, govet, staticcheck, unused, ineffassign).
- Pre-commit hooks: gofmt + golangci-lint + gocyclo (threshold 30, tests excluded) on staged Go files.

## Key Reference Pages

- `ARCHITECTURE.md` - codemap, module relationships, invariants
- `docs/devcontainers-spec.md` - quick-lookup companion to the [official spec](https://containers.dev/implementors/spec/)
- `docs/implementation-notes.md` - quirks, workarounds, spec compliance status
- `docs/plugin-development.md` - plugin interface, response types, merge rules
- `docs/decisions/` - architecture decision records

## CHANGELOG

This project uses [Keep a Changelog](https://keepachangelog.com/) format. When adding features, fixing bugs, or making breaking changes, add an entry under the `[Unreleased]` section of `CHANGELOG.md` before the session ends. Categories: Added, Changed, Deprecated, Removed, Fixed, Security.

Before wrapping up a session, check whether CHANGELOG.md needs an update for the work done.

## Releasing

1. Move `CHANGELOG.md` `[Unreleased]` entries into `[X.Y.Z] - YYYY-MM-DD`.
2. Update `VERSION` file.
3. Commit: `chore: release vX.Y.Z`.
4. Tag and push: `git tag vX.Y.Z && git push origin main vX.Y.Z`

The rest is automated:

- `release.yml` (triggered by the tag) extracts release notes from `CHANGELOG.md`, runs GoReleaser, and force-pushes the `stable` branch.
- `docs.yml` (triggered by the push to `stable`) runs `scripts/sync-changelog.sh`, which regenerates `website/src/content/docs/reference/changelog.md` from `CHANGELOG.md` (strips `[Unreleased]`, rewrites version headers as GitHub release links), then builds and deploys the site.

Do not edit `website/src/content/docs/reference/changelog.md` by hand; it is overwritten on every docs build.
