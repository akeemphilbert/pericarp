package infrastructure_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

// stripPositionsFromFiles removes the "position" field from every persisted
// event in dir, simulating files written before global positions existed.
func stripPositionsFromFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry.Name(), err)
		}
		var events []map[string]any
		if err := json.Unmarshal(data, &events); err != nil {
			t.Fatalf("failed to unmarshal %s: %v", entry.Name(), err)
		}
		for _, event := range events {
			delete(event, "position")
		}
		updated, err := json.Marshal(events)
		if err != nil {
			t.Fatalf("failed to marshal %s: %v", entry.Name(), err)
		}
		if err := os.WriteFile(path, updated, 0644); err != nil {
			t.Fatalf("failed to write %s: %v", entry.Name(), err)
		}
	}
}

func setupFileStore(t *testing.T) domain.EventStore {
	t.Helper()
	store, err := infrastructure.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}
	return store
}

// appendFeedFixture appends five events across three aggregates in three
// separate Append calls, establishing a known global commit order.
func appendFeedFixture(t *testing.T, store domain.EventStore) []string {
	t.Helper()
	ctx := context.Background()

	if err := store.Append(ctx, "agg-a", -1,
		createTestEvent("agg-a", "ev-1", "test.created", 1),
		createTestEvent("agg-a", "ev-2", "test.updated", 2),
	); err != nil {
		t.Fatalf("failed to append agg-a events: %v", err)
	}
	if err := store.Append(ctx, "agg-b", -1,
		createTestEvent("agg-b", "ev-3", "test.created", 1),
	); err != nil {
		t.Fatalf("failed to append agg-b events: %v", err)
	}
	if err := store.Append(ctx, "agg-c", -1,
		createTestEvent("agg-c", "ev-4", "test.created", 1),
		createTestEvent("agg-c", "ev-5", "test.updated", 2),
	); err != nil {
		t.Fatalf("failed to append agg-c events: %v", err)
	}

	return []string{"ev-1", "ev-2", "ev-3", "ev-4", "ev-5"}
}

func assertEventIDs(t *testing.T, events []domain.EventEnvelope[any], want []string) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("expected %d events, got %d", len(want), len(events))
	}
	for i, event := range events {
		if event.ID != want[i] {
			t.Errorf("event %d: expected ID %q, got %q", i, want[i], event.ID)
		}
	}
}

func TestEventStore_ReadAfter(t *testing.T) {
	t.Parallel()

	stores := []struct {
		name       string
		setupStore func(t *testing.T) domain.EventStore
	}{
		{name: "memory", setupStore: setupMemoryStore},
		{name: "gorm", setupStore: setupGormStore},
		{name: "file", setupStore: setupFileStore},
	}

	for _, st := range stores {
		t.Run(st.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			t.Run("returns all events in commit order from position zero", func(t *testing.T) {
				store := st.setupStore(t)
				defer func() { _ = store.Close() }()
				want := appendFeedFixture(t, store)

				events, err := store.ReadAfter(ctx, 0, 0)
				if err != nil {
					t.Fatalf("ReadAfter failed: %v", err)
				}
				assertEventIDs(t, events, want)

				var lastPos int64
				for _, event := range events {
					if event.Position <= lastPos {
						t.Errorf("event %s: position %d not strictly increasing (previous %d)",
							event.ID, event.Position, lastPos)
					}
					lastPos = event.Position
				}
			})

			t.Run("resumes after a given position", func(t *testing.T) {
				store := st.setupStore(t)
				defer func() { _ = store.Close() }()
				want := appendFeedFixture(t, store)

				head, err := store.ReadAfter(ctx, 0, 2)
				if err != nil {
					t.Fatalf("ReadAfter failed: %v", err)
				}
				assertEventIDs(t, head, want[:2])

				rest, err := store.ReadAfter(ctx, head[len(head)-1].Position, 0)
				if err != nil {
					t.Fatalf("ReadAfter failed: %v", err)
				}
				assertEventIDs(t, rest, want[2:])
			})

			t.Run("respects limit", func(t *testing.T) {
				store := st.setupStore(t)
				defer func() { _ = store.Close() }()
				want := appendFeedFixture(t, store)

				events, err := store.ReadAfter(ctx, 0, 3)
				if err != nil {
					t.Fatalf("ReadAfter failed: %v", err)
				}
				assertEventIDs(t, events, want[:3])
			})

			t.Run("returns empty past the head", func(t *testing.T) {
				store := st.setupStore(t)
				defer func() { _ = store.Close() }()
				appendFeedFixture(t, store)

				all, err := store.ReadAfter(ctx, 0, 0)
				if err != nil {
					t.Fatalf("ReadAfter failed: %v", err)
				}
				events, err := store.ReadAfter(ctx, all[len(all)-1].Position, 0)
				if err != nil {
					t.Fatalf("ReadAfter failed: %v", err)
				}
				if len(events) != 0 {
					t.Errorf("expected no events past the head, got %d", len(events))
				}
			})

			t.Run("returns empty on an empty store", func(t *testing.T) {
				store := st.setupStore(t)
				defer func() { _ = store.Close() }()

				events, err := store.ReadAfter(ctx, 0, 10)
				if err != nil {
					t.Fatalf("ReadAfter failed: %v", err)
				}
				if len(events) != 0 {
					t.Errorf("expected no events, got %d", len(events))
				}
			})

			t.Run("does not mutate caller envelopes", func(t *testing.T) {
				store := st.setupStore(t)
				defer func() { _ = store.Close() }()

				event := createTestEvent("agg-a", "ev-1", "test.created", 1)
				if err := store.Append(ctx, "agg-a", -1, event); err != nil {
					t.Fatalf("failed to append: %v", err)
				}
				if event.Position != 0 {
					t.Errorf("store leaked its assigned position %d into the caller's envelope", event.Position)
				}
			})
		})
	}
}

func TestEventStore_HeadPosition(t *testing.T) {
	t.Parallel()

	stores := []struct {
		name       string
		setupStore func(t *testing.T) domain.EventStore
	}{
		{name: "memory", setupStore: setupMemoryStore},
		{name: "gorm", setupStore: setupGormStore},
		{name: "file", setupStore: setupFileStore},
	}

	for _, st := range stores {
		t.Run(st.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			store := st.setupStore(t)
			defer func() { _ = store.Close() }()

			head, err := store.HeadPosition(ctx)
			if err != nil {
				t.Fatalf("HeadPosition failed: %v", err)
			}
			if head != 0 {
				t.Errorf("expected head 0 on empty store, got %d", head)
			}

			appendFeedFixture(t, store)
			events, err := store.ReadAfter(ctx, 0, 0)
			if err != nil {
				t.Fatalf("ReadAfter failed: %v", err)
			}
			head, err = store.HeadPosition(ctx)
			if err != nil {
				t.Fatalf("HeadPosition failed: %v", err)
			}
			if want := events[len(events)-1].Position; head != want {
				t.Errorf("expected head %d (last readable position), got %d", want, head)
			}
		})
	}
}

func TestDynamoStore_FeedNotSupported(t *testing.T) {
	t.Parallel()

	// ReadAfter/HeadPosition never touch DynamoDB, so a client that points nowhere is fine.
	store := infrastructure.NewDynamoEventStore(dynamodb.New(dynamodb.Options{Region: "us-east-1"}), "events")
	_, err := store.ReadAfter(context.Background(), 0, 10)
	if !errors.Is(err, domain.ErrGlobalOrderingNotSupported) {
		t.Fatalf("expected ErrGlobalOrderingNotSupported from ReadAfter, got %v", err)
	}
	_, err = store.HeadPosition(context.Background())
	if !errors.Is(err, domain.ErrGlobalOrderingNotSupported) {
		t.Fatalf("expected ErrGlobalOrderingNotSupported from HeadPosition, got %v", err)
	}
}

func TestFileStore_ReadAfter_PositionsSurviveRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()

	store, err := infrastructure.NewFileStore(dir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}
	if err := store.Append(ctx, "agg-a", -1,
		createTestEvent("agg-a", "ev-1", "test.created", 1),
	); err != nil {
		t.Fatalf("failed to append: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("failed to close store: %v", err)
	}

	reopened, err := infrastructure.NewFileStore(dir)
	if err != nil {
		t.Fatalf("failed to reopen file store: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	if err := reopened.Append(ctx, "agg-b", -1,
		createTestEvent("agg-b", "ev-2", "test.created", 1),
	); err != nil {
		t.Fatalf("failed to append after reopen: %v", err)
	}

	events, err := reopened.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-1", "ev-2"})
	if events[0].Position >= events[1].Position {
		t.Errorf("positions not monotonic across restart: %d then %d",
			events[0].Position, events[1].Position)
	}
}

func TestFileStore_ReadAfter_BackfillsLegacyEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()

	// Write events through a store, then strip their positions on disk to
	// simulate files created before global positions existed.
	store, err := infrastructure.NewFileStore(dir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}
	// KSUID-style ordering: ev-1 < ev-2 lexically, matching creation order.
	if err := store.Append(ctx, "agg-a", -1, createTestEvent("agg-a", "ev-1", "test.created", 1)); err != nil {
		t.Fatalf("failed to append: %v", err)
	}
	if err := store.Append(ctx, "agg-b", -1, createTestEvent("agg-b", "ev-2", "test.created", 1)); err != nil {
		t.Fatalf("failed to append: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("failed to close store: %v", err)
	}
	stripPositionsFromFiles(t, dir)

	reopened, err := infrastructure.NewFileStore(dir)
	if err != nil {
		t.Fatalf("failed to reopen file store: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	events, err := reopened.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-1", "ev-2"})
	if events[0].Position != 1 || events[1].Position != 2 {
		t.Errorf("expected backfilled positions 1 and 2, got %d and %d",
			events[0].Position, events[1].Position)
	}
}

func TestGormStore_ReadAfter_BackfillsLegacyRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestGormDB(t)

	// Recreate the pre-position schema and rows by hand.
	legacyDDL := `CREATE TABLE events (
		id TEXT PRIMARY KEY,
		aggregate_id TEXT,
		event_type TEXT,
		sequence_no INTEGER,
		transaction_id TEXT,
		payload TEXT,
		metadata TEXT,
		created_at DATETIME
	)`
	if err := db.Exec(legacyDDL).Error; err != nil {
		t.Fatalf("failed to create legacy table: %v", err)
	}
	insert := `INSERT INTO events (id, aggregate_id, event_type, sequence_no, transaction_id, payload, metadata, created_at)
		VALUES (?, ?, ?, ?, '', '{}', '{}', ?)`
	now := time.Now()
	// IDs deliberately inserted out of lexical order; backfill must order by id.
	for _, row := range []struct {
		id, agg string
		seq     int
	}{
		{"ev-2", "agg-a", 2},
		{"ev-1", "agg-a", 1},
		{"ev-3", "agg-b", 1},
	} {
		if err := db.Exec(insert, row.id, row.agg, "test.created", row.seq, now).Error; err != nil {
			t.Fatalf("failed to insert legacy row %s: %v", row.id, err)
		}
	}

	store, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to create gorm event store: %v", err)
	}
	defer func() { _ = store.Close() }()

	events, err := store.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-1", "ev-2", "ev-3"})
	for i, event := range events {
		if event.Position != int64(i+1) {
			t.Errorf("event %s: expected backfilled position %d, got %d", event.ID, i+1, event.Position)
		}
	}

	// New appends continue past the backfilled range.
	if err := store.Append(ctx, "agg-c", -1, createTestEvent("agg-c", "ev-4", "test.created", 1)); err != nil {
		t.Fatalf("failed to append after backfill: %v", err)
	}
	events, err = store.ReadAfter(ctx, 3, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-4"})
	if events[0].Position != 4 {
		t.Errorf("expected new event at position 4, got %d", events[0].Position)
	}

	// Mixed backfill on a live table: a NULL-position row appearing after
	// positions exist (e.g. written by an old binary or restored from a
	// pre-position backup) must be slotted above existing positions by the
	// next migration run, not collide with them.
	if err := db.Exec(insert, "ev-5", "agg-d", "test.created", 1, now).Error; err != nil {
		t.Fatalf("failed to insert legacy row ev-5: %v", err)
	}
	store2, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to re-create store on live database: %v", err)
	}
	defer func() { _ = store2.Close() }()

	events, err = store2.ReadAfter(ctx, 4, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-5"})
	if events[0].Position != 5 {
		t.Errorf("expected mixed-backfill position 5, got %d", events[0].Position)
	}
}

// TestGormStore_MigrationRerun_PositionsContinue pins the migration's
// idempotency on SQLite: constructing the store again on the same database
// (the normal every-startup path) must not fail or restart positions.
func TestGormStore_MigrationRerun_PositionsContinue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestGormDB(t)

	store1, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to create first store: %v", err)
	}
	if err := store1.Append(ctx, "agg-a", -1, createTestEvent("agg-a", "ev-1", "test.created", 1)); err != nil {
		t.Fatalf("failed to append via first store: %v", err)
	}

	store2, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to re-create store on live database: %v", err)
	}
	defer func() { _ = store2.Close() }()
	if err := store2.Append(ctx, "agg-b", -1, createTestEvent("agg-b", "ev-2", "test.created", 1)); err != nil {
		t.Fatalf("failed to append via second store: %v", err)
	}

	events, err := store2.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-1", "ev-2"})
	if events[0].Position != 1 || events[1].Position != 2 {
		t.Errorf("expected positions to continue across constructions, got %d and %d",
			events[0].Position, events[1].Position)
	}
}
