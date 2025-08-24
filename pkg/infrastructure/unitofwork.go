package infrastructure

import (
	"context"
	"fmt"
	"sync"

	"github.com/example/pericarp/pkg/domain"
)

// UnitOfWorkImpl implements the UnitOfWork interface with Persist-then-Dispatch pattern
type UnitOfWorkImpl struct {
	eventStore      domain.EventStore
	eventDispatcher domain.EventDispatcher
	events          []domain.Event
	mu              sync.RWMutex // Use RWMutex for better read performance
	committed       bool
}

// NewUnitOfWork creates a new Unit of Work instance
func NewUnitOfWork(eventStore domain.EventStore, eventDispatcher domain.EventDispatcher) *UnitOfWorkImpl {
	return &UnitOfWorkImpl{
		eventStore:      eventStore,
		eventDispatcher: eventDispatcher,
		events:          make([]domain.Event, 0, 10), // Pre-allocate with reasonable capacity
		committed:       false,
	}
}

// RegisterEvents adds events to be persisted in the current transaction
func (uow *UnitOfWorkImpl) RegisterEvents(events []domain.Event) {
	if len(events) == 0 {
		return // Early return for empty slice
	}

	uow.mu.Lock()
	defer uow.mu.Unlock()

	if uow.committed {
		// This is a programming error - should not register events after commit
		panic("cannot register events after unit of work has been committed")
	}

	// Optimize append operation by pre-allocating if needed
	if cap(uow.events)-len(uow.events) < len(events) {
		// Need to grow the slice
		newCap := len(uow.events) + len(events)
		if newCap < cap(uow.events)*2 {
			newCap = cap(uow.events) * 2
		}
		newEvents := make([]domain.Event, len(uow.events), newCap)
		copy(newEvents, uow.events)
		uow.events = newEvents
	}

	uow.events = append(uow.events, events...)
}

// Commit persists all registered events and then dispatches them (Persist-then-Dispatch)
func (uow *UnitOfWorkImpl) Commit(ctx context.Context) ([]domain.Envelope, error) {
	uow.mu.Lock()
	defer uow.mu.Unlock()

	if uow.committed {
		return nil, fmt.Errorf("unit of work has already been committed")
	}

	// If no events to commit, return empty slice
	if len(uow.events) == 0 {
		uow.committed = true
		return []domain.Envelope{}, nil
	}

	// Step 1: Persist events to the event store
	envelopes, err := uow.eventStore.Save(ctx, uow.events)
	if err != nil {
		return nil, fmt.Errorf("failed to persist events: %w", err)
	}

	// Mark as committed before dispatch (events are persisted)
	uow.committed = true

	// Step 2: Dispatch the persisted events
	if err := uow.eventDispatcher.Dispatch(ctx, envelopes); err != nil {
		// Events are already persisted, so we log the dispatch error but don't fail the commit
		// In a production system, you might want to implement a retry mechanism or dead letter queue
		return envelopes, fmt.Errorf("events persisted but dispatch failed: %w", err)
	}

	return envelopes, nil
}

// Rollback discards all registered events
func (uow *UnitOfWorkImpl) Rollback() error {
	uow.mu.Lock()
	defer uow.mu.Unlock()

	if uow.committed {
		return fmt.Errorf("cannot rollback: unit of work has already been committed")
	}

	// Clear all registered events
	uow.events = uow.events[:0]

	return nil
}

// GetRegisteredEvents returns a copy of the currently registered events (for testing)
func (uow *UnitOfWorkImpl) GetRegisteredEvents() []domain.Event {
	uow.mu.RLock()
	defer uow.mu.RUnlock()

	events := make([]domain.Event, len(uow.events))
	copy(events, uow.events)
	return events
}

// IsCommitted returns whether the unit of work has been committed (for testing)
func (uow *UnitOfWorkImpl) IsCommitted() bool {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	return uow.committed
}

// EventCount returns the number of registered events (for testing)
func (uow *UnitOfWorkImpl) EventCount() int {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	return len(uow.events)
}
