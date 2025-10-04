package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTemplateEngine(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)

	require.NoError(t, err)
	assert.NotNil(t, engine)
	assert.NotEmpty(t, engine.templates)
	assert.NotEmpty(t, engine.funcMap)
}

func TestTemplateEngine_Execute(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	// Test data
	entity := Entity{
		Name: "User",
		Properties: []Property{
			{
				Name:     "id",
				Type:     "uuid",
				Required: true,
			},
			{
				Name:     "email",
				Type:     "string",
				Required: true,
			},
			{
				Name:     "name",
				Type:     "string",
				Required: false,
			},
		},
	}

	// Test entity template execution
	if engine.HasTemplate("entity.go") {
		result, err := engine.Execute("entity.go", entity)
		require.NoError(t, err)
		assert.Contains(t, result, "type User struct")
		assert.Contains(t, result, "func NewUser(")
		assert.Contains(t, result, "GetID() string")
		assert.Contains(t, result, "Version() int")
	}
}

func TestTemplateEngine_HasTemplate(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	// Test existing template
	assert.True(t, engine.HasTemplate("entity.go"))

	// Test non-existing template
	assert.False(t, engine.HasTemplate("nonexistent.go"))
}

func TestTemplateEngine_ListTemplates(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	templates := engine.ListTemplates()
	assert.NotEmpty(t, templates)

	// Check that expected templates are present
	expectedTemplates := []string{
		"entity.go",
		"entity_events.go",
		"repository_interface.go",
		"repository_implementation.go",
	}

	for _, expected := range expectedTemplates {
		assert.Contains(t, templates, expected)
	}
}

func TestTemplateEngine_HelperFunctions(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	tests := []struct {
		name     string
		function string
		input    string
		expected string
	}{
		{
			name:     "toCamelCase",
			function: "camelCase",
			input:    "user_name",
			expected: "userName",
		},
		{
			name:     "toPascalCase",
			function: "pascalCase",
			input:    "user_name",
			expected: "UserName",
		},
		{
			name:     "toSnakeCase",
			function: "snakeCase",
			input:    "UserName",
			expected: "user_name",
		},
		{
			name:     "toKebabCase",
			function: "kebabCase",
			input:    "UserName",
			expected: "user-name",
		},
		{
			name:     "toPlural",
			function: "plural",
			input:    "user",
			expected: "users",
		},
		{
			name:     "toSingular",
			function: "singular",
			input:    "users",
			expected: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			switch tt.function {
			case "camelCase":
				result = engine.toCamelCase(tt.input)
			case "pascalCase":
				result = engine.toPascalCase(tt.input)
			case "snakeCase":
				result = engine.toSnakeCase(tt.input)
			case "kebabCase":
				result = engine.toKebabCase(tt.input)
			case "plural":
				result = engine.toPlural(tt.input)
			case "singular":
				result = engine.toSingular(tt.input)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateEngine_GetZeroValue(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	tests := []struct {
		goType   string
		expected string
	}{
		{"string", `""`},
		{"int", "0"},
		{"int64", "0"},
		{"float64", "0.0"},
		{"bool", "false"},
		{"time.Time", "time.Time{}"},
		{"ksuid.KSUID", "ksuid.KSUID{}"},
		{"*User", "nil"},
		{"[]string", "nil"},
		{"map[string]string", "nil"},
		{"User", "User{}"},
	}

	for _, tt := range tests {
		t.Run(tt.goType, func(t *testing.T) {
			result := engine.getZeroValue(tt.goType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateEngine_ToGoType(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	tests := []struct {
		genericType string
		expected    string
	}{
		{"string", "string"},
		{"text", "string"},
		{"int", "int"},
		{"integer", "int"},
		{"int64", "int64"},
		{"long", "int64"},
		{"float", "float64"},
		{"double", "float64"},
		{"bool", "bool"},
		{"boolean", "bool"},
		{"uuid", "ksuid.KSUID"},
		{"guid", "ksuid.KSUID"},
		{"time", "time.Time"},
		{"datetime", "time.Time"},
		{"timestamp", "time.Time"},
		{"date", "time.Time"},
		{"custom", "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.genericType, func(t *testing.T) {
			result := engine.toGoType(tt.genericType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateEngine_ToJSONTag(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	tests := []struct {
		fieldName string
		required  bool
		expected  string
	}{
		{"UserName", true, `json:"user_name"`},
		{"UserName", false, `json:"user_name,omitempty"`},
		{"GetID", true, `json:"id"`},
		{"Email", false, `json:"email,omitempty"`},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			result := engine.toJSONTag(tt.fieldName, tt.required)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateEngine_ToValidationTag(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	tests := []struct {
		name     string
		property Property
		expected string
	}{
		{
			name: "required field",
			property: Property{
				Name:     "email",
				Required: true,
			},
			expected: `validate:"required"`,
		},
		{
			name: "optional field",
			property: Property{
				Name:     "name",
				Required: false,
			},
			expected: "",
		},
		{
			name: "required with validation",
			property: Property{
				Name:       "email",
				Required:   true,
				Validation: "email",
			},
			expected: `validate:"required,email"`,
		},
		{
			name: "optional with validation",
			property: Property{
				Name:       "age",
				Required:   false,
				Validation: "min=0,max=150",
			},
			expected: `validate:"min=0,max=150"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.toValidationTag(tt.property)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateEngine_FilterProperties(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	properties := []Property{
		{Name: "id", Required: true},
		{Name: "email", Required: true},
		{Name: "name", Required: false},
		{Name: "age", Required: false},
	}

	required := engine.filterRequired(properties)
	assert.Len(t, required, 2)
	assert.Equal(t, "id", required[0].Name)
	assert.Equal(t, "email", required[1].Name)

	optional := engine.filterOptional(properties)
	assert.Len(t, optional, 2)
	assert.Equal(t, "name", optional[0].Name)
	assert.Equal(t, "age", optional[1].Name)
}

func TestTemplateEngine_SliceString(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	tests := []struct {
		input    string
		start    int
		end      int
		expected string
	}{
		{"hello", 0, 1, "h"},
		{"hello", 1, 3, "el"},
		{"hello", 0, 5, "hello"},
		{"hello", 0, 10, "hello"}, // end beyond length
		{"hello", -1, 3, ""},      // negative start
		{"hello", 3, 1, ""},       // start >= end
		{"", 0, 1, ""},            // empty string
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := engine.sliceString(tt.input, tt.start, tt.end)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateEngine_Indent(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	input := "line1\nline2\n\nline4"
	expected := "  line1\n  line2\n\n  line4"

	result := engine.indent(input, 2)
	assert.Equal(t, expected, result)
}

// NewTestLogger creates a test logger for testing
func NewTestLogger() CliLogger {
	return &TestLogger{verbose: false}
}

// TestLogger is a simple test implementation of CliLogger
type TestLogger struct {
	verbose bool
}

// Structured logging methods
func (l *TestLogger) Debug(msg string, keysAndValues ...interface{}) {}
func (l *TestLogger) Info(msg string, keysAndValues ...interface{})  {}
func (l *TestLogger) Warn(msg string, keysAndValues ...interface{})  {}
func (l *TestLogger) Error(msg string, keysAndValues ...interface{}) {}
func (l *TestLogger) Fatal(msg string, keysAndValues ...interface{}) {}

// Formatted logging methods
func (l *TestLogger) Debugf(format string, args ...interface{}) {}
func (l *TestLogger) Infof(format string, args ...interface{})  {}
func (l *TestLogger) Warnf(format string, args ...interface{})  {}
func (l *TestLogger) Errorf(format string, args ...interface{}) {}
func (l *TestLogger) Fatalf(format string, args ...interface{}) {}

// CLI-specific methods
func (l *TestLogger) SetVerbose(enabled bool) { l.verbose = enabled }
func (l *TestLogger) IsVerbose() bool         { return l.verbose }
