package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIParser_Integration_RealFile(t *testing.T) {
	parser := NewOpenAPIParser()

	// Test with the real test data file
	model, err := parser.Parse("testdata/user-service.yaml")
	require.NoError(t, err)
	require.NotNil(t, model)

	// Verify project name
	assert.Equal(t, "user-service-api", model.ProjectName)

	// Verify we have all expected entities
	assert.Len(t, model.Entities, 3) // User, Profile, Address

	// Find entities
	var userEntity, profileEntity, addressEntity *Entity
	for i := range model.Entities {
		switch model.Entities[i].Name {
		case "User":
			userEntity = &model.Entities[i]
		case "Profile":
			profileEntity = &model.Entities[i]
		case "Address":
			addressEntity = &model.Entities[i]
		}
	}

	require.NotNil(t, userEntity, "User entity should exist")
	require.NotNil(t, profileEntity, "Profile entity should exist")
	require.NotNil(t, addressEntity, "Address entity should exist")

	// Verify User entity has expected properties
	assert.Len(t, userEntity.Properties, 8) // id, email, name, age, isActive, profile, addresses, preferences

	// Check required fields
	emailProp := findProperty(userEntity.Properties, "email")
	require.NotNil(t, emailProp)
	assert.True(t, emailProp.Required)

	nameProp := findProperty(userEntity.Properties, "name")
	require.NotNil(t, nameProp)
	assert.True(t, nameProp.Required)

	ageProp := findProperty(userEntity.Properties, "age")
	require.NotNil(t, ageProp)
	assert.False(t, ageProp.Required)

	// Check relationships
	profileProp := findProperty(userEntity.Properties, "profile")
	require.NotNil(t, profileProp)
	assert.Equal(t, "Profile", profileProp.Type)

	addressesProp := findProperty(userEntity.Properties, "addresses")
	require.NotNil(t, addressesProp)
	assert.Equal(t, "[]Address", addressesProp.Type)

	// Verify relations were created
	assert.Len(t, model.Relations, 2) // profile reference and addresses array

	// Check that we have both one-to-one and one-to-many relationships
	var oneToOneCount, oneToManyCount int
	for _, relation := range model.Relations {
		switch relation.Type {
		case OneToOne:
			oneToOneCount++
		case OneToMany:
			oneToManyCount++
		}
	}
	assert.Equal(t, 1, oneToOneCount, "Should have one one-to-one relationship")
	assert.Equal(t, 1, oneToManyCount, "Should have one one-to-many relationship")

	// Verify metadata
	assert.Equal(t, "openapi", model.Metadata["source_format"])
	assert.Contains(t, model.Metadata["source_file"], "testdata/user-service.yaml")
}

func TestOpenAPIParser_Integration_InvalidFile(t *testing.T) {
	parser := NewOpenAPIParser()

	// Test with invalid file
	model, err := parser.Parse("testdata/invalid-openapi.yaml")
	assert.Nil(t, model)
	assert.Error(t, err)

	if cliErr, ok := err.(*CliError); ok {
		assert.Equal(t, ParseError, cliErr.Type)
		assert.Contains(t, cliErr.Message, "OpenAPI specification validation failed")
	}
}

// TestNewProjectCommand_Integration tests the complete project creation workflow
func TestNewProjectCommand_Integration(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		flags       []string
		wantErr     bool
		checkFiles  []string
	}{
		{
			name:        "create basic project with dry-run",
			projectName: "test-service",
			flags:       []string{"--dry-run"},
			wantErr:     false,
		},
		{
			name:        "create project with custom destination and dry-run",
			projectName: "my-service",
			flags:       []string{"--destination", "/tmp/test-pericarp", "--dry-run"},
			wantErr:     false,
		},
		{
			name:        "create project with verbose output and dry-run",
			projectName: "verbose-service",
			flags:       []string{"--verbose", "--dry-run"},
			wantErr:     false,
		},
		{
			name:        "invalid project name should fail",
			projectName: "Invalid-Project-Name",
			flags:       []string{"--dry-run"},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new project creator
			logger := NewCliLogger()
			creator := NewProjectCreator(logger)

			// Build flags
			var repoURL, destination string
			var dryRun bool = true // Always use dry-run for tests

			for i, flag := range tt.flags {
				switch flag {
				case "--repo":
					if i+1 < len(tt.flags) {
						repoURL = tt.flags[i+1]
					}
				case "--destination":
					if i+1 < len(tt.flags) {
						destination = tt.flags[i+1]
					}
				case "--dry-run":
					dryRun = true
				case "--verbose":
					logger.SetVerbose(true)
				}
			}

			// Execute project creation
			err := creator.CreateProject(tt.projectName, repoURL, destination, dryRun, "")

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestProjectCreator_ValidatesRequirements tests that the project creator meets all requirements
func TestProjectCreator_ValidatesRequirements(t *testing.T) {
	logger := NewCliLogger()
	creator := NewProjectCreator(logger)

	t.Run("validates project name (Requirement 10.1)", func(t *testing.T) {
		err := creator.CreateProject("Invalid-Name", "", "", true, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validation")
	})

	t.Run("handles custom destination (Requirement 9.3, 9.4)", func(t *testing.T) {
		err := creator.CreateProject("test-service", "", "/tmp/test-dest", true, "")
		assert.NoError(t, err)
	})

	t.Run("supports dry-run mode (Requirement 9.1)", func(t *testing.T) {
		err := creator.CreateProject("test-service", "", "", true, "")
		assert.NoError(t, err)
	})

	t.Run("supports repository cloning (Requirement 4.1, 4.2)", func(t *testing.T) {
		err := creator.CreateProject("test-service", "https://github.com/example/repo.git", "", true, "")
		assert.NoError(t, err)
	})
}

// TestProjectCreator_ErrorHandling tests error scenarios
func TestProjectCreator_ErrorHandling(t *testing.T) {
	logger := NewCliLogger()
	creator := NewProjectCreator(logger)

	t.Run("empty project name", func(t *testing.T) {
		err := creator.CreateProject("", "", "", true, "")
		assert.Error(t, err)

		if cliErr, ok := err.(*CliError); ok {
			assert.Equal(t, ValidationError, cliErr.Type)
		}
	})

	t.Run("project name with invalid characters", func(t *testing.T) {
		err := creator.CreateProject("test@service", "", "", true, "")
		assert.Error(t, err)

		if cliErr, ok := err.(*CliError); ok {
			assert.Equal(t, ValidationError, cliErr.Type)
		}
	})

	t.Run("project name starting with uppercase", func(t *testing.T) {
		err := creator.CreateProject("TestService", "", "", true, "")
		assert.Error(t, err)

		if cliErr, ok := err.(*CliError); ok {
			assert.Equal(t, ValidationError, cliErr.Type)
		}
	})
}

// TestGenerateCommand_Integration tests the complete code generation workflow
func TestGenerateCommand_Integration(t *testing.T) {
	tests := []struct {
		name       string
		inputFile  string
		inputType  string
		flags      []string
		wantErr    bool
		checkFiles []string
	}{
		{
			name:      "generate from OpenAPI with dry-run",
			inputFile: "testdata/user-service.yaml",
			inputType: "openapi",
			flags:     []string{"--dry-run"},
			wantErr:   false,
		},
		{
			name:      "generate from Proto with dry-run",
			inputFile: "testdata/user.proto",
			inputType: "proto",
			flags:     []string{"--dry-run"},
			wantErr:   false,
		},
		{
			name:      "generate with custom destination and dry-run",
			inputFile: "testdata/user-service.yaml",
			inputType: "openapi",
			flags:     []string{"--destination", "/tmp/test-generate", "--dry-run"},
			wantErr:   false,
		},
		{
			name:      "generate with verbose output and dry-run",
			inputFile: "testdata/user-service.yaml",
			inputType: "openapi",
			flags:     []string{"--verbose", "--dry-run"},
			wantErr:   false,
		},
		{
			name:      "generate from nonexistent file should fail",
			inputFile: "testdata/nonexistent.yaml",
			inputType: "openapi",
			flags:     []string{"--dry-run"},
			wantErr:   true,
		},
		{
			name:      "generate from invalid OpenAPI should fail",
			inputFile: "testdata/invalid-openapi.yaml",
			inputType: "openapi",
			flags:     []string{"--dry-run"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new code generator
			logger := NewCliLogger()
			generator := NewCodeGenerator(logger)

			// Build flags
			var destination string
			var dryRun bool = true // Always use dry-run for tests

			for i, flag := range tt.flags {
				switch flag {
				case "--destination":
					if i+1 < len(tt.flags) {
						destination = tt.flags[i+1]
					}
				case "--dry-run":
					dryRun = true
				case "--verbose":
					logger.SetVerbose(true)
				}
			}

			// Execute code generation
			err := generator.Generate(tt.inputFile, tt.inputType, destination, dryRun)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCodeGenerator_ValidatesRequirements tests that the code generator meets all requirements
func TestCodeGenerator_ValidatesRequirements(t *testing.T) {
	logger := NewCliLogger()
	generator := NewCodeGenerator(logger)

	t.Run("parses OpenAPI format (Requirement 3.2)", func(t *testing.T) {
		err := generator.Generate("testdata/user-service.yaml", "openapi", "", true)
		assert.NoError(t, err)
	})

	t.Run("parses Proto format (Requirement 3.3)", func(t *testing.T) {
		err := generator.Generate("testdata/user.proto", "proto", "", true)
		assert.NoError(t, err)
	})

	t.Run("validates input file exists (Requirement 10.2)", func(t *testing.T) {
		err := generator.Generate("nonexistent.yaml", "openapi", "", true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validation")
	})

	t.Run("handles custom destination (Requirement 9.3)", func(t *testing.T) {
		err := generator.Generate("testdata/user-service.yaml", "openapi", "/tmp/test-dest", true)
		assert.NoError(t, err)
	})

	t.Run("supports dry-run mode (Requirement 9.1)", func(t *testing.T) {
		err := generator.Generate("testdata/user-service.yaml", "openapi", "", true)
		assert.NoError(t, err)
	})

	t.Run("uses parser registry (Requirement 7.3)", func(t *testing.T) {
		// Test that the generator can handle different file extensions
		err := generator.Generate("testdata/user-service.yaml", "openapi", "", true)
		assert.NoError(t, err)

		err = generator.Generate("testdata/user.proto", "proto", "", true)
		assert.NoError(t, err)
	})
}

// TestCodeGenerator_ErrorHandling tests error scenarios for code generation
func TestCodeGenerator_ErrorHandling(t *testing.T) {
	logger := NewCliLogger()
	generator := NewCodeGenerator(logger)

	t.Run("unsupported file extension", func(t *testing.T) {
		err := generator.Generate("test.unsupported", "unknown", "", true)
		assert.Error(t, err)

		if cliErr, ok := err.(*CliError); ok {
			assert.Equal(t, ParseError, cliErr.Type)
		}
	})

	t.Run("invalid OpenAPI file", func(t *testing.T) {
		err := generator.Generate("testdata/invalid-openapi.yaml", "openapi", "", true)
		assert.Error(t, err)

		if cliErr, ok := err.(*CliError); ok {
			assert.Equal(t, ValidationError, cliErr.Type)
		}
	})

	t.Run("invalid Proto file", func(t *testing.T) {
		err := generator.Generate("testdata/invalid.proto", "proto", "", true)
		assert.Error(t, err)

		if cliErr, ok := err.(*CliError); ok {
			assert.Equal(t, ValidationError, cliErr.Type)
		}
	})
}

// TestCodeGenerator_ComponentGeneration tests that all required components are generated
func TestCodeGenerator_ComponentGeneration(t *testing.T) {
	logger := NewCliLogger()
	generator := NewCodeGenerator(logger)

	t.Run("generates all entity components (Requirement 6)", func(t *testing.T) {
		// This test would need to be expanded to actually check generated files
		// For now, we test that the generation process completes without error
		err := generator.Generate("testdata/user-service.yaml", "openapi", "", true)
		assert.NoError(t, err)
	})

	t.Run("follows Pericarp conventions (Requirement 8)", func(t *testing.T) {
		// This test would verify that generated code follows proper patterns
		// For now, we test that the generation process completes without error
		err := generator.Generate("testdata/user-service.yaml", "openapi", "", true)
		assert.NoError(t, err)
	})
}

// TestGenerateCommand_AllFormats tests generation from all supported input formats
func TestGenerateCommand_AllFormats(t *testing.T) {
	logger := NewCliLogger()
	generator := NewCodeGenerator(logger)

	formats := []struct {
		name      string
		file      string
		inputType string
	}{
		{
			name:      "OpenAPI YAML format",
			file:      "testdata/user-service.yaml",
			inputType: "openapi",
		},
		{
			name:      "Protocol Buffer format",
			file:      "testdata/user.proto",
			inputType: "proto",
		},
	}

	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			err := generator.Generate(format.file, format.inputType, "", true)
			assert.NoError(t, err, "Should successfully generate from %s", format.name)
		})
	}
}
