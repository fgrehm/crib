# Development

## Building

Requires Go 1.26+.

```
make build            # produces bin/crib
make test             # run unit tests
make lint             # run linters (golangci-lint v2)
make test-integration # integration tests (requires Docker or Podman)
make setup-hooks      # configure git pre-commit hooks
```
