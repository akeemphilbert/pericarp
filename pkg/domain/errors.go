package domain

import "fmt"

// DomainError represents a business rule violation or domain-specific error
type DomainError struct {
	Code    string
	Message string
	Cause   error
}

// Error implements the error interface
func (e DomainError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause error
func (e DomainError) Unwrap() error {
	return e.Cause
}

// NewDomainError creates a new domain error
func NewDomainError(code, message string, cause error) DomainError {
	return DomainError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// ValidationError represents a validation failure in the domain
type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}

// Error implements the error interface
func (e ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation failed for field '%s': %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation failed: %s", e.Message)
}

// NewValidationError creates a new validation error
func NewValidationError(field, message string, value interface{}) ValidationError {
	return ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
	}
}

// ConcurrencyError represents an optimistic concurrency control violation
type ConcurrencyError struct {
	AggregateID string
	Expected    int
	Actual      int
}

// Error implements the error interface
func (e ConcurrencyError) Error() string {
	return fmt.Sprintf("concurrency conflict for aggregate '%s': expected version %d, but got %d",
		e.AggregateID, e.Expected, e.Actual)
}

// NewConcurrencyError creates a new concurrency error
func NewConcurrencyError(aggregateID string, expected, actual int) ConcurrencyError {
	return ConcurrencyError{
		AggregateID: aggregateID,
		Expected:    expected,
		Actual:      actual,
	}
}
