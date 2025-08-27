package main

import (
	"context"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// DomainParser defines the interface for parsing different input formats
// This enables pluggable parsers for ERD, OpenAPI, Proto, etc. (Requirement 7.1, 7.2)
type DomainParser interface {
	// Parse processes the input file and returns domain entities
	Parse(filePath string) (*DomainModel, error)

	// SupportedExtensions returns file extensions this parser handles
	SupportedExtensions() []string

	// FormatName returns the human-readable name of this format
	FormatName() string

	// Validate checks if the input file is valid for this parser
	Validate(filePath string) error
}

// ComponentFactory generates Pericarp components from domain model
// This ensures consistent code generation across all input formats (Requirement 6)
type ComponentFactory interface {
	// GenerateEntity creates domain entity code following aggregate root patterns (Requirement 8.2)
	GenerateEntity(entity Entity) (*GeneratedFile, error)

	// GenerateRepository creates repository interface and implementation (Requirement 6.3, 8.3)
	GenerateRepository(entity Entity) ([]*GeneratedFile, error)

	// GenerateCommands creates CRUD command structures (Requirement 6.4)
	GenerateCommands(entity Entity) ([]*GeneratedFile, error)

	// GenerateQueries creates query structures and handlers (Requirement 6.5)
	GenerateQueries(entity Entity) ([]*GeneratedFile, error)

	// GenerateEvents creates domain events following standard structure (Requirement 6.5, 8.5)
	GenerateEvents(entity Entity) ([]*GeneratedFile, error)

	// GenerateHandlers creates command and query handlers with error handling (Requirement 6.1, 8.4)
	GenerateHandlers(entity Entity) ([]*GeneratedFile, error)

	// GenerateServices creates service layer with CRUD operations (Requirement 6.6)
	GenerateServices(entity Entity) ([]*GeneratedFile, error)

	// GenerateTests creates unit tests for all components (Requirement 8.6)
	GenerateTests(entity Entity) ([]*GeneratedFile, error)

	// GenerateProjectStructure creates the complete project scaffold (Requirement 2.4)
	GenerateProjectStructure(model *DomainModel, destination string) error

	// GenerateProjectFiles generates all project files without writing them (for file preservation)
	GenerateProjectFiles(model *DomainModel) ([]*GeneratedFile, error)

	// GenerateMakefile creates comprehensive Makefile (Requirement 5)
	GenerateMakefile(projectName string) (*GeneratedFile, error)
}

// ParserRegistry manages available parsers for extensibility (Requirement 7.1, 7.2)
type ParserRegistry interface {
	// RegisterParser adds a new parser to the registry
	RegisterParser(parser DomainParser) error

	// GetParser returns the appropriate parser for a file extension
	GetParser(filePath string) (DomainParser, error)

	// ListFormats returns all supported formats (Requirement 7.5)
	ListFormats() []string

	// DetectFormat attempts to automatically detect the input format
	DetectFormat(filePath string) (string, error)
}

// CliLogger extends the domain Logger with CLI-specific functionality
type CliLogger interface {
	domain.Logger
	SetVerbose(enabled bool)
	IsVerbose() bool
}

// Validator provides comprehensive input validation (Requirement 10)
type Validator interface {
	// ValidateProjectName ensures project names follow Go module conventions
	ValidateProjectName(name string) error

	// ValidateInputFile checks if input files exist and are readable
	ValidateInputFile(filePath string) error

	// ValidateDestination ensures destination directory is writable
	ValidateDestination(path string) error
}

// Executor handles file generation with dry-run support (Requirement 9.1, 9.6)
type Executor interface {
	// Execute creates files or shows preview in dry-run mode
	Execute(ctx context.Context, files []*GeneratedFile, destination string, dryRun bool) error
}
