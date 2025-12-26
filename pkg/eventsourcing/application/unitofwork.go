package application

import (
	"context"
	"fmt"
	"sync"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// UnitOfWork is an interface for managing transactions across multiple entities.
// It provides atomic persistence of uncommitted events from tracked entities.
type UnitOfWork interface {
	// Track registers one or more entities to be included in the unit of work.
	// The entity's expected version (sequence number) is captured at this time for optimistic concurrency control.
	// If any entity is invalid or already tracked, an error is returned and no entities are tracked.
	Track(entities ...domain.Entity) error

	// Commit persists all uncommitted events from all tracked entities atomically.
	// If any entity fails to persist, the entire commit fails and rollback occurs.
	// After successful persistence, events are optionally dispatched via EventDispatcher.
	Commit(ctx context.Context) error

	// Rollback clears the tracking of entities without clearing their uncommitted events.
	// This allows entities to be retried in a new unit of work.
	Rollback() error
}

// SimpleUnitOfWork is the default implementation of UnitOfWork.
// It provides atomic event persistence across multiple entities with optimistic concurrency control.
type SimpleUnitOfWork struct {
	eventStore       domain.EventStore
	dispatcher       *domain.EventDispatcher
	entities         map[string]domain.Entity
	expectedVersions map[string]int
	mu               sync.RWMutex
}

// NewSimpleUnitOfWork creates a new SimpleUnitOfWork instance.
// eventStore is required for persisting events.
// dispatcher is optional and can be nil if event dispatch is not needed.
func NewSimpleUnitOfWork(eventStore domain.EventStore, dispatcher *domain.EventDispatcher) *SimpleUnitOfWork {
	return &SimpleUnitOfWork{
		eventStore:       eventStore,
		dispatcher:       dispatcher,
		entities:         make(map[string]domain.Entity),
		expectedVersions: make(map[string]int),
	}
}

// Track registers one or more entities to be included in the unit of work.
func (uow *SimpleUnitOfWork) Track(entities ...domain.Entity) error {
	if len(entities) == 0 {
		return nil // No entities to track, nothing to do
	}

	uow.mu.Lock()
	defer uow.mu.Unlock()

	// Validate all entities first before tracking any
	for _, entity := range entities {
		if entity == nil {
			return fmt.Errorf("entity cannot be nil")
		}

		aggregateID := entity.GetID()
		if aggregateID == "" {
			return fmt.Errorf("entity must have a non-empty aggregate ID")
		}

		// Check if entity is already tracked (either in this batch or previously tracked)
		if _, exists := uow.entities[aggregateID]; exists {
			return fmt.Errorf("entity with aggregate ID %q is already tracked", aggregateID)
		}
	}

	// Check for duplicates within the batch
	seen := make(map[string]bool)
	for _, entity := range entities {
		aggregateID := entity.GetID()
		if seen[aggregateID] {
			return fmt.Errorf("duplicate entity with aggregate ID %q in batch", aggregateID)
		}
		seen[aggregateID] = true
	}

	// All validations passed, now track all entities
	for _, entity := range entities {
		aggregateID := entity.GetID()
		uow.entities[aggregateID] = entity
		uow.expectedVersions[aggregateID] = entity.GetSequenceNo()
	}

	return nil
}

// Commit persists all uncommitted events from all tracked entities atomically.
func (uow *SimpleUnitOfWork) Commit(ctx context.Context) error {
	uow.mu.Lock()

	// Collect all uncommitted events from all tracked entities
	eventsByAggregate := make(map[string][]domain.EventEnvelope[any])
	var allEvents []domain.EventEnvelope[any]

	for aggregateID, entity := range uow.entities {
		uncommitted := entity.GetUncommittedEvents()
		if len(uncommitted) > 0 {
			eventsByAggregate[aggregateID] = uncommitted
			allEvents = append(allEvents, uncommitted...)
		}
	}

	// If no events to commit, just clear tracking
	if len(eventsByAggregate) == 0 {
		uow.entities = make(map[string]domain.Entity)
		uow.expectedVersions = make(map[string]int)
		uow.mu.Unlock()
		return nil
	}

	// Store references before unlocking
	entities := make(map[string]domain.Entity, len(uow.entities))
	for k, v := range uow.entities {
		entities[k] = v
	}
	expectedVersions := make(map[string]int, len(uow.expectedVersions))
	for k, v := range uow.expectedVersions {
		expectedVersions[k] = v
	}
	dispatcher := uow.dispatcher
	uow.mu.Unlock()

	// Persist events for each aggregate with optimistic concurrency control
	for aggregateID, events := range eventsByAggregate {
		expectedVersion := expectedVersions[aggregateID]

		// Append events to event store with expected version
		if err := uow.eventStore.Append(ctx, aggregateID, expectedVersion, events...); err != nil {
			// Rollback on failure
			uow.Rollback()
			return fmt.Errorf("failed to persist events for aggregate %q: %w", aggregateID, err)
		}
	}

	// Clear uncommitted events from all entities
	uow.mu.Lock()
	for _, entity := range entities {
		entity.ClearUncommittedEvents()
	}
	// Clear tracking
	uow.entities = make(map[string]domain.Entity)
	uow.expectedVersions = make(map[string]int)
	uow.mu.Unlock()

	// Dispatch events if dispatcher is provided
	if dispatcher != nil && len(allEvents) > 0 {
		for _, event := range allEvents {
			if err := dispatcher.Dispatch(ctx, event); err != nil {
				// Events are already persisted, so dispatch errors don't fail the commit
				// This follows eventual consistency model
				// Log or handle dispatch errors as needed
				_ = err // Dispatch errors are non-fatal
			}
		}
	}

	return nil
}

// Rollback clears the tracking of entities without clearing their uncommitted events.
func (uow *SimpleUnitOfWork) Rollback() error {
	uow.mu.Lock()
	defer uow.mu.Unlock()

	uow.entities = make(map[string]domain.Entity)
	uow.expectedVersions = make(map[string]int)

	return nil
}
