package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectCreator handles creating new Pericarp projects
type ProjectCreator struct {
	logger    CliLogger
	validator Validator
	executor  Executor
	factory   ComponentFactory
	cloner    *RepositoryCloner
}

// NewProjectCreator creates a new project creator instance
func NewProjectCreator(logger CliLogger) *ProjectCreator {
	factory, err := NewPericarpComponentFactory(logger)
	if err != nil {
		logger.Fatalf("Failed to create component factory: %v", err)
	}

	return &ProjectCreator{
		logger:    logger,
		validator: NewValidator(),
		executor:  NewExecutor(logger),
		factory:   factory,
		cloner:    NewRepositoryCloner(logger),
	}
}

// CreateProject creates a new Pericarp project with the specified configuration
func (p *ProjectCreator) CreateProject(projectName, repoURL, destination string, dryRun bool) error {
	// Enhanced logging for project creation process (Requirement 9.2, 9.6)
	if verboseLogger, ok := p.logger.(*VerboseLogger); ok {
		verboseLogger.LogSection("PROJECT CREATION")
		verboseLogger.LogValidationDetails("Project name", projectName, true)
		if repoURL != "" {
			verboseLogger.Debug("Repository cloning requested", "url", repoURL)
		}
		if destination != "" {
			verboseLogger.Debug("Custom destination specified", "path", destination)
		}
		if dryRun {
			verboseLogger.Debug("Dry-run mode enabled - no files will be created")
		}
	}

	p.logger.Infof("Creating new Pericarp project: %s", projectName)

	// Validate project name (Requirement 10.1)
	if verboseLogger, ok := p.logger.(*VerboseLogger); ok {
		verboseLogger.LogSubSection("Validation Phase")
	}

	if err := p.validator.ValidateProjectName(projectName); err != nil {
		if verboseLogger, ok := p.logger.(*VerboseLogger); ok {
			verboseLogger.LogValidationDetails("Project name", projectName, false)
		}
		return err
	}

	if verboseLogger, ok := p.logger.(*VerboseLogger); ok {
		verboseLogger.LogValidationDetails("Project name", projectName, true)
	}

	// Determine the actual destination directory
	actualDestination := destination
	if actualDestination == "" {
		actualDestination = projectName
	}

	// Validate destination if provided (Requirement 10.2)
	if destination != "" {
		if err := p.validator.ValidateDestination(destination); err != nil {
			if verboseLogger, ok := p.logger.(*VerboseLogger); ok {
				verboseLogger.LogValidationDetails("Destination", destination, false)
			}
			return err
		}
		if verboseLogger, ok := p.logger.(*VerboseLogger); ok {
			verboseLogger.LogValidationDetails("Destination", destination, true)
		}
	}

	// Check if repository cloning is requested (Requirement 4.3)
	var isExistingRepo bool
	if repoURL != "" {
		if verboseLogger, ok := p.logger.(*VerboseLogger); ok {
			verboseLogger.LogSubSection("Repository Cloning Phase")
		}
		if err := p.handleRepositoryCloning(repoURL, actualDestination, dryRun); err != nil {
			return err
		}
		isExistingRepo = true
	}

	// Create the project structure and files with existing file preservation (Requirement 4.4)
	if verboseLogger, ok := p.logger.(*VerboseLogger); ok {
		verboseLogger.LogSubSection("Project Structure Generation Phase")
	}
	if err := p.generateProjectStructure(projectName, actualDestination, dryRun, isExistingRepo); err != nil {
		return err
	}

	if !dryRun {
		p.logger.Infof("Successfully created project '%s' in '%s'", projectName, actualDestination)
		if isExistingRepo {
			p.logger.Info("Pericarp capabilities added to existing repository")
		}
		p.logger.Info("Next steps:")
		p.logger.Info("  1. cd " + actualDestination)
		p.logger.Info("  2. make deps")
		p.logger.Info("  3. make test")
	} else {
		p.logger.Info("Dry run completed - no files were created")
	}

	return nil
}

// handleRepositoryCloning handles the complete repository cloning workflow (Requirement 4.3)
func (p *ProjectCreator) handleRepositoryCloning(repoURL, destination string, dryRun bool) error {
	if dryRun {
		p.logger.Info("DRY RUN: Would clone repository to " + destination)
		return nil
	}

	// Check Git availability before attempting to clone (Requirement 4.1)
	if err := p.cloner.CheckGitAvailability(); err != nil {
		return err
	}

	// Clone the repository (Requirement 4.1, 4.2)
	if err := p.cloner.CloneRepository(repoURL, destination); err != nil {
		return err
	}

	// Validate the cloned repository (Requirement 4.2)
	if err := p.cloner.ValidateRepository(destination); err != nil {
		// Clean up on validation failure
		if removeErr := os.RemoveAll(destination); removeErr != nil {
			p.logger.Warnf("Failed to clean up invalid repository: %v", removeErr)
		}
		return err
	}

	p.logger.Info("Repository cloning completed successfully")
	return nil
}

// generateProjectStructure creates the project directory structure and files
func (p *ProjectCreator) generateProjectStructure(projectName, destination string, dryRun bool, isExistingRepo bool) error {
	p.logger.Debug("Generating project structure", "project", projectName, "destination", destination, "existing_repo", isExistingRepo)

	// Create a basic domain model for project scaffolding
	domainModel := &DomainModel{
		ProjectName: projectName,
		Entities:    []Entity{}, // Empty for basic project structure
		Relations:   []Relation{},
		Metadata: map[string]interface{}{
			"generated_by": "pericarp-cli",
			"version":      version,
		},
	}

	if dryRun {
		p.logger.Info("DRY RUN: Would generate project structure")
		return p.previewProjectStructure(domainModel, destination, isExistingRepo)
	}

	// For existing repositories, we need to preserve existing files (Requirement 4.4)
	if isExistingRepo {
		return p.generateWithFilePreservation(domainModel, destination)
	}

	// For new projects, generate normally
	return p.factory.GenerateProjectStructure(domainModel, destination)
}

// previewProjectStructure shows what would be created in dry-run mode
func (p *ProjectCreator) previewProjectStructure(model *DomainModel, destination string, isExistingRepo bool) error {
	if isExistingRepo {
		p.logger.Info("Pericarp structure that would be added to existing repository:")
	} else {
		p.logger.Info("Project structure that would be created:")
	}

	// List the directories and files that would be created
	structure := []string{
		"go.mod",
		"go.sum",
		"Makefile",
		"README.md",
		"config.yaml.example",
		"cmd/",
		"cmd/" + model.ProjectName + "/",
		"cmd/" + model.ProjectName + "/main.go",
		"internal/",
		"internal/application/",
		"internal/domain/",
		"internal/infrastructure/",
		"pkg/",
		"test/",
		"test/fixtures/",
		"test/integration/",
		"test/mocks/",
	}

	for _, item := range structure {
		fullPath := filepath.Join(destination, item)
		if strings.HasSuffix(item, "/") {
			p.logger.Infof("  [DIR]  %s", fullPath)
		} else {
			// Check if file exists in existing repo
			if isExistingRepo {
				if _, err := os.Stat(fullPath); err == nil {
					p.logger.Infof("  [SKIP] %s (already exists)", fullPath)
				} else {
					p.logger.Infof("  [FILE] %s", fullPath)
				}
			} else {
				p.logger.Infof("  [FILE] %s", fullPath)
			}
		}
	}

	return nil
}

// generateWithFilePreservation generates project structure while preserving existing files (Requirement 4.4, 4.5)
func (p *ProjectCreator) generateWithFilePreservation(model *DomainModel, destination string) error {
	p.logger.Info("Generating Pericarp structure with existing file preservation")

	// First, generate all files to a temporary structure to get the list
	tempFiles, err := p.factory.GenerateProjectFiles(model)
	if err != nil {
		return NewCliError(GenerationError,
			"failed to generate project files",
			err)
	}

	// Use the cloner to preserve existing files (Requirement 4.4)
	safeFiles, err := p.cloner.PreserveExistingFiles(destination, tempFiles)
	if err != nil {
		return NewCliError(FileSystemError,
			"failed to preserve existing files",
			err)
	}

	// Write only the safe files that don't conflict with existing ones
	ctx := context.Background()
	if err := p.executor.Execute(ctx, safeFiles, destination, false); err != nil {
		return NewCliError(GenerationError,
			"failed to write project files",
			err)
	}

	preservedCount := len(tempFiles) - len(safeFiles)
	if preservedCount > 0 {
		p.logger.Infof("Generated %d new files, preserved %d existing files", len(safeFiles), preservedCount)
	} else {
		p.logger.Infof("Generated %d files", len(safeFiles))
	}

	return nil
}

// CodeGenerator handles code generation from various input formats
type CodeGenerator struct {
	logger   CliLogger
	registry ParserRegistry
	factory  ComponentFactory
	executor Executor
}

// NewCodeGenerator creates a new code generator instance
func NewCodeGenerator(logger CliLogger) *CodeGenerator {
	registry := NewParserRegistry()

	// Register all available parsers (Requirement 7.1, 7.2)
	// TODO: Implement ERD parser
	// registry.RegisterParser(NewERDParser())
	registry.RegisterParser(NewOpenAPIParser())
	registry.RegisterParser(NewProtoParser())

	factory, err := NewPericarpComponentFactory(logger)
	if err != nil {
		logger.Fatalf("Failed to create component factory: %v", err)
	}

	return &CodeGenerator{
		logger:   logger,
		registry: registry,
		factory:  factory,
		executor: NewExecutor(logger),
	}
}

// Generate generates code from the specified input file and format
func (g *CodeGenerator) Generate(inputFile, inputType, destination string, dryRun bool) error {
	// Enhanced logging for code generation process (Requirement 9.2, 9.6)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogSection("CODE GENERATION")
		verboseLogger.Debug("Input parameters", "file", inputFile, "type", inputType, "destination", destination, "dry_run", dryRun)
	}

	g.logger.Infof("Generating code from %s file: %s", inputType, inputFile)

	// Get the appropriate parser (Requirement 7.3)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogSubSection("Parser Selection Phase")
	}

	parser, err := g.registry.GetParser(inputFile)
	if err != nil {
		if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
			verboseLogger.Debug("Parser selection failed", "file", inputFile, "error", err.Error())
		}
		return NewCliError(ParseError,
			fmt.Sprintf("no parser available for file: %s", inputFile),
			err)
	}

	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.Debug("Parser selected", "format", parser.FormatName(), "extensions", parser.SupportedExtensions())
	}

	// Validate the input file
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogSubSection("Input Validation Phase")
	}

	if err := parser.Validate(inputFile); err != nil {
		if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
			verboseLogger.LogValidationDetails("Input file", inputFile, false)
		}
		return NewCliError(ValidationError,
			fmt.Sprintf("input file validation failed: %s", inputFile),
			err)
	}

	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogValidationDetails("Input file", inputFile, true)
	}

	// Parse the input file (Requirement 3.5)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogSubSection("Parsing Phase")
	}

	domainModel, err := parser.Parse(inputFile)
	if err != nil {
		if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
			verboseLogger.Debug("Parsing failed", "file", inputFile, "error", err.Error())
		}
		return NewCliError(ParseError,
			fmt.Sprintf("failed to parse %s file: %s", inputType, inputFile),
			err)
	}

	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogParsingDetails(parser.FormatName(), inputFile, len(domainModel.Entities))
	}

	g.logger.Infof("Parsed %d entities from input file", len(domainModel.Entities))

	// Generate code for each entity
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogSubSection("Code Generation Phase")
	}

	var allFiles []*GeneratedFile
	for i, entity := range domainModel.Entities {
		if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
			verboseLogger.LogStep(fmt.Sprintf("Generating entity: %s", entity.Name), i+1, len(domainModel.Entities))
			verboseLogger.LogGenerationDetails("All components", entity)
		}

		g.logger.Debugf("Generating code for entity: %s", entity.Name)

		// Generate all components for this entity (Requirement 6)
		entityFiles, err := g.generateEntityComponents(entity)
		if err != nil {
			return NewCliError(GenerationError,
				fmt.Sprintf("failed to generate components for entity %s", entity.Name),
				err)
		}

		allFiles = append(allFiles, entityFiles...)

		if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
			verboseLogger.LogProgress("Entity generation", i+1, len(domainModel.Entities))
		}
	}

	// Determine destination directory
	actualDestination := destination
	if actualDestination == "" {
		actualDestination = "."
	}

	// Execute file generation (Requirement 9.1, 9.6)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogSubSection("File Generation Phase")
		verboseLogger.Debug("File generation summary", "total_files", len(allFiles), "destination", actualDestination)
	}

	ctx := context.Background()
	if err := g.executor.Execute(ctx, allFiles, actualDestination, dryRun); err != nil {
		return NewCliError(GenerationError,
			"failed to generate files",
			err)
	}

	if !dryRun {
		g.logger.Infof("Successfully generated %d files", len(allFiles))
	} else {
		g.logger.Info("Dry run completed - no files were created")
	}

	return nil
}

// generateEntityComponents generates all components for a single entity
func (g *CodeGenerator) generateEntityComponents(entity Entity) ([]*GeneratedFile, error) {
	var files []*GeneratedFile

	// Generate entity (Requirement 6.2)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogGenerationDetails("Entity", entity)
	}

	entityFile, err := g.factory.GenerateEntity(entity)
	if err != nil {
		return nil, fmt.Errorf("failed to generate entity: %w", err)
	}
	files = append(files, entityFile)

	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogFileOperation("Generated", entityFile.Path, len(entityFile.Content))
	}

	// Generate repository (Requirement 6.3)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogGenerationDetails("Repository", entity)
	}

	repoFiles, err := g.factory.GenerateRepository(entity)
	if err != nil {
		return nil, fmt.Errorf("failed to generate repository: %w", err)
	}
	files = append(files, repoFiles...)

	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		for _, file := range repoFiles {
			verboseLogger.LogFileOperation("Generated", file.Path, len(file.Content))
		}
	}

	// Generate commands (Requirement 6.4)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogGenerationDetails("Commands", entity)
	}
	commandFiles, err := g.factory.GenerateCommands(entity)
	if err != nil {
		return nil, fmt.Errorf("failed to generate commands: %w", err)
	}
	files = append(files, commandFiles...)

	// Generate queries (Requirement 6.5)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogGenerationDetails("Queries", entity)
	}
	queryFiles, err := g.factory.GenerateQueries(entity)
	if err != nil {
		return nil, fmt.Errorf("failed to generate queries: %w", err)
	}
	files = append(files, queryFiles...)

	// Generate events (Requirement 6.5)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogGenerationDetails("Events", entity)
	}
	eventFiles, err := g.factory.GenerateEvents(entity)
	if err != nil {
		return nil, fmt.Errorf("failed to generate events: %w", err)
	}
	files = append(files, eventFiles...)

	// Generate handlers (Requirement 6.1)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogGenerationDetails("Handlers", entity)
	}
	handlerFiles, err := g.factory.GenerateHandlers(entity)
	if err != nil {
		return nil, fmt.Errorf("failed to generate handlers: %w", err)
	}
	files = append(files, handlerFiles...)

	// Generate tests (Requirement 8.6)
	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.LogGenerationDetails("Tests", entity)
	}
	testFiles, err := g.factory.GenerateTests(entity)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tests: %w", err)
	}
	files = append(files, testFiles...)

	if verboseLogger, ok := g.logger.(*VerboseLogger); ok {
		verboseLogger.Debug("Entity component generation completed", "entity", entity.Name, "total_files", len(files))
	}

	return files, nil
}
