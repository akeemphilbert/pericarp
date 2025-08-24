package security

import (
	"os"
	"testing"
)

// TestEnvironmentVariableErrorHandling tests that environment variable operations
// properly handle and report errors, addressing CWE-703 requirements.
func TestEnvironmentVariableErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		expectError bool
	}{
		{
			name:        "valid environment variable",
			key:         "TEST_VAR",
			value:       "test_value",
			expectError: false,
		},
		{
			name:        "empty key should not cause panic",
			key:         "",
			value:       "test_value",
			expectError: false, // os.Setenv allows empty keys
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that os.Setenv error is properly checked
			err := os.Setenv(tt.key, tt.value)
			
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			// Cleanup
			if err == nil && tt.key != "" {
				if cleanupErr := os.Unsetenv(tt.key); cleanupErr != nil {
					t.Logf("Warning: Failed to cleanup environment variable %s: %v", tt.key, cleanupErr)
				}
			}
		})
	}
}

// TestFileOperationErrorHandling tests that file operations properly handle errors
func TestFileOperationErrorHandling(t *testing.T) {
	// Test removing non-existent file
	err := os.Remove("/non/existent/file")
	if err == nil {
		t.Error("Expected error when removing non-existent file, but got none")
	}
	
	// This demonstrates proper error checking - the error is expected and handled
	t.Logf("Expected error when removing non-existent file: %v", err)
}