# Requirements Document

## Introduction

This feature introduces a CLI tool for Pericarp that enables developers to easily scaffold new projects with automated code generation capabilities. The tool will be installable via `go install` and will support multiple input formats (starting with ERD) to generate domain entities, repositories, command handlers, query handlers, and other boilerplate code. The CLI will also generate project structure with proper Makefiles and development tooling setup.

## Requirements

### Requirement 1

**User Story:** As a developer, I want to install the Pericarp CLI tool using `go install`, so that I can easily access project scaffolding capabilities from anywhere on my system.

#### Acceptance Criteria

1. WHEN a developer runs `go install github.com/akeemphilbert/pericarp/cmd/pericarp` THEN the system SHALL install the CLI tool globally
2. WHEN the installation is complete THEN the developer SHALL be able to run `pericarp --help` from any directory
3. WHEN the CLI is executed without arguments THEN the system SHALL display usage information and available commands

### Requirement 2

**User Story:** As a developer, I want to create a new Pericarp project with a specified name, so that I can quickly bootstrap a new application with proper structure.

#### Acceptance Criteria

1. WHEN a developer runs `pericarp new <project-name>` THEN the system SHALL create a new directory with the project name
2. WHEN creating a new project THEN the system SHALL generate a proper Go module structure with go.mod file
3. WHEN creating a new project THEN the system SHALL include all necessary Pericarp dependencies in go.mod
4. WHEN the project is created THEN the system SHALL generate a basic directory structure following Pericarp conventions

### Requirement 3

**User Story:** As a developer, I want to generate code from multiple input formats (ERD, OpenAPI, Proto), so that I can automatically create domain entities, repositories, and handlers based on different types of specifications.

#### Acceptance Criteria

1. WHEN a developer runs `pericarp generate --erd <erd-file>` THEN the system SHALL parse the ERD file
2. WHEN a developer runs `pericarp generate --openapi <openapi-file>` THEN the system SHALL parse the OpenAPI specification
3. WHEN a developer runs `pericarp generate --proto <proto-file>` THEN the system SHALL parse the Protocol Buffer definition
4. WHEN parsing any format THEN the system SHALL use a parser that implements a common domain parser interface
5. WHEN generating from any format THEN the system SHALL extract domain entity information into a standardized internal model
6. WHEN generating from any format THEN the system SHALL create domain entities with proper struct definitions
7. WHEN generating from any format THEN the system SHALL create repository interfaces and implementations
8. WHEN generating from any format THEN the system SHALL create command handlers for CRUD operations
9. WHEN generating from any format THEN the system SHALL create query handlers for data retrieval
10. WHEN generating from any format THEN the system SHALL create appropriate domain events for entity changes

### Requirement 4

**User Story:** As a developer, I want to clone an existing repository and generate Pericarp code within it, so that I can add Pericarp capabilities to existing projects.

#### Acceptance Criteria

1. WHEN a developer runs `pericarp new <project-name> --repo <git-url>` THEN the system SHALL clone the specified repository
2. WHEN cloning a repository THEN the system SHALL create the project directory and clone into it
3. WHEN the repository is cloned THEN the system SHALL generate Pericarp code structure within the existing codebase
4. WHEN adding to existing repo THEN the system SHALL preserve existing files and add Pericarp components
5. IF the repository clone fails THEN the system SHALL display an error message and exit gracefully

### Requirement 5

**User Story:** As a developer, I want the generated project to include a comprehensive Makefile, so that I can easily manage dependencies, run tests, and perform security checks.

#### Acceptance Criteria

1. WHEN a project is generated THEN the system SHALL create a Makefile with standard targets
2. WHEN the Makefile is created THEN it SHALL include a `deps` target for installing dependencies
3. WHEN the Makefile is created THEN it SHALL include a `test` target for running all tests
4. WHEN the Makefile is created THEN it SHALL include a `gosec` target for security scanning
5. WHEN the Makefile is created THEN it SHALL include a `build` target for compiling the application
6. WHEN the Makefile is created THEN it SHALL include a `clean` target for cleanup operations
7. WHEN the Makefile is created THEN it SHALL include a `lint` target for code quality checks

### Requirement 6

**User Story:** As a developer, I want the CLI to use a factory pattern for code generation, so that domain entity information can be consistently transformed into Pericarp components regardless of input format.

#### Acceptance Criteria

1. WHEN the system processes domain entities THEN it SHALL use a factory to generate command handlers
2. WHEN the system processes domain entities THEN it SHALL use a factory to generate query handlers
3. WHEN the system processes domain entities THEN it SHALL use a factory to generate repositories
4. WHEN the system processes domain entities THEN it SHALL use a factory to generate CRUD commands
5. WHEN the factory generates components THEN it SHALL ensure consistency across all input formats
6. WHEN new parsers are added THEN they SHALL output to the same standardized domain entity model

### Requirement 7

**User Story:** As a developer, I want the CLI to support extensible generation options, so that I can use different input formats and generators in the future.

#### Acceptance Criteria

1. WHEN the CLI is designed THEN it SHALL use a plugin-like architecture for parsers
2. WHEN adding new parsers THEN the system SHALL support registration of new input formats
3. WHEN running generation THEN the system SHALL allow specifying the input format type via flags
4. WHEN no format is specified THEN the system SHALL attempt to detect the format automatically
5. WHEN listing available formats THEN the system SHALL provide a `pericarp formats` command

### Requirement 8

**User Story:** As a developer, I want the generated code to follow Pericarp best practices and conventions, so that the scaffolded project is production-ready and maintainable.

#### Acceptance Criteria

1. WHEN code is generated THEN it SHALL follow the established Pericarp project structure
2. WHEN entities are generated THEN they SHALL implement proper aggregate root patterns
3. WHEN repositories are generated THEN they SHALL include both interface and implementation
4. WHEN handlers are generated THEN they SHALL include proper error handling and validation
5. WHEN events are generated THEN they SHALL follow the standard event structure
6. WHEN tests are generated THEN they SHALL include unit tests for all generated components

### Requirement 9

**User Story:** As a developer, I want to control the CLI behavior with flags like dry-run, verbose output, and custom destination, so that I can preview changes and customize the generation process.

#### Acceptance Criteria

1. WHEN a developer runs `pericarp new --dry-run <project-name>` THEN the system SHALL show what would be generated without creating files
2. WHEN a developer uses `--verbose` flag THEN the system SHALL set log level to debug and output to stdout
3. WHEN a developer uses `--destination <path>` flag THEN the system SHALL create the project in the specified directory
4. WHEN destination directory doesn't exist THEN the system SHALL create it automatically
5. WHEN no destination is specified THEN the system SHALL default to using the project name as the directory
6. WHEN dry-run is enabled THEN the system SHALL display file paths and content previews without writing files

### Requirement 10

**User Story:** As a developer, I want clear error messages and validation, so that I can quickly identify and fix issues with my input or configuration.

#### Acceptance Criteria

1. WHEN invalid arguments are provided THEN the system SHALL display helpful error messages
2. WHEN required files are missing THEN the system SHALL indicate which files are needed
3. WHEN ERD parsing fails THEN the system SHALL show specific parsing errors with line numbers
4. WHEN generation fails THEN the system SHALL provide actionable error messages
5. WHEN the CLI encounters errors THEN it SHALL exit with appropriate error codes