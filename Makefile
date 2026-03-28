.PHONY: build install clean test lint fmt audit deadcode vendor test-integration test-e2e setup-hooks help docs

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## Build the crib binary
	@mkdir -p dist
	VERSION=$$(cat VERSION 2>/dev/null || echo "0.0.0")-dev; \
	COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILT=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	go build \
		-ldflags="-X github.com/fgrehm/crib/cmd.Version=$$VERSION -X github.com/fgrehm/crib/cmd.Commit=$$COMMIT -X github.com/fgrehm/crib/cmd.Built=$$BUILT" \
		-o dist/crib .

install: build ## Install crib to ~/.local/bin (symlink)
	@mkdir -p $(HOME)/.local/bin
	@ln -sf $(PWD)/dist/crib $(HOME)/.local/bin/crib
	@echo "✓ Installed to ~/.local/bin/crib"

test: ## Run unit tests
	go test -race -shuffle=on $(GO_TEST_FLAGS) ./internal/... -short -count=1

lint: ## Run linters
	go tool golangci-lint run

fmt: ## Format code with gofumpt and goimports
	go tool golangci-lint fmt ./...

audit: ## Run complexity and dead-code analysis (informational)
	@echo "=== Cyclomatic complexity (>15) ==="
	@go tool gocyclo -over 15 . || true
	@echo ""
	@echo "=== Dead code ==="
	@go tool deadcode ./... 2>&1 || true

deadcode: ## Check for dead code (hard gate, matches CI)
	@out=$$(go tool deadcode ./...); \
	if [ -n "$$out" ]; then \
		echo "Unreachable functions detected by deadcode:"; \
		echo "$$out"; \
		exit 1; \
	fi; \
	echo "No dead code found."

test-integration: ## Run integration tests (requires Docker or Podman)
	go test $(GO_TEST_FLAGS) ./internal/... -run Integration -v -count=1

test-e2e: ## Run end-to-end tests against the crib binary (requires Docker or Podman)
	go test $(GO_TEST_FLAGS) ./e2e/... -v -count=1 -timeout=10m

setup-hooks: ## Configure git hooks
	@git config core.hooksPath .githooks
	@chmod +x .githooks/*
	@echo "Git hooks configured"

docs: ## Serve documentation from http://localhost:4321/crib
	cd website && npm run dev

vendor: ## Tidy and vendor dependencies
	go mod tidy
	go mod vendor

clean: ## Remove build artifacts
	rm -rf dist/
