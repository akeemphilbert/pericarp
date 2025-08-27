package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEndToEnd_CompleteProjectGeneration tests the complete workflow from project creation to code generation
func TestEndToEnd_CompleteProjectGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	logger := NewTestLogger()
	tempDir := t.TempDir()

	// Step 1: Create a new project
	projectName := "e2e-test-service"
	projectDir := filepath.Join(tempDir, projectName)

	creator := NewProjectCreator(logger)
	err := creator.CreateProject(projectName, "", projectDir, false)
	require.NoError(t, err, "Project creation should succeed")

	// Verify project structure was created
	expectedDirs := []string{
		"cmd/" + projectName,
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

	for _, dir := range expectedDirs {
		assert.DirExists(t, filepath.Join(projectDir, dir), "Directory %s should exist", dir)
	}

	expectedFiles := []string{
		"go.mod",
		"Makefile",
		"README.md",
		"cmd/" + projectName + "/main.go",
	}

	for _, file := range expectedFiles {
		assert.FileExists(t, filepath.Join(projectDir, file), "File %s should exist", file)
	}

	// Step 2: Generate code from OpenAPI specification
	generator := NewCodeGenerator(logger)
	inputFile := "testdata/user-service.yaml"

	err = generator.Generate(inputFile, "openapi", projectDir, false)
	require.NoError(t, err, "Code generation should succeed")

	// Verify generated code files exist
	expectedGeneratedFiles := []string{
		"internal/domain/user.go",
		"internal/domain/user_events.go",
		"internal/domain/profile.go",
		"internal/domain/profile_events.go",
		"internal/domain/address.go",
		"internal/domain/address_events.go",
		"internal/application/user_commands.go",
		"internal/application/user_queries.go",
		"internal/application/user_command_handlers.go",
		"internal/application/user_query_handlers.go",
	}

	for _, file := range expectedGeneratedFiles {
		assert.FileExists(t, filepath.Join(projectDir, file), "Generated file %s should exist", file)
	}

	// Step 3: Verify generated code content is valid
	t.Run("verify generated entity code", func(t *testing.T) {
		userEntityPath := filepath.Join(projectDir, "internal/domain/user.go")
		content, err := os.ReadFile(userEntityPath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, "package domain")
		assert.Contains(t, contentStr, "type User struct")
		assert.Contains(t, contentStr, "func NewUser(")
		assert.Contains(t, contentStr, "ID() string")
		assert.Contains(t, contentStr, "Version() int")
		assert.Contains(t, contentStr, "UncommittedEvents()")
		assert.Contains(t, contentStr, "MarkEventsAsCommitted()")
	})

	t.Run("verify generated command code", func(t *testing.T) {
		commandsPath := filepath.Join(projectDir, "internal/application/user_commands.go")
		content, err := os.ReadFile(commandsPath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, "package application")
		assert.Contains(t, contentStr, "type CreateUserCommand struct")
		assert.Contains(t, contentStr, "type UpdateUserCommand struct")
		assert.Contains(t, contentStr, "type DeleteUserCommand struct")
	})

	t.Run("verify generated handler code", func(t *testing.T) {
		handlersPath := filepath.Join(projectDir, "internal/application/user_command_handlers.go")
		content, err := os.ReadFile(handlersPath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, "package application")
		assert.Contains(t, contentStr, "type CreateUserHandler struct")
		assert.Contains(t, contentStr, "func (h *CreateUserHandler) Handle")
	})
}

// TestEndToEnd_MultipleInputFormats tests generation from different input formats
func TestEndToEnd_MultipleInputFormats(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	logger := NewTestLogger()
	tempDir := t.TempDir()

	formats := []struct {
		name      string
		inputFile string
		inputType string
	}{
		{
			name:      "OpenAPI",
			inputFile: "testdata/user-service.yaml",
			inputType: "openapi",
		},
		{
			name:      "Protocol Buffers",
			inputFile: "testdata/user.proto",
			inputType: "proto",
		},
	}

	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			// Create separate project for each format
			projectName := "test-" + strings.ToLower(format.name)
			projectDir := filepath.Join(tempDir, projectName)

			// Create project
			creator := NewProjectCreator(logger)
			err := creator.CreateProject(projectName, "", projectDir, false)
			require.NoError(t, err)

			// Generate code
			generator := NewCodeGenerator(logger)
			err = generator.Generate(format.inputFile, format.inputType, projectDir, false)
			require.NoError(t, err)

			// Verify at least some files were generated
			domainDir := filepath.Join(projectDir, "internal/domain")
			entries, err := os.ReadDir(domainDir)
			require.NoError(t, err)

			// Should have generated at least some domain files
			domainFiles := 0
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".go") {
					domainFiles++
				}
			}
			assert.Greater(t, domainFiles, 0, "Should have generated domain files for %s", format.name)
		})
	}
}

// TestEndToEnd_CLICommandIntegration tests CLI commands with various flag combinations
func TestEndToEnd_CLICommandIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	logger := NewTestLogger()
	tempDir := t.TempDir()

	testCases := []struct {
		name        string
		command     string
		args        []string
		expectError bool
		validate    func(t *testing.T, tempDir string)
	}{
		{
			name:    "new command with dry-run",
			command: "new",
			args:    []string{"test-project", "--dry-run", "--destination", tempDir},
			validate: func(t *testing.T, tempDir string) {
				// In dry-run mode, no files should be created
				projectDir := filepath.Join(tempDir, "test-project")
				assert.NoDirExists(t, projectDir)
			},
		},
		{
			name:    "new command with verbose",
			command: "new",
			args:    []string{"verbose-project", "--dry-run", "--verbose", "--destination", tempDir},
			validate: func(t *testing.T, tempDir string) {
				// Should complete without error
				projectDir := filepath.Join(tempDir, "verbose-project")
				assert.NoDirExists(t, projectDir) // dry-run mode
			},
		},
		{
			name:    "generate command with dry-run",
			command: "generate",
			args:    []string{"testdata/user-service.yaml", "--dry-run", "--destination", tempDir},
			validate: func(t *testing.T, tempDir string) {
				// In dry-run mode, no files should be created
				entries, err := os.ReadDir(tempDir)
				require.NoError(t, err)
				assert.Empty(t, entries, "No files should be created in dry-run mode")
			},
		},
		{
			name:        "new command with invalid project name",
			command:     "new",
			args:        []string{"Invalid-Project", "--dry-run"},
			expectError: true,
		},
		{
			name:        "generate command with non-existent file",
			command:     "generate",
			args:        []string{"nonexistent.yaml", "--dry-run"},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testTempDir := filepath.Join(tempDir, tc.name)
			err := os.MkdirAll(testTempDir, 0755)
			require.NoError(t, err)

			var err2 error
			switch tc.command {
			case "new":
				creator := NewProjectCreator(logger)
				projectName := tc.args[0]

				// Parse flags
				var destination string
				var dryRun bool
				for i, arg := range tc.args[1:] {
					switch arg {
					case "--destination":
						if i+2 < len(tc.args) {
							destination = tc.args[i+2]
						}
					case "--dry-run":
						dryRun = true
					case "--verbose":
						logger.SetVerbose(true)
					}
				}

				if destination == "" {
					destination = testTempDir
				}

				err2 = creator.CreateProject(projectName, "", destination, dryRun)

			case "generate":
				generator := NewCodeGenerator(logger)
				inputFile := tc.args[0]

				// Parse flags
				var destination string
				var dryRun bool = true // Default to dry-run for tests
				for i, arg := range tc.args[1:] {
					switch arg {
					case "--destination":
						if i+2 < len(tc.args) {
							destination = tc.args[i+2]
						}
					case "--dry-run":
						dryRun = true
					}
				}

				if destination == "" {
					destination = testTempDir
				}

				err2 = generator.Generate(inputFile, "openapi", destination, dryRun)
			}

			if tc.expectError {
				assert.Error(t, err2, "Command should fail for test case: %s", tc.name)
			} else {
				assert.NoError(t, err2, "Command should succeed for test case: %s", tc.name)
				if tc.validate != nil {
					tc.validate(t, testTempDir)
				}
			}
		})
	}
}

// TestEndToEnd_FilePreservationWorkflow tests the complete workflow with existing repository
func TestEndToEnd_FilePreservationWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	logger := NewTestLogger()
	tempDir := t.TempDir()

	// Step 1: Create a mock existing repository
	repoDir := filepath.Join(tempDir, "existing-repo")
	err := os.MkdirAll(repoDir, 0755)
	require.NoError(t, err)

	// Create .git directory to simulate a Git repository
	err = os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)
	require.NoError(t, err)

	// Create some existing files that should be preserved
	existingFiles := map[string]string{
		"README.md":   "# Existing Project\n\nThis is an existing project with custom content.",
		"go.mod":      "module existing-project\n\ngo 1.21\n\nrequire (\n\tgithub.com/custom/package v1.0.0\n)",
		"custom.go":   "package main\n\n// Custom existing code\nfunc customFunction() {}\n",
		"docs/api.md": "# API Documentation\n\nCustom API documentation.",
	}

	for filePath, content := range existingFiles {
		fullPath := filepath.Join(repoDir, filePath)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Step 2: Add Pericarp capabilities to existing repository
	creator := NewProjectCreator(logger)
	err = creator.CreateProject("existing-project", "", repoDir, false)
	require.NoError(t, err)

	// Step 3: Verify existing files were preserved
	for filePath, expectedContent := range existingFiles {
		fullPath := filepath.Join(repoDir, filePath)
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err, "Existing file %s should still exist", filePath)
		assert.Equal(t, expectedContent, string(content), "Existing file %s should not be modified", filePath)
	}

	// Step 4: Verify new Pericarp files were added (but not conflicting ones)
	// Makefile should be added (doesn't conflict)
	makefilePath := filepath.Join(repoDir, "Makefile")
	assert.FileExists(t, makefilePath, "Makefile should be added")

	// Main.go should be added
	mainGoPath := filepath.Join(repoDir, "cmd/existing-project/main.go")
	assert.FileExists(t, mainGoPath, "Main.go should be added")

	// Step 5: Generate code and verify file preservation continues
	generator := NewCodeGenerator(logger)
	err = generator.Generate("testdata/user-service.yaml", "openapi", repoDir, false)
	require.NoError(t, err)

	// Verify existing files are still preserved after code generation
	for filePath, expectedContent := range existingFiles {
		fullPath := filepath.Join(repoDir, filePath)
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err, "Existing file %s should still exist after code generation", filePath)
		assert.Equal(t, expectedContent, string(content), "Existing file %s should not be modified after code generation", filePath)
	}

	// Verify new domain files were generated
	userEntityPath := filepath.Join(repoDir, "internal/domain/user.go")
	assert.FileExists(t, userEntityPath, "Generated domain files should exist")
}

// TestEndToEnd_ErrorRecoveryAndValidation tests error handling and recovery scenarios
func TestEndToEnd_ErrorRecoveryAndValidation(t *testing.T) {
	logger := NewTestLogger()
	tempDir := t.TempDir()

	t.Run("invalid project name recovery", func(t *testing.T) {
		creator := NewProjectCreator(logger)

		// Try with invalid name first
		err := creator.CreateProject("Invalid-Name", "", tempDir, true)
		assert.Error(t, err)

		// Then try with valid name
		err = creator.CreateProject("valid-name", "", tempDir, true)
		assert.NoError(t, err)
	})

	t.Run("invalid input file recovery", func(t *testing.T) {
		generator := NewCodeGenerator(logger)

		// Try with non-existent file first
		err := generator.Generate("nonexistent.yaml", "openapi", tempDir, true)
		assert.Error(t, err)

		// Then try with valid file
		err = generator.Generate("testdata/user-service.yaml", "openapi", tempDir, true)
		assert.NoError(t, err)
	})

	t.Run("malformed input file handling", func(t *testing.T) {
		generator := NewCodeGenerator(logger)

		// Try with invalid OpenAPI file
		err := generator.Generate("testdata/invalid-openapi.yaml", "openapi", tempDir, true)
		assert.Error(t, err)

		// Verify error type and message
		if cliErr, ok := err.(*CliError); ok {
			assert.Contains(t, []ErrorType{ParseError, ValidationError}, cliErr.Type)
			assert.Contains(t, cliErr.Message, "validation failed")
		}
	})
}

// TestEndToEnd_PerformanceAndScalability tests performance with larger inputs
func TestEndToEnd_PerformanceAndScalability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	logger := NewTestLogger()
	tempDir := t.TempDir()

	t.Run("large project generation performance", func(t *testing.T) {
		start := time.Now()

		creator := NewProjectCreator(logger)
		err := creator.CreateProject("large-project", "", filepath.Join(tempDir, "large-project"), false)
		require.NoError(t, err)

		duration := time.Since(start)
		assert.Less(t, duration, 10*time.Second, "Project creation should complete within 10 seconds")
	})

	t.Run("code generation performance", func(t *testing.T) {
		start := time.Now()

		generator := NewCodeGenerator(logger)
		err := generator.Generate("testdata/user-service.yaml", "openapi", tempDir, true)
		require.NoError(t, err)

		duration := time.Since(start)
		assert.Less(t, duration, 5*time.Second, "Code generation should complete within 5 seconds")
	})
}

// TestEndToEnd_ConcurrentOperations tests concurrent operations
func TestEndToEnd_ConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency test in short mode")
	}

	logger := NewTestLogger()
	tempDir := t.TempDir()

	t.Run("concurrent project creation", func(t *testing.T) {
		const numProjects = 5
		results := make(chan error, numProjects)

		for i := 0; i < numProjects; i++ {
			go func(index int) {
				creator := NewProjectCreator(logger)
				projectName := fmt.Sprintf("concurrent-project-%d", index)
				projectDir := filepath.Join(tempDir, projectName)

				err := creator.CreateProject(projectName, "", projectDir, false)
				results <- err
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numProjects; i++ {
			err := <-results
			assert.NoError(t, err, "Concurrent project creation should succeed")
		}
	})

	t.Run("concurrent code generation", func(t *testing.T) {
		const numGenerations = 3
		results := make(chan error, numGenerations)

		for i := 0; i < numGenerations; i++ {
			go func(index int) {
				generator := NewCodeGenerator(logger)
				outputDir := filepath.Join(tempDir, fmt.Sprintf("concurrent-gen-%d", index))

				err := generator.Generate("testdata/user-service.yaml", "openapi", outputDir, true)
				results <- err
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGenerations; i++ {
			err := <-results
			assert.NoError(t, err, "Concurrent code generation should succeed")
		}
	})
}

// TestEndToEnd_ContextCancellation tests context cancellation scenarios
func TestEndToEnd_ContextCancellation(t *testing.T) {
	logger := NewTestLogger()
	tempDir := t.TempDir()

	t.Run("executor context cancellation", func(t *testing.T) {
		executor := NewExecutor(logger)

		// Create a context that will be cancelled
		ctx, cancel := context.WithCancel(context.Background())

		// Create some files to generate
		files := []*GeneratedFile{
			{Path: "test1.go", Content: "package main"},
			{Path: "test2.go", Content: "package main"},
		}

		// Cancel the context immediately
		cancel()

		err := executor.Execute(ctx, files, tempDir, false)
		assert.Error(t, err)

		if cliErr, ok := err.(*CliError); ok {
			assert.Equal(t, GenerationError, cliErr.Type)
			assert.Contains(t, cliErr.Message, "cancelled")
		}
	})

	t.Run("executor context timeout", func(t *testing.T) {
		executor := NewExecutor(logger)

		// Create a context with very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Wait for timeout
		time.Sleep(1 * time.Millisecond)

		files := []*GeneratedFile{
			{Path: "test.go", Content: "package main"},
		}

		err := executor.Execute(ctx, files, tempDir, false)
		assert.Error(t, err)

		if cliErr, ok := err.(*CliError); ok {
			assert.Equal(t, GenerationError, cliErr.Type)
			assert.Contains(t, cliErr.Message, "cancelled")
		}
	})
}

// TestEndToEnd_VerboseLoggingIntegration tests verbose logging throughout the workflow
func TestEndToEnd_VerboseLoggingIntegration(t *testing.T) {
	logger := NewVerboseLogger()
	logger.SetVerbose(true)
	tempDir := t.TempDir()

	t.Run("verbose project creation", func(t *testing.T) {
		creator := NewProjectCreator(logger)
		projectDir := filepath.Join(tempDir, "verbose-project")

		err := creator.CreateProject("verbose-project", "", projectDir, true)
		assert.NoError(t, err)
	})

	t.Run("verbose code generation", func(t *testing.T) {
		generator := NewCodeGenerator(logger)

		err := generator.Generate("testdata/user-service.yaml", "openapi", tempDir, true)
		assert.NoError(t, err)
	})
}

// Benchmark tests for end-to-end workflows
func BenchmarkEndToEnd_ProjectCreation(b *testing.B) {
	logger := NewTestLogger()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()
		creator := NewProjectCreator(logger)
		projectName := fmt.Sprintf("benchmark-project-%d", i)
		projectDir := filepath.Join(tempDir, projectName)

		err := creator.CreateProject(projectName, "", projectDir, true) // Use dry-run for speed
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEndToEnd_CodeGeneration(b *testing.B) {
	logger := NewTestLogger()
	generator := NewCodeGenerator(logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()

		err := generator.Generate("testdata/user-service.yaml", "openapi", tempDir, true) // Use dry-run for speed
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEndToEnd_CompleteWorkflow(b *testing.B) {
	logger := NewTestLogger()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()
		projectName := fmt.Sprintf("benchmark-complete-%d", i)
		projectDir := filepath.Join(tempDir, projectName)

		// Create project
		creator := NewProjectCreator(logger)
		err := creator.CreateProject(projectName, "", projectDir, true)
		if err != nil {
			b.Fatal(err)
		}

		// Generate code
		generator := NewCodeGenerator(logger)
		err = generator.Generate("testdata/user-service.yaml", "openapi", projectDir, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}
