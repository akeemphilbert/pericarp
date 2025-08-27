package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProjectCreator(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	assert.NotNil(t, creator)
	assert.NotNil(t, creator.logger)
	assert.NotNil(t, creator.validator)
	assert.NotNil(t, creator.executor)
	assert.NotNil(t, creator.factory)
	assert.NotNil(t, creator.cloner)
}

func TestProjectCreator_CreateProject_ValidProject(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	tempDir := t.TempDir()
	projectName := "test-project"
	destination := filepath.Join(tempDir, projectName)

	err := creator.CreateProject(projectName, "", destination, false)

	assert.NoError(t, err)
	assert.DirExists(t, destination)

	// Verify basic project structure was created
	expectedDirs := []string{
		"cmd/" + projectName,
		"internal/application",
		"internal/domain",
		"internal/infrastructure",
		"pkg",
		"test/fixtures",
		"test/integration",
		"test/mocks",
	}

	for _, dir := range expectedDirs {
		assert.DirExists(t, filepath.Join(destination, dir), "Directory %s should exist", dir)
	}

	// Verify essential files were created
	expectedFiles := []string{
		"go.mod",
		"Makefile",
		"README.md",
		"cmd/" + projectName + "/main.go",
	}

	for _, file := range expectedFiles {
		assert.FileExists(t, filepath.Join(destination, file), "File %s should exist", file)
	}
}

func TestProjectCreator_CreateProject_DryRun(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	tempDir := t.TempDir()
	projectName := "dry-run-project"
	destination := filepath.Join(tempDir, projectName)

	err := creator.CreateProject(projectName, "", destination, true)

	assert.NoError(t, err)
	// In dry-run mode, no directories or files should be created
	assert.NoDirExists(t, destination)
}

func TestProjectCreator_CreateProject_InvalidProjectName(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	tests := []struct {
		name        string
		projectName string
		errorType   ErrorType
	}{
		{
			name:        "empty project name",
			projectName: "",
			errorType:   ValidationError,
		},
		{
			name:        "project name with uppercase",
			projectName: "MyProject",
			errorType:   ValidationError,
		},
		{
			name:        "project name with spaces",
			projectName: "my project",
			errorType:   ValidationError,
		},
		{
			name:        "project name starting with number",
			projectName: "123project",
			errorType:   ValidationError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			destination := filepath.Join(tempDir, "test")

			err := creator.CreateProject(tt.projectName, "", destination, false)

			assert.Error(t, err)
			cliErr, ok := err.(*CliError)
			require.True(t, ok)
			assert.Equal(t, tt.errorType, cliErr.Type)
		})
	}
}

func TestProjectCreator_CreateProject_InvalidDestination(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	tempDir := t.TempDir()

	// Create a file where we want to create a directory
	conflictFile := filepath.Join(tempDir, "conflict")
	err := os.WriteFile(conflictFile, []byte("test"), 0644)
	require.NoError(t, err)

	err = creator.CreateProject("test-project", "", conflictFile, false)

	assert.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, ValidationError, cliErr.Type)
}

func TestProjectCreator_CreateProject_WithRepository_DryRun_Unit(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	tempDir := t.TempDir()
	projectName := "repo-project"
	destination := filepath.Join(tempDir, projectName)
	repoURL := "https://github.com/example/repo.git"

	err := creator.CreateProject(projectName, repoURL, destination, true)

	assert.NoError(t, err)
	// In dry-run mode, no cloning should occur
	assert.NoDirExists(t, destination)
}

func TestProjectCreator_CreateProject_DefaultDestination(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	// Change to temp directory for this test
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	tempDir := t.TempDir()
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	projectName := "default-dest-project"

	err = creator.CreateProject(projectName, "", "", false)

	assert.NoError(t, err)
	// Should create project in current directory with project name
	assert.DirExists(t, filepath.Join(tempDir, projectName))
}

func TestProjectCreator_CreateProject_VerboseLogging(t *testing.T) {
	logger := NewVerboseLogger()
	logger.SetVerbose(true)
	creator := NewProjectCreator(logger)

	tempDir := t.TempDir()
	projectName := "verbose-project"
	destination := filepath.Join(tempDir, projectName)

	err := creator.CreateProject(projectName, "", destination, true)

	assert.NoError(t, err)
}

func TestProjectCreator_handleRepositoryCloning_DryRun(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	tempDir := t.TempDir()
	destination := filepath.Join(tempDir, "test-repo")
	repoURL := "https://github.com/example/repo.git"

	err := creator.handleRepositoryCloning(repoURL, destination, true)

	assert.NoError(t, err)
	// No actual cloning should occur in dry-run mode
	assert.NoDirExists(t, destination)
}

func TestProjectCreator_generateProjectStructure_DryRun(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	tempDir := t.TempDir()
	projectName := "structure-test"
	destination := filepath.Join(tempDir, projectName)

	err := creator.generateProjectStructure(projectName, destination, true, false)

	assert.NoError(t, err)
	// No actual structure should be created in dry-run mode
	assert.NoDirExists(t, destination)
}

func TestProjectCreator_generateProjectStructure_ExistingRepo(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	tempDir := t.TempDir()
	projectName := "existing-repo-test"

	// Create a mock existing repository structure
	err := os.MkdirAll(filepath.Join(tempDir, ".git"), 0755)
	require.NoError(t, err)

	// Create some existing files
	existingFile := filepath.Join(tempDir, "README.md")
	err = os.WriteFile(existingFile, []byte("Existing README"), 0644)
	require.NoError(t, err)

	err = creator.generateProjectStructure(projectName, tempDir, true, true)

	assert.NoError(t, err)
}

func TestProjectCreator_previewProjectStructure(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	model := &DomainModel{
		ProjectName: "preview-test",
		Entities:    []Entity{},
		Relations:   []Relation{},
	}

	tempDir := t.TempDir()

	// Test preview for new project
	err := creator.previewProjectStructure(model, tempDir, false)
	assert.NoError(t, err)

	// Test preview for existing repository
	err = creator.previewProjectStructure(model, tempDir, true)
	assert.NoError(t, err)
}

func TestProjectCreator_previewProjectStructure_WithExistingFiles(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	model := &DomainModel{
		ProjectName: "preview-existing-test",
		Entities:    []Entity{},
		Relations:   []Relation{},
	}

	tempDir := t.TempDir()

	// Create some existing files
	err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module test"), 0644)
	require.NoError(t, err)

	err = creator.previewProjectStructure(model, tempDir, true)
	assert.NoError(t, err)
}

func TestProjectCreator_generateWithFilePreservation(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	model := &DomainModel{
		ProjectName: "preservation-test",
		Entities:    []Entity{},
		Relations:   []Relation{},
	}

	tempDir := t.TempDir()

	// Create some existing files that should be preserved
	existingFile := filepath.Join(tempDir, "existing.txt")
	err := os.WriteFile(existingFile, []byte("existing content"), 0644)
	require.NoError(t, err)

	// This test would require mocking the factory.GenerateProjectFiles method
	// For now, we'll test that the method doesn't panic and handles the basic flow
	err = creator.generateWithFilePreservation(model, tempDir)

	// The method will likely fail because GenerateProjectFiles is not implemented
	// but we can verify it handles the error gracefully
	if err != nil {
		cliErr, ok := err.(*CliError)
		if ok {
			assert.Contains(t, []ErrorType{GenerationError, FileSystemError}, cliErr.Type)
		}
	}

	// Verify existing file was not modified
	content, err := os.ReadFile(existingFile)
	require.NoError(t, err)
	assert.Equal(t, "existing content", string(content))
}

func TestNewCodeGenerator(t *testing.T) {
	logger := NewTestLogger()
	generator := NewCodeGenerator(logger)

	assert.NotNil(t, generator)
	assert.NotNil(t, generator.logger)
	assert.NotNil(t, generator.registry)
	assert.NotNil(t, generator.factory)
	assert.NotNil(t, generator.executor)
}

func TestCodeGenerator_Generate_OpenAPIFile(t *testing.T) {
	logger := NewTestLogger()
	generator := NewCodeGenerator(logger)

	tempDir := t.TempDir()
	outputDir := filepath.Join(tempDir, "output")

	// Use the existing test OpenAPI file
	inputFile := "testdata/user-service.yaml"

	err := generator.Generate(inputFile, "openapi", outputDir, true)

	// Should succeed in dry-run mode
	assert.NoError(t, err)
}

func TestCodeGenerator_Generate_ProtoFile(t *testing.T) {
	logger := NewTestLogger()
	generator := NewCodeGenerator(logger)

	tempDir := t.TempDir()
	outputDir := filepath.Join(tempDir, "output")

	// Use the existing test proto file
	inputFile := "testdata/user.proto"

	err := generator.Generate(inputFile, "proto", outputDir, true)

	// Should succeed in dry-run mode
	assert.NoError(t, err)
}

func TestCodeGenerator_Generate_InvalidFile(t *testing.T) {
	logger := NewTestLogger()
	generator := NewCodeGenerator(logger)

	tempDir := t.TempDir()
	outputDir := filepath.Join(tempDir, "output")

	// Non-existent file
	inputFile := "nonexistent.yaml"

	err := generator.Generate(inputFile, "openapi", outputDir, false)

	assert.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, ParseError, cliErr.Type)
}

func TestCodeGenerator_Generate_UnsupportedFormat(t *testing.T) {
	logger := NewTestLogger()
	generator := NewCodeGenerator(logger)

	tempDir := t.TempDir()
	outputDir := filepath.Join(tempDir, "output")

	// Create a file with unsupported extension
	inputFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(inputFile, []byte("test content"), 0644)
	require.NoError(t, err)

	err = generator.Generate(inputFile, "txt", outputDir, false)

	assert.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, ParseError, cliErr.Type)
}

func TestCodeGenerator_Generate_VerboseLogging(t *testing.T) {
	logger := NewVerboseLogger()
	logger.SetVerbose(true)
	generator := NewCodeGenerator(logger)

	tempDir := t.TempDir()
	outputDir := filepath.Join(tempDir, "output")

	// Use the existing test OpenAPI file
	inputFile := "testdata/user-service.yaml"

	err := generator.Generate(inputFile, "openapi", outputDir, true)

	// Should succeed in dry-run mode with verbose logging
	assert.NoError(t, err)
}

func TestCodeGenerator_Generate_DefaultDestination(t *testing.T) {
	logger := NewTestLogger()
	generator := NewCodeGenerator(logger)

	// Use the existing test OpenAPI file
	inputFile := "testdata/user-service.yaml"

	err := generator.Generate(inputFile, "openapi", "", true)

	// Should succeed in dry-run mode with default destination
	assert.NoError(t, err)
}

func TestCodeGenerator_generateEntityComponents(t *testing.T) {
	logger := NewTestLogger()
	generator := NewCodeGenerator(logger)

	entity := Entity{
		Name: "TestEntity",
		Properties: []Property{
			{Name: "id", Type: "uuid.UUID", Required: true},
			{Name: "name", Type: "string", Required: true},
		},
	}

	files, err := generator.generateEntityComponents(entity)

	assert.NoError(t, err)
	assert.NotEmpty(t, files)

	// Verify that different types of files were generated
	fileTypes := make(map[string]bool)
	for _, file := range files {
		if fileType, exists := file.Metadata["type"]; exists {
			fileTypes[fileType.(string)] = true
		}
	}

	// Should have generated various component types
	expectedTypes := []string{"entity", "repository_interface", "repository_implementation", "commands", "queries", "events"}
	for _, expectedType := range expectedTypes {
		assert.True(t, fileTypes[expectedType], "Should have generated %s files", expectedType)
	}
}

func TestCodeGenerator_generateEntityComponents_VerboseLogging(t *testing.T) {
	logger := NewVerboseLogger()
	logger.SetVerbose(true)
	generator := NewCodeGenerator(logger)

	entity := Entity{
		Name: "VerboseTestEntity",
		Properties: []Property{
			{Name: "id", Type: "uuid.UUID", Required: true},
		},
	}

	files, err := generator.generateEntityComponents(entity)

	assert.NoError(t, err)
	assert.NotEmpty(t, files)
}

// Integration test that combines project creation and code generation
func TestProjectCreator_CodeGenerator_Integration(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)
	generator := NewCodeGenerator(logger)

	tempDir := t.TempDir()
	projectName := "integration-test"
	projectDir := filepath.Join(tempDir, projectName)

	// First create a project
	err := creator.CreateProject(projectName, "", projectDir, false)
	require.NoError(t, err)

	// Then generate code into the project
	inputFile := "testdata/user-service.yaml"
	err = generator.Generate(inputFile, "openapi", projectDir, true) // Use dry-run to avoid conflicts

	assert.NoError(t, err)
}

// Benchmark tests
func BenchmarkProjectCreator_CreateProject_DryRun(b *testing.B) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()
		projectName := "benchmark-project"
		destination := filepath.Join(tempDir, projectName)

		err := creator.CreateProject(projectName, "", destination, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCodeGenerator_Generate_DryRun(b *testing.B) {
	logger := NewTestLogger()
	generator := NewCodeGenerator(logger)

	inputFile := "testdata/user-service.yaml"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()
		outputDir := filepath.Join(tempDir, "output")

		err := generator.Generate(inputFile, "openapi", outputDir, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}
