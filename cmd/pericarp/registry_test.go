package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockDomainParser implements DomainParser for testing
type MockDomainParser struct {
	name         string
	extensions   []string
	parseFunc    func(string) (*DomainModel, error)
	validateFunc func(string) error
}

func (m *MockDomainParser) Parse(filePath string) (*DomainModel, error) {
	if m.parseFunc != nil {
		return m.parseFunc(filePath)
	}
	return &DomainModel{ProjectName: "test"}, nil
}

func (m *MockDomainParser) SupportedExtensions() []string {
	return m.extensions
}

func (m *MockDomainParser) FormatName() string {
	return m.name
}

func (m *MockDomainParser) Validate(filePath string) error {
	if m.validateFunc != nil {
		return m.validateFunc(filePath)
	}
	return nil
}

func TestNewParserRegistry(t *testing.T) {
	registry := NewParserRegistry()
	assert.NotNil(t, registry)

	// Test that it implements the interface
	// registry is already of type ParserRegistry, so no type assertion needed
}

func TestDefaultParserRegistry_RegisterParser(t *testing.T) {
	tests := []struct {
		name      string
		parser    DomainParser
		wantError bool
		errorType ErrorType
	}{
		{
			name: "register valid parser",
			parser: &MockDomainParser{
				name:       "ERD",
				extensions: []string{".yaml", ".yml"},
			},
			wantError: false,
		},
		{
			name: "register parser with single extension",
			parser: &MockDomainParser{
				name:       "OpenAPI",
				extensions: []string{".json"},
			},
			wantError: false,
		},
		{
			name: "register parser with multiple extensions",
			parser: &MockDomainParser{
				name:       "Proto",
				extensions: []string{".proto", ".protobuf"},
			},
			wantError: false,
		},
		{
			name:      "register nil parser",
			parser:    nil,
			wantError: true,
			errorType: ArgumentError,
		},
		{
			name: "register parser with empty extension",
			parser: &MockDomainParser{
				name:       "Invalid",
				extensions: []string{""},
			},
			wantError: true,
			errorType: ArgumentError,
		},
		{
			name: "register parser with mixed valid and empty extensions",
			parser: &MockDomainParser{
				name:       "Mixed",
				extensions: []string{".valid", ""},
			},
			wantError: true,
			errorType: ArgumentError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewParserRegistry()
			err := registry.RegisterParser(tt.parser)

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

func TestDefaultParserRegistry_GetParser(t *testing.T) {
	registry := NewParserRegistry()

	// Register test parsers
	erdParser := &MockDomainParser{
		name:       "ERD",
		extensions: []string{".yaml", ".yml"},
	}
	openAPIParser := &MockDomainParser{
		name:       "OpenAPI",
		extensions: []string{".json"},
	}
	protoParser := &MockDomainParser{
		name:       "Protocol Buffers",
		extensions: []string{".proto"},
	}

	registry.RegisterParser(erdParser)
	registry.RegisterParser(openAPIParser)
	registry.RegisterParser(protoParser)

	tests := []struct {
		name         string
		filePath     string
		wantParser   DomainParser
		wantError    bool
		errorType    ErrorType
		errorMessage string
	}{
		{
			name:       "get parser for .yaml file",
			filePath:   "test.yaml",
			wantParser: erdParser,
			wantError:  false,
		},
		{
			name:       "get parser for .yml file",
			filePath:   "test.yml",
			wantParser: erdParser,
			wantError:  false,
		},
		{
			name:       "get parser for .json file",
			filePath:   "test.json",
			wantParser: openAPIParser,
			wantError:  false,
		},
		{
			name:       "get parser for .proto file",
			filePath:   "test.proto",
			wantParser: protoParser,
			wantError:  false,
		},
		{
			name:       "get parser for file with path",
			filePath:   "/path/to/file.yaml",
			wantParser: erdParser,
			wantError:  false,
		},
		{
			name:       "get parser for uppercase extension",
			filePath:   "test.YAML",
			wantParser: erdParser,
			wantError:  false,
		},
		{
			name:         "get parser for unsupported extension",
			filePath:     "test.txt",
			wantParser:   nil,
			wantError:    true,
			errorType:    ParseError,
			errorMessage: "no parser found for file extension: .txt",
		},
		{
			name:         "get parser for file without extension",
			filePath:     "test",
			wantParser:   nil,
			wantError:    true,
			errorType:    ParseError,
			errorMessage: "no parser found for file extension: ",
		},
		{
			name:         "get parser for empty file path",
			filePath:     "",
			wantParser:   nil,
			wantError:    true,
			errorType:    ArgumentError,
			errorMessage: "file path cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := registry.GetParser(tt.filePath)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, parser)
				if cliErr, ok := err.(*CliError); ok {
					assert.Equal(t, tt.errorType, cliErr.Type)
					assert.Contains(t, cliErr.Message, tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantParser, parser)
			}
		})
	}
}

func TestDefaultParserRegistry_ListFormats(t *testing.T) {
	tests := []struct {
		name            string
		parsers         []DomainParser
		expectedFormats []string
	}{
		{
			name:            "empty registry",
			parsers:         []DomainParser{},
			expectedFormats: []string{},
		},
		{
			name: "single parser",
			parsers: []DomainParser{
				&MockDomainParser{
					name:       "ERD",
					extensions: []string{".yaml"},
				},
			},
			expectedFormats: []string{"ERD"},
		},
		{
			name: "multiple parsers with unique formats",
			parsers: []DomainParser{
				&MockDomainParser{
					name:       "ERD",
					extensions: []string{".yaml", ".yml"},
				},
				&MockDomainParser{
					name:       "OpenAPI",
					extensions: []string{".json"},
				},
				&MockDomainParser{
					name:       "Protocol Buffers",
					extensions: []string{".proto"},
				},
			},
			expectedFormats: []string{"ERD", "OpenAPI", "Protocol Buffers"},
		},
		{
			name: "multiple parsers with duplicate format names",
			parsers: []DomainParser{
				&MockDomainParser{
					name:       "JSON",
					extensions: []string{".json"},
				},
				&MockDomainParser{
					name:       "JSON",
					extensions: []string{".jsonl"},
				},
			},
			expectedFormats: []string{"JSON"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewParserRegistry()

			// Register all parsers
			for _, parser := range tt.parsers {
				err := registry.RegisterParser(parser)
				assert.NoError(t, err)
			}

			formats := registry.ListFormats()

			// Check that all expected formats are present
			assert.Len(t, formats, len(tt.expectedFormats))
			for _, expectedFormat := range tt.expectedFormats {
				assert.Contains(t, formats, expectedFormat)
			}
		})
	}
}

func TestDefaultParserRegistry_DetectFormat(t *testing.T) {
	registry := NewParserRegistry()

	// Register test parsers with validation
	erdParser := &MockDomainParser{
		name:       "ERD",
		extensions: []string{".yaml", ".yml"},
		validateFunc: func(filePath string) error {
			if filePath == "invalid.yaml" {
				return errors.New("invalid ERD format")
			}
			return nil
		},
	}
	openAPIParser := &MockDomainParser{
		name:       "OpenAPI",
		extensions: []string{".json"},
		validateFunc: func(filePath string) error {
			if filePath == "invalid.json" {
				return errors.New("invalid OpenAPI format")
			}
			return nil
		},
	}

	registry.RegisterParser(erdParser)
	registry.RegisterParser(openAPIParser)

	tests := []struct {
		name           string
		filePath       string
		expectedFormat string
		wantError      bool
		errorType      ErrorType
	}{
		{
			name:           "detect ERD format from .yaml file",
			filePath:       "valid.yaml",
			expectedFormat: "ERD",
			wantError:      false,
		},
		{
			name:           "detect ERD format from .yml file",
			filePath:       "valid.yml",
			expectedFormat: "ERD",
			wantError:      false,
		},
		{
			name:           "detect OpenAPI format from .json file",
			filePath:       "valid.json",
			expectedFormat: "OpenAPI",
			wantError:      false,
		},
		{
			name:      "detect format for unsupported extension",
			filePath:  "test.txt",
			wantError: true,
			errorType: ParseError,
		},
		{
			name:      "detect format for invalid file content",
			filePath:  "invalid.yaml",
			wantError: true,
			errorType: ParseError,
		},
		{
			name:      "detect format for invalid JSON content",
			filePath:  "invalid.json",
			wantError: true,
			errorType: ParseError,
		},
		{
			name:      "detect format for empty file path",
			filePath:  "",
			wantError: true,
			errorType: ArgumentError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, err := registry.DetectFormat(tt.filePath)

			if tt.wantError {
				assert.Error(t, err)
				assert.Empty(t, format)
				if cliErr, ok := err.(*CliError); ok {
					assert.Equal(t, tt.errorType, cliErr.Type)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedFormat, format)
			}
		})
	}
}

func TestDefaultParserRegistry_CaseInsensitiveExtensions(t *testing.T) {
	registry := NewParserRegistry()

	parser := &MockDomainParser{
		name:       "Test",
		extensions: []string{".TEST", ".Test", ".test"},
	}

	err := registry.RegisterParser(parser)
	assert.NoError(t, err)

	// Test that all case variations work
	testCases := []string{
		"file.test",
		"file.TEST",
		"file.Test",
		"file.tEsT",
	}

	for _, testCase := range testCases {
		t.Run(testCase, func(t *testing.T) {
			foundParser, err := registry.GetParser(testCase)
			assert.NoError(t, err)
			assert.Equal(t, parser, foundParser)
		})
	}
}

func TestDefaultParserRegistry_OverwriteParser(t *testing.T) {
	registry := NewParserRegistry()

	// Register first parser
	parser1 := &MockDomainParser{
		name:       "Parser1",
		extensions: []string{".test"},
	}
	err := registry.RegisterParser(parser1)
	assert.NoError(t, err)

	// Register second parser with same extension (should overwrite)
	parser2 := &MockDomainParser{
		name:       "Parser2",
		extensions: []string{".test"},
	}
	err = registry.RegisterParser(parser2)
	assert.NoError(t, err)

	// Should return the second parser
	foundParser, err := registry.GetParser("file.test")
	assert.NoError(t, err)
	assert.Equal(t, parser2, foundParser)
	assert.Equal(t, "Parser2", foundParser.FormatName())
}

func TestDefaultParserRegistry_MultipleExtensionsPerParser(t *testing.T) {
	registry := NewParserRegistry()

	parser := &MockDomainParser{
		name:       "MultiExt",
		extensions: []string{".ext1", ".ext2", ".ext3"},
	}

	err := registry.RegisterParser(parser)
	assert.NoError(t, err)

	// Test all extensions return the same parser
	extensions := []string{".ext1", ".ext2", ".ext3"}
	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			foundParser, err := registry.GetParser("file" + ext)
			assert.NoError(t, err)
			assert.Equal(t, parser, foundParser)
		})
	}
}

func TestDefaultParserRegistry_Integration(t *testing.T) {
	// Test a complete workflow with multiple parsers
	registry := NewParserRegistry()

	// Register multiple parsers
	parsers := []DomainParser{
		&MockDomainParser{
			name:       "ERD",
			extensions: []string{".yaml", ".yml"},
		},
		&MockDomainParser{
			name:       "OpenAPI",
			extensions: []string{".json"},
		},
		&MockDomainParser{
			name:       "Protocol Buffers",
			extensions: []string{".proto"},
		},
	}

	for _, parser := range parsers {
		err := registry.RegisterParser(parser)
		assert.NoError(t, err)
	}

	// Test ListFormats
	formats := registry.ListFormats()
	assert.Len(t, formats, 3)
	assert.Contains(t, formats, "ERD")
	assert.Contains(t, formats, "OpenAPI")
	assert.Contains(t, formats, "Protocol Buffers")

	// Test GetParser for each format
	testFiles := map[string]string{
		"test.yaml":  "ERD",
		"test.yml":   "ERD",
		"test.json":  "OpenAPI",
		"test.proto": "Protocol Buffers",
	}

	for filePath, expectedFormat := range testFiles {
		parser, err := registry.GetParser(filePath)
		assert.NoError(t, err)
		assert.Equal(t, expectedFormat, parser.FormatName())

		// Test DetectFormat
		format, err := registry.DetectFormat(filePath)
		assert.NoError(t, err)
		assert.Equal(t, expectedFormat, format)
	}
}
