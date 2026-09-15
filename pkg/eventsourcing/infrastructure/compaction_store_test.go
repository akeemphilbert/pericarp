package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

// compactableStores gives each test both implementations of the retire
// capability, so the two never drift on the behaviour compaction depends on.
func compactableStores(t *testing.T) map[string]func(*testing.T) domain.CompactableEventStore {
	t.Helper()
	return map[string]func(*testing.T) domain.CompactableEventStore{
		"memory": func(t *testing.T) domain.CompactableEventStore {
			return infrastructure.NewMemoryStore()
		},
		"sqlite": func(t *testing.T) domain.CompactableEventStore {
			store, err := infrastructure.NewGormEventStore(newTestGormDB(t))
			if err != nil {
				t.Fatalf("failed to create gorm event store: %v", err)
			}
			return store
		},
	}
}

func testManifest(from, to int64, count int) domain.CompactionManifest {
	return domain.CompactionManifest{
		ID:           "manifest-1",
		FromPosition: from,
		ToPosition:   to,
		Watermark:    to,
		EventCount:   count,
		Checksum:     "0f00",
		CreatedAt:    time.Now().UTC(),
	}
}

func TestRetireEvents(t *testing.T) {
	t.Parallel()

	for name, newStore := range compactableStores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			t.Run("removes the events and records the manifest", func(t *testing.T) {
				store := newStore(t)
				seedForRetire(t, store, "agg-1", 3)

				events, err := store.GetEvents(ctx, "agg-1")
				if err != nil {
					t.Fatalf("read events: %v", err)
				}

				manifest := testManifest(events[0].Position, events[1].Position, 2)
				if err := store.RetireEvents(ctx, []string{events[0].ID, events[1].ID}, manifest); err != nil {
					t.Fatalf("retire events: %v", err)
				}

				left, err := store.GetEvents(ctx, "agg-1")
				if err != nil {
					t.Fatalf("read events: %v", err)
				}
				if len(left) != 1 || left[0].ID != events[2].ID {
					t.Fatalf("expected only the third event to survive, got %d events", len(left))
				}

				recorded, err := store.Compactions(ctx)
				if err != nil {
					t.Fatalf("read compactions: %v", err)
				}
				if len(recorded) != 1 || recorded[0].EventCount != 2 || recorded[0].Checksum != "0f00" {
					t.Fatalf("expected the manifest to be recorded, got %+v", recorded)
				}
			})

			t.Run("leaves the retired positions as permanent gaps", func(t *testing.T) {
				store := newStore(t)
				seedForRetire(t, store, "agg-2", 3)

				events, _ := store.GetEvents(ctx, "agg-2")
				retired := events[0].Position
				if err := store.RetireEvents(ctx, []string{events[0].ID}, testManifest(retired, retired, 1)); err != nil {
					t.Fatalf("retire events: %v", err)
				}

				feed, err := store.ReadAfter(ctx, 0, 0)
				if err != nil {
					t.Fatalf("read feed: %v", err)
				}
				for _, event := range feed {
					if event.Position == retired {
						t.Fatalf("position %d was handed out again", retired)
					}
				}
			})

			t.Run("keeps assigning positions above everything it ever retired", func(t *testing.T) {
				// The whole aggregate goes, so the surviving rows say nothing
				// about how far the feed reached. A store that reset to
				// MAX(position)+1 here would reuse a position a feed reader
				// has already consumed.
				store := newStore(t)
				seedForRetire(t, store, "agg-3", 3)

				events, _ := store.GetEvents(ctx, "agg-3")
				ids := []string{events[0].ID, events[1].ID, events[2].ID}
				highest := events[2].Position
				if err := store.RetireEvents(ctx, ids, testManifest(events[0].Position, highest, 3)); err != nil {
					t.Fatalf("retire events: %v", err)
				}

				next := createTestEvent("agg-4", "after-compaction", "test.created", 1)
				if err := store.Append(ctx, "agg-4", -1, next); err != nil {
					t.Fatalf("append after retire: %v", err)
				}

				appended, err := store.GetEvents(ctx, "agg-4")
				if err != nil {
					t.Fatalf("read events: %v", err)
				}
				if appended[0].Position <= highest {
					t.Fatalf("expected a position above %d, got %d", highest, appended[0].Position)
				}
			})

			t.Run("forgets an aggregate whose whole history is retired", func(t *testing.T) {
				store := newStore(t)
				seedForRetire(t, store, "agg-5", 2)

				events, _ := store.GetEvents(ctx, "agg-5")
				ids := []string{events[0].ID, events[1].ID}
				if err := store.RetireEvents(ctx, ids, testManifest(events[0].Position, events[1].Position, 2)); err != nil {
					t.Fatalf("retire events: %v", err)
				}

				version, err := store.GetCurrentVersion(ctx, "agg-5")
				if err != nil {
					t.Fatalf("read version: %v", err)
				}
				if version != 0 {
					t.Fatalf("expected a retired aggregate at version 0, got %d", version)
				}
			})

			t.Run("refuses a batch naming an event it does not hold", func(t *testing.T) {
				store := newStore(t)
				seedForRetire(t, store, "agg-6", 2)

				events, _ := store.GetEvents(ctx, "agg-6")
				err := store.RetireEvents(ctx, []string{events[0].ID, "no-such-event"}, testManifest(1, 2, 2))
				if !errors.Is(err, domain.ErrEventNotFound) {
					t.Fatalf("expected ErrEventNotFound, got %v", err)
				}

				left, _ := store.GetEvents(ctx, "agg-6")
				if len(left) != 2 {
					t.Fatalf("expected the refused batch to change nothing, %d events left", len(left))
				}
				recorded, _ := store.Compactions(ctx)
				if len(recorded) != 0 {
					t.Fatalf("expected no manifest from a refused batch, got %d", len(recorded))
				}
			})

			t.Run("records nothing before it is asked to", func(t *testing.T) {
				store := newStore(t)
				recorded, err := store.Compactions(ctx)
				if err != nil {
					t.Fatalf("read compactions: %v", err)
				}
				if len(recorded) != 0 {
					t.Fatalf("expected a fresh store to record no compactions, got %d", len(recorded))
				}
			})
		})
	}
}

func seedForRetire(t *testing.T, store domain.EventStore, aggregateID string, count int) {
	t.Helper()
	ctx := context.Background()
	for i := 1; i <= count; i++ {
		event := createTestEvent(aggregateID, aggregateID+"-"+string(rune('a'+i-1)), "test.happened", i)
		if err := store.Append(ctx, aggregateID, i-1, event); err != nil {
			t.Fatalf("seed %s: %v", aggregateID, err)
		}
	}
}

func TestMemoryStoreSeedEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("preserves the positions it is given", func(t *testing.T) {
		t.Parallel()
		store := infrastructure.NewMemoryStore()

		first := createTestEvent("agg-1", "e1", "test.created", 1)
		first.Position = 4
		second := createTestEvent("agg-1", "e2", "test.updated", 2)
		second.Position = 9

		if err := store.SeedEvents(ctx, first, second); err != nil {
			t.Fatalf("seed: %v", err)
		}

		events, err := store.GetEvents(ctx, "agg-1")
		if err != nil {
			t.Fatalf("read events: %v", err)
		}
		if len(events) != 2 || events[0].Position != 4 || events[1].Position != 9 {
			t.Fatalf("expected positions 4 and 9, got %+v", events)
		}

		version, _ := store.GetCurrentVersion(ctx, "agg-1")
		if version != 2 {
			t.Fatalf("expected version 2, got %d", version)
		}

		next := createTestEvent("agg-1", "e3", "test.updated", 3)
		if err := store.Append(ctx, "agg-1", 2, next); err != nil {
			t.Fatalf("append after seed: %v", err)
		}
		appended, _ := store.GetEvents(ctx, "agg-1")
		if got := appended[2].Position; got != 10 {
			t.Fatalf("expected the next position to continue at 10, got %d", got)
		}
	})

	t.Run("refuses a position the feed has already passed", func(t *testing.T) {
		t.Parallel()
		store := infrastructure.NewMemoryStore()

		first := createTestEvent("agg-2", "e1", "test.created", 1)
		first.Position = 5
		if err := store.SeedEvents(ctx, first); err != nil {
			t.Fatalf("seed: %v", err)
		}

		behind := createTestEvent("agg-2", "e2", "test.updated", 2)
		behind.Position = 3
		if err := store.SeedEvents(ctx, behind); !errors.Is(err, domain.ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent for a position below the last one, got %v", err)
		}
	})
}
