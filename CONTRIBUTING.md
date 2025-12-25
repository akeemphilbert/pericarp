# Contributing to Pericarp

Thank you for your interest in contributing to Pericarp! This document provides guidelines and instructions for contributing to the project.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/your-username/pericarp.git`
3. Create a feature branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Run tests: `make test`
6. Ensure code is formatted: `make fmt`
7. Run linter: `make lint`
8. Commit your changes: `git commit -m "Add your feature"`
9. Push to your fork: `git push origin feature/your-feature-name`
10. Open a pull request

## Development Workflow

### Prerequisites

- Go 1.21 or later
- Make (for using the Makefile)
- golangci-lint (install with `make install-tools`)

### Development Commands

```bash
# Set up development environment
make deps

# Run all tests
make test

# Format code
make fmt

# Run linter
make lint

# Run development workflow (format, lint, test)
make dev-test
```

## Code Style

- Follow Go standard formatting (`go fmt`)
- Use `golangci-lint` for code quality checks
- Write tests for all new functionality
- Use table-driven tests where appropriate
- Document exported functions and types

## Testing

- Write unit tests for all new code
- Ensure all tests pass before submitting a PR
- Aim for high test coverage
- Use integration tests for complex behavior

## Commit Messages

- Use clear, descriptive commit messages
- Reference issue numbers when applicable
- Follow conventional commit format when possible

## Pull Request Process

1. Ensure all tests pass
2. Update documentation if needed
3. Add tests for new functionality
4. Ensure code is formatted and linted
5. Request review from maintainers

## Project Structure

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for details on the project structure and architecture.

## Questions?

If you have questions, please open an issue or start a discussion.
