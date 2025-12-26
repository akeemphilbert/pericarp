package ddd

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

var (
	// ErrWrongAggregate is returned when an event is applied to the wrong aggregate.
	ErrWrongAggregate = errors.New("event does not belong to this aggregate")

	// ErrDuplicateEvent is returned when attempting to apply an event that has already been applied.
	ErrDuplicateEvent = errors.New("event has already been applied")

	// ErrInvalidEventSequenceNo is returned when an event sequence number is invalid.
	ErrInvalidEventSequenceNo = errors.New("event sequence number is invalid")
)

// BaseEntity provides event sourcing capabilities for domain entities.
// Entities should embed this struct to gain event tracking, sequence number management,
// and uncommitted event collection.
type BaseEntity struct {
	// aggregateID is the unique identifier for this aggregate.
	aggregateID string

	// sequenceNo tracks the last event sequence number from when the entity was hydrated.
	// This is used to determine the next sequence number for new events.
	sequenceNo int

	// uncommittedEvents holds events that have been recorded but not yet persisted.
	uncommittedEvents []domain.EventEnvelope[any]

	// appliedEventIDs tracks event IDs that have already been applied to prevent duplicates.
	appliedEventIDs map[string]bool

	// mu protects concurrent access to the entity state.
	mu sync.RWMutex
}

// NewBaseEntity creates a new BaseEntity with the given aggregate ID.
func NewBaseEntity(aggregateID string) *BaseEntity {
	return &BaseEntity{
		aggregateID:       aggregateID,
		sequenceNo:        -1, // Start at -1 so first event is 0
		uncommittedEvents: make([]domain.EventEnvelope[any], 0),
		appliedEventIDs:   make(map[string]bool),
	}
}

// GetID returns the aggregate ID.
func (e *BaseEntity) GetID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.aggregateID
}

// GetSequenceNo returns the last event sequence number from when the entity was hydrated.
func (e *BaseEntity) GetSequenceNo() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sequenceNo
}

// GetUncommittedEvents returns a copy of all uncommitted events.
func (e *BaseEntity) GetUncommittedEvents() []domain.EventEnvelope[any] {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]domain.EventEnvelope[any], len(e.uncommittedEvents))
	copy(result, e.uncommittedEvents)
	return result
}

// ClearUncommittedEvents removes all uncommitted events, typically called after persistence.
func (e *BaseEntity) ClearUncommittedEvents() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.uncommittedEvents = make([]domain.EventEnvelope[any], 0)
}

// applyEventInternal performs the actual event application logic.
// It assumes the caller holds the lock.
func (e *BaseEntity) applyEventInternal(event domain.EventEnvelope[any]) error {
	// Validate event belongs to this aggregate
	if event.AggregateID != e.aggregateID {
		return fmt.Errorf("%w: expected %s, got %s", ErrWrongAggregate, e.aggregateID, event.AggregateID)
	}

	// Check for duplicate event (idempotency)
	if e.appliedEventIDs[event.ID] {
		return fmt.Errorf("%w: event ID %s", ErrDuplicateEvent, event.ID)
	}

	// Validate event sequence number matches expected sequence number
	expectedSequenceNo := e.sequenceNo + 1
	if event.SequenceNo != expectedSequenceNo {
		return fmt.Errorf("%w: expected %d, got %d", ErrInvalidEventSequenceNo, expectedSequenceNo, event.SequenceNo)
	}

	// Mark event as applied
	e.appliedEventIDs[event.ID] = true

	// Update sequence number
	e.sequenceNo = event.SequenceNo

	return nil
}

// ApplyEvent applies an event to the entity, updating its state and sequence number.
// This method validates the event, checks for idempotency, and updates the sequence number.
// This is used for replaying events from the event store.
// The actual state mutation logic should be implemented by entities embedding BaseEntity.
func (e *BaseEntity) ApplyEvent(ctx context.Context, event domain.EventEnvelope[any]) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Validate context
	if ctx.Err() != nil {
		return fmt.Errorf("context error: %w", ctx.Err())
	}

	return e.applyEventInternal(event)
}

// RecordEvent records a new event by creating an EventEnvelope internally.
// The payload can be any type and will be stored in the event envelope.
// This method is thread-safe and validates that the event belongs to this aggregate.
func (e *BaseEntity) RecordEvent(payload any, eventType string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Create the event envelope with the correct aggregate ID and sequence number
	nextSequenceNo := e.sequenceNo + 1
	envelope := domain.NewEventEnvelope(payload, e.aggregateID, eventType, nextSequenceNo)

	// Check for duplicate event (by ID)
	if e.appliedEventIDs[envelope.ID] {
		return fmt.Errorf("%w: event ID %s", ErrDuplicateEvent, envelope.ID)
	}

	// Mark as applied
	e.appliedEventIDs[envelope.ID] = true

	// Update sequence number
	e.sequenceNo = nextSequenceNo

	// Add to uncommitted events
	e.uncommittedEvents = append(e.uncommittedEvents, envelope)

	return nil
}
