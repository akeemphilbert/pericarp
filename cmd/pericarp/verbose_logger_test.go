package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVerboseLogger_SetVerbose(t *testing.T) {
	logger := NewVerboseLogger()

	// Initially not verbose
	assert.False(t, logger.IsVerbose())

	// Enable verbose
	logger.SetVerbose(true)
	assert.True(t, logger.IsVerbose())

	// Disable verbose
	logger.SetVerbose(false)
	assert.False(t, logger.IsVerbose())
}

func TestVerboseLogger_LogLevels(t *testing.T) {
	tests := []struct {
		name         string
		verbose      bool
		level        LogLevel
		logFunc      func(*VerboseLogger)
		shouldOutput bool
	}{
		{
			name:         "debug message with verbose enabled",
			verbose:      true,
			level:        LogLevelDebug,
			logFunc:      func(l *VerboseLogger) { l.Debug("debug message") },
			shouldOutput: true,
		},
		{
			name:         "debug message with verbose disabled",
			verbose:      false,
			level:        LogLevelInfo,
			logFunc:      func(l *VerboseLogger) { l.Debug("debug message") },
			shouldOutput: false,
		},
		{
			name:         "info message always outputs",
			verbose:      false,
			level:        LogLevelInfo,
			logFunc:      func(l *VerboseLogger) { l.Info("info message") },
			shouldOutput: true,
		},
		{
			name:         "warn message always outputs",
			verbose:      false,
			level:        LogLevelInfo,
			logFunc:      func(l *VerboseLogger) { l.Warn("warn message") },
			shouldOutput: true,
		},
		{
			name:         "error message always outputs",
			verbose:      false,
			level:        LogLevelInfo,
			logFunc:      func(l *VerboseLogger) { l.Error("error message") },
			shouldOutput: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			var errBuf bytes.Buffer

			logger := NewVerboseLogger()
			logger.SetWriter(&buf)
			logger.SetErrorWriter(&errBuf)
			logger.SetVerbose(tt.verbose)
			logger.SetLevel(tt.level)

			tt.logFunc(logger)

			output := buf.String() + errBuf.String()
			if tt.shouldOutput {
				assert.NotEmpty(t, output)
			} else {
				assert.Empty(t, output)
			}
		})
	}
}

func TestVerboseLogger_FormattedLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := NewVerboseLogger()
	logger.SetWriter(&buf)
	logger.SetVerbose(true)

	// Test formatted logging
	logger.Debugf("Processing file: %s (size: %d)", "test.go", 1024)

	output := buf.String()
	assert.Contains(t, output, "Processing file: test.go (size: 1024)")
	assert.Contains(t, output, "[DEBUG]")
}

func TestVerboseLogger_StructuredLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := NewVerboseLogger()
	logger.SetWriter(&buf)
	logger.SetVerbose(true)

	// Test structured logging with key-value pairs
	logger.Debug("Processing entity", "name", "User", "properties", 5)

	output := buf.String()
	assert.Contains(t, output, "Processing entity")
	assert.Contains(t, output, "name=User")
	assert.Contains(t, output, "properties=5")
	assert.Contains(t, output, "[DEBUG]")
}

func TestVerboseLogger_EnhancedMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := NewVerboseLogger()
	logger.SetWriter(&buf)
	logger.SetVerbose(true)

	// Test LogStep
	logger.LogStep("Generating entity", 1, 3)
	assert.Contains(t, buf.String(), "[1/3] Generating entity")

	// Reset buffer
	buf.Reset()

	// Test LogProgress
	logger.LogProgress("File generation", 2, 5)
	assert.Contains(t, buf.String(), "File generation: 40.0% (2/5)")

	// Reset buffer
	buf.Reset()

	// Test LogDuration
	duration := 150 * time.Millisecond
	logger.LogDuration("Template processing", duration)
	assert.Contains(t, buf.String(), "Template processing completed in")
	assert.Contains(t, buf.String(), "150ms")

	// Reset buffer
	buf.Reset()

	// Test LogFileOperation
	logger.LogFileOperation("Created", "/tmp/user.go", 2048)
	assert.Contains(t, buf.String(), "Created: /tmp/user.go (size: 2048 bytes)")
}

func TestVerboseLogger_ParsingAndGenerationDetails(t *testing.T) {
	var buf bytes.Buffer
	logger := NewVerboseLogger()
	logger.SetWriter(&buf)
	logger.SetVerbose(true)

	// Test LogParsingDetails
	logger.LogParsingDetails("OpenAPI", "user-service.yaml", 3)
	output := buf.String()
	assert.Contains(t, output, "Parsing OpenAPI file: user-service.yaml")
	assert.Contains(t, output, "Extracted 3 entities")

	// Reset buffer
	buf.Reset()

	// Test LogGenerationDetails
	entity := Entity{
		Name: "User",
		Properties: []Property{
			{Name: "GetID", Type: "string"},
			{Name: "Email", Type: "string"},
		},
		Methods: []Method{
			{Name: "UpdateEmail"},
		},
	}
	logger.LogGenerationDetails("Repository", entity)
	output = buf.String()
	assert.Contains(t, output, "Generating Repository for entity: User")
	assert.Contains(t, output, "Entity properties: 2")
	assert.Contains(t, output, "Entity methods: 1")

	// Reset buffer
	buf.Reset()

	// Test LogTemplateDetails
	logger.LogTemplateDetails("entity.go.tmpl", "internal/domain/user.go")
	assert.Contains(t, buf.String(), "Processing template: entity.go.tmpl -> internal/domain/user.go")

	// Reset buffer
	buf.Reset()

	// Test LogValidationDetails
	logger.LogValidationDetails("Project name", "user-service", true)
	assert.Contains(t, buf.String(), "✓ Project name validation: user-service")

	logger.LogValidationDetails("Input file", "invalid.yaml", false)
	assert.Contains(t, buf.String(), "✗ Input file validation: invalid.yaml")
}

func TestVerboseLogger_Sections(t *testing.T) {
	var buf bytes.Buffer
	logger := NewVerboseLogger()
	logger.SetWriter(&buf)
	logger.SetVerbose(true)

	// Test LogSection
	logger.LogSection("PARSING PHASE")
	output := buf.String()
	assert.Contains(t, output, "PARSING PHASE")
	assert.Contains(t, output, strings.Repeat("=", 50))

	// Reset buffer
	buf.Reset()

	// Test LogSubSection
	logger.LogSubSection("Entity Generation")
	output = buf.String()
	assert.Contains(t, output, "Entity Generation")
	assert.Contains(t, output, strings.Repeat("-", 30))
}

func TestVerboseLogger_WithTimestamp(t *testing.T) {
	var buf bytes.Buffer
	logger := NewVerboseLogger()
	logger.SetWriter(&buf)
	logger.SetVerbose(true)
	logger.SetTimestamp(true)

	logger.Info("Test message")

	output := buf.String()
	assert.Contains(t, output, "Test message")
	assert.Contains(t, output, "[INFO]")
	// Should contain timestamp in HH:MM:SS format
	assert.Regexp(t, `\d{2}:\d{2}:\d{2}`, output)
}

func TestVerboseLogger_WithPrefix(t *testing.T) {
	var buf bytes.Buffer
	logger := NewVerboseLogger()
	logger.SetWriter(&buf)
	logger.SetVerbose(true)
	logger.SetPrefix("CLI")

	logger.Info("Test message")

	output := buf.String()
	assert.Contains(t, output, "CLI [INFO] Test message")
}

func TestVerboseLogger_LogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LogLevelError, "ERROR"},
		{LogLevelWarn, "WARN"},
		{LogLevelInfo, "INFO"},
		{LogLevelDebug, "DEBUG"},
		{LogLevel(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.level.String())
		})
	}
}

func TestVerboseLogger_SetLevel(t *testing.T) {
	var buf bytes.Buffer
	var errBuf bytes.Buffer
	logger := NewVerboseLogger()
	logger.SetWriter(&buf)
	logger.SetErrorWriter(&errBuf)

	// Set level to WARN, should not log INFO or DEBUG
	logger.SetLevel(LogLevelWarn)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")

	output := buf.String() + errBuf.String()
	assert.NotContains(t, output, "debug message")
	assert.NotContains(t, output, "info message")
	assert.Contains(t, output, "warn message")
}

func TestVerboseLogger_NonVerboseMode(t *testing.T) {
	var buf bytes.Buffer
	logger := NewVerboseLogger()
	logger.SetWriter(&buf)
	logger.SetVerbose(false) // Explicitly set to false

	// These methods should not output anything when not in verbose mode
	logger.LogStep("Step", 1, 3)
	logger.LogProgress("Progress", 1, 3)
	logger.LogDuration("Operation", time.Second)
	logger.LogFileOperation("Created", "file.go", 100)
	logger.LogParsingDetails("OpenAPI", "file.yaml", 2)
	logger.LogSection("Section")
	logger.LogSubSection("SubSection")

	entity := Entity{Name: "Test"}
	logger.LogGenerationDetails("Component", entity)
	logger.LogTemplateDetails("template", "output")
	logger.LogValidationDetails("validation", "target", true)

	// Buffer should be empty since verbose is disabled
	assert.Empty(t, buf.String())
}
