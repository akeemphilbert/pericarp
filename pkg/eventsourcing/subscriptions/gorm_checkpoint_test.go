package subscriptions_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	return newGormFixtureDSN(t, dsn)
}

// newGormFixtureDSN is newGormFixture with an explicit DSN, so tests that need
// specific SQLite locking behaviour (e.g. _txlock=immediate) can dictate it.
func newGormFixtureDSN(t *testing.T, dsn string) (*gorm.DB, domain.EventStore, *subscriptions.GormCheckpointStore) {
	t.Helper()
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

// TestGormCheckpointStore_EnsuresRowOncePerSubscriber pins the memoization: the
// checkpoint row is INSERTed once for a subscriber, not on every Acquire. A
// gorm create-callback counts INSERT statements against subscriber_checkpoints
// across two Acquire/Commit cycles and expects exactly one.
func TestGormCheckpointStore_EnsuresRowOncePerSubscriber(t *testing.T) {
	t.Parallel()

	db, _, checkpoints := newGormFixture(t)
	ctx := context.Background()

	var inserts int64
	if err := db.Callback().Create().After("gorm:create").
		Register("test:count_checkpoint_inserts", func(tx *gorm.DB) {
			if tx.Statement.Table == "subscriber_checkpoints" {
				atomic.AddInt64(&inserts, 1)
			}
		}); err != nil {
		t.Fatalf("failed to register callback: %v", err)
	}

	for i := 0; i < 2; i++ {
		batch, acquired, err := checkpoints.Acquire(ctx, "memoized")
		if err != nil || !acquired {
			t.Fatalf("cycle %d: failed to acquire (acquired=%v): %v", i, acquired, err)
		}
		if err := batch.Commit(ctx, int64(i+1)); err != nil {
			t.Fatalf("cycle %d: failed to commit: %v", i, err)
		}
	}

	if got := atomic.LoadInt64(&inserts); got != 1 {
		t.Fatalf("expected the checkpoint row ensured once across cycles, got %d INSERTs", got)
	}
}

// TestGormCheckpointStore_EnsuresRowOnceUnderConcurrentAcquire pins the
// concurrency side of the memoization: a wake burst that races several first
// Acquires for the same fresh subscriber must still ensure the checkpoint row
// exactly once. A plain load-then-ensure would let every racer miss the memo
// and fire its own INSERT — the write amplification the memo exists to remove.
func TestGormCheckpointStore_EnsuresRowOnceUnderConcurrentAcquire(t *testing.T) {
	t.Parallel()

	db, _, checkpoints := newGormFixture(t)

	var inserts int64
	if err := db.Callback().Create().After("gorm:create").
		Register("test:count_concurrent_checkpoint_inserts", func(tx *gorm.DB) {
			if tx.Statement.Table == "subscriber_checkpoints" {
				atomic.AddInt64(&inserts, 1)
			}
		}); err != nil {
		t.Fatalf("failed to register callback: %v", err)
	}

	const racers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(racers)
	for range racers {
		go func() {
			defer wg.Done()
			<-start // release all goroutines into Acquire together
			batch, acquired, err := checkpoints.Acquire(context.Background(), "raced")
			if err != nil {
				t.Errorf("failed to acquire: %v", err)
				return
			}
			if acquired {
				// Release the batch transaction; the row stays ensured.
				if err := batch.Rollback(); err != nil {
					t.Errorf("failed to rollback: %v", err)
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt64(&inserts); got != 1 {
		t.Fatalf("expected the checkpoint row ensured once under concurrent Acquire, got %d INSERTs", got)
	}
}

// TestGormCheckpointStore_EnsureRespectsCallerCancellation pins the
// cancellation contract of the first-time ensure: a caller whose ctx expires
// while the ensure INSERT is stuck behind the database write lock returns
// promptly with the ctx error instead of blocking out the full busy_timeout —
// and the ensure itself, deliberately detached from any one caller's ctx,
// still completes once the lock frees, so the store converges.
func TestGormCheckpointStore_EnsureRespectsCallerCancellation(t *testing.T) {
	t.Parallel()

	db, _, checkpoints := newGormFixture(t)

	// Hold SQLite's database write lock in a separate transaction so the
	// flight's ensure INSERT blocks against busy_timeout (10s in the fixture).
	blocker := db.Begin()
	if blocker.Error != nil {
		t.Fatalf("failed to begin blocking tx: %v", blocker.Error)
	}
	if err := blocker.Create(&projectionRow{EventID: "lock-holder"}).Error; err != nil {
		t.Fatalf("failed to take the write lock: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := checkpoints.Acquire(ctx, "cancellable")
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	// Well under the 10s busy_timeout the blocked INSERT would otherwise burn.
	if elapsed > 5*time.Second {
		t.Fatalf("Acquire blocked %v despite ctx deadline; want prompt return", elapsed)
	}

	// Release the lock: the detached flight finishes its INSERT and memoizes,
	// so a later Acquire with a healthy ctx succeeds.
	if err := blocker.Rollback().Error; err != nil {
		t.Fatalf("failed to release the write lock: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		batch, acquired, err := checkpoints.Acquire(context.Background(), "cancellable")
		if err == nil && acquired {
			if err := batch.Rollback(); err != nil {
				t.Fatalf("failed to rollback: %v", err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("store never converged after lock release (acquired=%v): %v", acquired, err)
		}
		time.Sleep(20 * time.Millisecond)
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
