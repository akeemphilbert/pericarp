# Pericarp Go Library

A comprehensive Go library implementing Domain-Driven Design (DDD), Command Query Responsibility Segregation (CQRS), and Event Sourcing patterns with clean architecture principles.

## Quick Start

```go
package main

import (
    "context"
    "log"
    
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing"
    "github.com/akeemphilbert/pericarp/pkg/application"
    "go.uber.org/fx"
)

func main() {
    app := fx.New(
        application.Module,
        fx.Invoke(func(bus application.CommandBus) {
            // Use the command bus
        }),
    )
    app.Run()
}
```

## Features

- **Clean Architecture**: Strict separation between domain, application, and infrastructure layers
- **CQRS Pattern**: Separate command and query handling with unified middleware support
- **Event Sourcing**: Persist-then-dispatch pattern with event store and dispatcher
- **Database Flexibility**: Support for SQLite (development) and PostgreSQL (production)
- **Dependency Injection**: Built-in Fx modules for easy configuration
- **Comprehensive Testing**: Unit tests and integration tests

## Installation

### Prerequisites

- Go 1.21 or later
- Go modules enabled (default in Go 1.16+)

### Install Latest Version

Install the latest version of Pericarp with a single command:

```bash
go get github.com/akeemphilbert/pericarp
```

### Install Specific Version

To install a specific version, use the version tag:

```bash
# Install a specific version (e.g., v1.0.0)
go get github.com/akeemphilbert/pericarp@v1.0.0

# Install the latest version from a specific branch
go get github.com/akeemphilbert/pericarp@main
```

### Verify Installation

After installation, verify the package is available:

```bash
go list -m github.com/akeemphilbert/pericarp
```

### Documentation

- 📦 [pkg.go.dev](https://pkg.go.dev/github.com/akeemphilbert/pericarp) - Official Go package documentation
- 📖 [GitHub Repository](https://github.com/akeemphilbert/pericarp) - Source code and issues

## Project Structure

The project follows the [golang-standards/project-layout](https://github.com/golang-standards/project-layout) structure:

```
pericarp/
├── pkg/              # Public API packages
│   ├── eventsourcing/ # Event sourcing primitives (events, event stores)
│   ├── application/  # Command/query handlers, CQRS bus
│   └── infrastructure/ # Event store, database implementations
├── internal/         # Private implementation packages
├── cmd/pericarp/     # CLI tools/demos
├── examples/         # Runnable examples for users
├── test/             # Integration tests
├── docs/             # Documentation
├── scripts/          # Build/automation scripts
└── configs/          # Configuration templates
```

For detailed architecture information, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Documentation

- 📚 [Tutorial](docs/tutorial/README.md) - Step-by-step tutorials
- 🔧 [How-to Guides](docs/how-to/README.md) - Practical solutions
- 📖 [Reference](docs/reference/README.md) - API documentation
- 💡 [Explanation](docs/explanation/README.md) - Design decisions

## Development

### Prerequisites

- Go 1.21 or later
- Make (for using the Makefile)

### Development Commands

```bash
# Set up development environment
make deps

# Run tests
make test

# Format code
make fmt

# Run linter
make lint

# Run development workflow (format, lint, test)
make dev-test
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on contributing to the project.

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Support

- 📖 [Documentation](docs/)
- 🐛 [Issues](https://github.com/akeemphilbert/pericarp/issues)
