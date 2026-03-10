package application

import "context"

// Logger defines the interface for structured logging in the auth package.
type Logger interface {
	Info(ctx context.Context, msg string, keysAndValues ...interface{})
	Warn(ctx context.Context, msg string, keysAndValues ...interface{})
	Error(ctx context.Context, msg string, keysAndValues ...interface{})
}

// NoOpLogger is the default Logger that silently discards all log messages.
// It is exported so infrastructure packages can share the same no-op default.
type NoOpLogger struct{}

func (NoOpLogger) Info(_ context.Context, _ string, _ ...interface{})  {}
func (NoOpLogger) Warn(_ context.Context, _ string, _ ...interface{})  {}
func (NoOpLogger) Error(_ context.Context, _ string, _ ...interface{}) {}
