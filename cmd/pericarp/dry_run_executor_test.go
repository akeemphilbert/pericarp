package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoggerWithOutput is a test logger that captures output for testing
type TestLoggerWithOutput struct {
	verbose bool
	output  strings.Builder
}

// NewTestLoggerWithOutput creates a test logger that captures output
func NewTestLoggerWithOutput() *TestLoggerWithOutput {
	return &TestLoggerWithOutput{verbose: false}
}

// GetOutput returns the captured output
func (l *TestLoggerWithOutput) GetOutput() string {
	return l.output.String()
}

// Structured logging methods
func (l *TestLoggerWithOutput) Debug(msg string, keysAndValues ...interface{}) {
	if l.verbose {
		formatted := l.formatMessage(msg, keysAndValues...)
		l.output.WriteString("[DEBUG] " + formatted + "\n")
	}
}

func (l *TestLoggerWithOutput) Info(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage(msg, keysAndValues...)
	l.output.WriteString("[INFO] " + formatted + "\n")
}

func (l *TestLoggerWithOutput) Warn(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage(msg, keysAndValues...)
	l.output.WriteString("[WARN] " + formatted + "\n")
}

func (l *TestLoggerWithOutput) Error(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage(msg, keysAndValues...)
	l.output.WriteString("[ERROR] " + formatted + "\n")
}

func (l *TestLoggerWithOutput) Fatal(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage(msg, keysAndValues...)
	l.output.WriteString("[FATAL] " + formatted + "\n")
}

// Formatted logging methods
func (l *TestLoggerWithOutput) Debugf(format string, args ...interface{}) {
	if l.verbose {
		message := fmt.Sprintf(format, args...)
		l.output.WriteString("[DEBUG] " + message + "\n")
	}
}

func (l *TestLoggerWithOutput) Infof(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.output.WriteString("[INFO] " + message + "\n")
}

func (l *TestLoggerWithOutput) Warnf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.output.WriteString("[WARN] " + message + "\n")
}

func (l *TestLoggerWithOutput) Errorf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.output.WriteString("[ERROR] " + message + "\n")
}

func (l *TestLoggerWithOutput) Fatalf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.output.WriteString("[FATAL] " + message + "\n")
}

// formatMessage formats a message with key-value pairs for CLI output
func (l *TestLoggerWithOutput) formatMessage(msg string, keysAndValues ...interface{}) string {
	if len(keysAndValues) == 0 {
		return msg
	}

	formatted := msg
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprintf("%v", keysAndValues[i])
			value := fmt.Sprintf("%v", keysAndValues[i+1])
			formatted += fmt.Sprintf(" %s=%s", key, value)
		}
	}
	return formatted
}

// CLI-specific methods
func (l *TestLoggerWithOutput) SetVerbose(enabled bool) { l.verbose = enabled }
func (l *TestLoggerWithOutput) IsVerbose() bool         { return l.verbose }

func TestDryRunExecutor_Execute(t *testing.T) {
	tests := []struct {
		name        string
		files       []*GeneratedFile
		destination string
		dryRun      bool
		verbose     bool
		wantErr     bool
		errContains string
	}{
		{
			name: "successful dry run with files",
			files: []*GeneratedFile{
				{
					Path:    "internal/domain/user.go",
					Content: "package domain\n\ntype User struct {\n\tID string\n}",
				},
				{
					Path:    "internal/application/commands.go",
					Content: "package application\n\ntype CreateUserCommand struct {}",
				},
			},
			destination: "/tmp/test-project",
			dryRun:      true,
			verbose:     false,
			wantErr:     false,
		},
		{
			name: "successful dry run with verbose output",
			files: []*GeneratedFile{
				{
					Path:    "go.mod",
					Content: "module test-project\n\ngo 1.21",
					Metadata: map[string]interface{}{
						"type": "module",
					},
				},
			},
			destination: "/tmp/test-project",
			dryRun:      true,
			verbose:     true,
			wantErr:     false,
		},
		{
			name:        "empty files list",
			files:       []*GeneratedFile{},
			destination: "/tmp/test-project",
			dryRun:      true,
			verbose:     false,
			wantErr:     false,
		},
		{
			name: "error when not in dry-run mode",
			files: []*GeneratedFile{
				{Path: "test.go", Content: "package main"},
			},
			destination: "/tmp/test-project",
			dryRun:      false,
			verbose:     false,
			wantErr:     true,
			errContains: "DryRunExecutor should only be used in dry-run mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock logger
			logger := NewTestLoggerWithOutput()
			logger.SetVerbose(tt.verbose)

			// Create dry-run executor
			executor := NewDryRunExecutor(logger)

			// Execute
			err := executor.Execute(context.Background(), tt.files, tt.destination, tt.dryRun)

			// Verify results
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)

			// Verify output contains expected information
			output := logger.GetOutput()
			assert.Contains(t, output, "DRY RUN MODE")
			assert.Contains(t, output, tt.destination)

			if len(tt.files) > 0 {
				assert.Contains(t, output, "Files to be generated")
				for _, file := range tt.files {
					assert.Contains(t, output, file.Path)
				}

				// Check verbose output
				if tt.verbose {
					assert.Contains(t, output, "Content preview")
				} else {
					assert.Contains(t, output, "Use --verbose flag")
				}
			} else {
				assert.Contains(t, output, "No files to generate")
			}
		})
	}
}

func TestDryRunExecutor_PreviewProjectStructure(t *testing.T) {
	tests := []struct {
		name    string
		model   *DomainModel
		dest    string
		verbose bool
	}{
		{
			name: "simple project structure",
			model: &DomainModel{
				ProjectName: "user-service",
				Entities: []Entity{
					{Name: "User", Properties: []Property{{Name: "ID", Type: "string"}}},
					{Name: "Order", Properties: []Property{{Name: "ID", Type: "string"}}},
				},
			},
			dest:    "/tmp/user-service",
			verbose: false,
		},
		{
			name: "project structure with verbose output",
			model: &DomainModel{
				ProjectName: "inventory-service",
				Entities: []Entity{
					{Name: "Product", Properties: []Property{
						{Name: "ID", Type: "string"},
						{Name: "Name", Type: "string"},
						{Name: "Price", Type: "float64"},
					}},
				},
			},
			dest:    "/tmp/inventory-service",
			verbose: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock logger
			logger := NewTestLoggerWithOutput()
			logger.SetVerbose(tt.verbose)

			// Create dry-run executor
			executor := NewDryRunExecutor(logger)

			// Execute
			err := executor.PreviewProjectStructure(tt.model, tt.dest)
			require.NoError(t, err)

			// Verify output
			output := logger.GetOutput()
			assert.Contains(t, output, "Project Structure Preview")
			assert.Contains(t, output, tt.model.ProjectName)
			assert.Contains(t, output, tt.dest)
			assert.Contains(t, output, "go.mod")
			assert.Contains(t, output, "Makefile")
			assert.Contains(t, output, "├── internal/")

			// Check entity-specific files
			for _, entity := range tt.model.Entities {
				entityFile := strings.ToLower(entity.Name) + ".go"
				assert.Contains(t, output, entityFile)
			}

			// Check verbose output
			if tt.verbose {
				assert.Contains(t, output, "Entities to be generated")
				for _, entity := range tt.model.Entities {
					assert.Contains(t, output, entity.Name)
				}
			}
		})
	}
}

func TestDryRunExecutor_groupFilesByDirectory(t *testing.T) {
	logger := NewTestLoggerWithOutput()
	executor := NewDryRunExecutor(logger)

	files := []*GeneratedFile{
		{Path: "go.mod", Content: "module test"},
		{Path: "internal/domain/user.go", Content: "package domain"},
		{Path: "internal/domain/order.go", Content: "package domain"},
		{Path: "internal/application/commands.go", Content: "package application"},
		{Path: "cmd/main.go", Content: "package main"},
	}

	result := executor.groupFilesByDirectory(files, "/tmp/test")

	// Verify grouping
	assert.Len(t, result, 4) // root, internal/domain, internal/application, cmd

	assert.Len(t, result["root"], 1)
	assert.Equal(t, "go.mod", result["root"][0].Path)

	assert.Len(t, result["internal/domain"], 2)
	assert.Len(t, result["internal/application"], 1)
	assert.Len(t, result["cmd"], 1)
}

func TestDryRunExecutor_truncateContent(t *testing.T) {
	logger := NewTestLoggerWithOutput()
	executor := NewDryRunExecutor(logger)

	tests := []struct {
		name      string
		content   string
		maxLength int
		expected  string
	}{
		{
			name:      "content shorter than max",
			content:   "short content",
			maxLength: 100,
			expected:  "short content",
		},
		{
			name:      "content longer than max",
			content:   "this is a very long content that should be truncated",
			maxLength: 20,
			expected:  "this is a very long ",
		},
		{
			name:      "empty content",
			content:   "",
			maxLength: 100,
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.truncateContent(tt.content, tt.maxLength)
			assert.Equal(t, tt.expected, result)
		})
	}
}
