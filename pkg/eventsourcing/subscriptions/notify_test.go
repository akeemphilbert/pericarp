package subscriptions_test

import (
	"context"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/subscriptions"
)

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
	store := subscriptions.NewNotifyingEventStore(infrastructure.NewMemoryStore(), notifier.Notify)
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

	// Give the subscriber its first (empty) cycle, then commit: the appended
	// events must be processed long before the one-hour poll would fire.
	waitFor(t, 10*time.Second, func() bool {
		_, err := checkpoints.Position(context.Background(), "woken")
		return err == nil
	}, "subscriber started")

	appendNumberedEvents(t, store, 1, 3)
	waitForCheckpoint(t, checkpoints, "woken", 3)

	if got := handler.handled(); len(got) != 3 {
		t.Fatalf("expected 3 events processed via wake signal, got %v", got)
	}
}
