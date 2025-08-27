package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// LogLevel represents different logging levels
type LogLevel int

const (
	LogLevelError LogLevel = iota
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelError:
		return "ERROR"
	case LogLevelWarn:
		return "WARN"
	case LogLevelInfo:
		return "INFO"
	case LogLevelDebug:
		return "DEBUG"
	default:
		return "UNKNOWN"
	}
}

// VerboseLogger provides detailed output for debugging (Requirement 9.2, 9.6)
type VerboseLogger struct {
	verbose   bool
	level     LogLevel
	writer    io.Writer
	errWriter io.Writer
	timestamp bool
	prefix    string
}

// NewVerboseLogger creates a new verbose logger with debug output to stdout
func NewVerboseLogger() *VerboseLogger {
	return &VerboseLogger{
		verbose:   false,
		level:     LogLevelInfo,
		writer:    os.Stdout,
		errWriter: os.Stderr,
		timestamp: false,
		prefix:    "",
	}
}

// SetVerbose enables or disables verbose logging (Requirement 9.2, 9.6)
func (v *VerboseLogger) SetVerbose(enabled bool) {
	v.verbose = enabled
	if enabled {
		v.level = LogLevelDebug
	} else {
		v.level = LogLevelInfo
	}
}

// IsVerbose returns whether verbose logging is enabled
func (v *VerboseLogger) IsVerbose() bool {
	return v.verbose
}

// SetLevel sets the minimum log level
func (v *VerboseLogger) SetLevel(level LogLevel) {
	v.level = level
}

// SetTimestamp enables or disables timestamp in log messages
func (v *VerboseLogger) SetTimestamp(enabled bool) {
	v.timestamp = enabled
}

// SetPrefix sets a prefix for all log messages
func (v *VerboseLogger) SetPrefix(prefix string) {
	v.prefix = prefix
}

// SetWriter sets the output writer for info and debug messages
func (v *VerboseLogger) SetWriter(w io.Writer) {
	v.writer = w
}

// SetErrorWriter sets the output writer for error and warning messages
func (v *VerboseLogger) SetErrorWriter(w io.Writer) {
	v.errWriter = w
}

// Structured logging methods (implementing domain.Logger interface)

// Debug logs detailed information (only when verbose is enabled) (Requirement 9.2)
func (v *VerboseLogger) Debug(msg string, keysAndValues ...interface{}) {
	if v.shouldLog(LogLevelDebug) {
		formatted := v.formatMessage(msg, keysAndValues...)
		v.writeLog(LogLevelDebug, formatted, v.writer)
	}
}

// Info logs general information
func (v *VerboseLogger) Info(msg string, keysAndValues ...interface{}) {
	if v.shouldLog(LogLevelInfo) {
		formatted := v.formatMessage(msg, keysAndValues...)
		v.writeLog(LogLevelInfo, formatted, v.writer)
	}
}

// Warn logs warning messages
func (v *VerboseLogger) Warn(msg string, keysAndValues ...interface{}) {
	if v.shouldLog(LogLevelWarn) {
		formatted := v.formatMessage(msg, keysAndValues...)
		v.writeLog(LogLevelWarn, formatted, v.errWriter)
	}
}

// Error logs error messages
func (v *VerboseLogger) Error(msg string, keysAndValues ...interface{}) {
	if v.shouldLog(LogLevelError) {
		formatted := v.formatMessage(msg, keysAndValues...)
		v.writeLog(LogLevelError, formatted, v.errWriter)
	}
}

// Fatal logs critical errors and exits
func (v *VerboseLogger) Fatal(msg string, keysAndValues ...interface{}) {
	formatted := v.formatMessage(msg, keysAndValues...)
	v.writeLog(LogLevelError, formatted, v.errWriter)
	os.Exit(1)
}

// Formatted logging methods (implementing domain.Logger interface)

// Debugf logs a formatted debug message (only when verbose is enabled)
func (v *VerboseLogger) Debugf(format string, args ...interface{}) {
	if v.shouldLog(LogLevelDebug) {
		message := fmt.Sprintf(format, args...)
		v.writeLog(LogLevelDebug, message, v.writer)
	}
}

// Infof logs a formatted info message
func (v *VerboseLogger) Infof(format string, args ...interface{}) {
	if v.shouldLog(LogLevelInfo) {
		message := fmt.Sprintf(format, args...)
		v.writeLog(LogLevelInfo, message, v.writer)
	}
}

// Warnf logs a formatted warning message
func (v *VerboseLogger) Warnf(format string, args ...interface{}) {
	if v.shouldLog(LogLevelWarn) {
		message := fmt.Sprintf(format, args...)
		v.writeLog(LogLevelWarn, message, v.errWriter)
	}
}

// Errorf logs a formatted error message
func (v *VerboseLogger) Errorf(format string, args ...interface{}) {
	if v.shouldLog(LogLevelError) {
		message := fmt.Sprintf(format, args...)
		v.writeLog(LogLevelError, message, v.errWriter)
	}
}

// Fatalf logs a formatted fatal message and exits
func (v *VerboseLogger) Fatalf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	v.writeLog(LogLevelError, message, v.errWriter)
	os.Exit(1)
}

// Enhanced logging methods for detailed generation process

// LogStep logs a step in the generation process with progress information
func (v *VerboseLogger) LogStep(step string, current, total int) {
	if v.verbose {
		progress := fmt.Sprintf("[%d/%d]", current, total)
		v.Debugf("%s %s", progress, step)
	}
}

// LogProgress logs progress with percentage
func (v *VerboseLogger) LogProgress(message string, current, total int) {
	if v.verbose {
		percentage := float64(current) / float64(total) * 100
		v.Debugf("%s: %.1f%% (%d/%d)", message, percentage, current, total)
	}
}

// LogDuration logs the duration of an operation
func (v *VerboseLogger) LogDuration(operation string, duration time.Duration) {
	if v.verbose {
		v.Debugf("%s completed in %v", operation, duration)
	}
}

// LogFileOperation logs file operations with details
func (v *VerboseLogger) LogFileOperation(operation, filePath string, size int) {
	if v.verbose {
		v.Debugf("%s: %s (size: %d bytes)", operation, filePath, size)
	}
}

// LogParsingDetails logs detailed parsing information
func (v *VerboseLogger) LogParsingDetails(format, filePath string, entityCount int) {
	if v.verbose {
		v.Debugf("Parsing %s file: %s", format, filePath)
		v.Debugf("Extracted %d entities from %s", entityCount, filePath)
	}
}

// LogGenerationDetails logs detailed generation information
func (v *VerboseLogger) LogGenerationDetails(component string, entity Entity) {
	if v.verbose {
		v.Debugf("Generating %s for entity: %s", component, entity.Name)
		v.Debugf("Entity properties: %d", len(entity.Properties))
		if len(entity.Methods) > 0 {
			v.Debugf("Entity methods: %d", len(entity.Methods))
		}
	}
}

// LogTemplateDetails logs template processing information
func (v *VerboseLogger) LogTemplateDetails(templateName, outputPath string) {
	if v.verbose {
		v.Debugf("Processing template: %s -> %s", templateName, outputPath)
	}
}

// LogValidationDetails logs validation information
func (v *VerboseLogger) LogValidationDetails(validationType, target string, success bool) {
	if v.verbose {
		status := "✓"
		if !success {
			status = "✗"
		}
		v.Debugf("%s %s validation: %s", status, validationType, target)
	}
}

// Helper methods

// shouldLog determines if a message should be logged based on the current level
func (v *VerboseLogger) shouldLog(level LogLevel) bool {
	return level <= v.level
}

// writeLog writes a log message with proper formatting
func (v *VerboseLogger) writeLog(level LogLevel, message string, writer io.Writer) {
	var parts []string

	// Add timestamp if enabled
	if v.timestamp {
		parts = append(parts, time.Now().Format("15:04:05"))
	}

	// Add prefix if set
	if v.prefix != "" {
		parts = append(parts, v.prefix)
	}

	// Add log level
	parts = append(parts, fmt.Sprintf("[%s]", level.String()))

	// Add message
	parts = append(parts, message)

	// Write the complete log line
	fmt.Fprintln(writer, strings.Join(parts, " "))
}

// formatMessage formats a message with key-value pairs for CLI output
func (v *VerboseLogger) formatMessage(msg string, keysAndValues ...interface{}) string {
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

// LogSection creates a visual section separator for better readability
func (v *VerboseLogger) LogSection(title string) {
	if v.verbose {
		separator := strings.Repeat("=", 50)
		v.Debug(separator)
		v.Debugf(" %s", title)
		v.Debug(separator)
	}
}

// LogSubSection creates a visual subsection separator
func (v *VerboseLogger) LogSubSection(title string) {
	if v.verbose {
		separator := strings.Repeat("-", 30)
		v.Debug(separator)
		v.Debugf(" %s", title)
		v.Debug(separator)
	}
}
