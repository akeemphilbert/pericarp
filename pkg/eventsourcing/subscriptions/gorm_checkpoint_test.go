package subscriptions_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/subscriptions"
)

// projectionRow is the downstream state subscribers build in these tests.
type projectionRow struct {
	ID      uint   `gorm:"primaryKey;autoIncrement"`
	EventID string `gorm:"column:event_id"`
}

func (projectionRow) TableName() string { return "projection_rows" }

// newGormFixture provisions one SQLite database holding the events table,
// the checkpoint table, and a projection table — the same-database setup the
// exactly-once contract is about. File-backed (not :memory:) because the
// subscriber loop and test assertions use concurrent pool connections, and
// each glebarez :memory: connection gets a private database.
func newGormFixture(t *testing.T) (*gorm.DB, domain.EventStore, *subscriptions.GormCheckpointStore) {
	t.Helper()
	// WAL so readers (the test's assertions, the subscriber's feed reads)
	// never block writers — also what a production SQLite deployment of a
	// background subscriber would run.
	dsn := filepath.Join(t.TempDir(), "subscriptions.db") + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	store, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	checkpoints, err := subscriptions.NewGormCheckpointStore(db)
	if err != nil {
		t.Fatalf("failed to create checkpoint store: %v", err)
	}
	if err := db.AutoMigrate(&projectionRow{}); err != nil {
		t.Fatalf("failed to migrate projection table: %v", err)
	}
	return db, store, checkpoints
}

// projectingHandler writes one projection row per event through the batch
// transaction, failing once on failOn (simulating a crash mid-batch: the
// transaction rolls back exactly as it would on kill -9).
func projectingHandler(t *testing.T, failOn string) subscriptions.Handler {
	t.Helper()
	var mu sync.Mutex
	failed := false
	return func(ctx context.Context, event domain.EventEnvelope[any]) error {
		tx := subscriptions.TxFromContext(ctx)
		if tx == nil {
			return errors.New("handler expected the batch transaction in context")
		}
		if err := tx.Create(&projectionRow{EventID: event.ID}).Error; err != nil {
			return err
		}
		// Fail AFTER writing so the test proves the partial write rolls back.
		mu.Lock()
		defer mu.Unlock()
		if event.ID == failOn && !failed {
			failed = true
			return errors.New("simulated crash mid-batch")
		}
		return nil
	}
}

func projectedEventIDs(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var rows []projectionRow
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("failed to read projection: %v", err)
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.EventID
	}
	return ids
}

// TestSubscriber_ExactlyOnce_CrashMidBatchThenRestart is the story's core
// scenario: a batch dies mid-flight (handler error == kill -9 as far as the
// database is concerned — the transaction never commits), the subscriber
// resumes from the last committed checkpoint, and the downstream state ends
// correct: every event projected exactly once despite redelivery.
func TestSubscriber_ExactlyOnce_CrashMidBatchThenRestart(t *testing.T) {
	t.Parallel()

	db, store, checkpoints := newGormFixture(t)
	appendNumberedEvents(t, store, 1, 5)

	// Batch size 2 → batches [1,2], [3,4], [5]. The handler crashes on ev-4
	// after already writing ev-3 and ev-4 rows inside the batch transaction.
	sub, err := subscriptions.NewSubscriber("projector", store, checkpoints,
		projectingHandler(t, "ev-4"),
		subscriptions.WithBatchSize(2),
		subscriptions.WithPollInterval(subscriptionTestPollInterval))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	stop := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "projector", 5)
	stop()

	got := projectedEventIDs(t, db)
	want := []string{"ev-1", "ev-2", "ev-3", "ev-4", "ev-5"}
	if len(got) != len(want) {
		t.Fatalf("expected each event projected exactly once %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected projection %v, got %v", want, got)
		}
	}
}

// TestGormBatch_AbandonedBatchLeavesNoTrace pins crash semantics at the batch
// level: handler writes in a batch that never commits (process killed) are
// invisible and the checkpoint does not move.
func TestGormBatch_AbandonedBatchLeavesNoTrace(t *testing.T) {
	t.Parallel()

	db, store, checkpoints := newGormFixture(t)
	appendNumberedEvents(t, store, 1, 2)
	ctx := context.Background()

	batch, acquired, err := checkpoints.Acquire(ctx, "victim")
	if err != nil {
		t.Fatalf("failed to acquire batch: %v", err)
	}
	if !acquired {
		t.Fatal("expected to acquire the checkpoint")
	}
	if batch.Position() != 0 {
		t.Fatalf("expected fresh checkpoint at 0, got %d", batch.Position())
	}

	tx := subscriptions.TxFromContext(batch.HandlerContext(ctx))
	if tx == nil {
		t.Fatal("expected the batch transaction in the handler context")
	}
	if err := tx.Create(&projectionRow{EventID: "ev-1"}).Error; err != nil {
		t.Fatalf("failed to write through batch transaction: %v", err)
	}

	// Kill: the transaction is abandoned, never committed.
	if err := batch.Rollback(); err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	if got := projectedEventIDs(t, db); len(got) != 0 {
		t.Fatalf("expected no projected rows after abandoned batch, got %v", got)
	}
	position, err := checkpoints.Position(ctx, "victim")
	if err != nil {
		t.Fatalf("failed to read checkpoint: %v", err)
	}
	if position != 0 {
		t.Fatalf("expected checkpoint to stay at 0, got %d", position)
	}
}

// TestSubscriber_DrainsGormBatchOnShutdown is the database-backed drain test:
// cancelling the run context mid-batch must not kill the batch transaction —
// handler writes and the checkpoint advance still commit before Run returns.
// (database/sql auto-rolls-back transactions begun on a cancelled context,
// which is exactly the regression this pins.)
func TestSubscriber_DrainsGormBatchOnShutdown(t *testing.T) {
	t.Parallel()

	db, store, checkpoints := newGormFixture(t)
	appendNumberedEvents(t, store, 1, 3)

	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	handler := func(ctx context.Context, event domain.EventEnvelope[any]) error {
		startOnce.Do(func() {
			close(started)
			<-release
		})
		tx := subscriptions.TxFromContext(ctx)
		if tx == nil {
			return errors.New("handler expected the batch transaction in context")
		}
		return tx.Create(&projectionRow{EventID: event.ID}).Error
	}

	sub, err := subscriptions.NewSubscriber("gorm-drainer", store, checkpoints, handler,
		subscriptions.WithPollInterval(subscriptionTestPollInterval), subscriptions.WithBatchSize(10))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sub.Run(ctx) }()

	<-started
	cancel()
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("subscriber did not stop within 10s")
	}

	got := projectedEventIDs(t, db)
	if len(got) != 3 {
		t.Fatalf("expected the in-flight batch of 3 events to drain and commit, got %v", got)
	}
	position, err := checkpoints.Position(context.Background(), "gorm-drainer")
	if err != nil {
		t.Fatalf("failed to read checkpoint: %v", err)
	}
	if position != 3 {
		t.Fatalf("expected checkpoint 3 after drain, got %d", position)
	}
}

// TestGormCheckpointStore_ResetWinsOverInFlightBatch pins the reset contract
// on SQLite, where the batch holds no row lock: an in-flight batch whose
// checkpoint is reset underneath it must abort at Commit rather than
// silently clobber the reset.
func TestGormCheckpointStore_ResetWinsOverInFlightBatch(t *testing.T) {
	t.Parallel()

	_, _, checkpoints := newGormFixture(t)
	ctx := context.Background()

	// Establish a checkpoint at 3, then open a batch on top of it.
	if err := checkpoints.Reset(ctx, "resettable", 3); err != nil {
		t.Fatalf("failed to seed checkpoint: %v", err)
	}
	batch, acquired, err := checkpoints.Acquire(ctx, "resettable")
	if err != nil || !acquired {
		t.Fatalf("failed to acquire (acquired=%v): %v", acquired, err)
	}
	if batch.Position() != 3 {
		t.Fatalf("expected batch at position 3, got %d", batch.Position())
	}

	// Operator resets to 0 while the batch is in flight.
	if err := checkpoints.Reset(ctx, "resettable", 0); err != nil {
		t.Fatalf("failed to reset: %v", err)
	}

	// The batch must notice the moved checkpoint and abort.
	if err := batch.Commit(ctx, 5); err == nil {
		t.Fatal("expected Commit to fail after a concurrent reset")
	}

	position, err := checkpoints.Position(ctx, "resettable")
	if err != nil {
		t.Fatalf("failed to read checkpoint: %v", err)
	}
	if position != 0 {
		t.Fatalf("expected the reset to win (checkpoint 0), got %d", position)
	}
}

func TestGormCheckpointStore_AcquireCommitReset(t *testing.T) {
	t.Parallel()

	_, _, checkpoints := newGormFixture(t)
	ctx := context.Background()

	// Unknown subscriber reads as 0.
	position, err := checkpoints.Position(ctx, "fresh")
	if err != nil {
		t.Fatalf("failed to read checkpoint: %v", err)
	}
	if position != 0 {
		t.Fatalf("expected unknown subscriber at 0, got %d", position)
	}

	// Commit advances.
	batch, acquired, err := checkpoints.Acquire(ctx, "fresh")
	if err != nil || !acquired {
		t.Fatalf("failed to acquire (acquired=%v): %v", acquired, err)
	}
	if err := batch.Commit(ctx, 42); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
	position, err = checkpoints.Position(ctx, "fresh")
	if err != nil {
		t.Fatalf("failed to read checkpoint: %v", err)
	}
	if position != 42 {
		t.Fatalf("expected checkpoint 42, got %d", position)
	}

	// The next batch starts where the last one committed.
	batch, acquired, err = checkpoints.Acquire(ctx, "fresh")
	if err != nil || !acquired {
		t.Fatalf("failed to re-acquire (acquired=%v): %v", acquired, err)
	}
	if batch.Position() != 42 {
		t.Fatalf("expected batch to start at 42, got %d", batch.Position())
	}
	if err := batch.Rollback(); err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	// Reset to 0 rewinds for replay; Reset also creates unknown subscribers.
	if err := checkpoints.Reset(ctx, "fresh", 0); err != nil {
		t.Fatalf("failed to reset: %v", err)
	}
	position, err = checkpoints.Position(ctx, "fresh")
	if err != nil {
		t.Fatalf("failed to read checkpoint: %v", err)
	}
	if position != 0 {
		t.Fatalf("expected checkpoint 0 after reset, got %d", position)
	}
	if err := checkpoints.Reset(ctx, "brand-new", 7); err != nil {
		t.Fatalf("failed to reset unknown subscriber: %v", err)
	}
	position, err = checkpoints.Position(ctx, "brand-new")
	if err != nil {
		t.Fatalf("failed to read checkpoint: %v", err)
	}
	if position != 7 {
		t.Fatalf("expected checkpoint 7, got %d", position)
	}
}

// TestSubscriber_ResetReplayRebuildsProjection covers the reset-to-replay flow
// end to end on the database-backed stores: wipe the projection, reset the
// checkpoint to 0, and the subscriber rebuilds the projection from history.
func TestSubscriber_ResetReplayRebuildsProjection(t *testing.T) {
	t.Parallel()

	db, store, checkpoints := newGormFixture(t)
	appendNumberedEvents(t, store, 1, 3)

	sub, err := subscriptions.NewSubscriber("rebuilder", store, checkpoints,
		projectingHandler(t, ""),
		subscriptions.WithPollInterval(subscriptionTestPollInterval))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	stop := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "rebuilder", 3)
	stop()

	// Wipe the projection and replay.
	if err := db.Exec("DELETE FROM projection_rows").Error; err != nil {
		t.Fatalf("failed to wipe projection: %v", err)
	}
	if err := sub.ResetCheckpoint(context.Background(), 0); err != nil {
		t.Fatalf("failed to reset checkpoint: %v", err)
	}

	stop2 := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "rebuilder", 3)
	stop2()

	got := projectedEventIDs(t, db)
	want := []string{"ev-1", "ev-2", "ev-3"}
	if len(got) != len(want) {
		t.Fatalf("expected rebuilt projection %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected rebuilt projection %v, got %v", want, got)
		}
	}
}
