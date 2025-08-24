package domain

//go:generate moq -out mocks/event_store_mock.go . EventStore
//go:generate moq -out mocks/event_dispatcher_mock.go . EventDispatcher
//go:generate moq -out mocks/event_handler_mock.go . EventHandler
//go:generate moq -out mocks/unit_of_work_mock.go . UnitOfWork
//go:generate moq -out mocks/event_mock.go . Event
//go:generate moq -out mocks/envelope_mock.go . Envelope

import (
	"context"
	"time"
)

// Event represents a domain event that occurred in the system
type Event interface {
	// EventType returns the type identifier for this event
	EventType() string

	// AggregateID returns the ID of the aggregate that generated this event
	AggregateID() string

	// Version returns the version of the aggregate when this event occurred
	Version() int

	// OccurredAt returns the timestamp when this event occurred
	OccurredAt() time.Time
}

// Envelope wraps events with metadata
type Envelope interface {
	// Event returns the wrapped domain event
	Event() Event

	// Metadata returns additional metadata associated with the event
	Metadata() map[string]interface{}

	// EventID returns the unique identifier for this event envelope
	EventID() string

	// Timestamp returns when this envelope was created
	Timestamp() time.Time
}

// EventStore handles event persistence
type EventStore interface {
	// Save persists events and returns envelopes with metadata
	Save(ctx context.Context, events []Event) ([]Envelope, error)

	// Load retrieves all events for an aggregate
	Load(ctx context.Context, aggregateID string) ([]Envelope, error)

	// LoadFromVersion retrieves events for an aggregate starting from a specific version
	LoadFromVersion(ctx context.Context, aggregateID string, version int) ([]Envelope, error)
}

// EventDispatcher handles event distribution
type EventDispatcher interface {
	// Dispatch sends envelopes to registered event handlers
	Dispatch(ctx context.Context, envelopes []Envelope) error

	// Subscribe registers an event handler for specific event types
	Subscribe(eventType string, handler EventHandler) error
}

// EventHandler processes events (projectors/sagas)
type EventHandler interface {
	// Handle processes a single event envelope
	Handle(ctx context.Context, envelope Envelope) error

	// EventTypes returns the list of event types this handler can process
	EventTypes() []string
}

// UnitOfWork manages transactional event persistence
type UnitOfWork interface {
	// RegisterEvents adds events to be persisted in the current transaction
	RegisterEvents(events []Event)

	// Commit persists all registered events and returns envelopes
	Commit(ctx context.Context) ([]Envelope, error)

	// Rollback discards all registered events
	Rollback() error
}
