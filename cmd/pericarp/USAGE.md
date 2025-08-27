# Pericarp CLI Usage Guide

This guide provides comprehensive documentation for all CLI commands, flags, and options.

## Global Options

These flags are available for all commands:

- `--verbose, -v`: Enable verbose output with detailed logging
- `--help, -h`: Show help information for any command

## Commands

### `pericarp new <project-name>`

Creates a new Pericarp project with proper directory structure and boilerplate code.

#### Synopsis

```bash
pericarp new <project-name> [flags]
```

#### Arguments

- `<project-name>`: Name of the project to create (required)
  - Must start with a lowercase letter
  - Can contain lowercase letters, numbers, and hyphens
  - Will be used as the Go module name and directory name

#### Flags

- `--repo, -r <url>`: Git repository URL to clone from
  - Clones the specified repository and adds Pericarp capabilities
  - Preserves existing files in the repository
  - Example: `--repo https://github.com/company/base-service.git`

- `--destination, -d <path>`: Destination directory for the project
  - Defaults to the project name if not specified
  - Creates the directory if it doesn't exist
  - Example: `--destination /workspace/services`

- `--dry-run`: Preview what would be created without actually creating files
  - Shows all files that would be generated
  - Displays file paths and content previews (with `--verbose`)
  - Useful for testing and validation

#### Examples

```bash
# Basic project creation
pericarp new user-service

# Create from existing repository
pericarp new user-service --repo https://github.com/company/base-service.git

# Create in specific directory
pericarp new user-service --destination /workspace/services

# Preview creation with verbose output
pericarp new user-service --dry-run --verbose

# Create with all options
pericarp new user-service \
  --repo https://github.com/company/base.git \
  --destination /workspace/services \
  --verbose
```

#### Generated Structure

```
<project-name>/
├── go.mod                          # Go module with Pericarp dependencies
├── go.sum
├── Makefile                        # Development targets
├── README.md                       # Project documentation
├── config.yaml.example            # Configuration template
├── cmd/<project-name>/main.go      # Application entry point
├── internal/
│   ├── application/                # Application layer
│   ├── domain/                     # Domain layer
│   └── infrastructure/             # Infrastructure layer
└── test/                          # Test files and fixtures
```

### `pericarp generate`

Generates Pericarp code from various input format specifications.

#### Synopsis

```bash
pericarp generate [flags]
```

#### Input Format Flags (exactly one required)

- `--openapi <file>`: OpenAPI 3.0 specification file
  - Supports YAML and JSON formats
  - Extracts schema definitions as domain entities
  - Example: `--openapi user-api.yaml`

- `--proto <file>`: Protocol Buffer definition file
  - Supports .proto files with message definitions
  - Extracts messages as domain entities and services as handlers
  - Example: `--proto user.proto`

#### Output Flags

- `--destination, -d <path>`: Output directory for generated code
  - Defaults to current directory if not specified
  - Creates the directory structure if it doesn't exist
  - Example: `--destination ./internal/generated`

- `--dry-run`: Preview what would be generated without creating files
  - Shows all files that would be generated
  - Displays file paths and content previews (with `--verbose`)
  - Useful for testing and validation

#### Examples

```bash
# Generate from OpenAPI specification
pericarp generate --openapi user-api.yaml

# Generate from Protocol Buffer definition
pericarp generate --proto user.proto

# Generate to specific destination
pericarp generate --openapi api.yaml --destination ./internal/generated

# Preview generation without creating files
pericarp generate --openapi api.yaml --dry-run --verbose

# Generate with verbose output
pericarp generate --proto user.proto --verbose
```

#### Generated Components

For each entity found in the input specification, the following components are generated:

1. **Domain Entity** (`internal/domain/<entity>.go`)
   - Aggregate root with proper DDD patterns
   - Event recording and application methods
   - Version tracking for optimistic concurrency

2. **Domain Events** (`internal/domain/<entity>_events.go`)
   - Event definitions following standard structure
   - Event application logic

3. **Repository Interface** (`internal/domain/<entity>_repository.go`)
   - Repository interface following repository pattern
   - CRUD operations with proper error handling

4. **Repository Implementation** (`internal/infrastructure/<entity>_repository.go`)
   - Concrete repository implementation
   - Database operations with context support

5. **Commands** (`internal/application/<entity>_commands.go`)
   - Command structures with validation
   - CRUD command definitions

6. **Command Handlers** (`internal/application/<entity>_command_handlers.go`)
   - Command handler implementations
   - Business logic and validation

7. **Queries** (`internal/application/<entity>_queries.go`)
   - Query structures for data retrieval
   - Filtering and pagination support

8. **Query Handlers** (`internal/application/<entity>_query_handlers.go`)
   - Query handler implementations
   - Data retrieval logic

9. **Unit Tests** (`internal/domain/<entity>_test.go`, etc.)
   - Comprehensive test coverage
   - Test fixtures and mocks

### `pericarp formats`

Lists all supported input formats for code generation.

#### Synopsis

```bash
pericarp formats
```

#### Examples

```bash
# List all supported formats
pericarp formats
```

#### Output

```
Supported input formats:

  OpenAPI 3.0
    File extensions: .yaml, .yml, .json

  Protocol Buffers
    File extensions: .proto
```

### `pericarp version`

Shows version information including build details.

#### Synopsis

```bash
pericarp version
```

#### Examples

```bash
# Show version information
pericarp version
```

#### Output

```
Pericarp CLI Generator
Version:    v1.0.0
Commit:     abc1234
Built:      2024-01-15T10:30:00Z
Go version: go1.21.5
Built by:   release-script
Platform:   darwin/arm64
```

## Environment Variables

The CLI respects the following environment variables:

- `PERICARP_LOG_LEVEL`: Set log level (debug, info, warn, error)
  - Default: info
  - Example: `export PERICARP_LOG_LEVEL=debug`

- `PERICARP_CONFIG_PATH`: Custom configuration file path
  - Default: looks for config files in standard locations
  - Example: `export PERICARP_CONFIG_PATH=/etc/pericarp/config.yaml`

## Exit Codes

The CLI uses specific exit codes to indicate different types of errors:

- `0`: Success
- `1`: General error
- `2`: Argument error (invalid command line arguments)
- `3`: Validation error (invalid input data)
- `4`: Parse error (failed to parse input files)
- `5`: Generation error (failed to generate code)
- `6`: File system error (file/directory access issues)
- `7`: Network error (repository cloning issues)

## Error Handling

The CLI provides comprehensive error handling with actionable messages:

### Validation Errors

```bash
$ pericarp new ""
Error: validation: project name cannot be empty

$ pericarp new My-Service
Error: validation: project name must start with lowercase letter and contain only lowercase letters, numbers, and hyphens
```

### Parse Errors

```bash
$ pericarp generate --openapi invalid.yaml
Error: parse: failed to parse OpenAPI specification at line 15: invalid YAML syntax
```

### File System Errors

```bash
$ pericarp generate --openapi nonexistent.yaml
Error: filesystem: input file does not exist: nonexistent.yaml

$ pericarp new my-service --destination /readonly
Error: filesystem: destination parent directory is not writable: /readonly
```

### Network Errors

```bash
$ pericarp new my-service --repo https://invalid-repo.git
Error: network: failed to clone repository https://invalid-repo.git (caused by: repository not found)
```

## Configuration

### Project Configuration

Generated projects include a `config.yaml.example` file with common configuration options:

```yaml
# Database configuration
database:
  driver: sqlite
  dsn: "events.db"

# Server configuration
server:
  host: "localhost"
  port: 8080

# Logging configuration
logging:
  level: info
  format: json
```

### CLI Configuration

The CLI can be configured using configuration files in the following locations (in order of precedence):

1. `./pericarp.yaml` (current directory)
2. `$HOME/.pericarp/config.yaml` (user home directory)
3. `/etc/pericarp/config.yaml` (system-wide)

Example CLI configuration:

```yaml
# Default settings for new projects
defaults:
  destination: "/workspace/services"
  verbose: false

# Parser settings
parsers:
  openapi:
    strict_validation: true
  proto:
    include_services: true

# Generation settings
generation:
  include_tests: true
  include_mocks: true
```

## Best Practices

### Project Naming

- Use kebab-case for project names: `user-service`, `order-management`
- Keep names descriptive but concise
- Avoid special characters and spaces
- Consider the domain context: `auth-service`, `payment-gateway`

### Input File Organization

- Keep input files in a dedicated directory: `specs/`, `api/`, `proto/`
- Use descriptive filenames: `user-service.yaml`, `order-api.proto`
- Version your specifications alongside your code
- Validate specifications before generation

### Generated Code Integration

- Review generated code before committing
- Customize generated templates if needed
- Add business logic to generated handlers
- Extend generated tests with specific scenarios

### Development Workflow

```bash
# 1. Create new project
pericarp new my-service

# 2. Navigate to project
cd my-service

# 3. Generate code from specification
pericarp generate --openapi ../specs/my-service.yaml

# 4. Build and test
make deps
make test
make build

# 5. Customize and extend generated code
# Edit internal/application/*_handlers.go
# Add business logic to domain entities
# Extend tests as needed
```

## Troubleshooting

### Common Issues

1. **"command not found: pericarp"**
   - Ensure `$GOPATH/bin` is in your `$PATH`
   - Reinstall with `go install github.com/akeemphilbert/pericarp/cmd/pericarp@latest`

2. **"failed to parse OpenAPI specification"**
   - Validate your OpenAPI file with online validators
   - Check for YAML syntax errors
   - Ensure you're using OpenAPI 3.0 format

3. **"project name validation failed"**
   - Use lowercase letters, numbers, and hyphens only
   - Start with a lowercase letter
   - Avoid Go reserved keywords

4. **"destination directory not writable"**
   - Check directory permissions
   - Ensure parent directory exists
   - Use absolute paths when in doubt

### Debug Mode

Enable verbose output for detailed debugging information:

```bash
pericarp new my-service --verbose --dry-run
```

This will show:
- Detailed parsing information
- File generation steps
- Template processing details
- Validation checks performed

### Getting Help

- Use `pericarp --help` for general help
- Use `pericarp <command> --help` for command-specific help
- Check the [documentation](../../docs/) for detailed guides
- Report issues on [GitHub](https://github.com/akeemphilbert/pericarp/issues)