# Pericarp Go Library Makefile

.PHONY: help build build-cli test test-unit test-integration clean deps fmt lint

# The one golangci-lint version this repository runs, locally and in CI.
# Bump it here and in .github/workflows/ci.yml in the same commit.
GOLANGCI_LINT_VERSION ?= v2.11.3

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: ## Build the library
	go build -v ./...

build-cli: ## Build the pericarp CLI (migration tool) into bin/
	mkdir -p bin
	go build -o bin/pericarp ./cmd/pericarp

# Test targets
test: test-unit ## Run all tests

test-unit: ## Run unit tests
	go test -v -race -coverprofile=coverage.out ./...

test-integration: ## Run integration tests
	go test -v -tags=integration ./test/integration/...

test-coverage: ## Generate test coverage report
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Development targets
deps: ## Download and tidy dependencies
	go mod download
	go mod tidy

fmt: ## Format code
	go fmt ./...

lint: ## Run linter (requires golangci-lint $(GOLANGCI_LINT_VERSION))
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint is not installed. Run: make install-tools"; \
		exit 1; \
	fi
	@golangci-lint --version | grep -q "$(patsubst v%,%,$(GOLANGCI_LINT_VERSION))" || \
		echo "warning: golangci-lint is not $(GOLANGCI_LINT_VERSION) (CI runs that version). Run: make install-tools"
	golangci-lint run

# Clean targets
clean: ## Clean build artifacts
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f *.db
	go clean -testcache

# Development workflow
dev-test: fmt lint test ## Run development tests (format, lint, test)

# CI targets
ci: deps fmt lint test ## Run CI pipeline

# Install tools
install-tools: ## Install development tools
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

# Version targets
version: ## Show version information
	@echo "Go version: $(shell go version)"
	@echo "Git commit: $(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
	@echo "Build date: $(shell date -u +%Y-%m-%dT%H:%M:%SZ)"

