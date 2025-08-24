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

// MetricsCollector interface for collecting metrics with unified approach
type MetricsCollector interface {
	RecordRequestDuration(requestType string, duration time.Duration)
	IncrementRequestErrors(requestType string)
}

// LoggingMiddleware creates unified middleware that logs both command and query execution
func LoggingMiddleware[Req any, Res any]() Middleware[Req, Res] {
	return func(next Handler[Req, Res]) Handler[Req, Res] {
		return func(ctx context.Context, log domain.Logger, p Payload[Req]) (Response[Res], error) {
			start := time.Now()

			// Extract request type for logging
			var requestType string
			if cmd, ok := any(p.Data).(Command); ok {
				requestType = cmd.CommandType()
			} else if query, ok := any(p.Data).(Query); ok {
				requestType = query.QueryType()
			} else {
				requestType = fmt.Sprintf("%T", p.Data)
			}

			log.Info("Processing request",
				"type", requestType,
				"traceId", p.TraceID,
				"userId", p.UserID)

			response, err := next(ctx, log, p)

			duration := time.Since(start)
			if err != nil {
				log.Error("Request failed",
					"type", requestType,
					"duration", duration,
					"error", err,
					"traceId", p.TraceID)
			} else {
				log.Info("Request completed",
					"type", requestType,
					"duration", duration,
					"traceId", p.TraceID)
			}

			return response, err
		}
	}
}

// ValidationMiddleware creates unified middleware that validates both commands and queries
func ValidationMiddleware[Req any, Res any]() Middleware[Req, Res] {
	return func(next Handler[Req, Res]) Handler[Req, Res] {
		return func(ctx context.Context, log domain.Logger, p Payload[Req]) (Response[Res], error) {
			if validator, ok := any(p.Data).(Validator); ok {
				if err := validator.Validate(); err != nil {
					var requestType string
					if cmd, ok := any(p.Data).(Command); ok {
						requestType = cmd.CommandType()
					} else if query, ok := any(p.Data).(Query); ok {
						requestType = query.QueryType()
					} else {
						requestType = fmt.Sprintf("%T", p.Data)
					}

					log.Warn("Request validation failed",
						"type", requestType,
						"error", err,
						"traceId", p.TraceID)

					var zero Res
					return Response[Res]{
						Data:  zero,
						Error: NewValidationError("", err.Error()),
						Metadata: map[string]any{
							"validation_failed": true,
						},
					}, NewValidationError("", err.Error())
				}
			}
			return next(ctx, log, p)
		}
	}
}

// MetricsMiddleware creates unified middleware that collects metrics for both commands and queries
func MetricsMiddleware[Req any, Res any](metrics MetricsCollector) Middleware[Req, Res] {
	return func(next Handler[Req, Res]) Handler[Req, Res] {
		return func(ctx context.Context, log domain.Logger, p Payload[Req]) (Response[Res], error) {
			start := time.Now()

			var requestType string
			if cmd, ok := any(p.Data).(Command); ok {
				requestType = cmd.CommandType()
			} else if query, ok := any(p.Data).(Query); ok {
				requestType = query.QueryType()
			} else {
				requestType = fmt.Sprintf("%T", p.Data)
			}

			response, err := next(ctx, log, p)
			duration := time.Since(start)

			metrics.RecordRequestDuration(requestType, duration)
			if err != nil {
				metrics.IncrementRequestErrors(requestType)
				log.Error("Request failed with metrics recorded",
					"type", requestType,
					"duration", duration,
					"error", err,
					"traceId", p.TraceID)
			} else {
				log.Debug("Request completed with metrics recorded",
					"type", requestType,
					"duration", duration,
					"traceId", p.TraceID)
			}

			return response, err
		}
	}
}

// ErrorHandlingMiddleware creates unified middleware that provides consistent error handling
func ErrorHandlingMiddleware[Req any, Res any]() Middleware[Req, Res] {
	return func(next Handler[Req, Res]) Handler[Req, Res] {
		return func(ctx context.Context, log domain.Logger, p Payload[Req]) (Response[Res], error) {
			var requestType string
			if cmd, ok := any(p.Data).(Command); ok {
				requestType = cmd.CommandType()
			} else if query, ok := any(p.Data).(Query); ok {
				requestType = query.QueryType()
			} else {
				requestType = fmt.Sprintf("%T", p.Data)
			}

			defer func() {
				if r := recover(); r != nil {
					log.Fatal("Handler panicked",
						"type", requestType,
						"panic", r,
						"traceId", p.TraceID)
				}
			}()

			response, err := next(ctx, log, p)
			if err != nil {
				// Wrap non-application errors
				if _, ok := err.(ApplicationError); !ok {
					if _, ok := err.(ValidationError); !ok {
						if _, ok := err.(ConcurrencyError); !ok {
							log.Error("Wrapping unexpected error",
								"type", requestType,
								"error", err,
								"traceId", p.TraceID)

							wrappedErr := NewApplicationError("REQUEST_ERROR", "Request execution failed", err)
							response.Error = wrappedErr
							return response, wrappedErr
						}
					}
				}
			}

			return response, err
		}
	}
}

// CacheProvider interface for caching query results
type CacheProvider interface {
	Get(key string) (any, bool)
	Set(key string, value any)
	Delete(key string)
}

// CachingMiddleware creates unified middleware that caches query results (typically used for queries only)
func CachingMiddleware[Req any, Res any](cache CacheProvider) Middleware[Req, Res] {
	return func(next Handler[Req, Res]) Handler[Req, Res] {
		return func(ctx context.Context, log domain.Logger, p Payload[Req]) (Response[Res], error) {
			// Only cache queries, not commands
			if _, ok := any(p.Data).(Query); !ok {
				return next(ctx, log, p)
			}

			// Create cache key based on query type and content
			cacheKey := generateCacheKey(p.Data)

			// Try to get from cache first
			if cached, found := cache.Get(cacheKey); found {
				if cachedResponse, ok := cached.(Response[Res]); ok {
					log.Debug("Query result found in cache",
						"cache_key", cacheKey,
						"traceId", p.TraceID)
					return cachedResponse, nil
				}
			}

			// Execute query
			response, err := next(ctx, log, p)
			if err != nil {
				return response, err
			}

			// Cache the result
			cache.Set(cacheKey, response)
			log.Debug("Query result cached",
				"cache_key", cacheKey,
				"traceId", p.TraceID)

			return response, nil
		}
	}
}

// generateCacheKey creates a cache key for a request
func generateCacheKey(data any) string {
	// Simple implementation - in production you might want to use JSON marshaling
	// or a more sophisticated key generation strategy
	if query, ok := data.(Query); ok {
		return query.QueryType() + "_" + fmt.Sprintf("%+v", data)
	}
	return fmt.Sprintf("%T_%+v", data, data)
}
