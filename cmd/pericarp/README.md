# Pericarp CLI Generator

The Pericarp CLI Generator is a command-line tool that enables developers to scaffold new Pericarp-based projects with automated code generation from various input formats.

## Installation

### Via go install (Recommended)

```bash
go install github.com/akeemphilbert/pericarp/cmd/pericarp@latest
```

### From Source

```bash
git clone https://github.com/akeemphilbert/pericarp.git
cd pericarp
make build-cli
# Binary will be available at bin/pericarp
```

### Pre-built Binaries

Download pre-built binaries from the [releases page](https://github.com/akeemphilbert/pericarp/releases).

## Quick Start

### Create a New Project

```bash
# Create a new project
pericarp new my-service

# Create a new project in a specific directory
pericarp new my-service --destination /path/to/projects

# Create a new project from existing repository
pericarp new my-service --repo https://github.com/user/repo.git
```

### Generate Code from Specifications

```bash
# Generate from OpenAPI specification
pericarp generate --openapi api.yaml

# Generate from Protocol Buffer definition
pericarp generate --proto user.proto

# Generate to specific destination
pericarp generate --openapi api.yaml --destination ./generated
```

### Preview Changes (Dry Run)

```bash
# Preview what would be created without actually creating files
pericarp new my-service --dry-run
pericarp generate --openapi api.yaml --dry-run
```

### Verbose Output

```bash
# Enable verbose output for debugging
pericarp new my-service --verbose
pericarp generate --openapi api.yaml --verbose
```

## Commands

### `pericarp new <project-name>`

Creates a new Pericarp project with proper directory structure and boilerplate code.

**Flags:**
- `--repo, -r`: Git repository URL to clone from
- `--destination, -d`: Destination directory (defaults to project name)
- `--dry-run`: Preview what would be created without actually creating files
- `--verbose, -v`: Enable verbose output

**Examples:**
```bash
# Basic project creation
pericarp new user-service

# Create from existing repository
pericarp new user-service --repo https://github.com/company/base-service.git

# Create in specific directory
pericarp new user-service --destination /workspace/services

# Preview creation
pericarp new user-service --dry-run --verbose
```

### `pericarp generate`

Generates Pericarp code from various input format specifications.

**Flags:**
- `--openapi`: OpenAPI specification file (YAML or JSON)
- `--proto`: Protocol Buffer definition file (.proto)
- `--destination, -d`: Output directory for generated code
- `--dry-run`: Preview what would be generated without creating files
- `--verbose, -v`: Enable verbose output

**Examples:**
```bash
# Generate from OpenAPI
pericarp generate --openapi user-api.yaml

# Generate from Protocol Buffers
pericarp generate --proto user.proto

# Generate to specific directory
pericarp generate --openapi api.yaml --destination ./internal/generated

# Preview generation
pericarp generate --openapi api.yaml --dry-run --verbose
```

### `pericarp formats`

Lists all supported input formats for code generation.

**Example:**
```bash
pericarp formats
```

### `pericarp version`

Shows version information including build details.

**Example:**
```bash
pericarp version
```

## Supported Input Formats

### OpenAPI 3.0 Specifications

The CLI supports OpenAPI 3.0 specifications in both YAML and JSON formats.

**Supported features:**
- Schema definitions → Domain entities
- Property types and validation → Entity properties with validation tags
- Required fields → Required entity properties
- Relationships → Entity associations

**Example OpenAPI schema:**
```yaml
components:
  schemas:
    User:
      type: object
      required:
        - email
        - name
      properties:
        id:
          type: string
          format: uuid
        email:
          type: string
          format: email
        name:
          type: string
          minLength: 1
          maxLength: 100
        isActive:
          type: boolean
          default: true
```

### Protocol Buffer Definitions

The CLI supports Protocol Buffer (.proto) files with message definitions.

**Supported features:**
- Message definitions → Domain entities
- Field types → Entity properties
- Field options → Validation tags
- Service definitions → Command/Query handlers

**Example Proto definition:**
```protobuf
syntax = "proto3";

message User {
  string id = 1;
  string email = 2;
  string name = 3;
  bool is_active = 4;
}

message CreateUserRequest {
  string email = 1;
  string name = 2;
}

service UserService {
  rpc CreateUser(CreateUserRequest) returns (User);
  rpc GetUser(GetUserRequest) returns (User);
}
```

## Generated Project Structure

The CLI generates projects following Pericarp conventions:

```
<project-name>/
├── go.mod                          # Go module with Pericarp dependencies
├── go.sum
├── Makefile                        # Comprehensive development targets
├── README.md                       # Project documentation
├── config.yaml.example            # Configuration template
├── cmd/
│   └── <project-name>/
│       └── main.go                 # Application entry point
├── internal/
│   ├── application/
│   │   ├── commands.go             # Command definitions
│   │   ├── queries.go              # Query definitions
│   │   ├── handlers.go             # Command/Query handlers
│   │   └── projectors.go           # Event projectors
│   ├── domain/
│   │   ├── <entity>.go             # Domain entities
│   │   ├── <entity>_events.go      # Domain events
│   │   └── <entity>_test.go        # Entity tests
│   └── infrastructure/
│       ├── repositories.go         # Repository implementations
│       └── <entity>_repository.go  # Entity-specific repositories
├── pkg/                            # Shared packages (if needed)
└── test/
    ├── fixtures/                   # Test fixtures
    ├── integration/                # Integration tests
    └── mocks/                      # Test mocks
```

## Generated Code Features

### Domain Entities

Generated entities follow aggregate root patterns with:
- Proper struct definitions with validation tags
- Event recording and application methods
- Version tracking for optimistic concurrency
- Comprehensive unit tests

### Repositories

Generated repositories include:
- Interface definitions following repository pattern
- Implementation with CRUD operations
- Proper error handling and context usage
- Unit tests with mocks

### Command/Query Handlers

Generated handlers provide:
- Command and query structures with validation
- Handler implementations with proper error handling
- Integration with domain entities and repositories
- Comprehensive test coverage

### Makefile Targets

Generated Makefiles include:
- `deps`: Install dependencies
- `build`: Build the application
- `test`: Run all tests
- `lint`: Run code quality checks
- `gosec`: Run security scans
- `clean`: Clean build artifacts

## Configuration

### Global Flags

- `--verbose, -v`: Enable verbose output for debugging
- `--help, -h`: Show help information

### Environment Variables

- `PERICARP_LOG_LEVEL`: Set log level (debug, info, warn, error)
- `PERICARP_CONFIG_PATH`: Custom configuration file path

## Error Handling

The CLI provides comprehensive error handling with:
- Clear error messages with actionable guidance
- Appropriate exit codes for different error types
- Validation errors with specific line numbers (for parsing errors)
- Network error handling with retry suggestions

### Exit Codes

- `0`: Success
- `1`: General error
- `2`: Argument error
- `3`: Validation error
- `4`: Parse error
- `5`: Generation error
- `6`: File system error
- `7`: Network error

## Development

### Building from Source

```bash
# Clone the repository
git clone https://github.com/akeemphilbert/pericarp.git
cd pericarp

# Build the CLI
make build-cli

# Install locally
make install-cli

# Build for multiple platforms
make build-cli-release
```

### Running Tests

```bash
# Run all tests
make test

# Run CLI-specific tests
go test ./cmd/pericarp/...

# Run integration tests
go test -tags=integration ./cmd/pericarp/...
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run the test suite
6. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](../../LICENSE) file for details.

## Support

- [Documentation](../../docs/)
- [Issues](https://github.com/akeemphilbert/pericarp/issues)
- [Discussions](https://github.com/akeemphilbert/pericarp/discussions)