package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCliError_Error(t *testing.T) {
	tests := []struct {
		name     string
		cliError *CliError
		expected string
	}{
		{
			name: "error without cause",
			cliError: &CliError{
				Type:    ValidationError,
				Message: "project name is invalid",
			},
			expected: "validation: project name is invalid",
		},
		{
			name: "error with cause",
			cliError: &CliError{
				Type:    FileSystemError,
				Message: "cannot create directory",
				Cause:   errors.New("permission denied"),
			},
			expected: "filesystem: cannot create directory (caused by: permission denied)",
		},
		{
			name: "parse error with cause",
			cliError: &CliError{
				Type:    ParseError,
				Message: "failed to parse ERD file",
				Cause:   errors.New("invalid YAML syntax at line 5"),
			},
			expected: "parse: failed to parse ERD file (caused by: invalid YAML syntax at line 5)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cliError.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCliError_ExitCode(t *testing.T) {
	tests := []struct {
		name     string
		cliError *CliError
		expected int
	}{
		{
			name: "custom exit code",
			cliError: &CliError{
				Type: ValidationError,
				Code: 99,
			},
			expected: 99,
		},
		{
			name: "argument error default code",
			cliError: &CliError{
				Type: ArgumentError,
			},
			expected: 2,
		},
		{
			name: "validation error default code",
			cliError: &CliError{
				Type: ValidationError,
			},
			expected: 3,
		},
		{
			name: "parse error default code",
			cliError: &CliError{
				Type: ParseError,
			},
			expected: 4,
		},
		{
			name: "generation error default code",
			cliError: &CliError{
				Type: GenerationError,
			},
			expected: 5,
		},
		{
			name: "filesystem error default code",
			cliError: &CliError{
				Type: FileSystemError,
			},
			expected: 6,
		},
		{
			name: "network error default code",
			cliError: &CliError{
				Type: NetworkError,
			},
			expected: 7,
		},
		{
			name: "unknown error type default code",
			cliError: &CliError{
				Type: ErrorType("unknown"),
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cliError.ExitCode()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewCliError(t *testing.T) {
	tests := []struct {
		name      string
		errorType ErrorType
		message   string
		cause     error
		expected  *CliError
	}{
		{
			name:      "create validation error without cause",
			errorType: ValidationError,
			message:   "invalid project name",
			cause:     nil,
			expected: &CliError{
				Type:    ValidationError,
				Message: "invalid project name",
				Cause:   nil,
			},
		},
		{
			name:      "create parse error with cause",
			errorType: ParseError,
			message:   "failed to parse file",
			cause:     errors.New("syntax error"),
			expected: &CliError{
				Type:    ParseError,
				Message: "failed to parse file",
				Cause:   errors.New("syntax error"),
			},
		},
		{
			name:      "create filesystem error with cause",
			errorType: FileSystemError,
			message:   "cannot write file",
			cause:     errors.New("disk full"),
			expected: &CliError{
				Type:    FileSystemError,
				Message: "cannot write file",
				Cause:   errors.New("disk full"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewCliError(tt.errorType, tt.message, tt.cause)

			assert.Equal(t, tt.expected.Type, result.Type)
			assert.Equal(t, tt.expected.Message, result.Message)

			if tt.expected.Cause != nil {
				assert.Equal(t, tt.expected.Cause.Error(), result.Cause.Error())
			} else {
				assert.Nil(t, result.Cause)
			}
		})
	}
}

func TestErrorType_Constants(t *testing.T) {
	// Test that all error type constants are properly defined
	assert.Equal(t, ErrorType("validation"), ValidationError)
	assert.Equal(t, ErrorType("parse"), ParseError)
	assert.Equal(t, ErrorType("generation"), GenerationError)
	assert.Equal(t, ErrorType("filesystem"), FileSystemError)
	assert.Equal(t, ErrorType("network"), NetworkError)
	assert.Equal(t, ErrorType("argument"), ArgumentError)
}

func TestCliError_ErrorCategorization(t *testing.T) {
	tests := []struct {
		name         string
		errorType    ErrorType
		message      string
		cause        error
		expectedMsg  string
		expectedCode int
	}{
		{
			name:         "validation error for project name",
			errorType:    ValidationError,
			message:      "project name must start with lowercase letter",
			cause:        nil,
			expectedMsg:  "validation: project name must start with lowercase letter",
			expectedCode: 3,
		},
		{
			name:         "parse error for ERD file",
			errorType:    ParseError,
			message:      "invalid ERD format",
			cause:        errors.New("YAML parsing failed at line 10"),
			expectedMsg:  "parse: invalid ERD format (caused by: YAML parsing failed at line 10)",
			expectedCode: 4,
		},
		{
			name:         "filesystem error for directory creation",
			errorType:    FileSystemError,
			message:      "failed to create project directory",
			cause:        errors.New("permission denied"),
			expectedMsg:  "filesystem: failed to create project directory (caused by: permission denied)",
			expectedCode: 6,
		},
		{
			name:         "network error for repository cloning",
			errorType:    NetworkError,
			message:      "failed to clone repository",
			cause:        errors.New("connection timeout"),
			expectedMsg:  "network: failed to clone repository (caused by: connection timeout)",
			expectedCode: 7,
		},
		{
			name:         "argument error for invalid flags",
			errorType:    ArgumentError,
			message:      "cannot specify multiple input formats",
			cause:        nil,
			expectedMsg:  "argument: cannot specify multiple input formats",
			expectedCode: 2,
		},
		{
			name:         "generation error for template processing",
			errorType:    GenerationError,
			message:      "failed to generate entity code",
			cause:        errors.New("template execution failed"),
			expectedMsg:  "generation: failed to generate entity code (caused by: template execution failed)",
			expectedCode: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cliError := NewCliError(tt.errorType, tt.message, tt.cause)

			assert.Equal(t, tt.expectedMsg, cliError.Error())
			assert.Equal(t, tt.expectedCode, cliError.ExitCode())
			assert.Equal(t, tt.errorType, cliError.Type)
			assert.Equal(t, tt.message, cliError.Message)

			if tt.cause != nil {
				assert.Equal(t, tt.cause.Error(), cliError.Cause.Error())
			} else {
				assert.Nil(t, cliError.Cause)
			}
		})
	}
}

func TestCliError_ChainedErrors(t *testing.T) {
	// Test error chaining scenarios
	originalErr := errors.New("original error")
	wrappedErr := NewCliError(ParseError, "parsing failed", originalErr)
	finalErr := NewCliError(GenerationError, "generation failed", wrappedErr)

	expectedMsg := "generation: generation failed (caused by: parse: parsing failed (caused by: original error))"
	assert.Equal(t, expectedMsg, finalErr.Error())
	assert.Equal(t, 5, finalErr.ExitCode()) // Should use GenerationError exit code
}
