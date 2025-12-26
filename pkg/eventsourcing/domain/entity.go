package domain

// Entity is an interface for entities that can be tracked by UnitOfWork.
// It provides methods to access entity state and uncommitted events.
type Entity interface {
	// GetID returns the aggregate ID of the entity.
	GetID() string

	// GetSequenceNo returns the last event sequence number from when the entity was hydrated.
	GetSequenceNo() int

	// GetUncommittedEvents returns all uncommitted events that have been recorded but not yet persisted.
	GetUncommittedEvents() []EventEnvelope[any]

	// ClearUncommittedEvents removes all uncommitted events, typically called after successful persistence.
	ClearUncommittedEvents()
}
