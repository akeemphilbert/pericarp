package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultValidator_ValidateProjectName(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name        string
		projectName string
		expectError bool
		errorType   ErrorType
		errorMsg    string
	}{
		{
			name:        "valid simple name",
			projectName: "myproject",
			expectError: false,
		},
		{
			name:        "valid name with hyphens",
			projectName: "my-project",
			expectError: false,
		},
		{
			name:        "valid name with numbers",
			projectName: "project123",
			expectError: false,
		},
		{
			name:        "valid name with hyphens and numbers",
			projectName: "my-project-v2",
			expectError: false,
		},
		{
			name:        "single character name",
			projectName: "a",
			expectError: false,
		},
		{
			name:        "empty name",
			projectName: "",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name cannot be empty",
		},
		{
			name:        "name starting with uppercase",
			projectName: "MyProject",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name must start with lowercase letter and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:        "name starting with number",
			projectName: "123project",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name must start with lowercase letter and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:        "name starting with hyphen",
			projectName: "-project",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name must start with lowercase letter and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:        "name ending with hyphen",
			projectName: "project-",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name must start with lowercase letter and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:        "name with underscore",
			projectName: "my_project",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name must start with lowercase letter and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:        "name with spaces",
			projectName: "my project",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name must start with lowercase letter and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:        "name with special characters",
			projectName: "my@project",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name must start with lowercase letter and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:        "reserved name - con",
			projectName: "con",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name 'con' is reserved and cannot be used",
		},
		{
			name:        "reserved name - aux",
			projectName: "aux",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name 'aux' is reserved and cannot be used",
		},
		{
			name:        "reserved name - nul",
			projectName: "nul",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name 'nul' is reserved and cannot be used",
		},
		{
			name:        "reserved name - com1",
			projectName: "com1",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name 'com1' is reserved and cannot be used",
		},
		{
			name:        "reserved name - lpt1",
			projectName: "lpt1",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "project name 'lpt1' is reserved and cannot be used",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateProjectName(tt.projectName)

			if tt.expectError {
				require.Error(t, err)
				cliErr, ok := err.(*CliError)
				require.True(t, ok, "expected CliError")
				assert.Equal(t, tt.errorType, cliErr.Type)
				assert.Contains(t, cliErr.Message, tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultValidator_ValidateInputFile(t *testing.T) {
	validator := NewValidator()

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pericarp-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "test.yaml")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Create a test directory
	testDir := filepath.Join(tempDir, "testdir")
	err = os.Mkdir(testDir, 0755)
	require.NoError(t, err)

	// Create a file with restricted permissions (if not on Windows)
	restrictedFile := filepath.Join(tempDir, "restricted.yaml")
	err = os.WriteFile(restrictedFile, []byte("restricted content"), 0000)
	require.NoError(t, err)

	tests := []struct {
		name        string
		filePath    string
		expectError bool
		errorType   ErrorType
		errorMsg    string
	}{
		{
			name:        "valid existing file",
			filePath:    testFile,
			expectError: false,
		},
		{
			name:        "empty file path",
			filePath:    "",
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "input file path cannot be empty",
		},
		{
			name:        "non-existent file",
			filePath:    filepath.Join(tempDir, "nonexistent.yaml"),
			expectError: true,
			errorType:   FileSystemError,
			errorMsg:    "input file does not exist",
		},
		{
			name:        "directory instead of file",
			filePath:    testDir,
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "input path is a directory, not a file",
		},
		{
			name:        "file with restricted permissions",
			filePath:    restrictedFile,
			expectError: true,
			errorType:   FileSystemError,
			errorMsg:    "cannot read input file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateInputFile(tt.filePath)

			if tt.expectError {
				require.Error(t, err)
				cliErr, ok := err.(*CliError)
				require.True(t, ok, "expected CliError")
				assert.Equal(t, tt.errorType, cliErr.Type)
				assert.Contains(t, cliErr.Message, tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	// Clean up restricted file permissions for removal
	os.Chmod(restrictedFile, 0644)
}

func TestDefaultValidator_ValidateDestination(t *testing.T) {
	validator := NewValidator()

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pericarp-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a writable directory
	writableDir := filepath.Join(tempDir, "writable")
	err = os.Mkdir(writableDir, 0755)
	require.NoError(t, err)

	// Create a file (not a directory)
	testFile := filepath.Join(tempDir, "testfile.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Create a directory with restricted permissions
	restrictedDir := filepath.Join(tempDir, "restricted")
	err = os.Mkdir(restrictedDir, 0000)
	require.NoError(t, err)

	tests := []struct {
		name        string
		destination string
		expectError bool
		errorType   ErrorType
		errorMsg    string
	}{
		{
			name:        "empty destination (should use default)",
			destination: "",
			expectError: false,
		},
		{
			name:        "existing writable directory",
			destination: writableDir,
			expectError: false,
		},
		{
			name:        "new directory in existing parent",
			destination: filepath.Join(tempDir, "newdir"),
			expectError: false,
		},
		{
			name:        "existing file instead of directory",
			destination: testFile,
			expectError: true,
			errorType:   ValidationError,
			errorMsg:    "destination exists but is not a directory",
		},
		{
			name:        "directory with restricted permissions",
			destination: restrictedDir,
			expectError: true,
			errorType:   FileSystemError,
			errorMsg:    "destination directory is not writable",
		},
		{
			name:        "new directory in non-existent parent",
			destination: filepath.Join(tempDir, "nonexistent", "newdir"),
			expectError: true,
			errorType:   FileSystemError,
			errorMsg:    "destination parent directory does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateDestination(tt.destination)

			if tt.expectError {
				require.Error(t, err)
				cliErr, ok := err.(*CliError)
				require.True(t, ok, "expected CliError")
				assert.Equal(t, tt.errorType, cliErr.Type)
				assert.Contains(t, cliErr.Message, tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	// Clean up restricted directory permissions for removal
	os.Chmod(restrictedDir, 0755)
}

func TestDefaultValidator_ValidateDestination_AbsolutePath(t *testing.T) {
	validator := NewValidator()

	// Test with relative path
	relativePath := "relative/path"
	err := validator.ValidateDestination(relativePath)

	// Should handle relative paths by converting to absolute
	if err != nil {
		cliErr, ok := err.(*CliError)
		require.True(t, ok, "expected CliError")
		// Should be a filesystem error about parent directory not existing
		assert.Equal(t, FileSystemError, cliErr.Type)
	}
}

func TestDefaultValidator_ValidateDestination_WritePermissionTest(t *testing.T) {
	validator := NewValidator()

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pericarp-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test that the validator actually tries to write a test file
	writableDir := filepath.Join(tempDir, "writable")
	err = os.Mkdir(writableDir, 0755)
	require.NoError(t, err)

	// This should succeed and not leave any test files behind
	err = validator.ValidateDestination(writableDir)
	assert.NoError(t, err)

	// Verify no test files were left behind
	entries, err := os.ReadDir(writableDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no test files should be left behind")
}

func TestDefaultValidator_Integration(t *testing.T) {
	validator := NewValidator()

	// Create a temporary directory for integration testing
	tempDir, err := os.MkdirTemp("", "pericarp-integration-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a valid input file
	inputFile := filepath.Join(tempDir, "valid.yaml")
	err = os.WriteFile(inputFile, []byte("entities:\n  - name: User"), 0644)
	require.NoError(t, err)

	// Create a valid destination directory
	destDir := filepath.Join(tempDir, "output")
	err = os.Mkdir(destDir, 0755)
	require.NoError(t, err)

	// Test all validations together
	t.Run("all valid inputs", func(t *testing.T) {
		assert.NoError(t, validator.ValidateProjectName("my-project"))
		assert.NoError(t, validator.ValidateInputFile(inputFile))
		assert.NoError(t, validator.ValidateDestination(destDir))
	})

	t.Run("mixed valid and invalid inputs", func(t *testing.T) {
		// Valid project name, invalid file, valid destination
		assert.NoError(t, validator.ValidateProjectName("valid-project"))
		assert.Error(t, validator.ValidateInputFile(filepath.Join(tempDir, "nonexistent.yaml")))
		assert.NoError(t, validator.ValidateDestination(destDir))
	})
}

func TestNewValidator(t *testing.T) {
	validator := NewValidator()
	assert.NotNil(t, validator)

	// Test that it implements the Validator interface
	_, ok := validator.(Validator)
	assert.True(t, ok, "NewValidator should return a Validator interface")
}
