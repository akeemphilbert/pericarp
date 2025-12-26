package application_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/application"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

// TestEntity is a simple entity for testing that embeds BaseEntity
type TestEntity struct {
	*ddd.BaseEntity
	name  string
	email string
}

func NewTestEntity(id, name, email string) *TestEntity {
	return &TestEntity{
		BaseEntity: ddd.NewBaseEntity(id),
		name:       name,
		email:      email,
	}
}

func TestNewSimpleUnitOfWork(t *testing.T) {
	t.Parallel()

	t.Run("creates new unit of work", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		dispatcher := domain.NewEventDispatcher()

		uow := application.NewSimpleUnitOfWork(eventStore, dispatcher)
		if uow == nil {
			t.Fatal("Expected non-nil SimpleUnitOfWork")
		}
	})

	t.Run("creates unit of work without dispatcher", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()

		uow := application.NewSimpleUnitOfWork(eventStore, nil)
		if uow == nil {
			t.Fatal("Expected non-nil SimpleUnitOfWork")
		}
	})
}

func TestTrack(t *testing.T) {
	t.Parallel()

	t.Run("track single entity", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity := NewTestEntity("entity-1", "Test", "test@example.com")
		if err := uow.Track(entity); err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("track multiple entities", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity1 := NewTestEntity("entity-1", "Test1", "test1@example.com")
		entity2 := NewTestEntity("entity-2", "Test2", "test2@example.com")
		entity3 := NewTestEntity("entity-3", "Test3", "test3@example.com")

		if err := uow.Track(entity1); err != nil {
			t.Fatalf("Failed to track entity1: %v", err)
		}
		if err := uow.Track(entity2); err != nil {
			t.Fatalf("Failed to track entity2: %v", err)
		}
		if err := uow.Track(entity3); err != nil {
			t.Fatalf("Failed to track entity3: %v", err)
		}
	})

	t.Run("track multiple entities in single call", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity1 := NewTestEntity("entity-1", "Test1", "test1@example.com")
		entity2 := NewTestEntity("entity-2", "Test2", "test2@example.com")
		entity3 := NewTestEntity("entity-3", "Test3", "test3@example.com")

		if err := uow.Track(entity1, entity2, entity3); err != nil {
			t.Fatalf("Failed to track entities: %v", err)
		}
	})

	t.Run("track empty array does nothing", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		if err := uow.Track(); err != nil {
			t.Errorf("Expected no error for empty array, got %v", err)
		}
	})

	t.Run("nil entity returns error", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		err := uow.Track(nil)
		if err == nil {
			t.Error("Expected error for nil entity")
		}
		if err.Error() != "entity cannot be nil" {
			t.Errorf("Expected 'entity cannot be nil', got %q", err.Error())
		}
	})

	t.Run("duplicate entity returns error", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity := NewTestEntity("entity-1", "Test", "test@example.com")
		if err := uow.Track(entity); err != nil {
			t.Fatalf("Failed to track entity: %v", err)
		}

		// Try to track same entity again
		err := uow.Track(entity)
		if err == nil {
			t.Error("Expected error for duplicate entity")
		}
		if err.Error() != "entity with aggregate ID \"entity-1\" is already tracked" {
			t.Errorf("Expected duplicate tracking error, got %q", err.Error())
		}
	})

	t.Run("duplicate entities in same batch returns error", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity1 := NewTestEntity("entity-1", "Test1", "test1@example.com")
		entity2 := NewTestEntity("entity-1", "Test2", "test2@example.com")

		err := uow.Track(entity1, entity2)
		if err == nil {
			t.Error("Expected error for duplicate entities in batch")
		}
		if err.Error() != "duplicate entity with aggregate ID \"entity-1\" in batch" {
			t.Errorf("Expected duplicate in batch error, got %q", err.Error())
		}
	})
}

func TestCommit(t *testing.T) {
	t.Parallel()

	t.Run("commit with single entity", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity := NewTestEntity("entity-1", "Test", "test@example.com")
		if err := uow.Track(entity); err != nil {
			t.Fatalf("Failed to track entity: %v", err)
		}

		// Record an event
		if err := entity.RecordEvent(map[string]string{"name": "Test"}, "test.created"); err != nil {
			t.Fatalf("Failed to record event: %v", err)
		}

		ctx := context.Background()
		if err := uow.Commit(ctx); err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Verify event was persisted
		events, err := eventStore.GetEvents(ctx, "entity-1")
		if err != nil {
			t.Fatalf("Failed to get events: %v", err)
		}
		if len(events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(events))
		}

		// Verify uncommitted events were cleared
		uncommitted := entity.GetUncommittedEvents()
		if len(uncommitted) != 0 {
			t.Errorf("Expected 0 uncommitted events, got %d", len(uncommitted))
		}
	})

	t.Run("commit with multiple entities", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity1 := NewTestEntity("entity-1", "Test1", "test1@example.com")
		entity2 := NewTestEntity("entity-2", "Test2", "test2@example.com")
		entity3 := NewTestEntity("entity-3", "Test3", "test3@example.com")

		if err := uow.Track(entity1); err != nil {
			t.Fatalf("Failed to track entity1: %v", err)
		}
		if err := uow.Track(entity2); err != nil {
			t.Fatalf("Failed to track entity2: %v", err)
		}
		if err := uow.Track(entity3); err != nil {
			t.Fatalf("Failed to track entity3: %v", err)
		}

		// Record events on each entity
		if err := entity1.RecordEvent(map[string]string{"name": "Test1"}, "test.created"); err != nil {
			t.Fatalf("Failed to record event on entity1: %v", err)
		}
		if err := entity2.RecordEvent(map[string]string{"name": "Test2"}, "test.created"); err != nil {
			t.Fatalf("Failed to record event on entity2: %v", err)
		}
		if err := entity3.RecordEvent(map[string]string{"name": "Test3"}, "test.created"); err != nil {
			t.Fatalf("Failed to record event on entity3: %v", err)
		}

		ctx := context.Background()
		if err := uow.Commit(ctx); err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Verify all events were persisted
		events1, _ := eventStore.GetEvents(ctx, "entity-1")
		events2, _ := eventStore.GetEvents(ctx, "entity-2")
		events3, _ := eventStore.GetEvents(ctx, "entity-3")

		if len(events1) != 1 {
			t.Errorf("Expected 1 event for entity1, got %d", len(events1))
		}
		if len(events2) != 1 {
			t.Errorf("Expected 1 event for entity2, got %d", len(events2))
		}
		if len(events3) != 1 {
			t.Errorf("Expected 1 event for entity3, got %d", len(events3))
		}

		// Verify uncommitted events were cleared from all entities
		if len(entity1.GetUncommittedEvents()) != 0 {
			t.Error("Expected entity1 to have no uncommitted events")
		}
		if len(entity2.GetUncommittedEvents()) != 0 {
			t.Error("Expected entity2 to have no uncommitted events")
		}
		if len(entity3.GetUncommittedEvents()) != 0 {
			t.Error("Expected entity3 to have no uncommitted events")
		}
	})

	t.Run("commit with no uncommitted events", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity := NewTestEntity("entity-1", "Test", "test@example.com")
		if err := uow.Track(entity); err != nil {
			t.Fatalf("Failed to track entity: %v", err)
		}

		ctx := context.Background()
		if err := uow.Commit(ctx); err != nil {
			t.Fatalf("Commit failed: %v", err)
		}
	})

	t.Run("commit with event dispatch", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		dispatcher := domain.NewEventDispatcher()

		callCount := 0
		handler := func(ctx context.Context, env domain.EventEnvelope[map[string]string]) error {
			callCount++
			return nil
		}

		if err := domain.Subscribe[map[string]string](dispatcher, "test.created", handler); err != nil {
			t.Fatalf("Failed to subscribe handler: %v", err)
		}

		uow := application.NewSimpleUnitOfWork(eventStore, dispatcher)

		entity := NewTestEntity("entity-1", "Test", "test@example.com")
		if err := uow.Track(entity); err != nil {
			t.Fatalf("Failed to track entity: %v", err)
		}

		// Record an event
		if err := entity.RecordEvent(map[string]string{"name": "Test"}, "test.created"); err != nil {
			t.Fatalf("Failed to record event: %v", err)
		}

		ctx := context.Background()
		if err := uow.Commit(ctx); err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Give dispatcher time to process (since it's async)
		time.Sleep(10 * time.Millisecond)

		if callCount != 1 {
			t.Errorf("Expected handler to be called once, got %d", callCount)
		}
	})

	t.Run("commit without dispatcher", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity := NewTestEntity("entity-1", "Test", "test@example.com")
		if err := uow.Track(entity); err != nil {
			t.Fatalf("Failed to track entity: %v", err)
		}

		if err := entity.RecordEvent(map[string]string{"name": "Test"}, "test.created"); err != nil {
			t.Fatalf("Failed to record event: %v", err)
		}

		ctx := context.Background()
		if err := uow.Commit(ctx); err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Verify event was persisted even without dispatcher
		events, err := eventStore.GetEvents(ctx, "entity-1")
		if err != nil {
			t.Fatalf("Failed to get events: %v", err)
		}
		if len(events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(events))
		}
	})
}

func TestCommitFailure(t *testing.T) {
	t.Parallel()

	t.Run("commit failure triggers rollback", func(t *testing.T) {
		t.Parallel()
		// Create a mock event store that fails on append
		eventStore := &failingEventStore{
			MemoryStore: infrastructure.NewMemoryStore(),
			shouldFail:  true,
		}
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity := NewTestEntity("entity-1", "Test", "test@example.com")
		if err := uow.Track(entity); err != nil {
			t.Fatalf("Failed to track entity: %v", err)
		}

		if err := entity.RecordEvent(map[string]string{"name": "Test"}, "test.created"); err != nil {
			t.Fatalf("Failed to record event: %v", err)
		}

		ctx := context.Background()
		err := uow.Commit(ctx)
		if err == nil {
			t.Error("Expected commit to fail")
		}

		// Verify uncommitted events are still present (not cleared)
		uncommitted := entity.GetUncommittedEvents()
		if len(uncommitted) != 1 {
			t.Errorf("Expected 1 uncommitted event after failed commit, got %d", len(uncommitted))
		}

		// Verify event was not persisted
		events, _ := eventStore.GetEvents(ctx, "entity-1")
		if len(events) != 0 {
			t.Errorf("Expected 0 events after failed commit, got %d", len(events))
		}
	})

	t.Run("optimistic concurrency conflict", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()

		// First, create and commit an initial entity
		entity1 := NewTestEntity("entity-1", "Test", "test@example.com")
		uow1 := application.NewSimpleUnitOfWork(eventStore, nil)
		if err := uow1.Track(entity1); err != nil {
			t.Fatalf("Failed to track entity1: %v", err)
		}
		if err := entity1.RecordEvent(map[string]string{"name": "Test1"}, "test.created"); err != nil {
			t.Fatalf("Failed to record event: %v", err)
		}

		ctx := context.Background()

		// Commit first UoW (succeeds, store version becomes 0, event has sequenceNo 0)
		if err := uow1.Commit(ctx); err != nil {
			t.Fatalf("First commit failed: %v", err)
		}

		// Simulate two concurrent reads: both entities load from store at the same time
		// Both see version 0, so both track with expectedVersion 0
		entity2 := NewTestEntity("entity-1", "Test", "test@example.com")
		events, err := eventStore.GetEvents(ctx, "entity-1")
		if err != nil {
			t.Fatalf("Failed to get events: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(events))
		}
		// Apply the single event to entity2 to simulate loading from store
		// entity2 starts with sequenceNo -1, event has sequenceNo 0, so this should work
		if err := entity2.ApplyEvent(ctx, events[0]); err != nil {
			t.Fatalf("Failed to apply event: %v", err)
		}
		// entity2 now has sequenceNo 0

		entity3 := NewTestEntity("entity-1", "Test", "test@example.com")
		events2, err := eventStore.GetEvents(ctx, "entity-1")
		if err != nil {
			t.Fatalf("Failed to get events: %v", err)
		}
		if len(events2) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(events2))
		}
		// Apply the single event to entity3 to simulate loading from store
		if err := entity3.ApplyEvent(ctx, events2[0]); err != nil {
			t.Fatalf("Failed to apply event: %v", err)
		}
		// entity3 now has sequenceNo 0

		// Both entities now have sequenceNo 0
		// Track both with separate UoWs (both capture expectedVersion = 0)
		uow2 := application.NewSimpleUnitOfWork(eventStore, nil)
		if err := uow2.Track(entity2); err != nil {
			t.Fatalf("Failed to track entity2: %v", err)
		}
		if err := entity2.RecordEvent(map[string]string{"name": "Test2"}, "test.updated"); err != nil {
			t.Fatalf("Failed to record event: %v", err)
		}

		uow3 := application.NewSimpleUnitOfWork(eventStore, nil)
		if err := uow3.Track(entity3); err != nil {
			t.Fatalf("Failed to track entity3: %v", err)
		}
		if err := entity3.RecordEvent(map[string]string{"name": "Test3"}, "test.updated"); err != nil {
			t.Fatalf("Failed to record event: %v", err)
		}

		// Commit first concurrent transaction (succeeds, store version becomes 1)
		if err := uow2.Commit(ctx); err != nil {
			t.Fatalf("Second commit failed: %v", err)
		}

		// Try to commit second concurrent transaction (should fail - expectedVersion 0 but store version is now 1)
		err = uow3.Commit(ctx)
		if err == nil {
			t.Error("Expected commit to fail due to concurrency conflict")
		}
		if err != nil && err.Error() == "" {
			t.Error("Expected non-empty error message")
		}
		// Verify it's a concurrency conflict error
		if !errors.Is(err, domain.ErrConcurrencyConflict) {
			t.Errorf("Expected ErrConcurrencyConflict, got: %v", err)
		}
	})
}

func TestRollback(t *testing.T) {
	t.Parallel()

	t.Run("rollback clears tracking", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity := NewTestEntity("entity-1", "Test", "test@example.com")
		if err := uow.Track(entity); err != nil {
			t.Fatalf("Failed to track entity: %v", err)
		}

		if err := uow.Rollback(); err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}

		// Try to track again (should succeed since tracking was cleared)
		if err := uow.Track(entity); err != nil {
			t.Errorf("Expected to be able to track entity after rollback, got error: %v", err)
		}
	})

	t.Run("rollback preserves uncommitted events", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		entity := NewTestEntity("entity-1", "Test", "test@example.com")
		if err := uow.Track(entity); err != nil {
			t.Fatalf("Failed to track entity: %v", err)
		}

		// Record an event
		if err := entity.RecordEvent(map[string]string{"name": "Test"}, "test.created"); err != nil {
			t.Fatalf("Failed to record event: %v", err)
		}

		// Rollback
		if err := uow.Rollback(); err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}

		// Verify uncommitted events are still present
		uncommitted := entity.GetUncommittedEvents()
		if len(uncommitted) != 1 {
			t.Errorf("Expected 1 uncommitted event after rollback, got %d", len(uncommitted))
		}
	})
}

func TestConcurrentUnitOfWork(t *testing.T) {
	t.Parallel()

	t.Run("thread-safe concurrent operations", func(t *testing.T) {
		t.Parallel()
		eventStore := infrastructure.NewMemoryStore()
		uow := application.NewSimpleUnitOfWork(eventStore, nil)

		var wg sync.WaitGroup
		concurrency := 10

		// Concurrent track operations
		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func(id int) {
				defer wg.Done()
				entity := NewTestEntity(fmt.Sprintf("entity-%d", id), "Test", "test@example.com")
				if err := uow.Track(entity); err != nil {
					t.Errorf("Failed to track entity %d: %v", id, err)
				}
			}(i)
		}

		wg.Wait()
	})
}

// failingEventStore is a test helper that fails on Append
type failingEventStore struct {
	*infrastructure.MemoryStore
	shouldFail bool
}

func (f *failingEventStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	if f.shouldFail {
		return errors.New("append failed")
	}
	return f.MemoryStore.Append(ctx, aggregateID, expectedVersion, events...)
}
