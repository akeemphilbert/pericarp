package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
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

			// Extract request type for logging - optimize type assertion
			var requestType string
			switch v := any(p.Data).(type) {
			case Command:
				requestType = v.CommandType()
			case Query:
				requestType = v.QueryType()
			default:
				// Use a more efficient type name extraction
				requestType = getTypeName(p.Data)
			}

			// Only log if trace ID or user ID are present to reduce log volume
			if p.TraceID != "" || p.UserID != "" {
				log.Info("Processing request",
					"type", requestType,
					"traceId", p.TraceID,
					"userId", p.UserID)
			} else {
				log.Debug("Processing request", "type", requestType)
			}

			response, err := next(ctx, log, p)

			duration := time.Since(start)
			if err != nil {
				log.Error("Request failed",
					"type", requestType,
					"duration", duration,
					"error", err,
					"traceId", p.TraceID)
			} else {
				// Use debug level for successful requests to reduce log volume in production
				log.Debug("Request completed",
					"type", requestType,
					"duration", duration,
					"traceId", p.TraceID)
			}

			return response, err
		}
	}
}

// getTypeName efficiently extracts type name without reflection in hot path
func getTypeName(v any) string {
	// This is a simple implementation - in production you might want to cache type names
	return fmt.Sprintf("%T", v)
}

// ValidationMiddleware creates unified middleware that validates both commands and queries
func ValidationMiddleware[Req any, Res any]() Middleware[Req, Res] {
	return func(next Handler[Req, Res]) Handler[Req, Res] {
		return func(ctx context.Context, log domain.Logger, p Payload[Req]) (Response[Res], error) {
			// Fast path: check if validation is needed
			validator, needsValidation := any(p.Data).(Validator)
			if !needsValidation {
				return next(ctx, log, p)
			}

			// Validate the request
			if err := validator.Validate(); err != nil {
				// Extract request type efficiently
				var requestType string
				switch v := any(p.Data).(type) {
				case Command:
					requestType = v.CommandType()
				case Query:
					requestType = v.QueryType()
				default:
					requestType = getTypeName(p.Data)
				}

				log.Warn("Request validation failed",
					"type", requestType,
					"error", err,
					"traceId", p.TraceID)

				// Create validation error response
				validationErr := NewValidationError("", err.Error())
				var zero Res
				return Response[Res]{
					Data:  zero,
					Error: validationErr,
					Metadata: map[string]any{
						"validation_failed": true,
					},
				}, validationErr
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

			// Extract request type efficiently
			var requestType string
			switch v := any(p.Data).(type) {
			case Command:
				requestType = v.CommandType()
			case Query:
				requestType = v.QueryType()
			default:
				requestType = getTypeName(p.Data)
			}

			response, err := next(ctx, log, p)
			duration := time.Since(start)

			// Record metrics (this should be fast)
			metrics.RecordRequestDuration(requestType, duration)
			if err != nil {
				metrics.IncrementRequestErrors(requestType)
				// Only log errors, not successful requests to reduce log volume
				log.Error("Request failed",
					"type", requestType,
					"duration", duration,
					"error", err,
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

// InMemoryMetricsCollector is a simple in-memory implementation of MetricsCollector
// optimized for performance with minimal locking and efficient data structures
type InMemoryMetricsCollector struct {
	requestDurations map[string][]time.Duration
	requestErrors    map[string]int64 // Use int64 for atomic operations
	mu               sync.RWMutex
	maxDurations     int // Limit stored durations to prevent memory leaks
}

// NewInMemoryMetricsCollector creates a new in-memory metrics collector
func NewInMemoryMetricsCollector() *InMemoryMetricsCollector {
	return &InMemoryMetricsCollector{
		requestDurations: make(map[string][]time.Duration),
		requestErrors:    make(map[string]int64),
		maxDurations:     1000, // Keep last 1000 durations per request type
	}
}

// RecordRequestDuration records the duration of a request with memory management
func (m *InMemoryMetricsCollector) RecordRequestDuration(requestType string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	durations := m.requestDurations[requestType]

	// Implement circular buffer to prevent memory leaks
	if len(durations) >= m.maxDurations {
		// Remove oldest entry by shifting slice
		copy(durations, durations[1:])
		durations = durations[:len(durations)-1]
	}

	m.requestDurations[requestType] = append(durations, duration)
}

// IncrementRequestErrors increments the error count for a request type using atomic operations
func (m *InMemoryMetricsCollector) IncrementRequestErrors(requestType string) {
	m.mu.RLock()
	_, exists := m.requestErrors[requestType]
	m.mu.RUnlock()

	if !exists {
		m.mu.Lock()
		// Double-check after acquiring write lock
		if _, exists := m.requestErrors[requestType]; !exists {
			m.requestErrors[requestType] = 0
		}
		m.mu.Unlock()
	}

	// Increment error count with proper mutex protection
	m.mu.Lock()
	m.requestErrors[requestType]++
	m.mu.Unlock()
}

// GetMetrics returns the collected metrics (for debugging/monitoring)
func (m *InMemoryMetricsCollector) GetMetrics() (map[string][]time.Duration, map[string]int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return copies to avoid race conditions
	durations := make(map[string][]time.Duration, len(m.requestDurations))
	errors := make(map[string]int64, len(m.requestErrors))

	for k, v := range m.requestDurations {
		// Pre-allocate slice with exact capacity
		durCopy := make([]time.Duration, len(v))
		copy(durCopy, v)
		durations[k] = durCopy
	}

	for k, v := range m.requestErrors {
		errors[k] = v
	}

	return durations, errors
}

// GetSummaryStats returns summary statistics for performance monitoring
func (m *InMemoryMetricsCollector) GetSummaryStats() map[string]map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]map[string]interface{})

	for requestType, durations := range m.requestDurations {
		if len(durations) == 0 {
			continue
		}

		// Calculate basic statistics
		var total, min, max time.Duration
		min = durations[0]
		max = durations[0]

		for _, d := range durations {
			total += d
			if d < min {
				min = d
			}
			if d > max {
				max = d
			}
		}

		avg := total / time.Duration(len(durations))

		stats[requestType] = map[string]interface{}{
			"count":  len(durations),
			"avg":    avg,
			"min":    min,
			"max":    max,
			"total":  total,
			"errors": m.requestErrors[requestType],
		}
	}

	return stats
}

// InMemoryCache is a simple in-memory implementation of CacheProvider
type InMemoryCache struct {
	data map[string]any
	mu   sync.RWMutex
}

// NewInMemoryCache creates a new in-memory cache
func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{
		data: make(map[string]any),
	}
}

// Get retrieves a value from the cache
func (c *InMemoryCache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, exists := c.data[key]
	return value, exists
}

// Set stores a value in the cache
func (c *InMemoryCache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = value
}

// Delete removes a value from the cache
func (c *InMemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
}

// Clear removes all values from the cache
func (c *InMemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]any)
}
