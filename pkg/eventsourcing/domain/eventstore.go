package domain

import (
	"context"
	"errors"
)

var (
	// ErrEventNotFound is returned when an event is not found in the store.
	ErrEventNotFound = errors.New("event not found")

	// ErrConcurrencyConflict is returned when there's a version conflict during event persistence.
	ErrConcurrencyConflict = errors.New("concurrency conflict: expected version mismatch")

	// ErrInvalidEvent is returned when an event is invalid or malformed.
	ErrInvalidEvent = errors.New("invalid event")
)

// EventStore defines the interface for persisting and retrieving events.
// Implementations should be thread-safe and handle concurrent access.
// Events are stored as EventEnvelope[any] to allow storing events with different payload types together.
type EventStore interface {
	// Append appends one or more events to the event store for a given aggregate.
	// It returns an error if the expected version doesn't match (optimistic concurrency control).
	// If expectedVersion is -1, no version check is performed.
	Append(ctx context.Context, aggregateID string, expectedVersion int, events ...EventEnvelope[any]) error

	// GetEvents retrieves all events for a given aggregate ID.
	// Returns an empty slice if no events are found.
	GetEvents(ctx context.Context, aggregateID string) ([]EventEnvelope[any], error)

	// GetEventsFromVersion retrieves events for an aggregate starting from a specific version.
	// Returns an empty slice if no events are found.
	GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]EventEnvelope[any], error)

	// GetEventsRange retrieves events for an aggregate within a version range.
	// If fromVersion is -1, it defaults to version 1 (the first event).
	// If toVersion is -1, it retrieves all events from fromVersion to the end.
	// Returns an empty slice if no events are found in the range.
	GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]EventEnvelope[any], error)

	// GetEventByID retrieves a specific event by its ID.
	// Returns ErrEventNotFound if the event doesn't exist.
	GetEventByID(ctx context.Context, eventID string) (EventEnvelope[any], error)

	// GetCurrentVersion returns the current version number for an aggregate.
	// Returns 0 if the aggregate doesn't exist.
	GetCurrentVersion(ctx context.Context, aggregateID string) (int, error)

	// Close closes the event store and releases any resources.
	Close() error
}

// ToAnyEnvelope converts an EventEnvelope[T] to EventEnvelope[any] for storage.
// This allows storing events with different payload types together in the event store.
func ToAnyEnvelope[T any](envelope EventEnvelope[T]) EventEnvelope[any] {
	return EventEnvelope[any]{
		ID:          envelope.ID,
		AggregateID: envelope.AggregateID,
		EventType:   envelope.EventType,
		Payload:     envelope.Payload,
		Created:     envelope.Created,
		SequenceNo:  envelope.SequenceNo,
		Metadata:    envelope.Metadata,
	}
}
