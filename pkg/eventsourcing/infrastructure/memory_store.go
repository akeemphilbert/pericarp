package infrastructure

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// MemoryStore is an in-memory implementation of EventStore.
// It's useful for testing and development, but not suitable for production
// as it doesn't persist data across restarts.
type MemoryStore struct {
	mu         sync.RWMutex
	events     map[string][]*domain.EventEnvelope[any] // aggregateID -> events
	eventsByID map[string]*domain.EventEnvelope[any]   // eventID -> event
	versions   map[string]int                          // aggregateID -> current version
}

// NewMemoryStore creates a new in-memory event store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		events:     make(map[string][]*domain.EventEnvelope[any]),
		eventsByID: make(map[string]*domain.EventEnvelope[any]),
		versions:   make(map[string]int),
	}
}

// Append appends events to the store for the given aggregate.
func (m *MemoryStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...*domain.EventEnvelope[any]) error {
	if len(events) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate events
	for _, event := range events {
		if event == nil {
			return fmt.Errorf("%w: nil event provided", domain.ErrInvalidEvent)
		}
		if event.AggregateID != aggregateID {
			return fmt.Errorf("%w: aggregate ID mismatch", domain.ErrInvalidEvent)
		}
		if event.ID == "" {
			return fmt.Errorf("%w: event ID is required", domain.ErrInvalidEvent)
		}
	}

	// Check current version
	currentVersion := m.versions[aggregateID]
	if expectedVersion != -1 && currentVersion != expectedVersion {
		return fmt.Errorf("%w: expected version %d, got %d", domain.ErrConcurrencyConflict, expectedVersion, currentVersion)
	}

	// Append events
	eventList := m.events[aggregateID]
	if eventList == nil {
		eventList = make([]*domain.EventEnvelope[any], 0)
	}

	// Assign versions sequentially
	nextVersion := currentVersion + 1
	for i, event := range events {
		event.Version = nextVersion + i
		eventList = append(eventList, event)
		m.eventsByID[event.ID] = event
	}

	m.events[aggregateID] = eventList
	m.versions[aggregateID] = nextVersion + len(events) - 1

	return nil
}

// GetEvents retrieves all events for the given aggregate ID.
func (m *MemoryStore) GetEvents(ctx context.Context, aggregateID string) ([]*domain.EventEnvelope[any], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	events := m.events[aggregateID]
	if events == nil {
		return []*domain.EventEnvelope[any]{}, nil
	}

	// Return a copy to prevent external modification
	result := make([]*domain.EventEnvelope[any], len(events))
	copy(result, events)
	return result, nil
}

// GetEventsFromVersion retrieves events starting from the specified version.
func (m *MemoryStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]*domain.EventEnvelope[any], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	events := m.events[aggregateID]
	if events == nil {
		return []*domain.EventEnvelope[any]{}, nil
	}

	// Find the first event with version >= fromVersion
	result := make([]*domain.EventEnvelope[any], 0)
	for _, event := range events {
		if event.Version >= fromVersion {
			result = append(result, event)
		}
	}

	return result, nil
}

// GetEventsRange retrieves events within a version range.
func (m *MemoryStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]*domain.EventEnvelope[any], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	events := m.events[aggregateID]
	if events == nil {
		return []*domain.EventEnvelope[any]{}, nil
	}

	// Default fromVersion to 1 if -1
	if fromVersion == -1 {
		fromVersion = 1
	}

	result := make([]*domain.EventEnvelope[any], 0)
	for _, event := range events {
		if event.Version < fromVersion {
			continue
		}
		// If toVersion is -1, include all events from fromVersion onwards
		if toVersion != -1 && event.Version > toVersion {
			break
		}
		result = append(result, event)
	}

	return result, nil
}

// GetEventByID retrieves a specific event by its ID.
func (m *MemoryStore) GetEventByID(ctx context.Context, eventID string) (*domain.EventEnvelope[any], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	event, exists := m.eventsByID[eventID]
	if !exists {
		return nil, domain.ErrEventNotFound
	}

	// Return a copy to prevent external modification
	result := *event
	return &result, nil
}

// GetCurrentVersion returns the current version for the aggregate.
func (m *MemoryStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.versions[aggregateID], nil
}

// Close closes the memory store (no-op for in-memory implementation).
func (m *MemoryStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear all data
	m.events = make(map[string][]*domain.EventEnvelope[any])
	m.eventsByID = make(map[string]*domain.EventEnvelope[any])
	m.versions = make(map[string]int)

	return nil
}

// GetAllAggregateIDs returns all aggregate IDs in the store (useful for testing).
func (m *MemoryStore) GetAllAggregateIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.events))
	for id := range m.events {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
