package security

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// mockLogger implements domain.Logger for testing
type mockLogger struct {
	logs []logEntry
}

type logEntry struct {
	level   string
	message string
	fields  map[string]interface{}
}

func (m *mockLogger) Debug(msg string, keysAndValues ...interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "debug",
		message: msg,
		fields:  parseKeysAndValues(keysAndValues...),
	})
}

func (m *mockLogger) Info(msg string, keysAndValues ...interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "info",
		message: msg,
		fields:  parseKeysAndValues(keysAndValues...),
	})
}

func (m *mockLogger) Warn(msg string, keysAndValues ...interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "warn",
		message: msg,
		fields:  parseKeysAndValues(keysAndValues...),
	})
}

func (m *mockLogger) Error(msg string, keysAndValues ...interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "error",
		message: msg,
		fields:  parseKeysAndValues(keysAndValues...),
	})
}

func (m *mockLogger) Fatal(msg string, keysAndValues ...interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "fatal",
		message: msg,
		fields:  parseKeysAndValues(keysAndValues...),
	})
}

func (m *mockLogger) Debugf(format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "debug",
		message: fmt.Sprintf(format, args...),
		fields:  make(map[string]interface{}),
	})
}

func (m *mockLogger) Infof(format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "info",
		message: fmt.Sprintf(format, args...),
		fields:  make(map[string]interface{}),
	})
}

func (m *mockLogger) Warnf(format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "warn",
		message: fmt.Sprintf(format, args...),
		fields:  make(map[string]interface{}),
	})
}

func (m *mockLogger) Errorf(format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "error",
		message: fmt.Sprintf(format, args...),
		fields:  make(map[string]interface{}),
	})
}

func (m *mockLogger) Fatalf(format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "fatal",
		message: fmt.Sprintf(format, args...),
		fields:  make(map[string]interface{}),
	})
}

func parseKeysAndValues(keysAndValues ...interface{}) map[string]interface{} {
	fields := make(map[string]interface{})
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprintf("%v", keysAndValues[i])
			fields[key] = keysAndValues[i+1]
		}
	}
	return fields
}

func TestSecurityErrorHandler_HandleSystemError(t *testing.T) {
	logger := &mockLogger{}
	handler := NewSecurityErrorHandler(logger)

	tests := []struct {
		name      string
		err       error
		operation string
		wantNil   bool
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			operation: "test_operation",
			wantNil:   true,
		},
		{
			name:      "system error is handled and sanitized",
			err:       errors.New("permission denied: /home/user/secret.txt"),
			operation: "file_read",
			wantNil:   false,
		},
		{
			name:      "error with sensitive data is sanitized",
			err:       errors.New("connection failed: password=secret123"),
			operation: "database_connect",
			wantNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.HandleSystemError(tt.err, tt.operation)
			
			if tt.wantNil && result != nil {
				t.Errorf("Expected nil error, got: %v", result)
			}
			
			if !tt.wantNil && result == nil {
				t.Error("Expected non-nil error, got nil")
			}
			
			if !tt.wantNil {
				// Check that sensitive data is not in the returned error
				if strings.Contains(result.Error(), "secret123") {
					t.Error("Sensitive data found in returned error")
				}
				if strings.Contains(result.Error(), "/home/user") {
					t.Error("Sensitive path found in returned error")
				}
			}
		})
	}
}

func TestSecurityErrorHandler_HandleValidationError(t *testing.T) {
	logger := &mockLogger{}
	handler := NewSecurityErrorHandler(logger)

	tests := []struct {
		name    string
		err     error
		input   string
		wantNil bool
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			input:   "test_input",
			wantNil: true,
		},
		{
			name:    "validation error is handled",
			err:     errors.New("invalid email format"),
			input:   "user@example.com",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.HandleValidationError(tt.err, tt.input)
			
			if tt.wantNil && result != nil {
				t.Errorf("Expected nil error, got: %v", result)
			}
			
			if !tt.wantNil && result == nil {
				t.Error("Expected non-nil error, got nil")
			}
			
			if !tt.wantNil && !strings.Contains(result.Error(), "validation failed") {
				t.Error("Expected validation error message")
			}
		})
	}
}

func TestSecurityErrorHandler_HandleConfigurationError(t *testing.T) {
	logger := &mockLogger{}
	handler := NewSecurityErrorHandler(logger)

	tests := []struct {
		name      string
		err       error
		configKey string
		wantNil   bool
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			configKey: "test_key",
			wantNil:   true,
		},
		{
			name:      "configuration error is handled",
			err:       errors.New("invalid configuration value: secret=password123"),
			configKey: "database.password",
			wantNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.HandleConfigurationError(tt.err, tt.configKey)
			
			if tt.wantNil && result != nil {
				t.Errorf("Expected nil error, got: %v", result)
			}
			
			if !tt.wantNil && result == nil {
				t.Error("Expected non-nil error, got nil")
			}
			
			if !tt.wantNil {
				// Check that sensitive data is not in the returned error
				if strings.Contains(result.Error(), "password123") {
					t.Error("Sensitive data found in returned error")
				}
				// Check that it contains the config key
				if !strings.Contains(result.Error(), tt.configKey) {
					t.Error("Expected config key in error message")
				}
			}
		})
	}
}

func TestErrorSanitizer_Sanitize(t *testing.T) {
	sanitizer := NewErrorSanitizer()

	tests := []struct {
		name     string
		err      error
		contains []string // strings that should NOT be in the result
	}{
		{
			name:     "nil error returns nil",
			err:      nil,
			contains: nil,
		},
		{
			name:     "password in error is redacted",
			err:      errors.New("authentication failed: password=secret123"),
			contains: []string{"secret123"},
		},
		{
			name:     "API key in error is redacted",
			err:      errors.New("API request failed: api_key=abc123def456"),
			contains: []string{"abc123def456"},
		},
		{
			name:     "database connection string is redacted",
			err:      errors.New("connection failed: postgres://user:pass@localhost/db"),
			contains: []string{"user:pass"},
		},
		{
			name:     "file path is redacted",
			err:      errors.New("file not found: /home/user/secret.txt"),
			contains: []string{"/home/user"},
		},
		{
			name:     "multiple sensitive patterns are redacted",
			err:      errors.New("error: password=secret token=abc123 /home/user/file"),
			contains: []string{"secret", "abc123", "/home/user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizer.Sanitize(tt.err)
			
			if tt.err == nil && result != nil {
				t.Error("Expected nil result for nil error")
				return
			}
			
			if tt.err != nil && result == nil {
				t.Error("Expected non-nil result for non-nil error")
				return
			}
			
			if result != nil {
				resultStr := result.Error()
				for _, sensitive := range tt.contains {
					if strings.Contains(resultStr, sensitive) {
						t.Errorf("Sensitive data '%s' found in sanitized error: %s", sensitive, resultStr)
					}
				}
				
				// Check that redaction text is present
				if len(tt.contains) > 0 && !strings.Contains(resultStr, "[REDACTED]") {
					t.Error("Expected [REDACTED] text in sanitized error")
				}
			}
		})
	}
}

func TestErrorSanitizer_SanitizeForUser(t *testing.T) {
	sanitizer := NewErrorSanitizer()

	tests := []struct {
		name      string
		err       error
		operation string
		wantNil   bool
		contains  string
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			operation: "test",
			wantNil:   true,
		},
		{
			name:      "permission denied error",
			err:       errors.New("permission denied: access to /secret/file"),
			operation: "file_read",
			contains:  "insufficient permissions",
		},
		{
			name:      "file not found error",
			err:       errors.New("no such file or directory"),
			operation: "file_open",
			contains:  "resource not found",
		},
		{
			name:      "connection refused error",
			err:       errors.New("connection refused"),
			operation: "database_connect",
			contains:  "service unavailable",
		},
		{
			name:      "timeout error",
			err:       errors.New("operation timeout"),
			operation: "api_call",
			contains:  "operation timed out",
		},
		{
			name:      "generic error",
			err:       errors.New("some other error"),
			operation: "generic_op",
			contains:  "operation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizer.SanitizeForUser(tt.err, tt.operation)
			
			if tt.wantNil && result != nil {
				t.Errorf("Expected nil error, got: %v", result)
			}
			
			if !tt.wantNil && result == nil {
				t.Error("Expected non-nil error, got nil")
			}
			
			if !tt.wantNil && !strings.Contains(result.Error(), tt.contains) {
				t.Errorf("Expected error to contain '%s', got: %s", tt.contains, result.Error())
			}
			
			if !tt.wantNil && !strings.Contains(result.Error(), tt.operation) {
				t.Errorf("Expected error to contain operation '%s', got: %s", tt.operation, result.Error())
			}
		})
	}
}

func TestErrorSanitizer_AddSensitivePattern(t *testing.T) {
	sanitizer := NewErrorSanitizer()

	tests := []struct {
		name        string
		pattern     string
		expectError bool
	}{
		{
			name:        "valid regex pattern",
			pattern:     `custom_secret[=:\s]+[^\s]+`,
			expectError: false,
		},
		{
			name:        "invalid regex pattern",
			pattern:     `[invalid`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sanitizer.AddSensitivePattern(tt.pattern)
			
			if tt.expectError && err == nil {
				t.Error("Expected error for invalid pattern")
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			// Test that the pattern works if it was added successfully
			if !tt.expectError && err == nil {
				testErr := errors.New("error with custom_secret=mysecret")
				sanitized := sanitizer.Sanitize(testErr)
				if strings.Contains(sanitized.Error(), "mysecret") {
					t.Error("Custom pattern did not sanitize the error")
				}
			}
		})
	}
}

func TestFailureRecovery_SafeExecute(t *testing.T) {
	logger := &mockLogger{}
	recovery := NewFailureRecovery(logger)

	tests := []struct {
		name        string
		fn          func() error
		expectError bool
		expectPanic bool
	}{
		{
			name: "successful execution",
			fn: func() error {
				return nil
			},
			expectError: false,
			expectPanic: false,
		},
		{
			name: "function returns error",
			fn: func() error {
				return errors.New("function error")
			},
			expectError: true,
			expectPanic: false,
		},
		{
			name: "function panics",
			fn: func() error {
				panic("test panic")
			},
			expectError: true,
			expectPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := recovery.SafeExecute("test_operation", tt.fn)
			
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			if tt.expectPanic {
				// Check that panic was logged
				found := false
				for _, log := range logger.logs {
					if log.level == "error" && strings.Contains(log.message, "Panic during safe execution") {
						found = true
						break
					}
				}
				if !found {
					t.Error("Expected panic to be logged")
				}
			}
		})
	}
}