package application

import "fmt"

// ApplicationError represents an error that occurred in the application layer
type ApplicationError struct {
	Code    string
	Message string
	Cause   error
}

func (e ApplicationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e ApplicationError) Unwrap() error {
	return e.Cause
}

// ValidationError represents a validation error in the application layer
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}

// ConcurrencyError represents a concurrency conflict error
type ConcurrencyError struct {
	AggregateID string
	Expected    int
	Actual      int
}

func (e ConcurrencyError) Error() string {
	return fmt.Sprintf("concurrency error for aggregate %s: expected version %d, got %d",
		e.AggregateID, e.Expected, e.Actual)
}

// HandlerNotFoundError represents an error when no handler is found for a command or query
type HandlerNotFoundError struct {
	Type string
	Kind string // "command" or "query"
}

func (e HandlerNotFoundError) Error() string {
	return fmt.Sprintf("no %s handler registered for type: %s", e.Kind, e.Type)
}

// NewApplicationError creates a new application error
func NewApplicationError(code, message string, cause error) ApplicationError {
	return ApplicationError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// NewValidationError creates a new validation error
func NewValidationError(field, message string) ValidationError {
	return ValidationError{
		Field:   field,
		Message: message,
	}
}

// NewConcurrencyError creates a new concurrency error
func NewConcurrencyError(aggregateID string, expected, actual int) ConcurrencyError {
	return ConcurrencyError{
		AggregateID: aggregateID,
		Expected:    expected,
		Actual:      actual,
	}
}

// NewHandlerNotFoundError creates a new handler not found error
func NewHandlerNotFoundError(handlerType, kind string) HandlerNotFoundError {
	return HandlerNotFoundError{
		Type: handlerType,
		Kind: kind,
	}
}
