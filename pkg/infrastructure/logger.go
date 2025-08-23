package infrastructure

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/example/pericarp/pkg/domain"
)

// logLevel represents the logging level
type logLevel int

const (
	debugLevel logLevel = iota
	infoLevel
	warnLevel
	errorLevel
	fatalLevel
)

// logFormat represents the logging format
type logFormat int

const (
	textFormat logFormat = iota
	jsonFormat
)

// simpleLogger implements the domain.Logger interface
type simpleLogger struct {
	level  logLevel
	format logFormat
	logger *log.Logger
}

// NewLogger creates a new logger with the specified level and format
func NewLogger(level, format string) domain.Logger {
	logLevel := parseLogLevel(level)
	logFormat := parseLogFormat(format)
	
	logger := log.New(os.Stdout, "", 0) // No default prefix or flags
	
	return &simpleLogger{
		level:  logLevel,
		format: logFormat,
		logger: logger,
	}
}

// parseLogLevel converts string level to logLevel
func parseLogLevel(level string) logLevel {
	switch strings.ToLower(level) {
	case "debug":
		return debugLevel
	case "info":
		return infoLevel
	case "warn", "warning":
		return warnLevel
	case "error":
		return errorLevel
	case "fatal":
		return fatalLevel
	default:
		return infoLevel
	}
}

// parseLogFormat converts string format to logFormat
func parseLogFormat(format string) logFormat {
	switch strings.ToLower(format) {
	case "json":
		return jsonFormat
	case "text":
		return textFormat
	default:
		return textFormat
	}
}

// Debug logs a debug message with key-value pairs
func (l *simpleLogger) Debug(msg string, keysAndValues ...interface{}) {
	if l.level <= debugLevel {
		l.log("DEBUG", msg, keysAndValues...)
	}
}

// Debugf logs a formatted debug message
func (l *simpleLogger) Debugf(format string, args ...interface{}) {
	if l.level <= debugLevel {
		l.logf("DEBUG", format, args...)
	}
}

// Info logs an info message with key-value pairs
func (l *simpleLogger) Info(msg string, keysAndValues ...interface{}) {
	if l.level <= infoLevel {
		l.log("INFO", msg, keysAndValues...)
	}
}

// Infof logs a formatted info message
func (l *simpleLogger) Infof(format string, args ...interface{}) {
	if l.level <= infoLevel {
		l.logf("INFO", format, args...)
	}
}

// Warn logs a warning message with key-value pairs
func (l *simpleLogger) Warn(msg string, keysAndValues ...interface{}) {
	if l.level <= warnLevel {
		l.log("WARN", msg, keysAndValues...)
	}
}

// Warnf logs a formatted warning message
func (l *simpleLogger) Warnf(format string, args ...interface{}) {
	if l.level <= warnLevel {
		l.logf("WARN", format, args...)
	}
}

// Error logs an error message with key-value pairs
func (l *simpleLogger) Error(msg string, keysAndValues ...interface{}) {
	if l.level <= errorLevel {
		l.log("ERROR", msg, keysAndValues...)
	}
}

// Errorf logs a formatted error message
func (l *simpleLogger) Errorf(format string, args ...interface{}) {
	if l.level <= errorLevel {
		l.logf("ERROR", format, args...)
	}
}

// Fatal logs a fatal message with key-value pairs and exits
func (l *simpleLogger) Fatal(msg string, keysAndValues ...interface{}) {
	l.log("FATAL", msg, keysAndValues...)
	os.Exit(1)
}

// Fatalf logs a formatted fatal message and exits
func (l *simpleLogger) Fatalf(format string, args ...interface{}) {
	l.logf("FATAL", format, args...)
	os.Exit(1)
}

// log handles structured logging with key-value pairs
func (l *simpleLogger) log(level, msg string, keysAndValues ...interface{}) {
	timestamp := time.Now().Format(time.RFC3339)
	
	if l.format == jsonFormat {
		l.logJSON(timestamp, level, msg, keysAndValues...)
	} else {
		l.logText(timestamp, level, msg, keysAndValues...)
	}
}

// logf handles formatted logging
func (l *simpleLogger) logf(level, format string, args ...interface{}) {
	timestamp := time.Now().Format(time.RFC3339)
	msg := fmt.Sprintf(format, args...)
	
	if l.format == jsonFormat {
		l.logJSON(timestamp, level, msg)
	} else {
		l.logText(timestamp, level, msg)
	}
}

// logJSON outputs logs in JSON format
func (l *simpleLogger) logJSON(timestamp, level, msg string, keysAndValues ...interface{}) {
	jsonLog := fmt.Sprintf(`{"timestamp":"%s","level":"%s","message":"%s"`, timestamp, level, msg)
	
	// Add key-value pairs
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprintf("%v", keysAndValues[i])
			value := fmt.Sprintf("%v", keysAndValues[i+1])
			jsonLog += fmt.Sprintf(`,"%s":"%s"`, key, value)
		}
	}
	
	jsonLog += "}"
	l.logger.Println(jsonLog)
}

// logText outputs logs in text format
func (l *simpleLogger) logText(timestamp, level, msg string, keysAndValues ...interface{}) {
	logLine := fmt.Sprintf("[%s] %s: %s", timestamp, level, msg)
	
	// Add key-value pairs
	if len(keysAndValues) > 0 {
		var pairs []string
		for i := 0; i < len(keysAndValues); i += 2 {
			if i+1 < len(keysAndValues) {
				key := fmt.Sprintf("%v", keysAndValues[i])
				value := fmt.Sprintf("%v", keysAndValues[i+1])
				pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
			}
		}
		if len(pairs) > 0 {
			logLine += " " + strings.Join(pairs, " ")
		}
	}
	
	l.logger.Println(logLine)
}