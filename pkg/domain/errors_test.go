package domain

import (
	"errors"
	"testing"
)

func TestDomainError(t *testing.T) {
	t.Run("create domain error with code and message", func(t *testing.T) {
		code := "INVALID_EMAIL"
		message := "Email format is invalid"

		err := NewDomainError(code, message, nil)

		if err.Code != code {
			t.Errorf("expected code '%s', got '%s'", code, err.Code)
		}

		if err.Message != message {
			t.Errorf("expected message '%s', got '%s'", message, err.Message)
		}

		if err.Cause != nil {
			t.Errorf("expected nil cause, got %v", err.Cause)
		}
	})

	t.Run("create domain error with cause", func(t *testing.T) {
		code := "VALIDATION_FAILED"
		message := "User validation failed"
		cause := errors.New("email is required")

		err := NewDomainError(code, message, cause)

		if err.Code != code {
			t.Errorf("expected code '%s', got '%s'", code, err.Code)
		}

		if err.Message != message {
			t.Errorf("expected message '%s', got '%s'", message, err.Message)
		}

		if err.Cause != cause {
			t.Errorf("expected cause '%v', got '%v'", cause, err.Cause)
		}
	})

	t.Run("domain error should implement error interface", func(t *testing.T) {
		err := NewDomainError("TEST_ERROR", "Test error message", nil)

		var _ error = err // This should compile if DomainError implements error

		expectedErrorString := "TEST_ERROR: Test error message"
		if err.Error() != expectedErrorString {
			t.Errorf("expected error string '%s', got '%s'", expectedErrorString, err.Error())
		}
	})

	t.Run("domain error with cause should include cause in error string", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := NewDomainError("TEST_ERROR", "Test error message", cause)

		expectedErrorString := "TEST_ERROR: Test error message (caused by: underlying error)"
		if err.Error() != expectedErrorString {
			t.Errorf("expected error string '%s', got '%s'", expectedErrorString, err.Error())
		}
	})
}

func TestValidationError(t *testing.T) {
	t.Run("create validation error", func(t *testing.T) {
		field := "email"
		message := "Email is required"
		value := ""

		err := NewValidationError(field, message, value)

		if err.Field != field {
			t.Errorf("expected field '%s', got '%s'", field, err.Field)
		}

		if err.Message != message {
			t.Errorf("expected message '%s', got '%s'", message, err.Message)
		}

		if err.Value != value {
			t.Errorf("expected value '%v', got '%v'", value, err.Value)
		}
	})

	t.Run("validation error should implement error interface", func(t *testing.T) {
		err := NewValidationError("email", "Email is required", "")

		var _ error = err // This should compile if ValidationError implements error

		expectedErrorString := "validation failed for field 'email': Email is required"
		if err.Error() != expectedErrorString {
			t.Errorf("expected error string '%s', got '%s'", expectedErrorString, err.Error())
		}
	})
}

func TestConcurrencyError(t *testing.T) {
	t.Run("create concurrency error", func(t *testing.T) {
		aggregateID := "user-123"
		expectedVersion := 5
		actualVersion := 3

		err := NewConcurrencyError(aggregateID, expectedVersion, actualVersion)

		if err.AggregateID != aggregateID {
			t.Errorf("expected aggregate GetID '%s', got '%s'", aggregateID, err.AggregateID)
		}

		if err.Expected != expectedVersion {
			t.Errorf("expected version %d, got %d", expectedVersion, err.Expected)
		}

		if err.Actual != actualVersion {
			t.Errorf("expected actual version %d, got %d", actualVersion, err.Actual)
		}
	})

	t.Run("concurrency error should implement error interface", func(t *testing.T) {
		err := NewConcurrencyError("user-123", 5, 3)

		var _ error = err // This should compile if ConcurrencyError implements error

		expectedErrorString := "concurrency conflict for aggregate 'user-123': expected version 5, but got 3"
		if err.Error() != expectedErrorString {
			t.Errorf("expected error string '%s', got '%s'", expectedErrorString, err.Error())
		}
	})
}

func TestErrorTypes(t *testing.T) {
	t.Run("different error types should be distinguishable", func(t *testing.T) {
		domainErr := NewDomainError("DOMAIN_ERROR", "Domain error", nil)
		validationErr := NewValidationError("field", "Validation error", nil)
		concurrencyErr := NewConcurrencyError("aggregate-123", 2, 1)

		// Test that they implement error interface
		var _ error = domainErr
		var _ error = validationErr
		var _ error = concurrencyErr

		// Test that error messages are different
		if domainErr.Error() == validationErr.Error() {
			t.Errorf("domain error and validation error should have different messages")
		}

		if validationErr.Error() == concurrencyErr.Error() {
			t.Errorf("validation error and concurrency error should have different messages")
		}

		if domainErr.Error() == concurrencyErr.Error() {
			t.Errorf("domain error and concurrency error should have different messages")
		}
	})
}

func TestErrorWrapping(t *testing.T) {
	t.Run("domain error should wrap underlying cause", func(t *testing.T) {
		originalErr := errors.New("database connection failed")
		domainErr := NewDomainError("DATABASE_ERROR", "Failed to save user", originalErr)

		// Test that the cause is preserved
		if domainErr.Cause != originalErr {
			t.Errorf("expected cause to be preserved")
		}

		// Test that error message includes the cause
		errorString := domainErr.Error()
		expectedSubstring := "database connection failed"
		if !containsString(errorString, expectedSubstring) {
			t.Errorf("error string should contain cause message")
		}
	})

	t.Run("domain error without cause should not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("creating domain error without cause should not panic")
			}
		}()

		err := NewDomainError("TEST_ERROR", "Test message", nil)
		_ = err.Error() // This should not panic
	})
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) != -1
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
