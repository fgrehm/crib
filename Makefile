.PHONY: build install clean test lint fmt audit deadcode govulncheck coverage vendor test-integration test-e2e setup-hooks help docs

# Build variables
BASE_VERSION := $(shell cat VERSION 2>/dev/null || echo "0.0.0")
GIT_TAG := $(shell git describe --exact-match --tags 2>/dev/null)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

ifeq ($(GIT_TAG),)
  VERSION := $(BASE_VERSION)-dev+$(shell date -u +"%Y%m%d%H%M%S")
else
  VERSION := $(patsubst v%,%,$(GIT_TAG))
endif

LDFLAGS := -X github.com/fgrehm/crib/cmd.version=$(VERSION) \
           -X github.com/fgrehm/crib/cmd.commit=$(COMMIT) \
           -X github.com/fgrehm/crib/cmd.date=$(DATE)

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## Build the crib binary
	@mkdir -p dist
	@go build -ldflags "$(LDFLAGS)" -o dist/crib .
	@echo "✓ Built to dist/crib"

install: build ## Install crib to ~/.local/bin (symlink)
	@mkdir -p "$(HOME)/.local/bin"
	@ln -sf "$(CURDIR)/dist/crib" "$(HOME)/.local/bin/crib"
	@echo "✓ Installed to ~/.local/bin/crib"

test: ## Run unit tests
	go test -race -shuffle=on $(GO_TEST_FLAGS) ./internal/... -short -count=1

lint: ## Run linters
	go tool golangci-lint run

fmt: ## Format code with gofumpt and goimports
	go tool golangci-lint fmt ./...

audit: ## Run complexity and vulnerability checks (informational)
	@echo "=== Cyclomatic complexity (>15) ==="
	@go tool gocyclo -over 15 -ignore 'vendor/' . || true
	@echo ""
	@echo "=== Vulnerability check ==="
	@go tool govulncheck ./... || true

deadcode: ## Check for dead code (hard gate, matches CI)
	@out=$$(go tool deadcode -test ./...); \
	if [ -n "$$out" ]; then \
		echo "Unreachable functions detected by deadcode:"; \
		echo "$$out"; \
		exit 1; \
	fi; \
	echo "No dead code found."

govulncheck: ## Run vulnerability check
	@go tool govulncheck ./...

test-integration: ## Run integration tests (requires Docker or Podman)
	go test $(GO_TEST_FLAGS) ./internal/... -run Integration -v -count=1

test-e2e: ## Run end-to-end tests against the crib binary (requires Docker or Podman)
	go test $(GO_TEST_FLAGS) ./e2e/... -v -count=1 -timeout=10m

setup-hooks: ## Configure git hooks
	@git config core.hooksPath .githooks
	@chmod +x .githooks/*
	@echo "✓ Git hooks configured"

docs: ## Serve documentation from http://localhost:4321/crib
	cd website && npm run dev

coverage: ## Generate test coverage report
	go test -race -shuffle=on -coverprofile=coverage.txt ./internal/... -short -count=1
	go tool cover -html=coverage.txt -o coverage.html

vendor: ## Tidy and vendor dependencies
	go mod tidy
	go mod vendor

clean: ## Remove build artifacts
	rm -rf dist/
	rm -f coverage.txt coverage.html
