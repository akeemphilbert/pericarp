package subscriptions_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/subscriptions"
)

// newBatchLockedFixture opens a file-backed SQLite database whose transactions
// BEGIN IMMEDIATE (_txlock=immediate), so a subscription batch grabs the
// database write lock for its whole lifetime — the exact condition under which
// a nested append that opened its own transaction would self-deadlock. The
// busy_timeout is short so that, absent the ambient-tx join, the self-wait
// fails fast instead of hanging the suite.
func newBatchLockedFixture(t *testing.T) (domain.EventStore, *subscriptions.GormCheckpointStore) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "ambient.db") +
		"?_pragma=busy_timeout(500)&_pragma=journal_mode(WAL)&_txlock=immediate"
	_, store, checkpoints := newGormFixtureDSN(t, dsn)
	return store, checkpoints
}

// TestGormEventStore_AppendJoinsAmbientBatchTx is the core regression for
// wepala/weos#426: an append issued from inside a subscription batch (the
// batch already holding the SQLite write lock) must join the batch transaction
// and return promptly, then commit atomically with the batch — not open a
// second transaction and burn the busy_timeout in a self-wait.
func TestGormEventStore_AppendJoinsAmbientBatchTx(t *testing.T) {
	t.Parallel()

	store, checkpoints := newBatchLockedFixture(t)
	ctx := context.Background()

	batch, acquired, err := checkpoints.Acquire(ctx, "appender")
	if err != nil || !acquired {
		t.Fatalf("failed to acquire batch (acquired=%v): %v", acquired, err)
	}
	hctx := batch.HandlerContext(ctx)

	// Take the batch write lock the way a real handler would: write a
	// projection row through the batch transaction before appending. The batch
	// now holds SQLite's single writer slot, so a nested append that opened its
	// own transaction would deadlock.
	if tx := subscriptions.TxFromContext(hctx); tx != nil {
		if err := tx.Create(&projectionRow{EventID: "lock-holder"}).Error; err != nil {
			t.Fatalf("failed to take the batch write lock: %v", err)
		}
	} else {
		t.Fatal("expected the batch transaction in the handler context")
	}

	// Append from inside the batch. Without the ambient-tx join this opens a
	// second write transaction on another pool connection, self-waits on the
	// write lock the batch already holds, and BUSY-fails after busy_timeout.
	// With the fix it writes through the batch tx and returns promptly.
	done := make(chan error, 1)
	go func() {
		done <- store.Append(hctx, "agg-inbatch", -1,
			createTestEvent("agg-inbatch", "ev-inbatch", "test.created", 1))
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ambient append failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ambient append did not return promptly — BUSY self-wait?")
	}

	// The event lives in the uncommitted batch transaction: invisible to a
	// fresh connection until the batch commits.
	if _, err := store.GetEventByID(ctx, "ev-inbatch"); !errors.Is(err, domain.ErrEventNotFound) {
		t.Fatalf("expected ev-inbatch invisible before commit, got %v", err)
	}

	if err := batch.Commit(ctx, 1); err != nil {
		t.Fatalf("failed to commit batch: %v", err)
	}

	// Committed atomically with the batch — now visible.
	got, err := store.GetEventByID(ctx, "ev-inbatch")
	if err != nil {
		t.Fatalf("expected ev-inbatch visible after commit: %v", err)
	}
	if got.ID != "ev-inbatch" {
		t.Fatalf("expected ev-inbatch, got %s", got.ID)
	}
}

// TestGormEventStore_AmbientAppendFailureRollsBackToSavepoint pins the
// savepoint bracketing: a failing append inside a batch rolls back only to its
// own savepoint, leaving earlier appends intact and the batch usable for
// further appends and its commit.
func TestGormEventStore_AmbientAppendFailureRollsBackToSavepoint(t *testing.T) {
	t.Parallel()

	store, checkpoints := newBatchLockedFixture(t)
	ctx := context.Background()

	batch, acquired, err := checkpoints.Acquire(ctx, "savepointer")
	if err != nil || !acquired {
		t.Fatalf("failed to acquire batch (acquired=%v): %v", acquired, err)
	}
	hctx := batch.HandlerContext(ctx)

	// A good append inside the batch (also takes the write lock).
	if err := store.Append(hctx, "agg-a", -1,
		createTestEvent("agg-a", "ev-a", "test.created", 1)); err != nil {
		t.Fatalf("first append failed: %v", err)
	}

	// A failing append (wrong expectedVersion) must roll back to its savepoint
	// and surface the conflict, without poisoning the batch.
	err = store.Append(hctx, "agg-b", 7,
		createTestEvent("agg-b", "ev-b", "test.created", 1))
	if !errors.Is(err, domain.ErrConcurrencyConflict) {
		t.Fatalf("expected ErrConcurrencyConflict from the bad append, got %v", err)
	}

	// The batch is still usable: another good append then a commit succeed.
	if err := store.Append(hctx, "agg-c", -1,
		createTestEvent("agg-c", "ev-c", "test.created", 1)); err != nil {
		t.Fatalf("post-failure append failed (batch poisoned?): %v", err)
	}
	if err := batch.Commit(ctx, 1); err != nil {
		t.Fatalf("failed to commit batch: %v", err)
	}

	// ev-a and ev-c committed; ev-b rolled back to its savepoint.
	if _, err := store.GetEventByID(ctx, "ev-a"); err != nil {
		t.Fatalf("expected ev-a committed: %v", err)
	}
	if _, err := store.GetEventByID(ctx, "ev-c"); err != nil {
		t.Fatalf("expected ev-c committed: %v", err)
	}
	if _, err := store.GetEventByID(ctx, "ev-b"); !errors.Is(err, domain.ErrEventNotFound) {
		t.Fatalf("expected ev-b rolled back and absent, got %v", err)
	}
}
