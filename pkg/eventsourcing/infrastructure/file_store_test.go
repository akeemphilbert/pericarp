package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

func TestFileStore_Integration(t *testing.T) {
	t.Parallel()

	t.Run("full workflow", func(t *testing.T) {
		t.Parallel()

		baseDir := setupTestDir(t)
		store, err := infrastructure.NewFileStore(baseDir)
		if err != nil {
			t.Fatalf("failed to create file store: %v", err)
		}
		defer store.Close()
		defer cleanupTestDir(t, baseDir)

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

		// Close and reopen to test persistence
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}

		store2, err := infrastructure.NewFileStore(baseDir)
		if err != nil {
			t.Fatalf("failed to recreate file store: %v", err)
		}
		defer store2.Close()

		// Verify events persist after reopening
		retrieved2, err := store2.GetEvents(ctx, aggregateID)
		if err != nil {
			t.Fatalf("failed to get events after reopen: %v", err)
		}

		if len(retrieved2) != 2 {
			t.Fatalf("expected 2 events after reopen, got %d", len(retrieved2))
		}

		if retrieved2[0].ID != "event-1" {
			t.Errorf("expected first event ID event-1, got %s", retrieved2[0].ID)
		}
	})

	t.Run("concurrency conflict", func(t *testing.T) {
		t.Parallel()

		baseDir := setupTestDir(t)
		store, err := infrastructure.NewFileStore(baseDir)
		if err != nil {
			t.Fatalf("failed to create file store: %v", err)
		}
		defer store.Close()
		defer cleanupTestDir(t, baseDir)

		ctx := context.Background()
		aggregateID := "conflict-test"

		// Append initial event
		event1 := createTestEvent(aggregateID, "event-1", "test.created", 0)
		if err := store.Append(ctx, aggregateID, -1, event1); err != nil {
			t.Fatalf("failed to append initial event: %v", err)
		}

		// Try to append with wrong version
		event2 := createTestEvent(aggregateID, "event-2", "test.updated", 0)
		err = store.Append(ctx, aggregateID, 0, event2)
		if err == nil {
			t.Fatal("expected concurrency conflict error, got nil")
		}
		if !errors.Is(err, domain.ErrConcurrencyConflict) {
			t.Fatalf("expected ErrConcurrencyConflict, got %v", err)
		}
	})

	t.Run("multiple aggregates", func(t *testing.T) {
		t.Parallel()

		baseDir := setupTestDir(t)
		store, err := infrastructure.NewFileStore(baseDir)
		if err != nil {
			t.Fatalf("failed to create file store: %v", err)
		}
		defer store.Close()
		defer cleanupTestDir(t, baseDir)

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

		// Verify files were created
		agg1File := filepath.Join(baseDir, "agg-1.json")
		if _, err := os.Stat(agg1File); err != nil {
			t.Errorf("expected file %s to exist: %v", agg1File, err)
		}

		agg2File := filepath.Join(baseDir, "agg-2.json")
		if _, err := os.Stat(agg2File); err != nil {
			t.Errorf("expected file %s to exist: %v", agg2File, err)
		}
	})

	t.Run("get event by ID across aggregates", func(t *testing.T) {
		t.Parallel()

		baseDir := setupTestDir(t)
		store, err := infrastructure.NewFileStore(baseDir)
		if err != nil {
			t.Fatalf("failed to create file store: %v", err)
		}
		defer store.Close()
		defer cleanupTestDir(t, baseDir)

		ctx := context.Background()

		// Add events for multiple aggregates
		if err := store.Append(ctx, "agg-1", -1, createTestEvent("agg-1", "event-1", "test.created", 0)); err != nil {
			t.Fatalf("failed to append: %v", err)
		}
		if err := store.Append(ctx, "agg-2", -1, createTestEvent("agg-2", "event-2", "test.created", 0)); err != nil {
			t.Fatalf("failed to append: %v", err)
		}

		// Get event by ID
		event, err := store.GetEventByID(ctx, "event-1")
		if err != nil {
			t.Fatalf("failed to get event by ID: %v", err)
		}

		if event.ID != "event-1" {
			t.Errorf("expected event ID event-1, got %s", event.ID)
		}
		if event.AggregateID != "agg-1" {
			t.Errorf("expected aggregate ID agg-1, got %s", event.AggregateID)
		}
	})
}

func TestFileStore_NewFileStore(t *testing.T) {
	t.Parallel()

	t.Run("create with valid directory", func(t *testing.T) {
		t.Parallel()

		baseDir := setupTestDir(t)
		defer cleanupTestDir(t, baseDir)

		store, err := infrastructure.NewFileStore(baseDir)
		if err != nil {
			t.Fatalf("failed to create file store: %v", err)
		}
		defer store.Close()
	})

	t.Run("create with non-existent directory", func(t *testing.T) {
		t.Parallel()

		baseDir := filepath.Join(t.TempDir(), "nonexistent", "subdir")
		store, err := infrastructure.NewFileStore(baseDir)
		if err != nil {
			t.Fatalf("expected directory to be created, got error: %v", err)
		}
		defer store.Close()

		// Verify directory was created
		if _, err := os.Stat(baseDir); err != nil {
			t.Errorf("expected directory to exist: %v", err)
		}
	})

	t.Run("create with empty directory", func(t *testing.T) {
		t.Parallel()

		_, err := infrastructure.NewFileStore("")
		if err == nil {
			t.Fatal("expected error for empty directory, got nil")
		}
	})
}
