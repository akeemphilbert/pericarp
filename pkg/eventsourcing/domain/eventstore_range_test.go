package domain_test

import (
	"context"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

func TestEventStore_GetEventsRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupStore  func(t *testing.T) domain.EventStore
		aggregateID string
		fromVersion int
		toVersion   int
		wantCount   int
		wantErr     bool
	}{
		{
			name:           "get events from version 1 to 2",
			setupStore:     setupMemoryStoreWithMultipleEvents,
			aggregateID:    "agg-4",
			fromSequenceNo: 1,
			toSequenceNo:   2,
			wantCount:      2,
			wantErr:        false,
		},
		{
			name:           "get events from version 2 to 3",
			setupStore:     setupMemoryStoreWithMultipleEvents,
			aggregateID:    "agg-4",
			fromSequenceNo: 2,
			toSequenceNo:   3,
			wantCount:      2,
			wantErr:        false,
		},
		{
			name:           "get events with default fromVersion (1)",
			setupStore:     setupMemoryStoreWithMultipleEvents,
			aggregateID:    "agg-4",
			fromSequenceNo: -1,
			toSequenceNo:   2,
			wantCount:      2,
			wantErr:        false,
		},
		{
			name:           "get events with toVersion -1 (all remaining)",
			setupStore:     setupMemoryStoreWithMultipleEvents,
			aggregateID:    "agg-4",
			fromSequenceNo: 2,
			toSequenceNo:   -1,
			wantCount:      2,
			wantErr:        false,
		},
		{
			name:           "get events with both defaults (all events)",
			setupStore:     setupMemoryStoreWithMultipleEvents,
			aggregateID:    "agg-4",
			fromSequenceNo: -1,
			toSequenceNo:   -1,
			wantCount:      3,
			wantErr:        false,
		},
		{
			name:           "get events with range beyond existing",
			setupStore:     setupMemoryStoreWithMultipleEvents,
			aggregateID:    "agg-4",
			fromSequenceNo: 10,
			toSequenceNo:   20,
			wantCount:      0,
			wantErr:        false,
		},
		{
			name:           "get events with toVersion before fromVersion",
			setupStore:     setupMemoryStoreWithMultipleEvents,
			aggregateID:    "agg-4",
			fromSequenceNo: 3,
			toSequenceNo:   1,
			wantCount:      0,
			wantErr:        false,
		},
		{
			name:           "get events for non-existent aggregate",
			setupStore:     setupMemoryStore,
			aggregateID:    "agg-nonexistent",
			fromSequenceNo: 1,
			toSequenceNo:   10,
			wantCount:      0,
			wantErr:        false,
		},
		{
			name:           "get single event in range",
			setupStore:     setupMemoryStoreWithMultipleEvents,
			aggregateID:    "agg-4",
			fromSequenceNo: 2,
			toSequenceNo:   2,
			wantCount:      1,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := tt.setupStore(t)
			defer store.Close()

			ctx := context.Background()
			events, err := store.GetEventsRange(ctx, tt.aggregateID, tt.fromVersion, tt.toVersion)

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

			// Verify all events are within the range
			expectedFrom := tt.fromVersion
			if expectedFrom == -1 {
				expectedFrom = 1
			}
			expectedTo := tt.toVersion
			if expectedTo == -1 {
				// Check that all events from fromVersion onwards are included
				for _, event := range events {
					if event.SequenceNo < expectedFrom {
						t.Errorf("event version %d is less than fromVersion %d", event.SequenceNo, expectedFrom)
					}
				}
			} else {
				for _, event := range events {
					if event.SequenceNo < expectedFrom || event.SequenceNo > expectedTo {
						t.Errorf("event version %d is outside range [%d, %d]", event.SequenceNo, expectedFrom, expectedTo)
					}
				}
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

func TestEventStore_GetEventsRange_FileStore(t *testing.T) {
	t.Parallel()

	t.Run("file store range retrieval", func(t *testing.T) {
		t.Parallel()

		baseDir := setupTestDir(t)
		store, err := infrastructure.NewFileStore(baseDir)
		if err != nil {
			t.Fatalf("failed to create file store: %v", err)
		}
		defer store.Close()
		defer cleanupTestDir(t, baseDir)

		ctx := context.Background()
		aggregateID := "range-test"

		// Append multiple events
		events := []domain.EventEnvelope[any]{
			createTestEvent(aggregateID, "event-1", "test.created", 0),
			createTestEvent(aggregateID, "event-2", "test.updated", 0),
			createTestEvent(aggregateID, "event-3", "test.updated", 0),
			createTestEvent(aggregateID, "event-4", "test.updated", 0),
		}

		if err := store.Append(ctx, aggregateID, -1, events...); err != nil {
			t.Fatalf("failed to append events: %v", err)
		}

		// Test range retrieval
		rangeEvents, err := store.GetEventsRange(ctx, aggregateID, 2, 3)
		if err != nil {
			t.Fatalf("failed to get events range: %v", err)
		}

		if len(rangeEvents) != 2 {
			t.Fatalf("expected 2 events in range, got %d", len(rangeEvents))
		}

		if rangeEvents[0].SequenceNo != 2 {
			t.Errorf("expected first event version 2, got %d", rangeEvents[0].SequenceNo)
		}
		if rangeEvents[1].SequenceNo != 3 {
			t.Errorf("expected second event version 3, got %d", rangeEvents[1].SequenceNo)
		}

		// Test with default fromVersion
		allFromStart, err := store.GetEventsRange(ctx, aggregateID, -1, 2)
		if err != nil {
			t.Fatalf("failed to get events range: %v", err)
		}

		if len(allFromStart) != 2 {
			t.Fatalf("expected 2 events from start to version 2, got %d", len(allFromStart))
		}

		// Test with toVersion -1 (all remaining)
		allFromVersion2, err := store.GetEventsRange(ctx, aggregateID, 2, -1)
		if err != nil {
			t.Fatalf("failed to get events range: %v", err)
		}

		if len(allFromVersion2) != 3 {
			t.Fatalf("expected 3 events from version 2 onwards, got %d", len(allFromVersion2))
		}
	})
}
