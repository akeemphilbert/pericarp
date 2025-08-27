package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProtoParser(t *testing.T) {
	parser := NewProtoParser()
	assert.NotNil(t, parser)
	assert.IsType(t, &ProtoParser{}, parser)
}

func TestProtoParser_SupportedExtensions(t *testing.T) {
	parser := NewProtoParser()
	extensions := parser.SupportedExtensions()

	assert.Equal(t, []string{".proto"}, extensions)
}

func TestProtoParser_FormatName(t *testing.T) {
	parser := NewProtoParser()
	formatName := parser.FormatName()

	assert.Equal(t, "Protocol Buffers", formatName)
}

func TestProtoParser_Validate(t *testing.T) {
	parser := NewProtoParser()

	tests := []struct {
		name      string
		filePath  string
		wantError bool
		errorType ErrorType
	}{
		{
			name:      "valid proto file",
			filePath:  "testdata/simple.proto",
			wantError: false,
		},
		{
			name:      "empty file path",
			filePath:  "",
			wantError: true,
			errorType: ValidationError,
		},
		{
			name:      "non-existent file",
			filePath:  "testdata/nonexistent.proto",
			wantError: true,
			errorType: FileSystemError,
		},
		{
			name:      "wrong file extension",
			filePath:  "testdata/user-service.yaml",
			wantError: true,
			errorType: ValidationError,
		},
		{
			name:      "invalid proto syntax",
			filePath:  "testdata/invalid.proto",
			wantError: true,
			errorType: ParseError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.Validate(tt.filePath)

			if tt.wantError {
				assert.Error(t, err)
				if cliErr, ok := err.(*CliError); ok {
					assert.Equal(t, tt.errorType, cliErr.Type)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProtoParser_Parse_SimpleMessage(t *testing.T) {
	parser := NewProtoParser()

	model, err := parser.Parse("testdata/simple.proto")
	require.NoError(t, err)
	require.NotNil(t, model)

	// Check basic model properties
	assert.Equal(t, "simple", model.ProjectName)
	assert.Len(t, model.Entities, 1)

	// Check entity properties
	entity := model.Entities[0]
	assert.Equal(t, "Product", entity.Name)
	assert.Len(t, entity.Properties, 4)

	// Check properties
	expectedProperties := map[string]struct {
		Type     string
		Required bool
	}{
		"Id":        {"string", true},
		"Name":      {"string", true},
		"Price":     {"float64", true},
		"Available": {"bool", true},
	}

	for _, prop := range entity.Properties {
		expected, exists := expectedProperties[prop.Name]
		assert.True(t, exists, "Unexpected property: %s", prop.Name)
		assert.Equal(t, expected.Type, prop.Type, "Wrong type for property %s", prop.Name)
		assert.Equal(t, expected.Required, prop.Required, "Wrong required flag for property %s", prop.Name)

		// Check tags
		assert.NotEmpty(t, prop.Tags["json"])
		assert.NotEmpty(t, prop.Tags["protobuf"])
	}

	// Check metadata
	assert.Equal(t, "protobuf", model.Metadata["source_format"])
	assert.Equal(t, "testdata/simple.proto", model.Metadata["source_file"])
	assert.Equal(t, "simple", model.Metadata["package"])
}

func TestProtoParser_Parse_ComplexMessage(t *testing.T) {
	parser := NewProtoParser()

	model, err := parser.Parse("testdata/user.proto")
	require.NoError(t, err)
	require.NotNil(t, model)

	// Should have User and Profile entities (excluding request/response messages)
	assert.Len(t, model.Entities, 2)

	// Find User entity
	var userEntity *Entity
	var profileEntity *Entity
	for i := range model.Entities {
		if model.Entities[i].Name == "User" {
			userEntity = &model.Entities[i]
		} else if model.Entities[i].Name == "Profile" {
			profileEntity = &model.Entities[i]
		}
	}

	require.NotNil(t, userEntity, "User entity should be present")
	require.NotNil(t, profileEntity, "Profile entity should be present")

	// Check User entity properties
	assert.True(t, len(userEntity.Properties) > 5, "User should have multiple properties")

	// Check for specific properties
	propertyNames := make(map[string]bool)
	for _, prop := range userEntity.Properties {
		propertyNames[prop.Name] = true
	}

	assert.True(t, propertyNames["Id"], "User should have Id property")
	assert.True(t, propertyNames["Email"], "User should have Email property")
	assert.True(t, propertyNames["Name"], "User should have Name property")
	assert.True(t, propertyNames["Profile"], "User should have Profile property")
	assert.True(t, propertyNames["Roles"], "User should have Roles property")

	// Check for repeated field (roles should be []string)
	for _, prop := range userEntity.Properties {
		if prop.Name == "Roles" {
			assert.Equal(t, "[]string", prop.Type, "Roles should be a slice of strings")
		}
	}

	// Check relations
	assert.True(t, len(model.Relations) > 0, "Should have relations between entities")

	// Check for User -> Profile relation
	hasUserProfileRelation := false
	for _, rel := range model.Relations {
		if rel.From == "User" && rel.To == "Profile" {
			hasUserProfileRelation = true
			assert.Equal(t, OneToOne, rel.Type)
		}
	}
	assert.True(t, hasUserProfileRelation, "Should have User -> Profile relation")
}

func TestProtoParser_Parse_OnlyRequestResponse(t *testing.T) {
	parser := NewProtoParser()

	// This should fail because there are no domain entities, only request/response messages
	_, err := parser.Parse("testdata/empty.proto")
	require.Error(t, err)

	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, ParseError, cliErr.Type)
	assert.Contains(t, cliErr.Message, "no valid domain entities found")
}

func TestProtoParser_Parse_InvalidFile(t *testing.T) {
	parser := NewProtoParser()

	tests := []struct {
		name      string
		filePath  string
		errorType ErrorType
	}{
		{
			name:      "non-existent file",
			filePath:  "testdata/nonexistent.proto",
			errorType: FileSystemError,
		},
		{
			name:      "invalid proto syntax",
			filePath:  "testdata/invalid.proto",
			errorType: ParseError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.filePath)
			require.Error(t, err)

			cliErr, ok := err.(*CliError)
			require.True(t, ok)
			assert.Equal(t, tt.errorType, cliErr.Type)
		})
	}
}

func TestProtoParser_convertFieldName(t *testing.T) {
	parser := &ProtoParser{}

	tests := []struct {
		input    string
		expected string
	}{
		{"user_id", "UserId"},
		{"first_name", "FirstName"},
		{"is_active", "IsActive"},
		{"created_at", "CreatedAt"},
		{"simple", "Simple"},
		{"", ""},
		{"a_b_c_d", "ABCD"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parser.convertFieldName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProtoParser_isRequestResponseMessage(t *testing.T) {
	parser := &ProtoParser{}

	tests := []struct {
		messageName string
		expected    bool
	}{
		{"CreateUserRequest", true},
		{"CreateUserResponse", true},
		{"GetUserReq", true},
		{"GetUserResp", true},
		{"User", false},
		{"Profile", false},
		{"UserData", false},
		{"RequestData", false}, // This should be false as it doesn't end with request
		{"DataRequest", true},
		{"DataResponse", true},
	}

	for _, tt := range tests {
		t.Run(tt.messageName, func(t *testing.T) {
			result := parser.isRequestResponseMessage(tt.messageName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProtoParser_extractProjectName(t *testing.T) {
	parser := NewProtoParser()

	// Test with actual proto file
	model, err := parser.Parse("testdata/user.proto")
	require.NoError(t, err)

	// Should extract from go_package option or package name
	assert.NotEmpty(t, model.ProjectName)

	// Test with simple proto file
	model2, err := parser.Parse("testdata/simple.proto")
	require.NoError(t, err)
	assert.Equal(t, "simple", model2.ProjectName)
}

func TestProtoParser_Integration_WithRegistry(t *testing.T) {
	// Test that the proto parser works with the registry
	registry := NewParserRegistry()
	parser := NewProtoParser()

	err := registry.RegisterParser(parser)
	require.NoError(t, err)

	// Test getting parser by file extension
	retrievedParser, err := registry.GetParser("test.proto")
	require.NoError(t, err)
	assert.Equal(t, parser.FormatName(), retrievedParser.FormatName())

	// Test format detection
	format, err := registry.DetectFormat("testdata/simple.proto")
	require.NoError(t, err)
	assert.Equal(t, "Protocol Buffers", format)
}

func TestProtoParser_ErrorHandling(t *testing.T) {
	parser := NewProtoParser()

	// Test with empty file path
	_, err := parser.Parse("")
	require.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, ValidationError, cliErr.Type)

	// Test with non-proto file
	_, err = parser.Parse("testdata/user-service.yaml")
	require.Error(t, err)
	cliErr, ok = err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, ValidationError, cliErr.Type)
}

func TestProtoParser_MapProtoTypes(t *testing.T) {
	// This is tested indirectly through the Parse tests, but we can add specific tests
	// for edge cases if needed. The type mapping is covered by the integration tests above.
	parser := NewProtoParser()

	// Test parsing a file with various types
	model, err := parser.Parse("testdata/user.proto")
	require.NoError(t, err)

	// Find User entity and check type mappings
	var userEntity *Entity
	for i := range model.Entities {
		if model.Entities[i].Name == "User" {
			userEntity = &model.Entities[i]
			break
		}
	}
	require.NotNil(t, userEntity)

	// Check specific type mappings
	typeMap := make(map[string]string)
	for _, prop := range userEntity.Properties {
		typeMap[prop.Name] = prop.Type
	}

	assert.Equal(t, "string", typeMap["Id"])
	assert.Equal(t, "string", typeMap["Email"])
	assert.Equal(t, "bool", typeMap["IsActive"])
	assert.Equal(t, "int32", typeMap["Age"])
	assert.Equal(t, "[]string", typeMap["Roles"])
	assert.Equal(t, "int64", typeMap["CreatedAt"])
}

// TestProtoParser_FileNotFound tests error handling for missing files
func TestProtoParser_FileNotFound(t *testing.T) {
	parser := NewProtoParser()

	_, err := parser.Parse("nonexistent.proto")
	require.Error(t, err)

	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, FileSystemError, cliErr.Type)
	assert.Contains(t, cliErr.Message, "does not exist")
}

// TestProtoParser_ValidateContent tests the content validation
func TestProtoParser_ValidateContent(t *testing.T) {
	parser := &ProtoParser{}

	// Test valid content
	err := parser.validateContent("testdata/simple.proto")
	assert.NoError(t, err)

	// Test invalid content
	err = parser.validateContent("testdata/invalid.proto")
	assert.Error(t, err)

	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, ParseError, cliErr.Type)
}

// Benchmark tests for performance
func BenchmarkProtoParser_Parse(b *testing.B) {
	parser := NewProtoParser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parser.Parse("testdata/user.proto")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProtoParser_Validate(b *testing.B) {
	parser := NewProtoParser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := parser.Validate("testdata/user.proto")
		if err != nil {
			b.Fatal(err)
		}
	}
}
