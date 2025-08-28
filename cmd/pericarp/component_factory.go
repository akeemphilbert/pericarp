package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PericarpComponentFactory implements ComponentFactory with Pericarp best practices
// Generates all Pericarp components from domain model (Requirements 6.1, 6.2, 6.3, 6.4, 6.5)
type PericarpComponentFactory struct {
	templateEngine *TemplateEngine
	logger         CliLogger
}

// NewPericarpComponentFactory creates a factory with proper template loading
func NewPericarpComponentFactory(logger CliLogger) (*PericarpComponentFactory, error) {
	engine, err := NewTemplateEngine(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize template engine: %w", err)
	}

	return &PericarpComponentFactory{
		templateEngine: engine,
		logger:         logger,
	}, nil
}

// GenerateEntity creates domain entity code following aggregate root patterns (Requirement 6.2, 8.2)
func (f *PericarpComponentFactory) GenerateEntity(entity Entity) (*GeneratedFile, error) {
	f.logger.Debug(fmt.Sprintf("Generating entity: %s", entity.Name))

	// Ensure entity has an ID field if not present
	entity = f.ensureIDField(entity)

	content, err := f.templateEngine.Execute("entity.go", entity)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate entity %s", entity.Name), err)
	}

	return &GeneratedFile{
		Path:    fmt.Sprintf("internal/domain/%s.go", strings.ToLower(entity.Name)),
		Content: content,
		Metadata: map[string]interface{}{
			"type":   "entity",
			"entity": entity.Name,
		},
	}, nil
}

// GenerateRepository creates repository interface and implementation (Requirement 6.3, 8.3)
func (f *PericarpComponentFactory) GenerateRepository(entity Entity) ([]*GeneratedFile, error) {
	f.logger.Debug(fmt.Sprintf("Generating repository for entity: %s", entity.Name))

	var files []*GeneratedFile

	// Generate repository interface
	interfaceContent, err := f.templateEngine.Execute("repository_interface.go", entity)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate repository interface for %s", entity.Name), err)
	}

	files = append(files, &GeneratedFile{
		Path:    fmt.Sprintf("internal/domain/%s_repository.go", strings.ToLower(entity.Name)),
		Content: interfaceContent,
		Metadata: map[string]interface{}{
			"type":   "repository_interface",
			"entity": entity.Name,
		},
	})

	// Generate repository implementation
	implData := map[string]interface{}{
		"Entity":      entity,
		"Name":        entity.Name,
		"Properties":  entity.Properties,
		"ProjectName": "example.com/project", // Default project name
	}
	implContent, err := f.templateEngine.Execute("repository_implementation.go", implData)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate repository implementation for %s", entity.Name), err)
	}

	files = append(files, &GeneratedFile{
		Path:    fmt.Sprintf("internal/infrastructure/%s_repository.go", strings.ToLower(entity.Name)),
		Content: implContent,
		Metadata: map[string]interface{}{
			"type":   "repository_implementation",
			"entity": entity.Name,
		},
	})

	return files, nil
}

// GenerateCommands creates CRUD command structures (Requirement 6.4)
func (f *PericarpComponentFactory) GenerateCommands(entity Entity) ([]*GeneratedFile, error) {
	f.logger.Debug(fmt.Sprintf("Generating commands for entity: %s", entity.Name))

	// Create command data structure
	commandData := f.createCommandData(entity)

	content, err := f.templateEngine.Execute("commands.go", commandData)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate commands for %s", entity.Name), err)
	}

	return []*GeneratedFile{{
		Path:    fmt.Sprintf("internal/application/%s_commands.go", strings.ToLower(entity.Name)),
		Content: content,
		Metadata: map[string]interface{}{
			"type":   "commands",
			"entity": entity.Name,
		},
	}}, nil
}

// GenerateQueries creates query structures and handlers (Requirement 6.5)
func (f *PericarpComponentFactory) GenerateQueries(entity Entity) ([]*GeneratedFile, error) {
	f.logger.Debug(fmt.Sprintf("Generating queries for entity: %s", entity.Name))

	// Create query data structure
	queryData := f.createQueryData(entity)

	content, err := f.templateEngine.Execute("queries.go", queryData)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate queries for %s", entity.Name), err)
	}

	return []*GeneratedFile{{
		Path:    fmt.Sprintf("internal/application/%s_queries.go", strings.ToLower(entity.Name)),
		Content: content,
		Metadata: map[string]interface{}{
			"type":   "queries",
			"entity": entity.Name,
		},
	}}, nil
}

// GenerateEvents creates domain events following standard structure (Requirement 6.5, 8.5)
func (f *PericarpComponentFactory) GenerateEvents(entity Entity) ([]*GeneratedFile, error) {
	f.logger.Debug(fmt.Sprintf("Generating events for entity: %s", entity.Name))

	content, err := f.templateEngine.Execute("entity_events.go", entity)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate events for %s", entity.Name), err)
	}

	return []*GeneratedFile{{
		Path:    fmt.Sprintf("internal/domain/%s_events.go", strings.ToLower(entity.Name)),
		Content: content,
		Metadata: map[string]interface{}{
			"type":   "events",
			"entity": entity.Name,
		},
	}}, nil
}

// GenerateHandlers creates command and query handlers with error handling (Requirement 6.1, 8.4)
func (f *PericarpComponentFactory) GenerateHandlers(entity Entity) ([]*GeneratedFile, error) {
	f.logger.Debug(fmt.Sprintf("Generating handlers for entity: %s", entity.Name))

	var files []*GeneratedFile

	// Generate command handlers
	commandHandlerData := f.createCommandHandlerData(entity)
	commandContent, err := f.templateEngine.Execute("command_handlers.go", commandHandlerData)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate command handlers for %s", entity.Name), err)
	}

	files = append(files, &GeneratedFile{
		Path:    fmt.Sprintf("internal/application/%s_command_handlers.go", strings.ToLower(entity.Name)),
		Content: commandContent,
		Metadata: map[string]interface{}{
			"type":   "command_handlers",
			"entity": entity.Name,
		},
	})

	// Generate query handlers
	queryHandlerData := f.createQueryHandlerData(entity)
	queryContent, err := f.templateEngine.Execute("query_handlers.go", queryHandlerData)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate query handlers for %s", entity.Name), err)
	}

	files = append(files, &GeneratedFile{
		Path:    fmt.Sprintf("internal/application/%s_query_handlers.go", strings.ToLower(entity.Name)),
		Content: queryContent,
		Metadata: map[string]interface{}{
			"type":   "query_handlers",
			"entity": entity.Name,
		},
	})

	return files, nil
}

// GenerateServices creates service layer with CRUD operations (Requirement 6.6)
func (f *PericarpComponentFactory) GenerateServices(entity Entity) ([]*GeneratedFile, error) {
	f.logger.Debug(fmt.Sprintf("Generating service for entity: %s", entity.Name))

	var files []*GeneratedFile

	// Create service data structure
	serviceData := f.createServiceData(entity)

	// Generate service
	serviceContent, err := f.templateEngine.Execute("service.go", serviceData)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate service for %s", entity.Name), err)
	}

	files = append(files, &GeneratedFile{
		Path:    fmt.Sprintf("internal/application/%s_service.go", strings.ToLower(entity.Name)),
		Content: serviceContent,
		Metadata: map[string]interface{}{
			"type":   "service",
			"entity": entity.Name,
		},
	})

	// Generate service tests
	serviceTestContent, err := f.templateEngine.Execute("service_test.go", serviceData)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate service tests for %s", entity.Name), err)
	}

	files = append(files, &GeneratedFile{
		Path:    fmt.Sprintf("internal/application/%s_service_test.go", strings.ToLower(entity.Name)),
		Content: serviceTestContent,
		Metadata: map[string]interface{}{
			"type":   "service_test",
			"entity": entity.Name,
		},
	})

	return files, nil
}

// GenerateTests creates unit tests for all components (Requirement 8.6)
func (f *PericarpComponentFactory) GenerateTests(entity Entity) ([]*GeneratedFile, error) {
	f.logger.Debug(fmt.Sprintf("Generating tests for entity: %s", entity.Name))

	var files []*GeneratedFile

	// Generate entity tests
	entityTestContent, err := f.templateEngine.Execute("entity_test.go", entity)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate entity tests for %s", entity.Name), err)
	}

	files = append(files, &GeneratedFile{
		Path:    fmt.Sprintf("internal/domain/%s_test.go", strings.ToLower(entity.Name)),
		Content: entityTestContent,
		Metadata: map[string]interface{}{
			"type":   "entity_test",
			"entity": entity.Name,
		},
	})

	// Generate repository tests
	repoTestContent, err := f.templateEngine.Execute("repository_test.go", entity)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate repository tests for %s", entity.Name), err)
	}

	files = append(files, &GeneratedFile{
		Path:    fmt.Sprintf("internal/infrastructure/%s_repository_test.go", strings.ToLower(entity.Name)),
		Content: repoTestContent,
		Metadata: map[string]interface{}{
			"type":   "repository_test",
			"entity": entity.Name,
		},
	})

	// Generate handler tests
	handlerTestContent, err := f.templateEngine.Execute("handlers_test.go", entity)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate handler tests for %s", entity.Name), err)
	}

	files = append(files, &GeneratedFile{
		Path:    fmt.Sprintf("internal/application/%s_handlers_test.go", strings.ToLower(entity.Name)),
		Content: handlerTestContent,
		Metadata: map[string]interface{}{
			"type":   "handlers_test",
			"entity": entity.Name,
		},
	})

	return files, nil
}

// GenerateProjectStructure creates the complete project scaffold (Requirement 2.4)
func (f *PericarpComponentFactory) GenerateProjectStructure(model *DomainModel, destination string) error {
	f.logger.Debug(fmt.Sprintf("Generating project structure for: %s", model.ProjectName))

	// Create directory structure following Pericarp conventions
	directories := []string{
		"cmd/" + model.ProjectName,
		"internal/application",
		"internal/domain",
		"internal/infrastructure",
		"pkg",
		"test/fixtures",
		"test/integration",
		"test/mocks",
		"docs",
		"scripts",
	}

	for _, dir := range directories {
		fullPath := filepath.Join(destination, dir)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return NewCliError(FileSystemError,
				fmt.Sprintf("failed to create directory %s", fullPath), err)
		}
		f.logger.Debug(fmt.Sprintf("Created directory: %s", fullPath))
	}

	// Generate main.go
	mainContent, err := f.templateEngine.Execute("main.go", model)
	if err != nil {
		return NewCliError(GenerationError,
			fmt.Sprintf("failed to generate main.go for %s", model.ProjectName), err)
	}

	mainPath := filepath.Join(destination, "cmd", model.ProjectName, "main.go")
	if err := f.writeFile(mainPath, mainContent); err != nil {
		return err
	}

	// Generate go.mod
	goModContent, err := f.templateEngine.Execute("go.mod", model)
	if err != nil {
		return NewCliError(GenerationError,
			fmt.Sprintf("failed to generate go.mod for %s", model.ProjectName), err)
	}

	goModPath := filepath.Join(destination, "go.mod")
	if err := f.writeFile(goModPath, goModContent); err != nil {
		return err
	}

	// Generate README.md
	readmeContent, err := f.templateEngine.Execute("README.md", model)
	if err != nil {
		return NewCliError(GenerationError,
			fmt.Sprintf("failed to generate README.md for %s", model.ProjectName), err)
	}

	readmePath := filepath.Join(destination, "README.md")
	if err := f.writeFile(readmePath, readmeContent); err != nil {
		return err
	}

	// Generate config.yaml
	configContent, err := f.templateEngine.Execute("config.yaml", model)
	if err != nil {
		return NewCliError(GenerationError,
			fmt.Sprintf("failed to generate config.yaml for %s", model.ProjectName), err)
	}

	configPath := filepath.Join(destination, "config.yaml.example")
	if err := f.writeFile(configPath, configContent); err != nil {
		return err
	}

	// Generate Makefile
	makefile, err := f.GenerateMakefile(model.ProjectName)
	if err != nil {
		return NewCliError(GenerationError,
			fmt.Sprintf("failed to generate Makefile for %s", model.ProjectName), err)
	}

	makefilePath := filepath.Join(destination, "Makefile")
	if err := f.writeFile(makefilePath, makefile.Content); err != nil {
		return err
	}

	return nil
}

// GenerateProjectFiles generates all project files without writing them (for file preservation)
func (f *PericarpComponentFactory) GenerateProjectFiles(model *DomainModel) ([]*GeneratedFile, error) {
	f.logger.Debug(fmt.Sprintf("Generating project files for: %s", model.ProjectName))

	var files []*GeneratedFile

	// Generate main.go
	mainContent, err := f.templateEngine.Execute("main.go", model)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate main.go for %s", model.ProjectName), err)
	}
	files = append(files, &GeneratedFile{
		Path:    filepath.Join("cmd", model.ProjectName, "main.go"),
		Content: mainContent,
		Metadata: map[string]interface{}{
			"type":    "main",
			"project": model.ProjectName,
		},
	})

	// Generate go.mod
	goModContent, err := f.templateEngine.Execute("go.mod", model)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate go.mod for %s", model.ProjectName), err)
	}
	files = append(files, &GeneratedFile{
		Path:    "go.mod",
		Content: goModContent,
		Metadata: map[string]interface{}{
			"type":    "module",
			"project": model.ProjectName,
		},
	})

	// Generate README.md
	readmeContent, err := f.templateEngine.Execute("README.md", model)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate README.md for %s", model.ProjectName), err)
	}
	files = append(files, &GeneratedFile{
		Path:    "README.md",
		Content: readmeContent,
		Metadata: map[string]interface{}{
			"type":    "documentation",
			"project": model.ProjectName,
		},
	})

	// Generate Makefile
	makefile, err := f.GenerateMakefile(model.ProjectName)
	if err != nil {
		return nil, err
	}
	files = append(files, makefile)

	// Generate config.yaml
	configContent, err := f.templateEngine.Execute("config.yaml", model)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate config.yaml for %s", model.ProjectName), err)
	}
	files = append(files, &GeneratedFile{
		Path:    "config.yaml.example",
		Content: configContent,
		Metadata: map[string]interface{}{
			"type":    "configuration",
			"project": model.ProjectName,
		},
	})

	return files, nil
}

// GenerateMakefile creates comprehensive Makefile (Requirement 5)
func (f *PericarpComponentFactory) GenerateMakefile(projectName string) (*GeneratedFile, error) {
	f.logger.Debug(fmt.Sprintf("Generating Makefile for project: %s", projectName))

	makefileData := map[string]interface{}{
		"ProjectName": projectName,
		"HasDatabase": true, // Assume database usage for Pericarp projects
	}

	content, err := f.templateEngine.Execute("Makefile", makefileData)
	if err != nil {
		return nil, NewCliError(GenerationError,
			fmt.Sprintf("failed to generate Makefile for %s", projectName), err)
	}

	return &GeneratedFile{
		Path:    "Makefile",
		Content: content,
		Metadata: map[string]interface{}{
			"type":    "makefile",
			"project": projectName,
		},
	}, nil
}

// Helper methods

// ensureIDField ensures the entity has an ID field
func (f *PericarpComponentFactory) ensureIDField(entity Entity) Entity {
	hasID := false
	for _, prop := range entity.Properties {
		if strings.ToLower(prop.Name) == "id" {
			hasID = true
			break
		}
	}

	if !hasID {
		idProperty := Property{
			Name:     "Id",
			Type:     "ksuid.KSUID",
			Required: true,
			Tags: map[string]string{
				"json": "id",
			},
		}
		entity.Properties = append([]Property{idProperty}, entity.Properties...)
	}

	return entity
}

// createCommandData creates data structure for command generation
func (f *PericarpComponentFactory) createCommandData(entity Entity) map[string]interface{} {
	commands := []map[string]interface{}{
		{
			"Name":        fmt.Sprintf("Create%sCommand", entity.Name),
			"Description": fmt.Sprintf("create a new %s", strings.ToLower(entity.Name)),
			"Properties":  f.filterRequiredProperties(entity.Properties),
		},
		{
			"Name":        fmt.Sprintf("Update%sCommand", entity.Name),
			"Description": fmt.Sprintf("update an existing %s", strings.ToLower(entity.Name)),
			"Properties":  entity.Properties,
		},
		{
			"Name":        fmt.Sprintf("Delete%sCommand", entity.Name),
			"Description": fmt.Sprintf("delete a %s", strings.ToLower(entity.Name)),
			"Properties": []Property{
				{Name: "Id", Type: "ksuid.KSUID", Required: true},
			},
		},
	}

	return map[string]interface{}{
		"Entity":   entity,
		"Commands": commands,
	}
}

// createQueryData creates data structure for query generation
func (f *PericarpComponentFactory) createQueryData(entity Entity) map[string]interface{} {
	queries := []map[string]interface{}{
		{
			"Name":        fmt.Sprintf("Get%sByIdQuery", entity.Name),
			"Description": fmt.Sprintf("get a %s by ID", strings.ToLower(entity.Name)),
			"Properties": []Property{
				{Name: "Id", Type: "ksuid.KSUID", Required: true},
			},
			"ReturnType": fmt.Sprintf("*%s", entity.Name),
		},
		{
			"Name":        fmt.Sprintf("List%sQuery", entity.Name),
			"Description": fmt.Sprintf("list all %s", strings.ToLower(entity.Name)),
			"Properties": []Property{
				{Name: "Limit", Type: "int", Required: false, DefaultValue: "10"},
				{Name: "Offset", Type: "int", Required: false, DefaultValue: "0"},
			},
			"ReturnType": fmt.Sprintf("[]%s", entity.Name),
		},
	}

	return map[string]interface{}{
		"Entity":  entity,
		"Queries": queries,
	}
}

// createCommandHandlerData creates data structure for command handler generation
func (f *PericarpComponentFactory) createCommandHandlerData(entity Entity) map[string]interface{} {
	return map[string]interface{}{
		"Entity": entity,
		"Handlers": []string{
			fmt.Sprintf("Create%sHandler", entity.Name),
			fmt.Sprintf("Update%sHandler", entity.Name),
			fmt.Sprintf("Delete%sHandler", entity.Name),
		},
	}
}

// createQueryHandlerData creates data structure for query handler generation
func (f *PericarpComponentFactory) createQueryHandlerData(entity Entity) map[string]interface{} {
	return map[string]interface{}{
		"Entity": entity,
		"Handlers": []string{
			fmt.Sprintf("Get%sByIdHandler", entity.Name),
			fmt.Sprintf("List%sHandler", entity.Name),
		},
	}
}

// createServiceData creates data structure for service generation
func (f *PericarpComponentFactory) createServiceData(entity Entity) map[string]interface{} {
	return map[string]interface{}{
		"Entity":      entity,
		"ProjectName": "example.com/project", // Default project name, should be configurable
	}
}

// filterRequiredProperties filters properties to only required ones (excluding ID)
func (f *PericarpComponentFactory) filterRequiredProperties(properties []Property) []Property {
	var required []Property
	for _, prop := range properties {
		if prop.Required && strings.ToLower(prop.Name) != "id" {
			required = append(required, prop)
		}
	}
	return required
}

// writeFile writes content to a file
func (f *PericarpComponentFactory) writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return NewCliError(FileSystemError,
			fmt.Sprintf("failed to create directory for %s", path), err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return NewCliError(FileSystemError,
			fmt.Sprintf("failed to write file %s", path), err)
	}

	f.logger.Debug(fmt.Sprintf("Generated file: %s", path))
	return nil
}
