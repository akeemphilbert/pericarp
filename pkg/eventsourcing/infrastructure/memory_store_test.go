package infrastructure_test

import (
	"context"
	"errors"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

func TestMemoryStore_Integration(t *testing.T) {
	t.Parallel()

	t.Run("full workflow", func(t *testing.T) {
		t.Parallel()

		store := infrastructure.NewMemoryStore()
		defer store.Close()

		ctx := context.Background()
		aggregateID := "test-aggregate"

		// Append initial events
		events := []domain.EventEnvelope[any]{
			createTestEvent(aggregateID, "event-1", "test.created", 0),
			createTestEvent(aggregateID, "event-2", "test.updated", 0),
		}

		if err := store.Append(ctx, aggregateID, -1, events...); err != nil {
			t.Fatalf("failed to append events: %v", err)
		}

		// Verify events were stored
		retrieved, err := store.GetEvents(ctx, aggregateID)
		if err != nil {
			t.Fatalf("failed to get events: %v", err)
		}

		if len(retrieved) != 2 {
			t.Fatalf("expected 2 events, got %d", len(retrieved))
		}

		// Verify versions were assigned correctly
		if retrieved[0].Version != 1 {
			t.Errorf("expected first event version 1, got %d", retrieved[0].Version)
		}
		if retrieved[1].Version != 2 {
			t.Errorf("expected second event version 2, got %d", retrieved[1].Version)
		}

		// Get current version
		version, err := store.GetCurrentVersion(ctx, aggregateID)
		if err != nil {
			t.Fatalf("failed to get current version: %v", err)
		}
		if version != 2 {
			t.Errorf("expected current version 2, got %d", version)
		}

		// Append more events with version check
		newEvent := createTestEvent(aggregateID, "event-3", "test.updated", 0)
		if err := store.Append(ctx, aggregateID, 2, newEvent); err != nil {
			t.Fatalf("failed to append with version check: %v", err)
		}

		// Verify new event
		allEvents, err := store.GetEvents(ctx, aggregateID)
		if err != nil {
			t.Fatalf("failed to get all events: %v", err)
		}
		if len(allEvents) != 3 {
			t.Fatalf("expected 3 events, got %d", len(allEvents))
		}

		// Get events from version 2
		fromVersion2, err := store.GetEventsFromVersion(ctx, aggregateID, 2)
		if err != nil {
			t.Fatalf("failed to get events from version: %v", err)
		}
		if len(fromVersion2) != 2 {
			t.Fatalf("expected 2 events from version 2, got %d", len(fromVersion2))
		}

		// Get event by ID
		event, err := store.GetEventByID(ctx, "event-2")
		if err != nil {
			t.Fatalf("failed to get event by ID: %v", err)
		}
		if event.ID != "event-2" {
			t.Errorf("expected event ID event-2, got %s", event.ID)
		}
	})

	t.Run("concurrency conflict", func(t *testing.T) {
		t.Parallel()

		store := infrastructure.NewMemoryStore()
		defer store.Close()

		ctx := context.Background()
		aggregateID := "conflict-test"

		// Append initial event
		event1 := createTestEvent(aggregateID, "event-1", "test.created", 0)
		if err := store.Append(ctx, aggregateID, -1, event1); err != nil {
			t.Fatalf("failed to append initial event: %v", err)
		}

		// Try to append with wrong version
		event2 := createTestEvent(aggregateID, "event-2", "test.updated", 0)
		err := store.Append(ctx, aggregateID, 0, event2)
		if err == nil {
			t.Fatal("expected concurrency conflict error, got nil")
		}
		if !errors.Is(err, domain.ErrConcurrencyConflict) {
			t.Fatalf("expected ErrConcurrencyConflict, got %v", err)
		}
	})

	t.Run("multiple aggregates", func(t *testing.T) {
		t.Parallel()

		store := infrastructure.NewMemoryStore()
		defer store.Close()

		ctx := context.Background()

		// Append events for multiple aggregates
		agg1Events := []domain.EventEnvelope[any]{
			createTestEvent("agg-1", "event-1", "test.created", 0),
		}
		agg2Events := []domain.EventEnvelope[any]{
			createTestEvent("agg-2", "event-2", "test.created", 0),
			createTestEvent("agg-2", "event-3", "test.updated", 0),
		}

		if err := store.Append(ctx, "agg-1", -1, agg1Events...); err != nil {
			t.Fatalf("failed to append events for agg-1: %v", err)
		}
		if err := store.Append(ctx, "agg-2", -1, agg2Events...); err != nil {
			t.Fatalf("failed to append events for agg-2: %v", err)
		}

		// Verify isolation
		agg1EventsRetrieved, err := store.GetEvents(ctx, "agg-1")
		if err != nil {
			t.Fatalf("failed to get events for agg-1: %v", err)
		}
		if len(agg1EventsRetrieved) != 1 {
			t.Errorf("expected 1 event for agg-1, got %d", len(agg1EventsRetrieved))
		}

		agg2EventsRetrieved, err := store.GetEvents(ctx, "agg-2")
		if err != nil {
			t.Fatalf("failed to get events for agg-2: %v", err)
		}
		if len(agg2EventsRetrieved) != 2 {
			t.Errorf("expected 2 events for agg-2, got %d", len(agg2EventsRetrieved))
		}
	})
}

func TestMemoryStore_GetAllAggregateIDs(t *testing.T) {
	t.Parallel()

	store := infrastructure.NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Add events for multiple aggregates
	if err := store.Append(ctx, "agg-1", -1, createTestEvent("agg-1", "e1", "test.created", 0)); err != nil {
		t.Fatalf("failed to append: %v", err)
	}
	if err := store.Append(ctx, "agg-2", -1, createTestEvent("agg-2", "e2", "test.created", 0)); err != nil {
		t.Fatalf("failed to append: %v", err)
	}
	if err := store.Append(ctx, "agg-3", -1, createTestEvent("agg-3", "e3", "test.created", 0)); err != nil {
		t.Fatalf("failed to append: %v", err)
	}

	ids := store.GetAllAggregateIDs()
	if len(ids) != 3 {
		t.Fatalf("expected 3 aggregate IDs, got %d", len(ids))
	}

	// Verify all expected IDs are present
	expectedIDs := map[string]bool{"agg-1": true, "agg-2": true, "agg-3": true}
	for _, id := range ids {
		if !expectedIDs[id] {
			t.Errorf("unexpected aggregate ID: %s", id)
		}
	}
}
