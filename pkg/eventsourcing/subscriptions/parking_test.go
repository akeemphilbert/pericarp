package subscriptions_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/subscriptions"
)

// capturingLogHandler records slog output so tests can assert on the
// error-level parking log the story requires for alerting.
type capturingLogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingLogHandler) Enabled(ctx context.Context, level slog.Level) bool { return true }

func (h *capturingLogHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *capturingLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *capturingLogHandler) WithGroup(name string) slog.Handler      { return h }

func (h *capturingLogHandler) hasErrorRecord(messageSubstring string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Level == slog.LevelError && strings.Contains(r.Message, messageSubstring) {
			return true
		}
	}
	return false
}

// poisonProjectingHandler projects events through the batch transaction but
// fails on poisonID while healthy is false. It writes the projection row
// BEFORE failing so the tests prove failed attempts' writes are discarded by
// the savepoint, not just never made.
func poisonProjectingHandler(poisonID string, healthy *atomic.Bool, attempts *atomic.Int64) subscriptions.Handler {
	return func(ctx context.Context, event domain.EventEnvelope[any]) error {
		tx := subscriptions.TxFromContext(ctx)
		if tx == nil {
			return errors.New("handler expected the batch transaction in context")
		}
		if err := tx.Create(&projectionRow{EventID: event.ID}).Error; err != nil {
			return err
		}
		if event.ID == poisonID && !healthy.Load() {
			attempts.Add(1)
			return errors.New("cannot process this payload")
		}
		return nil
	}
}

func TestSubscriber_PoisonEventIsParkedAndOthersKeepFlowing(t *testing.T) {
	t.Parallel()

	db, store, checkpoints := newGormFixture(t)
	parking, err := subscriptions.NewGormParkingLot(db)
	if err != nil {
		t.Fatalf("failed to create parking lot: %v", err)
	}
	appendNumberedEvents(t, store, 1, 5)

	logs := &capturingLogHandler{}
	var healthy atomic.Bool
	var attempts atomic.Int64
	sub, err := subscriptions.NewSubscriber("poisoned", store, checkpoints,
		poisonProjectingHandler("ev-3", &healthy, &attempts),
		subscriptions.WithParkingLot(parking),
		subscriptions.WithMaxRetries(2),
		subscriptions.WithRetryBackoff(time.Millisecond, 2*time.Millisecond),
		subscriptions.WithPollInterval(subscriptionTestPollInterval),
		subscriptions.WithBatchSize(10),
		subscriptions.WithLogger(slog.New(logs)))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	stop := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "poisoned", 5)
	stop()

	// The checkpoint advanced past the poison event and every other event
	// was projected exactly once. ev-3's three attempted writes were all
	// discarded by the savepoint rollback.
	got := projectedEventIDs(t, db)
	want := []string{"ev-1", "ev-2", "ev-4", "ev-5"}
	if len(got) != len(want) {
		t.Fatalf("expected projection %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected projection %v, got %v", want, got)
		}
	}

	if n := attempts.Load(); n != 3 {
		t.Errorf("expected 1 initial attempt + 2 retries = 3 attempts, got %d", n)
	}

	parked, err := sub.ListParked(context.Background())
	if err != nil {
		t.Fatalf("failed to list parked events: %v", err)
	}
	if len(parked) != 1 {
		t.Fatalf("expected exactly one parked event, got %v", parked)
	}
	entry := parked[0]
	if entry.EventID != "ev-3" || entry.Subscriber != "poisoned" {
		t.Errorf("expected ev-3 parked for subscriber poisoned, got %+v", entry)
	}
	if entry.Attempts != 3 {
		t.Errorf("expected 3 recorded attempts, got %d", entry.Attempts)
	}
	if !strings.Contains(entry.Error, "cannot process this payload") {
		t.Errorf("expected the handler error recorded, got %q", entry.Error)
	}
	if entry.Position != 3 {
		t.Errorf("expected parked position 3, got %d", entry.Position)
	}

	if !logs.hasErrorRecord("event parked") {
		t.Error("expected an error-level log when the event was parked")
	}

	// Replay: the handler is healthy now; the parked event re-runs, the
	// projection gains the missing row, and the parking row clears.
	healthy.Store(true)
	if err := sub.ReplayParked(context.Background(), "ev-3"); err != nil {
		t.Fatalf("failed to replay parked event: %v", err)
	}
	got = projectedEventIDs(t, db)
	if len(got) != 5 || got[4] != "ev-3" {
		t.Fatalf("expected ev-3 appended to projection after replay, got %v", got)
	}
	parked, err = sub.ListParked(context.Background())
	if err != nil {
		t.Fatalf("failed to list parked events: %v", err)
	}
	if len(parked) != 0 {
		t.Fatalf("expected no parked events after replay, got %v", parked)
	}
}

func TestSubscriber_TransientFailureRecoversWithoutParking(t *testing.T) {
	t.Parallel()

	db, store, checkpoints := newGormFixture(t)
	parking, err := subscriptions.NewGormParkingLot(db)
	if err != nil {
		t.Fatalf("failed to create parking lot: %v", err)
	}
	appendNumberedEvents(t, store, 1, 3)

	// Fails twice on ev-2 (writing a row each time), then succeeds — within
	// the retry budget, so nothing is parked and the projection holds each
	// event exactly once.
	var failures atomic.Int64
	handler := func(ctx context.Context, event domain.EventEnvelope[any]) error {
		tx := subscriptions.TxFromContext(ctx)
		if tx == nil {
			return errors.New("handler expected the batch transaction in context")
		}
		if err := tx.Create(&projectionRow{EventID: event.ID}).Error; err != nil {
			return err
		}
		if event.ID == "ev-2" && failures.Add(1) <= 2 {
			return errors.New("transient glitch")
		}
		return nil
	}

	sub, err := subscriptions.NewSubscriber("glitchy", store, checkpoints, handler,
		subscriptions.WithParkingLot(parking),
		subscriptions.WithMaxRetries(3),
		subscriptions.WithRetryBackoff(time.Millisecond, 2*time.Millisecond),
		subscriptions.WithPollInterval(subscriptionTestPollInterval))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	stop := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "glitchy", 3)
	stop()

	got := projectedEventIDs(t, db)
	want := []string{"ev-1", "ev-2", "ev-3"}
	if len(got) != len(want) {
		t.Fatalf("expected projection %v (failed attempts discarded), got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected projection %v, got %v", want, got)
		}
	}

	parked, err := sub.ListParked(context.Background())
	if err != nil {
		t.Fatalf("failed to list parked events: %v", err)
	}
	if len(parked) != 0 {
		t.Fatalf("expected nothing parked for a transient failure, got %v", parked)
	}
}

func TestSubscriber_FailedReplayLeavesEventParked(t *testing.T) {
	t.Parallel()

	db, store, checkpoints := newGormFixture(t)
	parking, err := subscriptions.NewGormParkingLot(db)
	if err != nil {
		t.Fatalf("failed to create parking lot: %v", err)
	}
	appendNumberedEvents(t, store, 1, 1)

	var healthy atomic.Bool
	var attempts atomic.Int64
	sub, err := subscriptions.NewSubscriber("stuck", store, checkpoints,
		poisonProjectingHandler("ev-1", &healthy, &attempts),
		subscriptions.WithParkingLot(parking),
		subscriptions.WithMaxRetries(0),
		subscriptions.WithRetryBackoff(time.Millisecond, time.Millisecond),
		subscriptions.WithPollInterval(subscriptionTestPollInterval))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	stop := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "stuck", 1)
	stop()

	// Still poisoned: replay fails, the row stays parked, no projection row
	// leaks from the failed replay transaction.
	if err := sub.ReplayParked(context.Background(), "ev-1"); err == nil {
		t.Fatal("expected replay to fail while the handler is still poisoned")
	}
	parked, err := sub.ListParked(context.Background())
	if err != nil {
		t.Fatalf("failed to list parked events: %v", err)
	}
	if len(parked) != 1 {
		t.Fatalf("expected the event to stay parked after a failed replay, got %v", parked)
	}
	if got := projectedEventIDs(t, db); len(got) != 0 {
		t.Fatalf("expected no projection rows from the failed replay, got %v", got)
	}
}

func TestSubscriber_ReplayUnknownEvent(t *testing.T) {
	t.Parallel()

	db, store, checkpoints := newGormFixture(t)
	parking, err := subscriptions.NewGormParkingLot(db)
	if err != nil {
		t.Fatalf("failed to create parking lot: %v", err)
	}
	appendNumberedEvents(t, store, 1, 1)

	sub, err := subscriptions.NewSubscriber("replayer", store, checkpoints,
		projectingHandler(t, ""),
		subscriptions.WithParkingLot(parking),
		subscriptions.WithPollInterval(subscriptionTestPollInterval))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	// Stored event that was never parked.
	if err := sub.ReplayParked(context.Background(), "ev-1"); !errors.Is(err, subscriptions.ErrEventNotParked) {
		t.Fatalf("expected ErrEventNotParked, got %v", err)
	}
	// Event that doesn't exist at all.
	if err := sub.ReplayParked(context.Background(), "no-such-event"); !errors.Is(err, domain.ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound, got %v", err)
	}
	_ = db
}

func TestSubscriber_WithoutParkingLotPoisonEventHaltsCheckpoint(t *testing.T) {
	t.Parallel()

	store := infrastructure.NewMemoryStore()
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	appendNumberedEvents(t, store, 1, 2)

	var attempts atomic.Int64
	handler := func(ctx context.Context, event domain.EventEnvelope[any]) error {
		attempts.Add(1)
		return errors.New("always fails")
	}
	sub, err := subscriptions.NewSubscriber("halted", store, checkpoints, handler,
		subscriptions.WithPollInterval(subscriptionTestPollInterval),
		subscriptions.WithLogger(slog.New(&capturingLogHandler{})))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	stop := runSubscriber(t, sub)
	// Let it churn through several poll cycles.
	waitFor(t, 10*time.Second, func() bool { return attempts.Load() >= 3 }, "three failed attempts")
	stop()

	position, err := checkpoints.Position(context.Background(), "halted")
	if err != nil {
		t.Fatalf("failed to read checkpoint: %v", err)
	}
	if position != 0 {
		t.Fatalf("expected checkpoint to stay at 0 without a parking lot, got %d", position)
	}
}

// TestGormParkingLot_ForeignTransactionIsNotAdopted pins that Park never
// writes parked_events through a batch transaction belonging to a different
// database — the row must land where List/Replay can find it.
func TestGormParkingLot_ForeignTransactionIsNotAdopted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	lotDB, _, _ := newGormFixture(t)
	otherDB, _, otherCheckpoints := newGormFixture(t)

	lot, err := subscriptions.NewGormParkingLot(lotDB)
	if err != nil {
		t.Fatalf("failed to create parking lot: %v", err)
	}
	// Give the foreign database a parked_events table too — the dangerous
	// misconfiguration is exactly when the foreign write would succeed.
	if _, err := subscriptions.NewGormParkingLot(otherDB); err != nil {
		t.Fatalf("failed to migrate foreign parked_events table: %v", err)
	}

	// A batch transaction from the OTHER database in context.
	batch, acquired, err := otherCheckpoints.Acquire(ctx, "foreign")
	if err != nil || !acquired {
		t.Fatalf("failed to acquire foreign batch (acquired=%v): %v", acquired, err)
	}
	defer func() { _ = batch.Rollback() }()

	if err := lot.Park(batch.HandlerContext(ctx), subscriptions.ParkedEvent{
		Subscriber: "s", EventID: "ev-1", EventType: "test.created", Position: 1,
		Error: "boom", Attempts: 1, ParkedAt: time.Now(),
	}); err != nil {
		t.Fatalf("failed to park: %v", err)
	}

	// The row is in the lot's database (visible immediately, not part of the
	// foreign transaction), and not in the other database.
	parked, err := lot.List(ctx, "s")
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(parked) != 1 {
		t.Fatalf("expected the park to land in the lot's own database, got %v", parked)
	}
	var count int64
	if err := otherDB.Table("parked_events").Count(&count).Error; err != nil {
		t.Fatalf("failed to count foreign parked_events: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no parked rows in the foreign database, got %d", count)
	}
}

func TestMemoryParkingLot_ReparkedEntrySurvivesReplay(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	lot := subscriptions.NewMemoryParkingLot()
	event := createTestEvent("agg-1", "ev-1", "test.created", 1)
	park := func(errMsg string, attempts int) subscriptions.ParkedEvent {
		return subscriptions.ParkedEvent{
			Subscriber: "s", EventID: "ev-1", EventType: "test.created", Position: 1,
			Error: errMsg, Attempts: attempts, ParkedAt: time.Now(),
		}
	}
	if err := lot.Park(ctx, park("original failure", 6)); err != nil {
		t.Fatalf("failed to park: %v", err)
	}

	// The handler succeeds, but mid-replay the event is parked again (a
	// fresh failure, e.g. after a checkpoint reset). The fresh record must
	// survive the replay's clear.
	handler := func(ctx context.Context, e domain.EventEnvelope[any]) error {
		return lot.Park(ctx, park("fresh failure", 1))
	}
	if err := lot.Replay(ctx, "s", "ev-1", event, handler); err != nil {
		t.Fatalf("failed to replay: %v", err)
	}

	parked, err := lot.List(ctx, "s")
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(parked) != 1 || parked[0].Error != "fresh failure" {
		t.Fatalf("expected the fresh park record to survive the replay, got %v", parked)
	}
}

func TestMemoryParkingLot_ParkListReplay(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	lot := subscriptions.NewMemoryParkingLot()
	event := createTestEvent("agg-1", "ev-1", "test.created", 1)

	if err := lot.Park(ctx, subscriptions.ParkedEvent{
		Subscriber: "s", EventID: "ev-1", EventType: "test.created", Position: 1,
		Error: "boom", Attempts: 6, ParkedAt: time.Now(),
	}); err != nil {
		t.Fatalf("failed to park: %v", err)
	}

	parked, err := lot.List(ctx, "s")
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(parked) != 1 || parked[0].EventID != "ev-1" {
		t.Fatalf("expected ev-1 parked, got %v", parked)
	}

	// Failing replay keeps the entry.
	failing := func(ctx context.Context, e domain.EventEnvelope[any]) error { return errors.New("still bad") }
	if err := lot.Replay(ctx, "s", "ev-1", event, failing); err == nil {
		t.Fatal("expected failing replay to error")
	}
	if parked, _ := lot.List(ctx, "s"); len(parked) != 1 {
		t.Fatalf("expected entry retained after failed replay, got %v", parked)
	}

	// Successful replay clears it.
	ok := func(ctx context.Context, e domain.EventEnvelope[any]) error { return nil }
	if err := lot.Replay(ctx, "s", "ev-1", event, ok); err != nil {
		t.Fatalf("failed to replay: %v", err)
	}
	if parked, _ := lot.List(ctx, "s"); len(parked) != 0 {
		t.Fatalf("expected entry cleared, got %v", parked)
	}

	// Unknown event.
	if err := lot.Replay(ctx, "s", "ev-1", event, ok); !errors.Is(err, subscriptions.ErrEventNotParked) {
		t.Fatalf("expected ErrEventNotParked, got %v", err)
	}
}
