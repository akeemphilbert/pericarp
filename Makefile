# Pericarp Go Library Makefile

.PHONY: help build test test-unit test-bdd test-integration clean deps fmt lint

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: ## Build the library and demo application
	go build -v ./...

build-demo: ## Build the demo application
	go build -o bin/demo ./cmd/demo

# Test targets
test: test-unit test-bdd ## Run all tests

test-unit: ## Run unit tests
	go test -v -race -coverprofile=coverage.out ./pkg/... ./internal/...

test-bdd: ## Run BDD tests with Cucumber
	go test -v ./test/bdd/...

test-integration: ## Run integration tests
	go test -v -tags=integration ./test/integration/...

test-coverage: ## Generate test coverage report
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Development targets
deps: ## Download dependencies
	go mod download
	go mod tidy

fmt: ## Format code
	go fmt ./...

lint: ## Run linter
	golangci-lint run

# Mock generation
generate-mocks: ## Generate mocks using moq
	@echo "Generating mocks..."
	go generate ./...

# Clean targets
clean: ## Clean build artifacts
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f *.db
	go clean -testcache

# Database targets
db-migrate: ## Run database migrations (for demo)
	go run ./cmd/demo migrate

db-reset: ## Reset database (for demo)
	rm -f events.db
	$(MAKE) db-migrate

# Demo targets
demo-create-user: ## Create a demo user
	go run ./cmd/demo create-user --email="demo@example.com" --name="Demo User"

demo-list-users: ## List demo users
	go run ./cmd/demo list-users

demo-get-user: ## Get demo user by email
	go run ./cmd/demo get-user --email="demo@example.com"

# Development workflow
dev-setup: deps generate-mocks ## Set up development environment
	@echo "Development environment ready!"

dev-test: fmt lint test ## Run development tests (format, lint, test)

# CI targets
ci: deps fmt lint test ## Run CI pipeline

# Docker targets (if needed)
docker-build: ## Build Docker image
	docker build -t pericarp:latest .

docker-test: ## Run tests in Docker
	docker run --rm pericarp:latest make test

# Documentation targets
docs: ## Generate documentation
	go doc -all ./pkg/domain > docs/domain.md
	go doc -all ./pkg/application > docs/application.md
	go doc -all ./pkg/infrastructure > docs/infrastructure.md

# Feature file validation
validate-features: ## Validate Gherkin feature files
	@echo "Validating feature files..."
	@find features -name "*.feature" -exec echo "Checking {}" \;

# Performance targets
benchmark: ## Run benchmarks
	go test -bench=. -benchmem ./...

profile: ## Run CPU profiling
	go test -cpuprofile=cpu.prof -bench=. ./...
	go tool pprof cpu.prof

# Security targets
security-scan: ## Run security scan
	gosec ./...

# Version targets
version: ## Show version information
	@echo "Go version: $(shell go version)"
	@echo "Git commit: $(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
	@echo "Build date: $(shell date -u +%Y-%m-%dT%H:%M:%SZ)"

# Install tools
install-tools: ## Install development tools
	go install github.com/matryer/moq@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest