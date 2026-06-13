package subscriptions_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/subscriptions"
)

// countingStore counts ReadAfter calls so tests can (a) wait for a
// subscriber's first feed read before appending — proving later processing
// came from a wake, not the startup cycle — and (b) bound the cycle rate.
type countingStore struct {
	domain.EventStore
	reads atomic.Int64
}

func (c *countingStore) ReadAfter(ctx context.Context, afterPosition int64, limit int) ([]domain.EventEnvelope[any], error) {
	c.reads.Add(1)
	return c.EventStore.ReadAfter(ctx, afterPosition, limit)
}

func TestInProcessNotifier_NotifyNeverBlocks(t *testing.T) {
	t.Parallel()

	notifier := subscriptions.NewInProcessNotifier()
	wake := notifier.Subscribe()

	// Repeated notifies with nobody draining must not block; the subscriber
	// keeps exactly one pending wake.
	for range 10 {
		notifier.Notify()
	}
	select {
	case <-wake:
	default:
		t.Fatal("expected one pending wake signal")
	}
	select {
	case <-wake:
		t.Fatal("expected at most one pending wake signal")
	default:
	}
}

// TestSubscriber_WakesOnCommitSignal proves the wake path: the poll interval
// is far beyond the test deadline, so only the in-process commit signal can
// get the new events processed in time.
func TestSubscriber_WakesOnCommitSignal(t *testing.T) {
	t.Parallel()

	notifier := subscriptions.NewInProcessNotifier()
	store := &countingStore{
		EventStore: subscriptions.NewNotifyingEventStore(infrastructure.NewMemoryStore(), notifier.Notify),
	}
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	handler := &recordingHandler{}

	sub, err := subscriptions.NewSubscriber("woken", store, checkpoints, handler.handle,
		subscriptions.WithPollInterval(time.Hour),
		subscriptions.WithWakeSignal(notifier.Subscribe()))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	stop := runSubscriber(t, sub)
	defer stop()

	// Wait for the first (empty) feed read so the startup cycle cannot be
	// what processes the events; only the wake signal can beat the
	// one-hour poll after this point.
	waitFor(t, 10*time.Second, func() bool { return store.reads.Load() >= 1 }, "first feed read")

	appendNumberedEvents(t, store, 1, 3)
	waitForCheckpoint(t, checkpoints, "woken", 3)

	if got := handler.handled(); len(got) != 3 {
		t.Fatalf("expected 3 events processed via wake signal, got %v", got)
	}
}

// TestSubscriber_ClosedWakeChannelFallsBackToPolling pins the pathological
// configuration: a caller that closes its wake channel must degrade the
// subscriber to plain polling — progress continues and the loop must not
// spin hot on the always-ready closed channel.
func TestSubscriber_ClosedWakeChannelFallsBackToPolling(t *testing.T) {
	t.Parallel()

	store := &countingStore{EventStore: infrastructure.NewMemoryStore()}
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	handler := &recordingHandler{}
	appendNumberedEvents(t, store, 1, 2)

	wake := make(chan struct{})
	close(wake)

	sub, err := subscriptions.NewSubscriber("orphaned", store, checkpoints, handler.handle,
		subscriptions.WithPollInterval(20*time.Millisecond),
		subscriptions.WithWakeSignal(wake))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	stop := runSubscriber(t, sub)
	defer stop()
	waitForCheckpoint(t, checkpoints, "orphaned", 2)

	// Idle for a while: cycle count must track the poll interval, not a hot
	// loop on the closed channel (which would do tens of thousands).
	before := store.reads.Load()
	time.Sleep(500 * time.Millisecond)
	cycles := store.reads.Load() - before
	if cycles > 100 {
		t.Fatalf("expected polling-paced cycles after wake channel closed, got %d in 500ms", cycles)
	}
}
