package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

func TestEventStore_Append(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setupStore      func(t *testing.T) domain.EventStore
		aggregateID     string
		expectedVersion int
		events          []domain.EventEnvelope[any]
		wantErr         bool
		errType         error
	}{
		{
			name:            "append single event to new aggregate",
			setupStore:      setupMemoryStore,
			aggregateID:     "agg-1",
			expectedVersion: -1,
			events: []domain.EventEnvelope[any]{
				createTestEvent("agg-1", "event-1", "test.created", 0),
			},
			wantErr: false,
		},
		{
			name:            "append multiple events",
			setupStore:      setupMemoryStore,
			aggregateID:     "agg-2",
			expectedVersion: -1,
			events: []domain.EventEnvelope[any]{
				createTestEvent("agg-2", "event-1", "test.created", 0),
				createTestEvent("agg-2", "event-2", "test.updated", 0),
			},
			wantErr: false,
		},
		{
			name:            "append with version check success",
			setupStore:      setupMemoryStoreWithEvents,
			aggregateID:     "agg-3",
			expectedVersion: 1,
			events: []domain.EventEnvelope[any]{
				createTestEvent("agg-3", "event-2", "test.updated", 0),
			},
			wantErr: false,
		},
		{
			name:            "append with version check failure",
			setupStore:      setupMemoryStoreWithEvents,
			aggregateID:     "agg-3",
			expectedVersion: 0,
			events: []domain.EventEnvelope[any]{
				createTestEvent("agg-3", "event-2", "test.updated", 0),
			},
			wantErr: true,
			errType: domain.ErrConcurrencyConflict,
		},
		{
			name:            "append event with mismatched aggregate ID",
			setupStore:      setupMemoryStore,
			aggregateID:     "agg-5",
			expectedVersion: -1,
			events: []domain.EventEnvelope[any]{
				createTestEvent("agg-6", "event-1", "test.created", 0),
			},
			wantErr: true,
			errType: domain.ErrInvalidEvent,
		},
		{
			name:            "append empty events slice",
			setupStore:      setupMemoryStore,
			aggregateID:     "agg-7",
			expectedVersion: -1,
			events:          []domain.EventEnvelope[any]{},
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := tt.setupStore(t)
			defer store.Close()

			ctx := context.Background()
			err := store.Append(ctx, tt.aggregateID, tt.expectedVersion, tt.events...)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errType != nil && !errors.Is(err, tt.errType) {
					t.Fatalf("expected error type %v, got %v", tt.errType, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestEventStore_GetEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupStore  func(t *testing.T) domain.EventStore
		aggregateID string
		wantCount   int
		wantErr     bool
	}{
		{
			name:        "get events for existing aggregate",
			setupStore:  setupMemoryStoreWithEvents,
			aggregateID: "agg-3",
			wantCount:   1,
			wantErr:     false,
		},
		{
			name:        "get events for non-existent aggregate",
			setupStore:  setupMemoryStore,
			aggregateID: "agg-nonexistent",
			wantCount:   0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := tt.setupStore(t)
			defer store.Close()

			ctx := context.Background()
			events, err := store.GetEvents(ctx, tt.aggregateID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(events) != tt.wantCount {
				t.Fatalf("expected %d events, got %d", tt.wantCount, len(events))
			}

			// Verify events are in order
			for i := 1; i < len(events); i++ {
				if events[i].SequenceNo <= events[i-1].SequenceNo {
					t.Errorf("events not in version order: event %d has version %d, previous has %d",
						i, events[i].SequenceNo, events[i-1].SequenceNo)
				}
			}
		})
	}
}

func TestEventStore_GetEventsFromVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupStore  func(t *testing.T) domain.EventStore
		aggregateID string
		fromVersion int
		wantCount   int
		wantErr     bool
	}{
		{
			name:        "get events from version 1",
			setupStore:  setupMemoryStoreWithMultipleEvents,
			aggregateID: "agg-4",
			fromVersion: 1,
			wantCount:   3,
			wantErr:     false,
		},
		{
			name:        "get events from version 2",
			setupStore:  setupMemoryStoreWithMultipleEvents,
			aggregateID: "agg-4",
			fromVersion: 2,
			wantCount:   2,
			wantErr:     false,
		},
		{
			name:        "get events from version beyond existing",
			setupStore:  setupMemoryStoreWithMultipleEvents,
			aggregateID: "agg-4",
			fromVersion: 10,
			wantCount:   0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := tt.setupStore(t)
			defer store.Close()

			ctx := context.Background()
			events, err := store.GetEventsFromVersion(ctx, tt.aggregateID, tt.fromVersion)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(events) != tt.wantCount {
				t.Fatalf("expected %d events, got %d", tt.wantCount, len(events))
			}

			// Verify all events are >= fromVersion
			for _, event := range events {
				if event.SequenceNo < tt.fromVersion {
					t.Errorf("event version %d is less than fromVersion %d", event.SequenceNo, tt.fromVersion)
				}
			}
		})
	}
}

func TestEventStore_GetEventByID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupStore func(t *testing.T) domain.EventStore
		eventID    string
		wantFound  bool
		wantAggID  string
		wantErr    bool
		errType    error
	}{
		{
			name:       "get existing event by ID",
			setupStore: setupMemoryStoreWithEvents,
			eventID:    "event-1",
			wantFound:  true,
			wantAggID:  "agg-3",
			wantErr:    false,
		},
		{
			name:       "get non-existent event",
			setupStore: setupMemoryStore,
			eventID:    "event-nonexistent",
			wantFound:  false,
			wantErr:    true,
			errType:    domain.ErrEventNotFound,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := tt.setupStore(t)
			defer store.Close()

			ctx := context.Background()
			event, err := store.GetEventByID(ctx, tt.eventID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errType != nil && !errors.Is(err, tt.errType) {
					t.Fatalf("expected error type %v, got %v", tt.errType, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if event.ID != tt.eventID {
				t.Errorf("expected event ID %s, got %s", tt.eventID, event.ID)
			}

			if tt.wantAggID != "" && event.AggregateID != tt.wantAggID {
				t.Errorf("expected aggregate ID %s, got %s", tt.wantAggID, event.AggregateID)
			}
		})
	}
}

func TestEventStore_GetCurrentVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupStore  func(t *testing.T) domain.EventStore
		aggregateID string
		wantVersion int
		wantErr     bool
	}{
		{
			name:        "get version for existing aggregate",
			setupStore:  setupMemoryStoreWithEvents,
			aggregateID: "agg-3",
			wantVersion: 1,
			wantErr:     false,
		},
		{
			name:        "get version for non-existent aggregate",
			setupStore:  setupMemoryStore,
			aggregateID: "agg-nonexistent",
			wantVersion: 0,
			wantErr:     false,
		},
		{
			name:        "get version for aggregate with multiple events",
			setupStore:  setupMemoryStoreWithMultipleEvents,
			aggregateID: "agg-4",
			wantVersion: 3,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := tt.setupStore(t)
			defer store.Close()

			ctx := context.Background()
			version, err := store.GetCurrentVersion(ctx, tt.aggregateID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if version != tt.wantVersion {
				t.Errorf("expected version %d, got %d", tt.wantVersion, version)
			}
		})
	}
}

// Test helpers

func setupMemoryStore(t *testing.T) domain.EventStore {
	t.Helper()
	return infrastructure.NewMemoryStore()
}

func setupMemoryStoreWithEvents(t *testing.T) domain.EventStore {
	t.Helper()
	store := infrastructure.NewMemoryStore()
	ctx := context.Background()

	event := createTestEvent("agg-3", "event-1", "test.created", 0)
	if err := store.Append(ctx, "agg-3", -1, event); err != nil {
		t.Fatalf("failed to setup store: %v", err)
	}

	return store
}

func setupMemoryStoreWithMultipleEvents(t *testing.T) domain.EventStore {
	t.Helper()
	store := infrastructure.NewMemoryStore()
	ctx := context.Background()

	events := []domain.EventEnvelope[any]{
		createTestEvent("agg-4", "event-1", "test.created", 0),
		createTestEvent("agg-4", "event-2", "test.updated", 0),
		createTestEvent("agg-4", "event-3", "test.updated", 0),
	}

	if err := store.Append(ctx, "agg-4", -1, events...); err != nil {
		t.Fatalf("failed to setup store: %v", err)
	}

	return store
}

func createTestEvent(aggregateID, eventID, eventType string, version int) domain.EventEnvelope[any] {
	return domain.EventEnvelope[any]{
		ID:          eventID,
		AggregateID: aggregateID,
		EventType:   eventType,
		Payload:     map[string]interface{}{"test": "data"},
		Created:     time.Now(),
		Version:     version,
		Metadata:    make(map[string]interface{}),
	}
}

func TestToAnyEnvelope(t *testing.T) {
	t.Parallel()

	type TestPayload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	t.Run("conversion preserves all fields", func(t *testing.T) {
		t.Parallel()

		original := domain.NewEventEnvelope(TestPayload{Name: "test", Value: 42}, "agg-1", "test.created")

		// Convert to EventEnvelope[any]
		anyEnvelope := domain.ToAnyEnvelope(original)

		if anyEnvelope.AggregateID != original.AggregateID {
			t.Errorf("aggregate ID mismatch: expected %s, got %s", original.AggregateID, anyEnvelope.AggregateID)
		}
		if anyEnvelope.EventType != original.EventType {
			t.Errorf("event type mismatch: expected %s, got %s", original.EventType, anyEnvelope.EventType)
		}
		if anyEnvelope.ID != original.ID {
			t.Errorf("ID mismatch: expected %s, got %s", original.ID, anyEnvelope.ID)
		}

		// Verify payload can be accessed
		payload, ok := anyEnvelope.Payload.(TestPayload)
		if !ok {
			t.Fatalf("failed to assert payload type")
		}
		if payload.Name != original.Payload.Name {
			t.Errorf("payload name mismatch: expected %s, got %s", original.Payload.Name, payload.Name)
		}
		if payload.Value != original.Payload.Value {
			t.Errorf("payload value mismatch: expected %d, got %d", original.Payload.Value, payload.Value)
		}
	})
}

// Test helpers

func setupTestDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func cleanupTestDir(t *testing.T, dir string) {
	t.Helper()
	os.RemoveAll(dir)
}
