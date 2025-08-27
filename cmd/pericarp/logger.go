package main

import (
	"fmt"
	"os"
)

// DefaultCliLogger implements CliLogger interface
type DefaultCliLogger struct {
	verbose bool
}

// NewCliLogger creates a new CLI logger instance
func NewCliLogger() CliLogger {
	return &DefaultCliLogger{
		verbose: false,
	}
}

// Structured logging methods (implementing domain.Logger interface)

// Debug logs detailed information (only when verbose is enabled) (Requirement 9.2)
func (l *DefaultCliLogger) Debug(msg string, keysAndValues ...interface{}) {
	if l.verbose {
		formatted := l.formatMessage(msg, keysAndValues...)
		fmt.Fprintf(os.Stdout, "[DEBUG] %s\n", formatted)
	}
}

// Info logs general information
func (l *DefaultCliLogger) Info(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage(msg, keysAndValues...)
	fmt.Fprintf(os.Stdout, "[INFO] %s\n", formatted)
}

// Warn logs warning messages
func (l *DefaultCliLogger) Warn(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage(msg, keysAndValues...)
	fmt.Fprintf(os.Stderr, "[WARN] %s\n", formatted)
}

// Error logs error messages
func (l *DefaultCliLogger) Error(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage(msg, keysAndValues...)
	fmt.Fprintf(os.Stderr, "[ERROR] %s\n", formatted)
}

// Fatal logs critical errors and exits
func (l *DefaultCliLogger) Fatal(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage(msg, keysAndValues...)
	fmt.Fprintf(os.Stderr, "[FATAL] %s\n", formatted)
	os.Exit(1)
}

// Formatted logging methods (implementing domain.Logger interface)

// Debugf logs a formatted debug message (only when verbose is enabled)
func (l *DefaultCliLogger) Debugf(format string, args ...interface{}) {
	if l.verbose {
		fmt.Fprintf(os.Stdout, "[DEBUG] "+format+"\n", args...)
	}
}

// Infof logs a formatted info message
func (l *DefaultCliLogger) Infof(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "[INFO] "+format+"\n", args...)
}

// Warnf logs a formatted warning message
func (l *DefaultCliLogger) Warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
}

// Errorf logs a formatted error message
func (l *DefaultCliLogger) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
}

// Fatalf logs a formatted fatal message and exits
func (l *DefaultCliLogger) Fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[FATAL] "+format+"\n", args...)
	os.Exit(1)
}

// CLI-specific methods

// SetVerbose enables or disables verbose logging (Requirement 9.2, 9.6)
func (l *DefaultCliLogger) SetVerbose(enabled bool) {
	l.verbose = enabled
}

// IsVerbose returns whether verbose logging is enabled
func (l *DefaultCliLogger) IsVerbose() bool {
	return l.verbose
}

// formatMessage formats a message with key-value pairs for CLI output
func (l *DefaultCliLogger) formatMessage(msg string, keysAndValues ...interface{}) string {
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
