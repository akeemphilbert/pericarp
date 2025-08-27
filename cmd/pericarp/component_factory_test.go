package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPericarpComponentFactory(t *testing.T) {
	logger := NewTestLogger()

	factory, err := NewPericarpComponentFactory(logger)

	assert.NoError(t, err)
	assert.NotNil(t, factory)
	assert.NotNil(t, factory.templateEngine)
	assert.Equal(t, logger, factory.logger)
}

func TestPericarpComponentFactory_GenerateEntity(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "User",
		Properties: []Property{
			{Name: "Email", Type: "string", Required: true},
			{Name: "Name", Type: "string", Required: true},
			{Name: "IsActive", Type: "bool", Required: false, DefaultValue: "true"},
		},
	}

	file, err := factory.GenerateEntity(entity)

	assert.NoError(t, err)
	assert.NotNil(t, file)
	assert.Equal(t, "internal/domain/user.go", file.Path)
	assert.Contains(t, file.Content, "type User struct")
	assert.Contains(t, file.Content, "func NewUser(")
	assert.Contains(t, file.Content, "Email string")
	assert.Contains(t, file.Content, "Name string")
	assert.Contains(t, file.Content, "Isactive bool")
	assert.Equal(t, "entity", file.Metadata["type"])
	assert.Equal(t, "User", file.Metadata["entity"])
}

func TestPericarpComponentFactory_GenerateEntity_EnsuresIDField(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "Product",
		Properties: []Property{
			{Name: "Name", Type: "string", Required: true},
		},
	}

	file, err := factory.GenerateEntity(entity)

	assert.NoError(t, err)
	assert.Contains(t, file.Content, "Id uuid.UUID")
}

func TestPericarpComponentFactory_GenerateRepository(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "User",
		Properties: []Property{
			{Name: "Email", Type: "string", Required: true},
		},
	}

	files, err := factory.GenerateRepository(entity)

	assert.NoError(t, err)
	assert.Len(t, files, 2)

	// Check interface file
	interfaceFile := files[0]
	assert.Equal(t, "internal/domain/user_repository.go", interfaceFile.Path)
	assert.Contains(t, interfaceFile.Content, "type UserRepository interface")
	assert.Equal(t, "repository_interface", interfaceFile.Metadata["type"])

	// Check implementation file
	implFile := files[1]
	assert.Equal(t, "internal/infrastructure/user_repository.go", implFile.Path)
	assert.Contains(t, implFile.Content, "UserRepository")
	assert.Equal(t, "repository_implementation", implFile.Metadata["type"])
}

func TestPericarpComponentFactory_GenerateCommands(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "User",
		Properties: []Property{
			{Name: "Email", Type: "string", Required: true},
			{Name: "Name", Type: "string", Required: true},
		},
	}

	files, err := factory.GenerateCommands(entity)

	assert.NoError(t, err)
	assert.Len(t, files, 1)

	file := files[0]
	assert.Equal(t, "internal/application/user_commands.go", file.Path)
	assert.Contains(t, file.Content, "type CreateUserCommand struct")
	assert.Contains(t, file.Content, "type UpdateUserCommand struct")
	assert.Contains(t, file.Content, "type DeleteUserCommand struct")
	assert.Equal(t, "commands", file.Metadata["type"])
}

func TestPericarpComponentFactory_GenerateQueries(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "User",
		Properties: []Property{
			{Name: "Email", Type: "string", Required: true},
		},
	}

	files, err := factory.GenerateQueries(entity)

	assert.NoError(t, err)
	assert.Len(t, files, 1)

	file := files[0]
	assert.Equal(t, "internal/application/user_queries.go", file.Path)
	assert.Contains(t, file.Content, "type GetUserByIdQuery struct")
	assert.Contains(t, file.Content, "type ListUserQuery struct")
	assert.Equal(t, "queries", file.Metadata["type"])
}

func TestPericarpComponentFactory_GenerateEvents(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "User",
		Properties: []Property{
			{Name: "Email", Type: "string", Required: true},
		},
	}

	files, err := factory.GenerateEvents(entity)

	assert.NoError(t, err)
	assert.Len(t, files, 1)

	file := files[0]
	assert.Equal(t, "internal/domain/user_events.go", file.Path)
	assert.Contains(t, file.Content, "type UserCreatedEvent struct")
	assert.Contains(t, file.Content, "func NewUserCreatedEvent")
	assert.Equal(t, "events", file.Metadata["type"])
}

func TestPericarpComponentFactory_GenerateHandlers(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "User",
		Properties: []Property{
			{Name: "Email", Type: "string", Required: true},
		},
	}

	files, err := factory.GenerateHandlers(entity)

	assert.NoError(t, err)
	assert.Len(t, files, 2)

	// Check command handlers
	cmdFile := files[0]
	assert.Equal(t, "internal/application/user_command_handlers.go", cmdFile.Path)
	assert.Contains(t, cmdFile.Content, "type CreateUserHandler struct")
	assert.Contains(t, cmdFile.Content, "type UpdateUserHandler struct")
	assert.Contains(t, cmdFile.Content, "type DeleteUserHandler struct")
	assert.Equal(t, "command_handlers", cmdFile.Metadata["type"])

	// Check query handlers
	queryFile := files[1]
	assert.Equal(t, "internal/application/user_query_handlers.go", queryFile.Path)
	assert.Contains(t, queryFile.Content, "type GetUserByIdHandler struct")
	assert.Contains(t, queryFile.Content, "type ListUserHandler struct")
	assert.Equal(t, "query_handlers", queryFile.Metadata["type"])
}

func TestPericarpComponentFactory_GenerateTests(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "User",
		Properties: []Property{
			{Name: "Email", Type: "string", Required: true},
		},
	}

	files, err := factory.GenerateTests(entity)

	assert.NoError(t, err)
	assert.Len(t, files, 3)

	// Check entity test
	entityTest := files[0]
	assert.Equal(t, "internal/domain/user_test.go", entityTest.Path)
	assert.Contains(t, entityTest.Content, "func TestNewUser(t *testing.T)")
	assert.Equal(t, "entity_test", entityTest.Metadata["type"])

	// Check repository test
	repoTest := files[1]
	assert.Equal(t, "internal/infrastructure/user_repository_test.go", repoTest.Path)
	assert.Contains(t, repoTest.Content, "func TestUserRepository_Save(t *testing.T)")
	assert.Equal(t, "repository_test", repoTest.Metadata["type"])

	// Check handlers test
	handlersTest := files[2]
	assert.Equal(t, "internal/application/user_handlers_test.go", handlersTest.Path)
	assert.Contains(t, handlersTest.Content, "func TestCreateUserHandler_Handle(t *testing.T)")
	assert.Equal(t, "handlers_test", handlersTest.Metadata["type"])
}

func TestPericarpComponentFactory_GenerateProjectStructure(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	// Create temporary directory for testing
	tempDir := t.TempDir()

	model := &DomainModel{
		ProjectName: "test-project",
		Entities: []Entity{
			{Name: "User", Properties: []Property{{Name: "Email", Type: "string", Required: true}}},
		},
	}

	err = factory.GenerateProjectStructure(model, tempDir)

	assert.NoError(t, err)

	// Check that directories were created
	expectedDirs := []string{
		"cmd/test-project",
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
		fullPath := filepath.Join(tempDir, dir)
		assert.DirExists(t, fullPath, "Directory %s should exist", dir)
	}

	// Check that main.go was created
	mainPath := filepath.Join(tempDir, "cmd", "test-project", "main.go")
	assert.FileExists(t, mainPath)

	// Check that go.mod was created
	goModPath := filepath.Join(tempDir, "go.mod")
	assert.FileExists(t, goModPath)

	// Check that README.md was created
	readmePath := filepath.Join(tempDir, "README.md")
	assert.FileExists(t, readmePath)
}

func TestPericarpComponentFactory_GenerateMakefile(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	file, err := factory.GenerateMakefile("test-project")

	assert.NoError(t, err)
	assert.NotNil(t, file)
	assert.Equal(t, "Makefile", file.Path)
	assert.Contains(t, file.Content, "# Generated Makefile for test-project")
	assert.Contains(t, file.Content, "deps: ## Install dependencies")
	assert.Contains(t, file.Content, "test: ## Run all tests")
	assert.Contains(t, file.Content, "build: ## Build the application")
	assert.Contains(t, file.Content, "lint: ## Run linter")
	assert.Contains(t, file.Content, "gosec: ## Run security scan")
	assert.Contains(t, file.Content, "clean: ## Clean build artifacts")
	assert.Equal(t, "makefile", file.Metadata["type"])
	assert.Equal(t, "test-project", file.Metadata["project"])
}

func TestPericarpComponentFactory_filterRequiredProperties(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	properties := []Property{
		{Name: "Id", Type: "uuid.UUID", Required: true},
		{Name: "Email", Type: "string", Required: true},
		{Name: "Name", Type: "string", Required: true},
		{Name: "IsActive", Type: "bool", Required: false},
	}

	filtered := factory.filterRequiredProperties(properties)

	assert.Len(t, filtered, 2) // Should exclude ID and optional properties
	assert.Equal(t, "Email", filtered[0].Name)
	assert.Equal(t, "Name", filtered[1].Name)
}

func TestPericarpComponentFactory_ensureIDField(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	tests := []struct {
		name       string
		entity     Entity
		hasIDAfter bool
	}{
		{
			name: "entity without ID field",
			entity: Entity{
				Name: "User",
				Properties: []Property{
					{Name: "Email", Type: "string", Required: true},
				},
			},
			hasIDAfter: true,
		},
		{
			name: "entity with ID field",
			entity: Entity{
				Name: "User",
				Properties: []Property{
					{Name: "Id", Type: "uuid.UUID", Required: true},
					{Name: "Email", Type: "string", Required: true},
				},
			},
			hasIDAfter: true,
		},
		{
			name: "entity with lowercase id field",
			entity: Entity{
				Name: "User",
				Properties: []Property{
					{Name: "id", Type: "uuid.UUID", Required: true},
					{Name: "Email", Type: "string", Required: true},
				},
			},
			hasIDAfter: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := factory.ensureIDField(tt.entity)

			if tt.hasIDAfter {
				hasID := false
				for _, prop := range result.Properties {
					if strings.ToLower(prop.Name) == "id" {
						hasID = true
						break
					}
				}
				assert.True(t, hasID, "Entity should have an ID field")
			}
		})
	}
}
func TestPericarpComponentFactory_GenerateProjectStructure_FollowsConventions(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	// Create temporary directory for testing
	tempDir := t.TempDir()

	model := &DomainModel{
		ProjectName: "user-service",
		Entities: []Entity{
			{
				Name: "User",
				Properties: []Property{
					{Name: "Email", Type: "string", Required: true},
					{Name: "Name", Type: "string", Required: true},
				},
			},
			{
				Name: "Order",
				Properties: []Property{
					{Name: "UserId", Type: "uuid.UUID", Required: true},
					{Name: "Total", Type: "float64", Required: true},
				},
			},
		},
	}

	err = factory.GenerateProjectStructure(model, tempDir)
	require.NoError(t, err)

	// Verify Pericarp directory structure conventions
	expectedDirs := []string{
		"cmd/user-service",
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
		fullPath := filepath.Join(tempDir, dir)
		assert.DirExists(t, fullPath, "Pericarp convention directory %s should exist", dir)
	}

	// Verify main.go contains proper imports and structure
	mainPath := filepath.Join(tempDir, "cmd", "user-service", "main.go")
	mainContent, err := os.ReadFile(mainPath)
	require.NoError(t, err)

	mainStr := string(mainContent)
	assert.Contains(t, mainStr, "package main")
	assert.Contains(t, mainStr, "github.com/akeemphilbert/pericarp/pkg/application")
	assert.Contains(t, mainStr, "github.com/akeemphilbert/pericarp/pkg/domain")
	assert.Contains(t, mainStr, "github.com/akeemphilbert/pericarp/pkg/infrastructure")
	assert.Contains(t, mainStr, "user-service/internal/application")
	assert.Contains(t, mainStr, "user-service/internal/domain")
	assert.Contains(t, mainStr, "user-service/internal/infrastructure")

	// Verify go.mod has correct module name and dependencies
	goModPath := filepath.Join(tempDir, "go.mod")
	goModContent, err := os.ReadFile(goModPath)
	require.NoError(t, err)

	goModStr := string(goModContent)
	assert.Contains(t, goModStr, "module user-service")
	assert.Contains(t, goModStr, "github.com/akeemphilbert/pericarp")
	assert.Contains(t, goModStr, "github.com/google/uuid")
	assert.Contains(t, goModStr, "github.com/stretchr/testify")

	// Verify README.md contains project information
	readmePath := filepath.Join(tempDir, "README.md")
	readmeContent, err := os.ReadFile(readmePath)
	require.NoError(t, err)

	readmeStr := string(readmeContent)
	assert.Contains(t, readmeStr, "# User-Service")
	assert.Contains(t, readmeStr, "### User")
	assert.Contains(t, readmeStr, "### Order")
	assert.Contains(t, readmeStr, "Domain-Driven Design")
	assert.Contains(t, readmeStr, "Event Sourcing")
	assert.Contains(t, readmeStr, "CQRS")
}

func TestPericarpComponentFactory_GenerateMakefile_AllTargets(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	file, err := factory.GenerateMakefile("test-project")
	require.NoError(t, err)

	content := file.Content

	// Verify all required Makefile targets are present (Requirements 5.1-5.7)
	requiredTargets := []string{
		"deps:", "## Install dependencies",
		"test:", "## Run all tests",
		"build:", "## Build the application",
		"lint:", "## Run linter",
		"gosec:", "## Run security scan",
		"clean:", "## Clean build artifacts",
		"help:", "## Show this help message",
	}

	for _, target := range requiredTargets {
		assert.Contains(t, content, target, "Makefile should contain target: %s", target)
	}

	// Verify additional development targets
	devTargets := []string{
		"test-unit:", "## Run unit tests only",
		"test-integration:", "## Run integration tests",
		"coverage:", "## Generate coverage report",
		"fmt:", "## Format code",
		"dev-setup:", "## Set up development environment",
		"dev-test:", "## Run complete development workflow",
		"validate-architecture:", "## Validate DDD architecture",
		"performance-test:", "## Run performance benchmarks",
	}

	for _, target := range devTargets {
		assert.Contains(t, content, target, "Makefile should contain development target: %s", target)
	}

	// Verify database targets are included when HasDatabase is true
	assert.Contains(t, content, "db-migrate:", "Makefile should contain database migration target")
	assert.Contains(t, content, "db-reset:", "Makefile should contain database reset target")

	// Verify proper Go commands are used
	assert.Contains(t, content, "go mod download")
	assert.Contains(t, content, "go mod tidy")
	assert.Contains(t, content, "go test -v -race")
	assert.Contains(t, content, "go build -v")
	assert.Contains(t, content, "golangci-lint run")
	assert.Contains(t, content, "gosec ./...")
}
func TestPericarpComponentFactory_GenerateTests_ComprehensiveTestSuite(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "Product",
		Properties: []Property{
			{Name: "Name", Type: "string", Required: true},
			{Name: "Price", Type: "float64", Required: true},
			{Name: "Description", Type: "string", Required: false},
			{Name: "IsActive", Type: "bool", Required: false, DefaultValue: "true"},
		},
	}

	files, err := factory.GenerateTests(entity)
	require.NoError(t, err)
	require.Len(t, files, 3)

	// Test entity test file
	entityTest := files[0]
	assert.Equal(t, "internal/domain/product_test.go", entityTest.Path)
	assert.Equal(t, "entity_test", entityTest.Metadata["type"])

	entityContent := entityTest.Content
	assert.Contains(t, entityContent, "package domain")
	assert.Contains(t, entityContent, "func TestNewProduct(t *testing.T)")
	assert.Contains(t, entityContent, "func TestProduct_LoadFromHistory(t *testing.T)")
	assert.Contains(t, entityContent, "func TestProduct_MarkEventsAsCommitted(t *testing.T)")

	// Verify test cases for required properties
	assert.Contains(t, entityContent, "valid product")
	assert.Contains(t, entityContent, "invalid product - empty name")
	assert.Contains(t, entityContent, "invalid product - empty price")

	// Verify test fixtures with proper test data
	assert.Contains(t, entityContent, `"test-name"`)
	assert.Contains(t, entityContent, "0.0") // float64 default

	// Verify proper assertions
	assert.Contains(t, entityContent, "assert.Error(t, err)")
	assert.Contains(t, entityContent, "require.NoError(t, err)")
	assert.Contains(t, entityContent, "assert.Equal(t,")
	assert.Contains(t, entityContent, "assert.Len(t,")

	// Test repository test file
	repoTest := files[1]
	assert.Equal(t, "internal/infrastructure/product_repository_test.go", repoTest.Path)
	assert.Equal(t, "repository_test", repoTest.Metadata["type"])

	repoContent := repoTest.Content
	assert.Contains(t, repoContent, "package infrastructure")
	assert.Contains(t, repoContent, "func TestProductRepository_Save(t *testing.T)")
	assert.Contains(t, repoContent, "func TestProductRepository_Load(t *testing.T)")
	assert.Contains(t, repoContent, "func TestProductRepository_Load_NotFound(t *testing.T)")
	assert.Contains(t, repoContent, "func TestIntegrationProductRepository(t *testing.T)")

	// Verify integration test setup
	assert.Contains(t, repoContent, "if testing.Short()")
	assert.Contains(t, repoContent, "t.Skip(\"Skipping integration test in short mode\")")

	// Test handlers test file
	handlersTest := files[2]
	assert.Equal(t, "internal/application/product_handlers_test.go", handlersTest.Path)
	assert.Equal(t, "handlers_test", handlersTest.Metadata["type"])

	handlersContent := handlersTest.Content
	assert.Contains(t, handlersContent, "package application")
	assert.Contains(t, handlersContent, "func TestCreateProductHandler_Handle(t *testing.T)")
	assert.Contains(t, handlersContent, "func TestUpdateProductHandler_Handle(t *testing.T)")
	assert.Contains(t, handlersContent, "func TestGetProductByIdHandler_Handle(t *testing.T)")

	// Verify mock usage
	assert.Contains(t, handlersContent, "type MockProductRepository struct")
	assert.Contains(t, handlersContent, "type MockLogger struct")
	assert.Contains(t, handlersContent, "mock.Mock")
	assert.Contains(t, handlersContent, "m.Called")
	assert.Contains(t, handlersContent, "mock.AnythingOfType")
	assert.Contains(t, handlersContent, "AssertExpectations(t)")

	// Verify test scenarios
	assert.Contains(t, handlersContent, "successful creation")
	assert.Contains(t, handlersContent, "invalid command")
	assert.Contains(t, handlersContent, "repository save error")
	assert.Contains(t, handlersContent, "product not found")
}

func TestPericarpComponentFactory_GenerateTests_ProperTestFixtures(t *testing.T) {
	logger := NewTestLogger()
	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "User",
		Properties: []Property{
			{Name: "Email", Type: "string", Required: true, Validation: "email"},
			{Name: "Age", Type: "int", Required: true, Validation: "min=18,max=120"},
			{Name: "IsVerified", Type: "bool", Required: false, DefaultValue: "false"},
		},
	}

	files, err := factory.GenerateTests(entity)
	require.NoError(t, err)

	entityTest := files[0]
	content := entityTest.Content

	// Verify test fixtures use appropriate data types
	assert.Contains(t, content, `"test-email"`) // String fixture
	assert.Contains(t, content, "123")          // Int fixture
	assert.Contains(t, content, "false")        // Bool fixture

	// Verify proper test structure with table-driven tests
	assert.Contains(t, content, "tests := []struct {")
	assert.Contains(t, content, "name    string")
	assert.Contains(t, content, "wantErr bool")
	assert.Contains(t, content, "for _, tt := range tests")
	assert.Contains(t, content, "t.Run(tt.name, func(t *testing.T)")
}
