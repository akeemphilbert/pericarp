package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// EventHandler is a type-safe handler function for processing events.
// The type parameter T represents the strongly-typed event payload.
type EventHandler[T any] func(ctx context.Context, env EventEnvelope[T]) error

// handlerFunc is the internal representation of a handler that accepts EventEnvelope[any].
type handlerFunc func(ctx context.Context, env EventEnvelope[any]) error

// typeFactory is a function that creates a new instance of an event payload type.
type typeFactory func() interface{}

// EventDispatcher is responsible for registering event handlers and dispatching events to them.
// It acts as both a handler registry and event dispatcher.
type EventDispatcher struct {
	mu               sync.RWMutex
	handlers         map[string][]handlerFunc
	wildcardHandlers []handlerFunc
	typeRegistry     map[string]typeFactory
}

// NewEventDispatcher creates a new EventDispatcher instance.
func NewEventDispatcher() *EventDispatcher {
	return &EventDispatcher{
		handlers:         make(map[string][]handlerFunc),
		wildcardHandlers: make([]handlerFunc, 0),
		typeRegistry:     make(map[string]typeFactory),
	}
}

// Subscribe registers a typed event handler for a specific event type.
// The handler will be called when events of the specified type are dispatched.
// Multiple handlers can be registered for the same event type.
// This is a generic function (not a method) because Go doesn't support generic methods on non-generic types.
func Subscribe[T any](d *EventDispatcher, eventType string, handler EventHandler[T]) error {
	if eventType == "" {
		return fmt.Errorf("event type cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	// Wrap the typed handler to accept EventEnvelope[any]
	wrappedHandler := func(ctx context.Context, env EventEnvelope[any]) error {
		// Type assert the payload to T
		payload, ok := env.Payload.(T)
		if !ok {
			return fmt.Errorf("type assertion failed: expected %T, got %T for event type %q", *new(T), env.Payload, eventType)
		}

		// Reconstruct EventEnvelope[T] with the typed payload
		typedEnv := EventEnvelope[T]{
			ID:          env.ID,
			AggregateID: env.AggregateID,
			EventType:   env.EventType,
			Payload:     payload,
			Created:     env.Created,
			SequenceNo:  env.SequenceNo,
			Metadata:    env.Metadata,
		}

		// Call the typed handler
		return handler(ctx, typedEnv)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Store handler in dispatcher's internal map (dispatcher acts as registry)
	d.handlers[eventType] = append(d.handlers[eventType], wrappedHandler)

	// Register type factory for deserialization support
	// Only register if not already registered for this event type
	if _, exists := d.typeRegistry[eventType]; !exists {
		d.typeRegistry[eventType] = func() interface{} {
			return new(T)
		}
	}

	return nil
}

// SubscribeWildcard registers a catch-all handler that will be called for all event types.
// Wildcard handlers are executed in parallel with pattern-matched handlers.
func (d *EventDispatcher) SubscribeWildcard(handler func(context.Context, EventEnvelope[any]) error) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.wildcardHandlers = append(d.wildcardHandlers, handler)
	return nil
}

// matchPattern checks if an event type matches a pattern.
// Patterns support wildcards: `*` matches any part, `entity.*` matches all actions for entity,
// `*.action` matches all entities for action, `*.*` matches all events.
func matchPattern(eventType, pattern string) bool {
	// Exact match
	if eventType == pattern {
		return true
	}

	// Split event type and pattern by dot
	eventParts := splitEventType(eventType)
	patternParts := splitEventType(pattern)

	// Both must have same number of parts
	if len(eventParts) != len(patternParts) {
		return false
	}

	// Match each part
	for i := 0; i < len(eventParts); i++ {
		if patternParts[i] != "*" && patternParts[i] != eventParts[i] {
			return false
		}
	}

	return true
}

// splitEventType splits an event type by dot, handling edge cases.
// It filters out empty strings that may result from consecutive dots.
func splitEventType(eventType string) []string {
	if eventType == "" {
		return []string{}
	}
	parts := strings.Split(eventType, ".")
	// Filter out empty strings from consecutive dots
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// getMatchingPatterns returns all patterns that match the given event type.
// For "user.created", it returns: ["user.created", "user.*", "*.created", "*.*"]
func getMatchingPatterns(eventType string) []string {
	parts := splitEventType(eventType)
	if len(parts) == 0 {
		return []string{eventType}
	}

	patterns := []string{
		eventType, // Exact match
	}

	// Build patterns based on number of parts
	if len(parts) == 1 {
		// Single part: "user" -> ["user", "*"]
		patterns = append(patterns, "*")
	} else if len(parts) == 2 {
		// Two parts: "user.created" -> ["user.created", "user.*", "*.created", "*.*"]
		patterns = append(patterns,
			parts[0]+".*", // "user.*"
			"*."+parts[1], // "*.created"
			"*.*",         // "*.*"
		)
	} else {
		// For more complex patterns, build wildcard variants
		// "user.account.created" -> ["user.account.created", "user.account.*", "user.*.created", "user.*.*", "*.account.created", etc.
		for i := 0; i < len(parts); i++ {
			wildcardParts := make([]string, len(parts))
			copy(wildcardParts, parts)
			wildcardParts[i] = "*"
			patterns = append(patterns, joinEventParts(wildcardParts))
		}
		// Add full wildcard
		allWildcards := make([]string, len(parts))
		for i := range allWildcards {
			allWildcards[i] = "*"
		}
		patterns = append(patterns, joinEventParts(allWildcards))
	}

	return patterns
}

// joinEventParts joins event parts with dots.
func joinEventParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "." + parts[i]
	}
	return result
}

// Dispatch dispatches an event to all registered handlers for the event type and matching patterns.
// Handlers are executed in parallel using goroutines.
// If any handler returns an error, it is collected and returned after all handlers complete.
// Pattern matching: "user.created" triggers handlers for "user.created", "user.*", "*.created", and "*.*"
func (d *EventDispatcher) Dispatch(ctx context.Context, envelope EventEnvelope[any]) error {
	d.mu.RLock()

	// Get all matching patterns for this event type
	patterns := getMatchingPatterns(envelope.EventType)

	// Collect all handlers that match any pattern
	// Note: If a handler is registered for multiple matching patterns, it will be called multiple times
	var allHandlers []handlerFunc
	for _, pattern := range patterns {
		allHandlers = append(allHandlers, d.handlers[pattern]...)
	}

	// Add wildcard handlers to the same slice
	allHandlers = append(allHandlers, d.wildcardHandlers...)
	d.mu.RUnlock()

	// If no handlers, return early
	if len(allHandlers) == 0 {
		return nil
	}

	// Use errgroup to run handlers in parallel
	// Note: We use errgroup for parallel execution but collect all errors manually
	// since errgroup only returns the first error by default
	g, gCtx := errgroup.WithContext(ctx)

	// Use mutex to safely collect all errors from concurrent handlers
	var errsMu sync.Mutex
	var errs []error

	// Execute all handlers in parallel
	for i := range allHandlers {
		idx := i // Capture loop variable
		g.Go(func() error {
			if err := allHandlers[idx](gCtx, envelope); err != nil {
				errsMu.Lock()
				errs = append(errs, fmt.Errorf("handler error for event type %q: %w", envelope.EventType, err))
				errsMu.Unlock()
			}
			return nil // Don't return error to errgroup so all handlers complete
		})
	}

	// Wait for all handlers to complete
	if err := g.Wait(); err != nil {
		// This shouldn't happen since we return nil from handlers,
		// but handle it just in case
		errsMu.Lock()
		errs = append(errs, err)
		errsMu.Unlock()
	}

	// Return all collected errors
	if len(errs) > 0 {
		return fmt.Errorf("dispatch errors: %v", errs)
	}

	return nil
}

// RegisterType registers a type factory for an event type to enable type-safe deserialization.
// This is separate from handler registration and is used when unmarshaling events from storage.
func RegisterType[T any](d *EventDispatcher, eventType string, factory func() T) error {
	if eventType == "" {
		return fmt.Errorf("event type cannot be empty")
	}
	if factory == nil {
		return fmt.Errorf("factory cannot be nil")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.typeRegistry[eventType] = func() interface{} {
		return factory()
	}

	return nil
}

// UnmarshalEvent unmarshals an event from JSON using the registered type factory.
// It looks up the type factory from the registry and reconstructs EventEnvelope[any] with the typed payload.
func (d *EventDispatcher) UnmarshalEvent(ctx context.Context, data []byte, eventType string) (EventEnvelope[any], error) {
	d.mu.RLock()
	factory, exists := d.typeRegistry[eventType]
	d.mu.RUnlock()

	if !exists {
		return EventEnvelope[any]{}, fmt.Errorf("type not registered for event type %q", eventType)
	}

	// Create a new instance of the event type
	payload := factory()

	// Unmarshal the JSON into a temporary struct to extract metadata
	var temp struct {
		ID          string                 `json:"id"`
		AggregateID string                 `json:"aggregate_id"`
		EventType   string                 `json:"event_type"`
		Payload     json.RawMessage        `json:"payload"`
		Created     string                 `json:"timestamp"`
		SequenceNo  int                    `json:"sequence_no"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		return EventEnvelope[any]{}, fmt.Errorf("failed to unmarshal event envelope: %w", err)
	}

	// Unmarshal the payload into the typed instance
	if err := json.Unmarshal(temp.Payload, payload); err != nil {
		return EventEnvelope[any]{}, fmt.Errorf("failed to unmarshal event payload: %w", err)
	}

	// Parse the timestamp
	created, err := parseTime(temp.Created)
	if err != nil {
		return EventEnvelope[any]{}, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	// Reconstruct EventEnvelope[any]
	envelope := EventEnvelope[any]{
		ID:          temp.ID,
		AggregateID: temp.AggregateID,
		EventType:   temp.EventType,
		Payload:     payload,
		Created:     created,
		SequenceNo:  temp.SequenceNo,
		Metadata:    temp.Metadata,
	}

	return envelope, nil
}

// parseTime is a helper function to parse time from JSON string.
// It handles RFC3339 format (Go's default JSON time format) and RFC3339Nano.
func parseTime(s string) (time.Time, error) {
	// Try RFC3339 first (Go's default JSON time format)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// Try RFC3339Nano for higher precision
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unable to parse time: %q (expected RFC3339 format)", s)
}
