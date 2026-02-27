# Development

## Branching

- All work happens on `main`. No long-lived feature branches.
- Releases are tagged (e.g. `v0.3.0`) and the `stable` branch is updated to match.
- `README.md` on `stable` is the source of truth for released functionality.
- `README.md` on `main` may describe unreleased work.

## Building

Requires Go 1.26+.

```
make build            # produces bin/crib
make test             # run unit tests
make lint             # run linters (golangci-lint v2)
make test-integration # integration tests (requires Docker or Podman)
make test-e2e         # end-to-end tests against the crib binary
make setup-hooks      # configure git pre-commit hooks
```
