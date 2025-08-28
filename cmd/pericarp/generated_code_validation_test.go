package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeneratedCode_CompilationValidation tests that generated code compiles successfully
func TestGeneratedCode_CompilationValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping generated code validation test in short mode")
	}

	logger := NewTestLogger()
	tempDir := t.TempDir()

	// Create a test project
	projectName := "validation-test-project"
	projectDir := filepath.Join(tempDir, projectName)

	creator := NewProjectCreator(logger)
	err := creator.CreateProject(projectName, "", projectDir, false)
	require.NoError(t, err)

	// Generate code from OpenAPI
	generator := NewCodeGenerator(logger)
	err = generator.Generate("testdata/user-service.yaml", "openapi", projectDir, false)
	require.NoError(t, err)

	// Test that the generated code compiles
	t.Run("generated project compiles", func(t *testing.T) {
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = projectDir
		output, err := cmd.CombinedOutput()

		if err != nil {
			t.Logf("Compilation output: %s", string(output))
		}
		assert.NoError(t, err, "Generated code should compile without errors")
	})

	// Test that go mod tidy works
	t.Run("go mod tidy succeeds", func(t *testing.T) {
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Dir = projectDir
		output, err := cmd.CombinedOutput()

		if err != nil {
			t.Logf("Go mod tidy output: %s", string(output))
		}
		assert.NoError(t, err, "go mod tidy should succeed")
	})

	// Test that generated tests compile and run
	t.Run("generated tests compile and run", func(t *testing.T) {
		cmd := exec.Command("go", "test", "./...")
		cmd.Dir = projectDir
		output, err := cmd.CombinedOutput()

		if err != nil {
			t.Logf("Test output: %s", string(output))
		}
		assert.NoError(t, err, "Generated tests should compile and run")
	})
}

// TestGeneratedCode_SyntaxValidation tests that generated code has valid Go syntax
func TestGeneratedCode_SyntaxValidation(t *testing.T) {
	logger := NewTestLogger()
	tempDir := t.TempDir()

	// Generate code to validate
	generator := NewCodeGenerator(logger)
	err := generator.Generate("testdata/user-service.yaml", "openapi", tempDir, false)
	require.NoError(t, err)

	// Walk through all generated .go files and validate syntax
	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		t.Run(fmt.Sprintf("syntax validation for %s", filepath.Base(path)), func(t *testing.T) {
			// Parse the Go file
			fset := token.NewFileSet()
			_, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			assert.NoError(t, err, "File %s should have valid Go syntax", path)
		})

		return nil
	})

	require.NoError(t, err, "Should be able to walk through generated files")
}

// TestGeneratedCode_StructureValidation tests that generated code follows expected structure
func TestGeneratedCode_StructureValidation(t *testing.T) {
	logger := NewTestLogger()
	tempDir := t.TempDir()

	// Generate code to validate
	generator := NewCodeGenerator(logger)
	err := generator.Generate("testdata/user-service.yaml", "openapi", tempDir, false)
	require.NoError(t, err)

	t.Run("domain entities have required methods", func(t *testing.T) {
		userEntityPath := filepath.Join(tempDir, "internal/domain/user.go")
		content, err := os.ReadFile(userEntityPath)
		require.NoError(t, err)

		contentStr := string(content)

		// Check for required aggregate root methods
		requiredMethods := []string{
			"func NewUser(",
			"func (u *User) ID() string",
			"func (u *User) Version() int",
			"func (u *User) UncommittedEvents()",
			"func (u *User) MarkEventsAsCommitted()",
			"func (u *User) LoadFromHistory(",
		}

		for _, method := range requiredMethods {
			assert.Contains(t, contentStr, method, "User entity should have method: %s", method)
		}

		// Check for proper struct definition
		assert.Contains(t, contentStr, "type User struct")
		assert.Contains(t, contentStr, "package domain")
	})

	t.Run("command handlers have required structure", func(t *testing.T) {
		handlersPath := filepath.Join(tempDir, "internal/application/user_command_handlers.go")
		content, err := os.ReadFile(handlersPath)
		require.NoError(t, err)

		contentStr := string(content)

		// Check for required handler methods
		requiredHandlers := []string{
			"type CreateUserHandler struct",
			"func (h *CreateUserHandler) Handle",
			"type UpdateUserHandler struct",
			"func (h *UpdateUserHandler) Handle",
			"type DeleteUserHandler struct",
			"func (h *DeleteUserHandler) Handle",
		}

		for _, handler := range requiredHandlers {
			assert.Contains(t, contentStr, handler, "Command handlers should have: %s", handler)
		}

		assert.Contains(t, contentStr, "package application")
	})

	t.Run("repository interfaces are properly defined", func(t *testing.T) {
		repoInterfacePath := filepath.Join(tempDir, "internal/domain/user_repository.go")
		content, err := os.ReadFile(repoInterfacePath)
		require.NoError(t, err)

		contentStr := string(content)

		// Check for repository interface
		assert.Contains(t, contentStr, "type UserRepository interface")
		assert.Contains(t, contentStr, "Save(ctx context.Context, user *User) error")
		assert.Contains(t, contentStr, "Load(ctx context.Context, id string) (*User, error)")
		assert.Contains(t, contentStr, "package domain")
	})

	t.Run("events are properly structured", func(t *testing.T) {
		eventsPath := filepath.Join(tempDir, "internal/domain/user_events.go")
		content, err := os.ReadFile(eventsPath)
		require.NoError(t, err)

		contentStr := string(content)

		// Check for event structures
		requiredEvents := []string{
			"type UserCreatedEvent struct",
			"func NewUserCreatedEvent(",
			"type UserUpdatedEvent struct",
			"func NewUserUpdatedEvent(",
			"type UserDeletedEvent struct",
			"func NewUserDeletedEvent(",
		}

		for _, event := range requiredEvents {
			assert.Contains(t, contentStr, event, "Events should have: %s", event)
		}

		assert.Contains(t, contentStr, "package domain")
	})
}

// TestGeneratedCode_ImportValidation tests that generated code has correct imports
func TestGeneratedCode_ImportValidation(t *testing.T) {
	logger := NewTestLogger()
	tempDir := t.TempDir()

	// Generate code to validate
	generator := NewCodeGenerator(logger)
	err := generator.Generate("testdata/user-service.yaml", "openapi", tempDir, false)
	require.NoError(t, err)

	t.Run("domain files have correct imports", func(t *testing.T) {
		userEntityPath := filepath.Join(tempDir, "internal/domain/user.go")

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, userEntityPath, nil, parser.ParseComments)
		require.NoError(t, err)

		// Check imports
		imports := make(map[string]bool)
		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			imports[importPath] = true
		}

		// Required imports for domain entities
		requiredImports := []string{
			"github.com/google/uuid",
			"github.com/akeemphilbert/pericarp/pkg/domain",
		}

		for _, reqImport := range requiredImports {
			assert.True(t, imports[reqImport], "Domain entity should import: %s", reqImport)
		}
	})

	t.Run("application files have correct imports", func(t *testing.T) {
		handlersPath := filepath.Join(tempDir, "internal/application/user_command_handlers.go")

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, handlersPath, nil, parser.ParseComments)
		require.NoError(t, err)

		// Check imports
		imports := make(map[string]bool)
		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			imports[importPath] = true
		}

		// Required imports for application handlers
		requiredImports := []string{
			"context",
			"github.com/google/uuid",
		}

		for _, reqImport := range requiredImports {
			assert.True(t, imports[reqImport], "Application handlers should import: %s", reqImport)
		}
	})
}

// TestGeneratedCode_TestFileValidation tests that generated test files are valid
func TestGeneratedCode_TestFileValidation(t *testing.T) {
	logger := NewTestLogger()
	tempDir := t.TempDir()

	// Generate code to validate
	generator := NewCodeGenerator(logger)
	err := generator.Generate("testdata/user-service.yaml", "openapi", tempDir, false)
	require.NoError(t, err)

	t.Run("entity test files are valid", func(t *testing.T) {
		testPath := filepath.Join(tempDir, "internal/domain/user_test.go")
		content, err := os.ReadFile(testPath)
		require.NoError(t, err)

		contentStr := string(content)

		// Check for test functions
		requiredTests := []string{
			"func TestNewUser(t *testing.T)",
			"func TestUser_LoadFromHistory(t *testing.T)",
			"func TestUser_MarkEventsAsCommitted(t *testing.T)",
		}

		for _, test := range requiredTests {
			assert.Contains(t, contentStr, test, "Entity tests should have: %s", test)
		}

		// Check for proper test structure
		assert.Contains(t, contentStr, "package domain")
		assert.Contains(t, contentStr, `"testing"`)
		assert.Contains(t, contentStr, "assert.NoError(t, err)")
		assert.Contains(t, contentStr, "require.NoError(t, err)")
	})

	t.Run("handler test files are valid", func(t *testing.T) {
		testPath := filepath.Join(tempDir, "internal/application/user_handlers_test.go")
		content, err := os.ReadFile(testPath)
		require.NoError(t, err)

		contentStr := string(content)

		// Check for handler tests
		requiredTests := []string{
			"func TestCreateUserHandler_Handle(t *testing.T)",
			"func TestUpdateUserHandler_Handle(t *testing.T)",
			"func TestDeleteUserHandler_Handle(t *testing.T)",
		}

		for _, test := range requiredTests {
			assert.Contains(t, contentStr, test, "Handler tests should have: %s", test)
		}

		// Check for mock usage
		assert.Contains(t, contentStr, "type MockUserRepository struct")
		assert.Contains(t, contentStr, "mock.Mock")
		assert.Contains(t, contentStr, "AssertExpectations(t)")
	})

	t.Run("repository test files are valid", func(t *testing.T) {
		testPath := filepath.Join(tempDir, "internal/infrastructure/user_repository_test.go")
		content, err := os.ReadFile(testPath)
		require.NoError(t, err)

		contentStr := string(content)

		// Check for repository tests
		requiredTests := []string{
			"func TestUserRepository_Save(t *testing.T)",
			"func TestUserRepository_Load(t *testing.T)",
			"func TestUserRepository_Load_NotFound(t *testing.T)",
		}

		for _, test := range requiredTests {
			assert.Contains(t, contentStr, test, "Repository tests should have: %s", test)
		}

		// Check for integration test setup
		assert.Contains(t, contentStr, "if testing.Short()")
		assert.Contains(t, contentStr, `t.Skip("Skipping integration test in short mode")`)
	})
}

// TestGeneratedCode_MakefileValidation tests that generated Makefile works correctly
func TestGeneratedCode_MakefileValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Makefile validation test in short mode")
	}

	logger := NewTestLogger()
	tempDir := t.TempDir()

	// Create a test project
	projectName := "makefile-test-project"
	projectDir := filepath.Join(tempDir, projectName)

	creator := NewProjectCreator(logger)
	err := creator.CreateProject(projectName, "", projectDir, false)
	require.NoError(t, err)

	// Generate code
	generator := NewCodeGenerator(logger)
	err = generator.Generate("testdata/user-service.yaml", "openapi", projectDir, false)
	require.NoError(t, err)

	// Test Makefile targets
	makeTargets := []struct {
		name        string
		target      string
		shouldPass  bool
		description string
	}{
		{
			name:        "deps target",
			target:      "deps",
			shouldPass:  true,
			description: "Should install dependencies",
		},
		{
			name:        "build target",
			target:      "build",
			shouldPass:  true,
			description: "Should build the application",
		},
		{
			name:        "test target",
			target:      "test",
			shouldPass:  true,
			description: "Should run tests",
		},
		{
			name:        "fmt target",
			target:      "fmt",
			shouldPass:  true,
			description: "Should format code",
		},
		{
			name:        "clean target",
			target:      "clean",
			shouldPass:  true,
			description: "Should clean build artifacts",
		},
	}

	for _, mt := range makeTargets {
		t.Run(mt.name, func(t *testing.T) {
			cmd := exec.Command("make", mt.target)
			cmd.Dir = projectDir
			output, err := cmd.CombinedOutput()

			if mt.shouldPass {
				if err != nil {
					t.Logf("Make %s output: %s", mt.target, string(output))
				}
				assert.NoError(t, err, "%s: %s", mt.name, mt.description)
			} else {
				assert.Error(t, err, "%s: %s", mt.name, mt.description)
			}
		})
	}

	// Test that help target shows all targets
	t.Run("help target shows available targets", func(t *testing.T) {
		cmd := exec.Command("make", "help")
		cmd.Dir = projectDir
		output, err := cmd.CombinedOutput()

		assert.NoError(t, err, "Make help should succeed")

		outputStr := string(output)
		expectedTargets := []string{"deps", "build", "test", "clean", "fmt", "lint", "gosec"}

		for _, target := range expectedTargets {
			assert.Contains(t, outputStr, target, "Help should show target: %s", target)
		}
	})
}

// TestGeneratedCode_GoModValidation tests that generated go.mod is valid
func TestGeneratedCode_GoModValidation(t *testing.T) {
	logger := NewTestLogger()
	tempDir := t.TempDir()

	// Create a test project
	projectName := "gomod-test-project"
	projectDir := filepath.Join(tempDir, projectName)

	creator := NewProjectCreator(logger)
	err := creator.CreateProject(projectName, "", projectDir, false)
	require.NoError(t, err)

	t.Run("go.mod has correct module name", func(t *testing.T) {
		goModPath := filepath.Join(projectDir, "go.mod")
		content, err := os.ReadFile(goModPath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, fmt.Sprintf("module %s", projectName))
		assert.Contains(t, contentStr, "go 1.21")
	})

	t.Run("go.mod has required dependencies", func(t *testing.T) {
		goModPath := filepath.Join(projectDir, "go.mod")
		content, err := os.ReadFile(goModPath)
		require.NoError(t, err)

		contentStr := string(content)

		// Check for Pericarp dependency
		assert.Contains(t, contentStr, "github.com/akeemphilbert/pericarp")

		// Check for other required dependencies
		requiredDeps := []string{
			"github.com/google/uuid",
			"github.com/stretchr/testify",
		}

		for _, dep := range requiredDeps {
			assert.Contains(t, contentStr, dep, "go.mod should include dependency: %s", dep)
		}
	})

	t.Run("go mod verify succeeds", func(t *testing.T) {
		cmd := exec.Command("go", "mod", "verify")
		cmd.Dir = projectDir
		output, err := cmd.CombinedOutput()

		if err != nil {
			t.Logf("Go mod verify output: %s", string(output))
		}
		assert.NoError(t, err, "go mod verify should succeed")
	})
}

// TestGeneratedCode_ReadmeValidation tests that generated README is comprehensive
func TestGeneratedCode_ReadmeValidation(t *testing.T) {
	logger := NewTestLogger()
	tempDir := t.TempDir()

	// Create a test project
	projectName := "readme-test-project"
	projectDir := filepath.Join(tempDir, projectName)

	creator := NewProjectCreator(logger)
	err := creator.CreateProject(projectName, "", projectDir, false)
	require.NoError(t, err)

	// Generate code to get entities in README
	generator := NewCodeGenerator(logger)
	err = generator.Generate("testdata/user-service.yaml", "openapi", projectDir, false)
	require.NoError(t, err)

	t.Run("README has project information", func(t *testing.T) {
		readmePath := filepath.Join(projectDir, "README.md")
		content, err := os.ReadFile(readmePath)
		require.NoError(t, err)

		contentStr := string(content)

		// Check for basic project information
		expectedSections := []string{
			"# Readme-Test-Project",
			"## Overview",
			"## Architecture",
			"## Getting Started",
			"## Development",
			"## Testing",
			"## Domain Entities",
		}

		for _, section := range expectedSections {
			assert.Contains(t, contentStr, section, "README should contain section: %s", section)
		}

		// Check for entity documentation
		assert.Contains(t, contentStr, "### User")
		assert.Contains(t, contentStr, "### Profile")
		assert.Contains(t, contentStr, "### Address")

		// Check for DDD concepts
		assert.Contains(t, contentStr, "Domain-Driven Design")
		assert.Contains(t, contentStr, "Event Sourcing")
		assert.Contains(t, contentStr, "CQRS")
	})
}

// TestGeneratedCode_MultipleFormatsValidation tests validation across different input formats
func TestGeneratedCode_MultipleFormatsValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multiple formats validation test in short mode")
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
		t.Run(fmt.Sprintf("%s format validation", format.name), func(t *testing.T) {
			// Create separate project for each format
			projectName := fmt.Sprintf("validation-%s-project", strings.ToLower(format.name))
			projectDir := filepath.Join(tempDir, projectName)

			// Create project
			creator := NewProjectCreator(logger)
			err := creator.CreateProject(projectName, "", projectDir, false)
			require.NoError(t, err)

			// Generate code
			generator := NewCodeGenerator(logger)
			err = generator.Generate(format.inputFile, format.inputType, projectDir, false)
			require.NoError(t, err)

			// Test compilation
			cmd := exec.Command("go", "build", "./...")
			cmd.Dir = projectDir
			output, err := cmd.CombinedOutput()

			if err != nil {
				t.Logf("Compilation output for %s: %s", format.name, string(output))
			}
			assert.NoError(t, err, "Generated code from %s should compile", format.name)

			// Test that tests run
			cmd = exec.Command("go", "test", "./...")
			cmd.Dir = projectDir
			output, err = cmd.CombinedOutput()

			if err != nil {
				t.Logf("Test output for %s: %s", format.name, string(output))
			}
			assert.NoError(t, err, "Generated tests from %s should run", format.name)
		})
	}
}

// Benchmark tests for generated code validation
func BenchmarkGeneratedCode_CompilationTime(b *testing.B) {
	logger := NewTestLogger()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()
		projectName := fmt.Sprintf("benchmark-compile-%d", i)
		projectDir := filepath.Join(tempDir, projectName)

		// Create and generate project
		creator := NewProjectCreator(logger)
		err := creator.CreateProject(projectName, "", projectDir, false)
		if err != nil {
			b.Fatal(err)
		}

		generator := NewCodeGenerator(logger)
		err = generator.Generate("testdata/user-service.yaml", "openapi", projectDir, false)
		if err != nil {
			b.Fatal(err)
		}

		// Measure compilation time
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = projectDir
		err = cmd.Run()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGeneratedCode_TestExecution(b *testing.B) {
	logger := NewTestLogger()
	tempDir := b.TempDir()
	projectName := "benchmark-test-project"
	projectDir := filepath.Join(tempDir, projectName)

	// Setup once
	creator := NewProjectCreator(logger)
	err := creator.CreateProject(projectName, "", projectDir, false)
	if err != nil {
		b.Fatal(err)
	}

	generator := NewCodeGenerator(logger)
	err = generator.Generate("testdata/user-service.yaml", "openapi", projectDir, false)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command("go", "test", "./...")
		cmd.Dir = projectDir
		err := cmd.Run()
		if err != nil {
			b.Fatal(err)
		}
	}
}
