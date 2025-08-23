package application

import (
	"context"
	"fmt"
	"time"

	"github.com/example/pericarp/pkg/domain"
)

// Validator interface for commands and queries that can be validated
type Validator interface {
	Validate() error
}

// MetricsCollector interface for collecting metrics
type MetricsCollector interface {
	RecordCommandDuration(commandType string, duration time.Duration)
	RecordQueryDuration(queryType string, duration time.Duration)
	IncrementCommandErrors(commandType string)
	IncrementQueryErrors(queryType string)
}

// LoggingCommandMiddleware creates middleware that logs command execution
func LoggingCommandMiddleware() CommandMiddleware {
	return func(next CommandHandlerFunc) CommandHandlerFunc {
		return func(ctx context.Context, logger domain.Logger, cmd Command) error {
			start := time.Now()
			logger.Info("Executing command", "type", cmd.CommandType())

			err := next(ctx, logger, cmd)

			duration := time.Since(start)
			if err != nil {
				logger.Error("Command failed", "type", cmd.CommandType(), "duration", duration, "error", err)
			} else {
				logger.Info("Command completed", "type", cmd.CommandType(), "duration", duration)
			}

			return err
		}
	}
}

// LoggingQueryMiddleware creates middleware that logs query execution
func LoggingQueryMiddleware() QueryMiddleware {
	return func(next QueryHandlerFunc) QueryHandlerFunc {
		return func(ctx context.Context, logger domain.Logger, query Query) (interface{}, error) {
			start := time.Now()
			logger.Info("Executing query", "type", query.QueryType())

			result, err := next(ctx, logger, query)

			duration := time.Since(start)
			if err != nil {
				logger.Error("Query failed", "type", query.QueryType(), "duration", duration, "error", err)
			} else {
				logger.Info("Query completed", "type", query.QueryType(), "duration", duration)
			}

			return result, err
		}
	}
}

// ValidationCommandMiddleware creates middleware that validates commands
func ValidationCommandMiddleware() CommandMiddleware {
	return func(next CommandHandlerFunc) CommandHandlerFunc {
		return func(ctx context.Context, logger domain.Logger, cmd Command) error {
			if validator, ok := cmd.(Validator); ok {
				if err := validator.Validate(); err != nil {
					logger.Warn("Command validation failed", "type", cmd.CommandType(), "error", err)
					return NewValidationError("", err.Error())
				}
				logger.Debug("Command validation passed", "type", cmd.CommandType())
			}
			return next(ctx, logger, cmd)
		}
	}
}

// ValidationQueryMiddleware creates middleware that validates queries
func ValidationQueryMiddleware() QueryMiddleware {
	return func(next QueryHandlerFunc) QueryHandlerFunc {
		return func(ctx context.Context, logger domain.Logger, query Query) (interface{}, error) {
			if validator, ok := query.(Validator); ok {
				if err := validator.Validate(); err != nil {
					logger.Warn("Query validation failed", "type", query.QueryType(), "error", err)
					return nil, NewValidationError("", err.Error())
				}
				logger.Debug("Query validation passed", "type", query.QueryType())
			}
			return next(ctx, logger, query)
		}
	}
}

// MetricsCommandMiddleware creates middleware that collects command metrics
func MetricsCommandMiddleware(metrics MetricsCollector) CommandMiddleware {
	return func(next CommandHandlerFunc) CommandHandlerFunc {
		return func(ctx context.Context, logger domain.Logger, cmd Command) error {
			start := time.Now()
			err := next(ctx, logger, cmd)
			duration := time.Since(start)

			metrics.RecordCommandDuration(cmd.CommandType(), duration)
			if err != nil {
				metrics.IncrementCommandErrors(cmd.CommandType())
				logger.Error("Command failed with metrics recorded", "type", cmd.CommandType(), "duration", duration, "error", err)
			} else {
				logger.Debug("Command completed with metrics recorded", "type", cmd.CommandType(), "duration", duration)
			}

			return err
		}
	}
}

// MetricsQueryMiddleware creates middleware that collects query metrics
func MetricsQueryMiddleware(metrics MetricsCollector) QueryMiddleware {
	return func(next QueryHandlerFunc) QueryHandlerFunc {
		return func(ctx context.Context, logger domain.Logger, query Query) (interface{}, error) {
			start := time.Now()
			result, err := next(ctx, logger, query)
			duration := time.Since(start)

			metrics.RecordQueryDuration(query.QueryType(), duration)
			if err != nil {
				metrics.IncrementQueryErrors(query.QueryType())
				logger.Error("Query failed with metrics recorded", "type", query.QueryType(), "duration", duration, "error", err)
			} else {
				logger.Debug("Query completed with metrics recorded", "type", query.QueryType(), "duration", duration)
			}

			return result, err
		}
	}
}

// ErrorHandlingCommandMiddleware creates middleware that provides consistent error handling
func ErrorHandlingCommandMiddleware() CommandMiddleware {
	return func(next CommandHandlerFunc) CommandHandlerFunc {
		return func(ctx context.Context, logger domain.Logger, cmd Command) error {
			defer func() {
				if r := recover(); r != nil {
					logger.Fatal("Command handler panicked", "type", cmd.CommandType(), "panic", r)
				}
			}()

			err := next(ctx, logger, cmd)
			if err != nil {
				// Wrap non-application errors
				if _, ok := err.(ApplicationError); !ok {
					if _, ok := err.(ValidationError); !ok {
						if _, ok := err.(ConcurrencyError); !ok {
							logger.Error("Wrapping unexpected error", "type", cmd.CommandType(), "error", err)
							return NewApplicationError("COMMAND_ERROR", "Command execution failed", err)
						}
					}
				}
			}

			return err
		}
	}
}

// ErrorHandlingQueryMiddleware creates middleware that provides consistent error handling
func ErrorHandlingQueryMiddleware() QueryMiddleware {
	return func(next QueryHandlerFunc) QueryHandlerFunc {
		return func(ctx context.Context, logger domain.Logger, query Query) (interface{}, error) {
			defer func() {
				if r := recover(); r != nil {
					logger.Fatal("Query handler panicked", "type", query.QueryType(), "panic", r)
				}
			}()

			result, err := next(ctx, logger, query)
			if err != nil {
				// Wrap non-application errors
				if _, ok := err.(ApplicationError); !ok {
					if _, ok := err.(ValidationError); !ok {
						logger.Error("Wrapping unexpected error", "type", query.QueryType(), "error", err)
						return nil, NewApplicationError("QUERY_ERROR", "Query execution failed", err)
					}
				}
			}

			return result, err
		}
	}
}

// CacheProvider interface for caching query results
type CacheProvider interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{})
	Delete(key string)
}

// CachingQueryMiddleware creates middleware that caches query results
func CachingQueryMiddleware(cache CacheProvider) QueryMiddleware {
	return func(next QueryHandlerFunc) QueryHandlerFunc {
		return func(ctx context.Context, logger domain.Logger, query Query) (interface{}, error) {
			// Create cache key based on query type and content
			cacheKey := generateCacheKey(query)

			// Try to get from cache first
			if cached, found := cache.Get(cacheKey); found {
				logger.Debug("Query result found in cache", "type", query.QueryType(), "cache_key", cacheKey)
				return cached, nil
			}

			// Execute query
			result, err := next(ctx, logger, query)
			if err != nil {
				return result, err
			}

			// Cache the result
			cache.Set(cacheKey, result)
			logger.Debug("Query result cached", "type", query.QueryType(), "cache_key", cacheKey)

			return result, nil
		}
	}
}

// generateCacheKey creates a cache key for a query
func generateCacheKey(query Query) string {
	// Simple implementation - in production you might want to use JSON marshaling
	// or a more sophisticated key generation strategy
	return query.QueryType() + "_" + fmt.Sprintf("%+v", query)
}
