package infrastructure

import (
	"github.com/ThreeDotsLabs/watermill"
	"github.com/example/pericarp/pkg/domain"
)

// WatermillLoggerAdapter adapts our domain.Logger to watermill.LoggerAdapter
type WatermillLoggerAdapter struct {
	Logger domain.Logger
}

// Error logs an error message
func (w *WatermillLoggerAdapter) Error(msg string, err error, fields watermill.LogFields) {
	args := make([]interface{}, 0, len(fields)*2+2)
	args = append(args, "error", err)

	for key, value := range fields {
		args = append(args, key, value)
	}

	w.Logger.Error(msg, args...)
}

// Info logs an info message
func (w *WatermillLoggerAdapter) Info(msg string, fields watermill.LogFields) {
	args := make([]interface{}, 0, len(fields)*2)

	for key, value := range fields {
		args = append(args, key, value)
	}

	w.Logger.Info(msg, args...)
}

// Debug logs a debug message
func (w *WatermillLoggerAdapter) Debug(msg string, fields watermill.LogFields) {
	args := make([]interface{}, 0, len(fields)*2)

	for key, value := range fields {
		args = append(args, key, value)
	}

	w.Logger.Debug(msg, args...)
}

// Trace logs a trace message (mapped to debug since our logger doesn't have trace)
func (w *WatermillLoggerAdapter) Trace(msg string, fields watermill.LogFields) {
	args := make([]interface{}, 0, len(fields)*2)

	for key, value := range fields {
		args = append(args, key, value)
	}

	w.Logger.Debug(msg, args...)
}

// With returns a new logger with additional fields (not implemented for simplicity)
func (w *WatermillLoggerAdapter) With(fields watermill.LogFields) watermill.LoggerAdapter {
	// For simplicity, return the same adapter
	// In a real implementation, you might want to store the fields and include them in all log messages
	return w
}
