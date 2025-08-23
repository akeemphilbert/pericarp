package domain

import "context"

// AggregateRoot defines the interface for domain aggregates in event sourcing
type AggregateRoot interface {
	// ID returns the unique identifier of the aggregate
	ID() string

	// Version returns the current version of the aggregate
	Version() int

	// UncommittedEvents returns the list of events that have been generated
	// but not yet persisted to the event store
	UncommittedEvents() []Event

	// MarkEventsAsCommitted clears the uncommitted events after they have
	// been successfully persisted to the event store
	MarkEventsAsCommitted()

	// LoadFromHistory reconstructs the aggregate state from a sequence of events
	LoadFromHistory(events []Event)
}

// Repository defines the interface for aggregate persistence
type Repository[T AggregateRoot] interface {
	// Save persists the aggregate and its uncommitted events
	Save(ctx context.Context, aggregate T) error

	// Load retrieves an aggregate by its ID, reconstructing it from events
	Load(ctx context.Context, id string) (T, error)
}
