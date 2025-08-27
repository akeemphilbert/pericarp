package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtoParser_RegistryIntegration(t *testing.T) {
	// Test that the Proto parser integrates properly with the registry
	registry := NewParserRegistry()
	protoParser := NewProtoParser()

	// Register the Proto parser
	err := registry.RegisterParser(protoParser)
	require.NoError(t, err)

	// Test getting parser by file extension
	retrievedParser, err := registry.GetParser("test.proto")
	require.NoError(t, err)
	assert.Equal(t, protoParser.FormatName(), retrievedParser.FormatName())

	// Test format detection with actual proto file
	format, err := registry.DetectFormat("testdata/simple.proto")
	require.NoError(t, err)
	assert.Equal(t, "Protocol Buffers", format)

	// Test that proto format is listed
	formats := registry.ListFormats()
	assert.Contains(t, formats, "Protocol Buffers")
}

func TestProtoParser_FullWorkflow(t *testing.T) {
	// Test complete workflow from file to domain model
	registry := NewParserRegistry()

	// Register all parsers
	err := registry.RegisterParser(NewOpenAPIParser())
	require.NoError(t, err)
	err = registry.RegisterParser(NewProtoParser())
	require.NoError(t, err)

	// Test proto file parsing
	parser, err := registry.GetParser("testdata/user.proto")
	require.NoError(t, err)
	assert.Equal(t, "Protocol Buffers", parser.FormatName())

	// Parse the file
	model, err := parser.Parse("testdata/user.proto")
	require.NoError(t, err)
	require.NotNil(t, model)

	// Verify the model structure
	assert.NotEmpty(t, model.ProjectName)
	assert.True(t, len(model.Entities) >= 2, "Should have at least User and Profile entities")
	assert.Equal(t, "protobuf", model.Metadata["source_format"])

	// Verify entities have proper structure
	for _, entity := range model.Entities {
		assert.NotEmpty(t, entity.Name)
		assert.True(t, len(entity.Properties) > 0, "Entity should have properties")
		assert.True(t, len(entity.Events) > 0, "Entity should have events")

		// Check that properties have proper types and tags
		for _, prop := range entity.Properties {
			assert.NotEmpty(t, prop.Name)
			assert.NotEmpty(t, prop.Type)
			assert.NotEmpty(t, prop.Tags["json"])
			assert.NotEmpty(t, prop.Tags["protobuf"])
		}
	}
}

func TestProtoParser_ErrorHandlingIntegration(t *testing.T) {
	registry := NewParserRegistry()
	protoParser := NewProtoParser()

	err := registry.RegisterParser(protoParser)
	require.NoError(t, err)

	// Test with invalid proto file
	parser, err := registry.GetParser("testdata/invalid.proto")
	require.NoError(t, err)

	// This should fail during parsing
	_, err = parser.Parse("testdata/invalid.proto")
	require.Error(t, err)

	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, ParseError, cliErr.Type)
}

func TestProtoParser_ComparisonWithOtherParsers(t *testing.T) {
	// Test that proto parser works alongside other parsers
	registry := NewParserRegistry()

	// Register multiple parsers
	err := registry.RegisterParser(NewOpenAPIParser())
	require.NoError(t, err)
	err = registry.RegisterParser(NewProtoParser())
	require.NoError(t, err)

	// Test that each parser handles its own format
	protoParser, err := registry.GetParser("test.proto")
	require.NoError(t, err)
	assert.Equal(t, "Protocol Buffers", protoParser.FormatName())

	openAPIParser, err := registry.GetParser("test.yaml")
	require.NoError(t, err)
	assert.Equal(t, "OpenAPI", openAPIParser.FormatName())

	// Test format listing includes both
	formats := registry.ListFormats()
	assert.Contains(t, formats, "Protocol Buffers")
	assert.Contains(t, formats, "OpenAPI")
	assert.True(t, len(formats) >= 2)
}

func TestProtoParser_RealWorldScenario(t *testing.T) {
	// Test with a realistic proto file that might be used in production
	parser := NewProtoParser()

	// Parse the complex user.proto file
	model, err := parser.Parse("testdata/user.proto")
	require.NoError(t, err)
	require.NotNil(t, model)

	// Verify realistic expectations
	assert.NotEmpty(t, model.ProjectName)
	assert.True(t, len(model.Entities) >= 1, "Should have at least one domain entity")

	// Check that request/response messages are filtered out
	entityNames := make(map[string]bool)
	for _, entity := range model.Entities {
		entityNames[entity.Name] = true
	}

	// Should have domain entities
	assert.True(t, entityNames["User"] || entityNames["Profile"], "Should have domain entities")

	// Should NOT have request/response messages as entities
	assert.False(t, entityNames["CreateUserRequest"], "Should not include request messages as entities")
	assert.False(t, entityNames["CreateUserResponse"], "Should not include response messages as entities")
	assert.False(t, entityNames["GetUserRequest"], "Should not include request messages as entities")

	// Check metadata includes service information
	services, ok := model.Metadata["services"].([]map[string]interface{})
	assert.True(t, ok, "Should have service metadata")
	assert.True(t, len(services) > 0, "Should have at least one service")
}

func TestProtoParser_EdgeCases(t *testing.T) {
	parser := NewProtoParser()

	tests := []struct {
		name      string
		file      string
		shouldErr bool
		errType   ErrorType
	}{
		{
			name:      "file with only request/response messages",
			file:      "testdata/empty.proto",
			shouldErr: true,
			errType:   ParseError,
		},
		{
			name:      "valid simple proto file",
			file:      "testdata/simple.proto",
			shouldErr: false,
		},
		{
			name:      "complex proto file with relationships",
			file:      "testdata/user.proto",
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, err := parser.Parse(tt.file)

			if tt.shouldErr {
				require.Error(t, err)
				if cliErr, ok := err.(*CliError); ok {
					assert.Equal(t, tt.errType, cliErr.Type)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, model)
				assert.NotEmpty(t, model.ProjectName)
				assert.True(t, len(model.Entities) > 0)
			}
		})
	}
}
