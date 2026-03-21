---
title: Development
description: Building, testing, and contributing to crib.
---

## Branching

- All work happens on `main`. No long-lived feature branches.
- Releases are tagged (e.g. `v0.3.0`). CI updates `stable` to match automatically after the tag is pushed.
- `README.md` on `stable` is the source of truth for released functionality.
- `README.md` on `main` may describe unreleased work.

## Getting started

```bash
git clone https://github.com/fgrehm/crib.git
cd crib
make setup-hooks      # configure pre-commit hooks (gofmt + golangci-lint)
make build            # produces bin/crib
make test             # run unit tests
```

## Building and testing

Requires Go 1.26+.

```bash
make build            # produces bin/crib
make install          # build + install to ~/.local/bin
make test             # unit tests (go test ./internal/... -short)
make lint             # golangci-lint v2 (errcheck, govet, staticcheck, unused, ineffassign)
make test-integration # integration tests (requires Docker or Podman)
make test-e2e         # end-to-end tests against the crib binary
make audit            # cyclomatic complexity + dead code analysis (informational)
make docs             # serve docs site at http://localhost:4321/crib
```

Run a single test:

```bash
go test ./internal/config/ -short -run TestParseFull
```

## Code style

- Go module: `github.com/fgrehm/crib`
- All packages under `internal/` (this is a binary, not a library)
- CLI: [spf13/cobra](https://github.com/spf13/cobra). Logging: `log/slog`.
- Linting: golangci-lint v2 with errcheck, govet, staticcheck, unused, ineffassign
- Pre-commit hooks run `gofmt` and `golangci-lint` on staged `.go` files

## Commit format

[Conventional commits](https://www.conventionalcommits.org/), present tense, under 72 characters.

```
feat(auth): add OAuth login support
fix: resolve memory leak in background tasks
docs: add troubleshooting entry for pasta networking
```

Use scopes when they clarify the component. Skip them for broad changes.

## Pull requests

- All work happens on `main`. No long-lived feature branches.
- Open PRs against `main`.
- Keep PRs focused. One feature or fix per PR.
- Run `make test && make lint` before pushing.
- Update `CHANGELOG.md` under `[Unreleased]` for user-facing changes. Internal refactors that preserve behavior need no entry.

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

Dependency flow: `cmd/ -> engine/ -> {config/, feature/, driver/, compose/, dockerfile/, workspace/}`. No cycles.

See [Implementation Notes](/crib/contributing/implementation-notes/) for deeper details on quirks, workarounds, and spec compliance.
