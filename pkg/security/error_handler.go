package security

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/example/pericarp/pkg/domain"
)

// SecurityErrorHandler provides centralized secure error handling
// that addresses CWE-703 by ensuring proper error handling and sanitization
type SecurityErrorHandler struct {
	logger    domain.Logger
	sanitizer *ErrorSanitizer
}

// NewSecurityErrorHandler creates a new SecurityErrorHandler instance
func NewSecurityErrorHandler(logger domain.Logger) *SecurityErrorHandler {
	return &SecurityErrorHandler{
		logger:    logger,
		sanitizer: NewErrorSanitizer(),
	}
}

// HandleSystemError handles system operation errors with proper logging and sanitization
func (h *SecurityErrorHandler) HandleSystemError(err error, operation string) error {
	if err == nil {
		return nil
	}

	// Log the full error for debugging (sanitized)
	sanitizedErr := h.sanitizer.Sanitize(err)
	h.logger.Error("System operation failed", 
		"operation", operation, 
		"error", sanitizedErr.Error(),
		"error_type", fmt.Sprintf("%T", err))

	// Return a sanitized error for user consumption
	return h.sanitizer.SanitizeForUser(err, operation)
}

// HandleValidationError handles input validation errors
func (h *SecurityErrorHandler) HandleValidationError(err error, input string) error {
	if err == nil {
		return nil
	}

	// Log validation failure (without exposing the actual input)
	h.logger.Warn("Input validation failed", 
		"error", err.Error(),
		"input_type", fmt.Sprintf("%T", input))

	// Return sanitized validation error
	return fmt.Errorf("validation failed: %w", err)
}

// HandleConfigurationError handles configuration-related errors
func (h *SecurityErrorHandler) HandleConfigurationError(err error, configKey string) error {
	if err == nil {
		return nil
	}

	// Log configuration error (without exposing sensitive config values)
	h.logger.Error("Configuration error", 
		"config_key", configKey,
		"error", h.sanitizer.Sanitize(err).Error())

	// Return generic configuration error to avoid information disclosure
	return fmt.Errorf("configuration error for key '%s'", configKey)
}

// ErrorSanitizer removes sensitive data from error messages
type ErrorSanitizer struct {
	sensitivePatterns []*regexp.Regexp
	redactionText     string
}

// NewErrorSanitizer creates a new ErrorSanitizer with default patterns
func NewErrorSanitizer() *ErrorSanitizer {
	patterns := []*regexp.Regexp{
		// Common password patterns
		regexp.MustCompile(`(?i)password[=:\s]+[^\s]+`),
		regexp.MustCompile(`(?i)pwd[=:\s]+[^\s]+`),
		regexp.MustCompile(`(?i)pass[=:\s]+[^\s]+`),
		
		// API keys and tokens
		regexp.MustCompile(`(?i)api[_-]?key[=:\s]+[^\s]+`),
		regexp.MustCompile(`(?i)token[=:\s]+[^\s]+`),
		regexp.MustCompile(`(?i)secret[=:\s]+[^\s]+`),
		
		// Database connection strings
		regexp.MustCompile(`(?i)://[^:]+:[^@]+@`), // user:pass@host pattern
		
		// File paths that might contain sensitive info
		regexp.MustCompile(`/home/[^/\s]+`),
		regexp.MustCompile(`/Users/[^/\s]+`),
		
		// Common sensitive environment variable patterns
		regexp.MustCompile(`(?i)[A-Z_]*SECRET[A-Z_]*[=:\s]+[^\s]+`),
		regexp.MustCompile(`(?i)[A-Z_]*KEY[A-Z_]*[=:\s]+[^\s]+`),
	}

	return &ErrorSanitizer{
		sensitivePatterns: patterns,
		redactionText:     "[REDACTED]",
	}
}

// Sanitize removes sensitive information from an error
func (s *ErrorSanitizer) Sanitize(err error) error {
	if err == nil {
		return nil
	}

	sanitized := err.Error()
	for _, pattern := range s.sensitivePatterns {
		sanitized = pattern.ReplaceAllString(sanitized, s.redactionText)
	}

	return fmt.Errorf("%s", sanitized)
}

// SanitizeForUser creates a user-friendly error message without sensitive details
func (s *ErrorSanitizer) SanitizeForUser(err error, operation string) error {
	if err == nil {
		return nil
	}

	// For user-facing errors, provide generic messages to avoid information disclosure
	switch {
	case strings.Contains(err.Error(), "permission denied"):
		return fmt.Errorf("insufficient permissions for operation: %s", operation)
	case strings.Contains(err.Error(), "no such file"):
		return fmt.Errorf("resource not found for operation: %s", operation)
	case strings.Contains(err.Error(), "connection refused"):
		return fmt.Errorf("service unavailable for operation: %s", operation)
	case strings.Contains(err.Error(), "timeout"):
		return fmt.Errorf("operation timed out: %s", operation)
	default:
		return fmt.Errorf("operation failed: %s", operation)
	}
}

// AddSensitivePattern adds a custom pattern to sanitize
func (s *ErrorSanitizer) AddSensitivePattern(pattern string) error {
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}
	
	s.sensitivePatterns = append(s.sensitivePatterns, compiled)
	return nil
}

// FailureRecovery provides secure failure recovery patterns
type FailureRecovery struct {
	logger domain.Logger
}

// NewFailureRecovery creates a new FailureRecovery instance
func NewFailureRecovery(logger domain.Logger) *FailureRecovery {
	return &FailureRecovery{
		logger: logger,
	}
}

// RecoverFromPanic recovers from panics and logs them securely
func (f *FailureRecovery) RecoverFromPanic(operation string) {
	if r := recover(); r != nil {
		f.logger.Error("Panic recovered", 
			"operation", operation,
			"panic", fmt.Sprintf("%v", r))
		
		// In a real application, you might want to:
		// 1. Send alerts
		// 2. Gracefully shutdown resources
		// 3. Return to a safe state
	}
}

// SafeExecute executes a function with panic recovery
func (f *FailureRecovery) SafeExecute(operation string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			f.logger.Error("Panic during safe execution", 
				"operation", operation,
				"panic", fmt.Sprintf("%v", r))
			err = fmt.Errorf("operation failed due to unexpected error: %s", operation)
		}
	}()
	
	return fn()
}