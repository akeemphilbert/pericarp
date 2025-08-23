package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/example/pericarp/pkg/domain"
)

// testEvent implements domain.Event for testing
type testEvent struct {
	eventType   string
	aggregateID string
	version     int
	occurredAt  time.Time
}

func (e *testEvent) EventType() string {
	return e.eventType
}

func (e *testEvent) AggregateID() string {
	return e.aggregateID
}

func (e *testEvent) Version() int {
	return e.version
}

func (e *testEvent) OccurredAt() time.Time {
	return e.occurredAt
}

func TestGormEventStore_SaveAndLoad(t *testing.T) {
	// Create in-memory SQLite database for testing
	config := DatabaseConfig{
		Driver: "sqlite",
		DSN:    ":memory:",
	}

	db, err := NewDatabase(config)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	store, err := NewGormEventStore(db)
	if err != nil {
		t.Fatalf("Failed to create event store: %v", err)
	}

	ctx := context.Background()
	aggregateID := "test-aggregate-123"
	now := time.Now()

	// Create test events
	events := []domain.Event{
		&testEvent{
			eventType:   "TestEventCreated",
			aggregateID: aggregateID,
			version:     1,
			occurredAt:  now,
		},
		&testEvent{
			eventType:   "TestEventUpdated",
			aggregateID: aggregateID,
			version:     2,
			occurredAt:  now.Add(time.Minute),
		},
	}

	// Save events
	envelopes, err := store.Save(ctx, events)
	if err != nil {
		t.Fatalf("Failed to save events: %v", err)
	}

	if len(envelopes) != 2 {
		t.Errorf("Expected 2 envelopes, got %d", len(envelopes))
	}

	// Verify envelope properties
	for i, envelope := range envelopes {
		if envelope.EventID() == "" {
			t.Errorf("Envelope %d has empty EventID", i)
		}
		if envelope.Event().EventType() != events[i].EventType() {
			t.Errorf("Envelope %d event type mismatch: expected %s, got %s",
				i, events[i].EventType(), envelope.Event().EventType())
		}
		if envelope.Metadata() == nil {
			t.Errorf("Envelope %d has nil metadata", i)
		}
	}

	// Load all events
	loadedEnvelopes, err := store.Load(ctx, aggregateID)
	if err != nil {
		t.Fatalf("Failed to load events: %v", err)
	}

	if len(loadedEnvelopes) != 2 {
		t.Errorf("Expected 2 loaded envelopes, got %d", len(loadedEnvelopes))
	}

	// Verify loaded events
	for i, envelope := range loadedEnvelopes {
		event := envelope.Event()
		if event.AggregateID() != aggregateID {
			t.Errorf("Loaded event %d has wrong aggregate ID: expected %s, got %s",
				i, aggregateID, event.AggregateID())
		}
		if event.Version() != i+1 {
			t.Errorf("Loaded event %d has wrong version: expected %d, got %d",
				i, i+1, event.Version())
		}
	}

	// Test LoadFromVersion
	fromVersionEnvelopes, err := store.LoadFromVersion(ctx, aggregateID, 1)
	if err != nil {
		t.Fatalf("Failed to load events from version: %v", err)
	}

	if len(fromVersionEnvelopes) != 1 {
		t.Errorf("Expected 1 envelope from version 1, got %d", len(fromVersionEnvelopes))
	}

	if fromVersionEnvelopes[0].Event().Version() != 2 {
		t.Errorf("Expected event version 2, got %d", fromVersionEnvelopes[0].Event().Version())
	}
}

func TestGormEventStore_EmptyEvents(t *testing.T) {
	config := DatabaseConfig{
		Driver: "sqlite",
		DSN:    ":memory:",
	}

	db, err := NewDatabase(config)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	store, err := NewGormEventStore(db)
	if err != nil {
		t.Fatalf("Failed to create event store: %v", err)
	}

	ctx := context.Background()

	// Test saving empty events slice
	envelopes, err := store.Save(ctx, []domain.Event{})
	if err != nil {
		t.Fatalf("Failed to save empty events: %v", err)
	}

	if len(envelopes) != 0 {
		t.Errorf("Expected 0 envelopes for empty events, got %d", len(envelopes))
	}

	// Test loading non-existent aggregate
	loadedEnvelopes, err := store.Load(ctx, "non-existent")
	if err != nil {
		t.Fatalf("Failed to load non-existent aggregate: %v", err)
	}

	if len(loadedEnvelopes) != 0 {
		t.Errorf("Expected 0 envelopes for non-existent aggregate, got %d", len(loadedEnvelopes))
	}
}