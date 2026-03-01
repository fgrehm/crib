.PHONY: build install clean test lint test-integration test-e2e setup-hooks help docs

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## Build the crib binary
	@mkdir -p bin
	VERSION=$$(cat VERSION 2>/dev/null || echo "0.0.0")-dev; \
	COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILT=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	go build \
		-ldflags="-X github.com/fgrehm/crib/cmd.Version=$$VERSION -X github.com/fgrehm/crib/cmd.Commit=$$COMMIT -X github.com/fgrehm/crib/cmd.Built=$$BUILT" \
		-o bin/crib .

install: build ## Install crib to ~/.local/bin
	install -d -m 755 ~/.local/bin
	install -m 755 bin/crib ~/.local/bin/crib

test: ## Run unit tests
	go test ./internal/... -short -count=1

lint: ## Run linters
	go tool golangci-lint run

test-integration: ## Run integration tests (requires Docker or Podman)
	go test ./internal/... -run Integration -count=1

test-e2e: ## Run end-to-end tests against the crib binary (requires Docker or Podman)
	go test ./e2e/... -v -count=1 -timeout=10m

setup-hooks: ## Configure git hooks
	@git config core.hooksPath .githooks
	@chmod +x .githooks/*
	@echo "Git hooks configured"

docs: ## Serve documentation from http://localhost:4321/crib
	cd website && npm run dev

clean: ## Remove build artifacts
	rm -rf bin/
