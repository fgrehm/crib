# CLAUDE.md

Guidance for Claude Code when working in this repository.

Detailed instructions for specific areas are in `docs/ai-instructions/`. Read
them when working on the relevant code:

- `engine.instructions.md` - dual code paths, remoteEnv invariants, save sites
- `plugins.instructions.md` - plugin dispatch, wiring, parameter order
- `logging.instructions.md` - output mechanisms, slog rules, verbose/debug
- `docs.instructions.md` - naming conventions, docs workflow, changelog

## What is crib

Dev containers without the ceremony. crib reads `.devcontainer` configs, builds
the container, and gets out of the way. CLI only, no IDE integration.

## Architecture

```
cmd/           -> CLI (cobra). Thin layer, delegates to engine/.
internal/
  config/      -> devcontainer.json parsing, variable substitution, merging
  feature/     -> DevContainer Features (OCI resolution, ordering, Dockerfile generation)
  engine/      -> Core orchestration (up/down/remove flows, lifecycle hooks)
  driver/      -> Container runtime abstraction (Docker/Podman via single OCI driver)
  compose/     -> Docker Compose / Podman Compose helper
  plugin/      -> Plugin system (codingagents, packagecache, shellhistory, ssh)
  workspace/   -> Workspace state management (~/.crib/workspaces/)
  dockerfile/  -> Dockerfile parsing and rewriting
```

Dependency flow: `cmd/ -> engine/ -> {config/, feature/, driver/, compose/,
dockerfile/, workspace/}`. No cycles.

## Key Design Decisions

- No agent injection. All container setup via `docker exec` from the host.
- Docker and Podman through a single `OCIDriver` (not separate implementations).
- Implicit workspace resolution from `cwd` (walk up to find `.devcontainer/`).
- Container naming: `crib-{workspace-id}`, labels: `crib.workspace=<id>`,
  `crib.home=<store-base-dir>` (for multi-store isolation in tests/CI).
- State stored in `~/.crib/workspaces/{id}/`.
- Runtime detection: `CRIB_RUNTIME` env var > podman > docker.
- Workspace state tracks `CribVersion` (refreshed on every access via
  `currentWorkspace()` in `cmd/root.go`). The field is recorded but no
  version-dependent logic exists yet. When adding migrations, snapshot
  invalidation, or breaking state changes, use `ws.CribVersion` to gate
  behavior by the version that last touched the workspace.

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

Integration tests live alongside unit tests in `*_integration_test.go` files
(primarily in `internal/engine/`). They require Docker or Podman and are skipped
by `-short`. Run them with `make test-integration`.

**Pattern**: `newTestEngine(t)` creates an engine with a real `OCIDriver` and a
temp-dir workspace store. Tests create a temp project dir, write
`.devcontainer/devcontainer.json`, build a `workspace.Workspace` struct, call
`e.Up(ctx, ws, UpOptions{})`, then verify side effects via
`d.ExecContainer(ctx, ...)`. Cleanup with `t.Cleanup` deletes containers and
images via `cleanupWorkspaceImages(t, d, wsID)`.

**Convention**: Test function names start with `TestIntegration`. Workspace IDs
use `test-engine-<suffix>` to avoid collisions. Use `alpine:3.20` as the base
image (small, fast to pull). Local features go in the temp project's
`.devcontainer/` directory.

**Requirement**: Always write integration tests for new engine features that
touch the container lifecycle (hooks, env, user, features). Unit tests with mock
drivers are good for logic but miss real Docker/Podman behavior.

## Conventions

- Go module: `github.com/fgrehm/crib`
- All packages under `internal/`; this is a binary, not a library.
- CLI: `spf13/cobra`. Logging: `log/slog`.
- Linting: golangci-lint v2 (errcheck, govet, staticcheck, unused, ineffassign).
- Pre-commit hooks: gofmt + golangci-lint + gocyclo (threshold 30, tests excluded)
  on staged Go files.

## Key Reference Pages

- `docs/devcontainers-spec.md` - quick-lookup companion to the [official spec](https://containers.dev/implementors/spec/)
- `docs/implementation-notes.md` - quirks, workarounds, spec compliance status
- `docs/plugin-development.md` - plugin interface, response types, merge rules
- `docs/decisions/` - architecture decision records

## CHANGELOG

This project uses [Keep a Changelog](https://keepachangelog.com/) format. When adding
features, fixing bugs, or making breaking changes, add an entry under the `[Unreleased]`
section of `CHANGELOG.md` before the session ends. Categories: Added, Changed, Deprecated,
Removed, Fixed, Security.

Before wrapping up a session, check whether CHANGELOG.md needs an update for the work done.

## Releasing

1. Move `CHANGELOG.md` `[Unreleased]` entries into `[X.Y.Z] - YYYY-MM-DD`.
2. Copy the new version section into
   `website/src/content/docs/reference/changelog.md`. Use GitHub release links
   in headers: `## [X.Y.Z](https://github.com/fgrehm/crib/releases/tag/vX.Y.Z) - YYYY-MM-DD`.
   No `[Unreleased]` on the website.
3. Update `VERSION` file.
4. Commit: `chore: release vX.Y.Z`.
5. Tag and push: `git tag vX.Y.Z && git push origin main vX.Y.Z`

CI extracts release notes from CHANGELOG.md and runs GoReleaser. The `stable` branch is
updated automatically after the tag is pushed.
