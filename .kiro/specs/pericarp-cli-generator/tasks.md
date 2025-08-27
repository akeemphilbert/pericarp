# Implementation Plan

- [x] 1. Set up CLI project structure and core interfaces
  - Create `cmd/pericarp/main.go` with basic Cobra setup
  - Define core interfaces: `DomainParser`, `ComponentFactory`, `ParserRegistry`
  - Set up Go module with necessary dependencies (cobra, templates, git)
  - _Requirements: 1.1, 1.2, 1.3_

- [x] 2. Implement error handling and validation system
  - [x] 2.1 Create comprehensive error types and CLI error handling
    - Implement `CliError` struct with error types and exit codes
    - Create `NewCliError` constructor with proper error categorization
    - Write unit tests for error handling and exit code mapping
    - _Requirements: 10.1, 10.4, 10.5_

  - [x] 2.2 Implement input validation system
    - Create `Validator` struct with project name, file, and destination validation
    - Implement `ValidateProjectName`, `ValidateInputFile`, `ValidateDestination` methods
    - Write comprehensive unit tests for all validation scenarios
    - _Requirements: 10.1, 10.2, 10.3_

- [x] 3. Create domain model and parser system
  - [x] 3.1 Implement core domain model structures
    - Create `DomainModel`, `Entity`, `Property`, `Relation` structs
    - Define `RelationType` constants and relationship handling
    - Write unit tests for domain model creation and manipulation
    - _Requirements: 3.5, 6.6_

  - [x] 3.2 Implement parser registry system
    - Create `ParserRegistry` with parser registration and lookup
    - Implement `RegisterParser`, `GetParser`, `ListFormats` methods
    - Write unit tests for parser registration and format detection
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5_

- [x] 4. Implement OpenAPI parser
  - [x] 4.1 Create OpenAPI parser implementation
    - Implement `OpenAPIParser` struct that implements `DomainParser` interface
    - Parse OpenAPI schema definitions into domain entities
    - Handle OpenAPI property types, validation rules, and relationships
    - _Requirements: 3.2, 3.5_

  - [x] 4.2 Add OpenAPI parser tests and validation
    - Write unit tests with valid and invalid OpenAPI specifications
    - Test entity extraction from complex OpenAPI schemas
    - Implement error handling for malformed OpenAPI files
    - _Requirements: 3.2, 10.3_

- [x] 5. Implement Protocol Buffer parser
  - [x] 5.1 Create Proto parser implementation
    - Implement `ProtoParser` struct that implements `DomainParser` interface
    - Parse Protocol Buffer message definitions into domain entities
    - Handle proto field types, options, and service definitions
    - _Requirements: 3.3, 3.5_

  - [x] 5.2 Add Proto parser tests and validation
    - Write unit tests with valid and invalid proto files
    - Test entity extraction from proto messages and services
    - Implement error handling for malformed proto definitions
    - _Requirements: 3.3, 10.3_

- [x] 6. Create template engine and code generation system
  - [x] 6.1 Implement template engine infrastructure
    - Create `TemplateEngine` struct with Go template loading and execution
    - Implement template helper functions for code generation
    - Create template directory structure and base templates
    - _Requirements: 8.1, 8.2, 8.3_

  - [x] 6.2 Create entity generation templates
    - Implement entity template with aggregate root patterns
    - Generate proper struct definitions with validation tags
    - Include event recording and application methods
    - Write tests for entity template generation
    - _Requirements: 8.2, 8.5, 3.6_

  - [x] 6.3 Create repository generation templates
    - Implement repository interface and implementation templates
    - Generate CRUD operations and query methods
    - Include proper error handling and context usage
    - Write tests for repository template generation
    - _Requirements: 8.3, 3.7_

- [x] 7. Implement component factory system
  - [x] 7.1 Create Pericarp component factory
    - Implement `PericarpComponentFactory` struct with all generation methods
    - Create `GenerateEntity`, `GenerateRepository`, `GenerateCommands` methods
    - Implement `GenerateQueries`, `GenerateEvents`, `GenerateHandlers` methods
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_

  - [x] 7.2 Add project structure and Makefile generation
    - Implement `GenerateProjectStructure` method for complete project scaffold
    - Create `GenerateMakefile` method with all required targets
    - Generate proper directory structure following Pericarp conventions
    - Write tests for project structure generation
    - _Requirements: 2.4, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7_

  - [x] 7.3 Implement test generation capabilities
    - Create `GenerateTests` method for unit test generation
    - Generate test files for entities, repositories, and handlers
    - Include proper test fixtures and mock usage
    - Write tests for test generation functionality
    - _Requirements: 8.6_

- [-] 8. Create CLI command implementations
  - [x] 8.1 Implement root command and help system
    - Create root Cobra command with proper help display
    - Implement version command and usage information
    - Add global flag handling and configuration
    - Write tests for command parsing and help display
    - _Requirements: 1.2, 1.3_

  - [x] 8.2 Implement new project command
    - Create `NewCmd` with project name validation and flag handling
    - Implement project creation logic with destination handling
    - Add dry-run and verbose output support
    - Write integration tests for project creation workflow
    - _Requirements: 2.1, 2.2, 2.3, 9.3, 9.4, 9.5, 9.1, 9.2_

  - [x] 8.3 Implement generate command
    - Create `GenerateCmd` with input format flag validation
    - Implement code generation workflow with parser selection
    - Add destination, dry-run, and verbose flag support
    - Write integration tests for code generation from all formats
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 9.1, 9.2, 9.3_

  - [x] 8.4 Implement formats command
    - Create `FormatsCmd` to list all supported input formats
    - Display format information from parser registry
    - Write tests for format listing functionality
    - _Requirements: 7.5_

- [x] 9. Add repository integration capabilities
  - [x] 9.1 Implement Git repository cloning
    - Create `RepositoryCloner` struct with Git operations
    - Implement `CloneRepository` method with error handling
    - Add repository validation and existing file preservation
    - _Requirements: 4.1, 4.2, 4.4, 4.5_

  - [x] 9.2 Add repository integration to new command
    - Integrate repository cloning into project creation workflow
    - Implement existing file preservation when adding to existing repos
    - Add proper error handling for network and Git operations
    - Write integration tests for repository cloning scenarios
    - _Requirements: 4.3, 4.4_

- [x] 10. Implement dry-run and verbose output system
  - [x] 10.1 Create dry-run execution system
    - Implement `DryRunExecutor` for preview mode without file creation
    - Add file content preview and destination path display
    - Integrate dry-run mode into all generation workflows
    - _Requirements: 9.1, 9.6_

  - [x] 10.2 Implement verbose logging system
    - Create `VerboseLogger` with debug output to stdout
    - Add detailed logging throughout generation process
    - Implement log level control and output formatting
    - Write tests for logging functionality
    - _Requirements: 9.2, 9.6_

- [x] 11. Create comprehensive test suite
  - [x] 11.1 Write unit tests for all core components
    - Test all parser implementations with valid and invalid inputs
    - Test component factory with various entity configurations
    - Test template engine with different data scenarios
    - _Requirements: All requirements validation_

  - [x] 11.2 Create integration tests for end-to-end workflows
    - Test complete project generation from sample inputs
    - Verify generated code compiles and passes tests
    - Test all CLI commands with various flag combinations
    - _Requirements: All requirements validation_

  - [x] 11.3 Add generated code validation tests
    - Ensure generated projects compile successfully
    - Run tests on generated code to verify functionality
    - Validate generated Makefiles work correctly
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

- [x] 12. Final integration and CLI packaging
  - [x] 12.1 Complete CLI binary setup for installation
    - Ensure proper module path for `go install` compatibility
    - Add version information and build metadata
    - Test installation process from repository
    - _Requirements: 1.1_

  - [x] 12.2 Create comprehensive documentation and examples
    - Add usage examples for all supported input formats
    - Create sample ERD, OpenAPI, and Proto files for testing
    - Document all CLI flags and command options
    - _Requirements: 1.3, 7.5_

  - [x] 12.3 Perform end-to-end validation
    - Test complete workflow from installation to code generation
    - Verify all error scenarios produce appropriate messages
    - Validate generated projects follow Pericarp best practices
    - _Requirements: All requirements final validation_