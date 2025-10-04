package domain

//go:generate moq -out mocks/logger_mock.go -pkg mocks . Logger

// Logger provides structured and formatted logging capabilities for the domain layer.
// The logger interface is designed to be implementation-agnostic, allowing different
// logging backends (logrus, zap, slog, etc.) to be used without changing domain code.
//
// The logger supports both structured logging (with key-value pairs) and traditional
// formatted logging for different use cases:
//
//   - Structured logging: Better for machine processing, monitoring, and searching
//   - Formatted logging: Better for human-readable messages and simple cases
//
// Example usage:
//
//	// Structured logging (preferred for production)
//	logger.Info("User created",
//	    "userId", user.GetID(),
//	    "email", user.Email(),
//	    "timestamp", time.Now())
//
//	// Formatted logging (good for development and simple messages)
//	logger.Infof("User %s created with email %s", user.GetID(), user.Email())
//
// Log levels follow standard conventions:
//   - Debug: Detailed information for diagnosing problems
//   - Info: General information about program execution
//   - Warn: Warning messages for potentially harmful situations
//   - Error: Error messages for error conditions that don't stop execution
//   - Fatal: Critical errors that cause program termination
type Logger interface {
	// Structured logging methods accept a message and key-value pairs.
	// Keys should be strings, values can be any type that can be serialized.
	// Key-value pairs should come in pairs (key1, value1, key2, value2, ...).

	// Debug logs detailed information typically only of interest when diagnosing problems.
	// Debug messages are usually disabled in production environments.
	//
	// Example: logger.Debug("Processing command", "commandType", cmd.Type(), "userId", userID)
	Debug(msg string, keysAndValues ...interface{})

	// Info logs general information about program execution.
	// Info messages provide insight into the normal operation of the application.
	//
	// Example: logger.Info("User registered", "userId", user.GetID(), "email", user.Email())
	Info(msg string, keysAndValues ...interface{})

	// Warn logs warning messages for potentially harmful situations.
	// Warnings indicate something unexpected happened but the application can continue.
	//
	// Example: logger.Warn("Deprecated API used", "endpoint", "/old-api", "userId", userID)
	Warn(msg string, keysAndValues ...interface{})

	// Error logs error messages for error conditions.
	// Errors indicate something went wrong but the application can continue running.
	//
	// Example: logger.Error("Failed to send email", "error", err, "userId", userID)
	Error(msg string, keysAndValues ...interface{})

	// Fatal logs critical error messages and typically causes program termination.
	// Fatal should be used sparingly, only for errors that make continued execution impossible.
	//
	// Example: logger.Fatal("Database connection failed", "error", err, "dsn", config.DSN)
	Fatal(msg string, keysAndValues ...interface{})

	// Formatted logging methods use printf-style formatting.
	// These are convenient for simple messages but structured logging is preferred
	// for production systems due to better searchability and machine processing.

	// Debugf logs a formatted debug message.
	// Use this for simple debug messages where structured logging is overkill.
	//
	// Example: logger.Debugf("Processing %d items for user %s", len(items), userID)
	Debugf(format string, args ...interface{})

	// Infof logs a formatted info message.
	// Use this for simple informational messages.
	//
	// Example: logger.Infof("User %s logged in successfully", userID)
	Infof(format string, args ...interface{})

	// Warnf logs a formatted warning message.
	// Use this for simple warning messages.
	//
	// Example: logger.Warnf("User %s attempted invalid operation: %s", userID, operation)
	Warnf(format string, args ...interface{})

	// Errorf logs a formatted error message.
	// Use this for simple error messages.
	//
	// Example: logger.Errorf("Failed to process order %s: %v", orderID, err)
	Errorf(format string, args ...interface{})

	// Fatalf logs a formatted fatal message and typically causes program termination.
	// Use this sparingly for critical errors.
	//
	// Example: logger.Fatalf("Cannot start server on port %d: %v", port, err)
	Fatalf(format string, args ...interface{})
}
