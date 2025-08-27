package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParserRegistry_OpenAPIIntegration(t *testing.T) {
	registry := NewParserRegistry()
	parser := NewOpenAPIParser()

	// Register the OpenAPI parser
	err := registry.RegisterParser(parser)
	require.NoError(t, err)

	// Test that we can get the parser for different extensions
	yamlParser, err := registry.GetParser("test.yaml")
	require.NoError(t, err)
	assert.Equal(t, "OpenAPI", yamlParser.FormatName())

	ymlParser, err := registry.GetParser("test.yml")
	require.NoError(t, err)
	assert.Equal(t, "OpenAPI", ymlParser.FormatName())

	jsonParser, err := registry.GetParser("test.json")
	require.NoError(t, err)
	assert.Equal(t, "OpenAPI", jsonParser.FormatName())

	// Test format listing
	formats := registry.ListFormats()
	assert.Contains(t, formats, "OpenAPI")

	// Test format detection with actual file
	format, err := registry.DetectFormat("testdata/user-service.yaml")
	require.NoError(t, err)
	assert.Equal(t, "OpenAPI", format)

	// Test format detection with invalid file (should still detect as OpenAPI format but fail validation)
	format, err = registry.DetectFormat("testdata/invalid-openapi.yaml")
	assert.Error(t, err)
	assert.Empty(t, format)
	if cliErr, ok := err.(*CliError); ok {
		assert.Equal(t, ParseError, cliErr.Type)
	}
}
