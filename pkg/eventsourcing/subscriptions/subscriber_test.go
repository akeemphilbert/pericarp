package subscriptions_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/subscriptions"
)

// subscriptionTestPollInterval keeps the subscriber loops fast in tests.
const subscriptionTestPollInterval = 5 * time.Millisecond

func createTestEvent(aggregateID, eventID, eventType string, version int) domain.EventEnvelope[any] {
	return domain.EventEnvelope[any]{
		ID:          eventID,
		AggregateID: aggregateID,
		EventType:   eventType,
		Payload:     map[string]any{"test": "data"},
		Created:     time.Now(),
		SequenceNo:  version,
		Metadata:    make(map[string]any),
	}
}

// appendNumberedEvents appends n events ev-<from>..ev-<from+n-1>, one
// aggregate each, in separate commits so they get distinct positions.
func appendNumberedEvents(t *testing.T, store domain.EventStore, from, n int) {
	t.Helper()
	ctx := context.Background()
	for i := from; i < from+n; i++ {
		aggregateID := fmt.Sprintf("agg-%d", i)
		eventID := fmt.Sprintf("ev-%d", i)
		if err := store.Append(ctx, aggregateID, -1, createTestEvent(aggregateID, eventID, "test.created", 1)); err != nil {
			t.Fatalf("failed to append %s: %v", eventID, err)
		}
	}
}

// recordingHandler collects handled event IDs in order.
type recordingHandler struct {
	mu  sync.Mutex
	ids []string
}

func (r *recordingHandler) handle(ctx context.Context, event domain.EventEnvelope[any]) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, event.ID)
	return nil
}

func (r *recordingHandler) handled() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}

// runSubscriber starts sub.Run in a goroutine and returns a stop function
// that cancels it and waits for Run to return (failing the test on a non-nil
// error).
func runSubscriber(t *testing.T, sub *subscriptions.Subscriber) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sub.Run(ctx) }()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("Run returned error: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("subscriber did not stop within 10s")
			}
		})
	}
}

// waitFor polls cond until it holds or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

func waitForCheckpoint(t *testing.T, checkpoints subscriptions.CheckpointStore, subscriber string, position int64) {
	t.Helper()
	waitFor(t, 10*time.Second, func() bool {
		got, err := checkpoints.Position(context.Background(), subscriber)
		if err != nil {
			t.Fatalf("failed to read checkpoint: %v", err)
		}
		return got >= position
	}, fmt.Sprintf("checkpoint %d", position))
}

func TestSubscriber_ProcessesInOrderAndResumesAcrossRestarts(t *testing.T) {
	t.Parallel()

	store := infrastructure.NewMemoryStore()
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	handler := &recordingHandler{}

	appendNumberedEvents(t, store, 1, 3)

	sub, err := subscriptions.NewSubscriber("projector", store, checkpoints, handler.handle,
		subscriptions.WithPollInterval(5*time.Millisecond), subscriptions.WithBatchSize(2))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	stop := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "projector", 3)
	stop()

	want := []string{"ev-1", "ev-2", "ev-3"}
	got := handler.handled()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected events in feed order %v, got %v", want, got)
		}
	}

	// "Restart": a fresh run resumes from the checkpoint and sees only new events.
	appendNumberedEvents(t, store, 4, 2)
	restarted := &recordingHandler{}
	sub2, err := subscriptions.NewSubscriber("projector", store, checkpoints, restarted.handle,
		subscriptions.WithPollInterval(5*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}
	stop2 := runSubscriber(t, sub2)
	waitForCheckpoint(t, checkpoints, "projector", 5)
	stop2()

	got = restarted.handled()
	if len(got) != 2 || got[0] != "ev-4" || got[1] != "ev-5" {
		t.Fatalf("expected restart to process only [ev-4 ev-5], got %v", got)
	}
}

func TestSubscriber_DrainsInFlightBatchOnShutdown(t *testing.T) {
	t.Parallel()

	store := infrastructure.NewMemoryStore()
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	appendNumberedEvents(t, store, 1, 3)

	started := make(chan struct{})
	release := make(chan struct{})
	recorder := &recordingHandler{}
	var startOnce sync.Once
	handler := func(ctx context.Context, event domain.EventEnvelope[any]) error {
		startOnce.Do(func() {
			close(started)
			<-release
		})
		return recorder.handle(ctx, event)
	}

	sub, err := subscriptions.NewSubscriber("drainer", store, checkpoints, handler,
		subscriptions.WithPollInterval(5*time.Millisecond), subscriptions.WithBatchSize(10))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sub.Run(ctx) }()

	// Cancel while the handler is mid-batch on the first event, then let it
	// proceed: the whole batch must still complete and the checkpoint advance.
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

	if got := recorder.handled(); len(got) != 3 {
		t.Fatalf("expected the in-flight batch of 3 events to drain, got %v", got)
	}
	position, err := checkpoints.Position(context.Background(), "drainer")
	if err != nil {
		t.Fatalf("failed to read checkpoint: %v", err)
	}
	if position != 3 {
		t.Fatalf("expected checkpoint 3 after drain, got %d", position)
	}
}

func TestSubscriber_ResetCheckpointReplaysHistory(t *testing.T) {
	t.Parallel()

	store := infrastructure.NewMemoryStore()
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	handler := &recordingHandler{}
	appendNumberedEvents(t, store, 1, 3)

	sub, err := subscriptions.NewSubscriber("replayer", store, checkpoints, handler.handle,
		subscriptions.WithPollInterval(5*time.Millisecond), subscriptions.WithBatchSize(2))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	stop := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "replayer", 3)
	stop()

	if err := sub.ResetCheckpoint(context.Background(), 0); err != nil {
		t.Fatalf("failed to reset checkpoint: %v", err)
	}
	lag, err := sub.Lag(context.Background())
	if err != nil {
		t.Fatalf("failed to read lag: %v", err)
	}
	if lag != 3 {
		t.Fatalf("expected lag 3 after reset, got %d", lag)
	}

	stop2 := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "replayer", 3)
	stop2()

	want := []string{"ev-1", "ev-2", "ev-3", "ev-1", "ev-2", "ev-3"}
	got := handler.handled()
	if len(got) != len(want) {
		t.Fatalf("expected full replay %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected replay in order %v, got %v", want, got)
		}
	}
}

func TestSubscriber_Lag(t *testing.T) {
	t.Parallel()

	store := infrastructure.NewMemoryStore()
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	handler := &recordingHandler{}
	appendNumberedEvents(t, store, 1, 4)

	sub, err := subscriptions.NewSubscriber("meter", store, checkpoints, handler.handle,
		subscriptions.WithPollInterval(5*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	lag, err := sub.Lag(context.Background())
	if err != nil {
		t.Fatalf("failed to read lag: %v", err)
	}
	if lag != 4 {
		t.Fatalf("expected lag 4 before processing, got %d", lag)
	}

	stop := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "meter", 4)
	stop()

	lag, err = sub.Lag(context.Background())
	if err != nil {
		t.Fatalf("failed to read lag: %v", err)
	}
	if lag != 0 {
		t.Fatalf("expected lag 0 after catching up, got %d", lag)
	}
}

// The package promises EventDispatcher.Dispatch wires directly as a Handler;
// pin the signatures together at compile time.
var _ subscriptions.Handler = (&domain.EventDispatcher{}).Dispatch

func TestSubscriber_SkipsCycleWhileCheckpointHeld(t *testing.T) {
	t.Parallel()

	store := infrastructure.NewMemoryStore()
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	handler := &recordingHandler{}
	appendNumberedEvents(t, store, 1, 2)

	// Hold the checkpoint from outside, as another replica would.
	held, acquired, err := checkpoints.Acquire(context.Background(), "shared")
	if err != nil || !acquired {
		t.Fatalf("failed to pre-acquire checkpoint (acquired=%v): %v", acquired, err)
	}

	sub, err := subscriptions.NewSubscriber("shared", store, checkpoints, handler.handle,
		subscriptions.WithPollInterval(subscriptionTestPollInterval))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}
	stop := runSubscriber(t, sub)
	defer stop()

	// While held, the subscriber must idle without errors or processing.
	time.Sleep(20 * subscriptionTestPollInterval)
	if got := handler.handled(); len(got) != 0 {
		t.Fatalf("expected no events processed while checkpoint held, got %v", got)
	}

	// Released, it must pick up where the checkpoint stands.
	if err := held.Rollback(); err != nil {
		t.Fatalf("failed to release checkpoint: %v", err)
	}
	waitForCheckpoint(t, checkpoints, "shared", 2)
}

func TestSubscriber_TwoSameNameSubscribersProcessExactlyOnce(t *testing.T) {
	t.Parallel()

	store := infrastructure.NewMemoryStore()
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	handler := &recordingHandler{}
	appendNumberedEvents(t, store, 1, 10)

	// Both instances share the handler and the checkpoint name; the
	// checkpoint store must let only one batch run at a time so each event
	// is handled once across the pair.
	mk := func() *subscriptions.Subscriber {
		sub, err := subscriptions.NewSubscriber("pair", store, checkpoints, handler.handle,
			subscriptions.WithPollInterval(subscriptionTestPollInterval), subscriptions.WithBatchSize(3))
		if err != nil {
			t.Fatalf("failed to create subscriber: %v", err)
		}
		return sub
	}
	stopA := runSubscriber(t, mk())
	stopB := runSubscriber(t, mk())
	waitForCheckpoint(t, checkpoints, "pair", 10)
	stopA()
	stopB()

	got := handler.handled()
	if len(got) != 10 {
		t.Fatalf("expected each of the 10 events handled exactly once across both instances, got %d: %v", len(got), got)
	}
	seen := make(map[string]bool, len(got))
	for _, id := range got {
		if seen[id] {
			t.Fatalf("event %s handled more than once: %v", id, got)
		}
		seen[id] = true
	}
}

// flakyStore fails ReadAfter once, then delegates — a transient infrastructure
// hiccup must never stop the subscriber.
type flakyStore struct {
	*infrastructure.MemoryStore
	mu     sync.Mutex
	failed bool
}

func (f *flakyStore) ReadAfter(ctx context.Context, afterPosition int64, limit int) ([]domain.EventEnvelope[any], error) {
	f.mu.Lock()
	if !f.failed {
		f.failed = true
		f.mu.Unlock()
		return nil, errors.New("transient: connection reset")
	}
	f.mu.Unlock()
	return f.MemoryStore.ReadAfter(ctx, afterPosition, limit)
}

func TestSubscriber_SurvivesTransientReadErrors(t *testing.T) {
	t.Parallel()

	store := &flakyStore{MemoryStore: infrastructure.NewMemoryStore()}
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	handler := &recordingHandler{}
	appendNumberedEvents(t, store, 1, 3)

	sub, err := subscriptions.NewSubscriber("resilient", store, checkpoints, handler.handle,
		subscriptions.WithPollInterval(subscriptionTestPollInterval))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}
	stop := runSubscriber(t, sub)
	waitForCheckpoint(t, checkpoints, "resilient", 3)
	stop()

	if got := handler.handled(); len(got) != 3 {
		t.Fatalf("expected all 3 events after the transient error, got %v", got)
	}
}

func TestSubscriber_HandlerPanicReleasesCheckpoint(t *testing.T) {
	t.Parallel()

	store := infrastructure.NewMemoryStore()
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	appendNumberedEvents(t, store, 1, 2)

	sub, err := subscriptions.NewSubscriber("panicky", store, checkpoints,
		func(ctx context.Context, event domain.EventEnvelope[any]) error { panic("handler bug") },
		subscriptions.WithPollInterval(subscriptionTestPollInterval))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	recovered := make(chan any, 1)
	go func() {
		defer func() { recovered <- recover() }()
		_ = sub.Run(context.Background())
	}()
	select {
	case r := <-recovered:
		if r == nil {
			t.Fatal("expected the handler panic to propagate out of Run")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("handler panic did not propagate within 10s")
	}

	// The batch must have been rolled back on the way out: a healthy
	// subscriber can acquire the checkpoint and process everything.
	handler := &recordingHandler{}
	healthy, err := subscriptions.NewSubscriber("panicky", store, checkpoints, handler.handle,
		subscriptions.WithPollInterval(subscriptionTestPollInterval))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}
	stop := runSubscriber(t, healthy)
	waitForCheckpoint(t, checkpoints, "panicky", 2)
	stop()

	if got := handler.handled(); len(got) != 2 {
		t.Fatalf("expected the events to process after the panicked batch was released, got %v", got)
	}
}

// noFeedStore simulates an event store without a global ordered feed.
type noFeedStore struct {
	*infrastructure.MemoryStore
}

func (n *noFeedStore) ReadAfter(ctx context.Context, afterPosition int64, limit int) ([]domain.EventEnvelope[any], error) {
	return nil, domain.ErrGlobalOrderingNotSupported
}

func TestSubscriber_RunFailsFastWithoutOrderedFeed(t *testing.T) {
	t.Parallel()

	store := &noFeedStore{MemoryStore: infrastructure.NewMemoryStore()}
	sub, err := subscriptions.NewSubscriber("doomed", store, subscriptions.NewMemoryCheckpointStore(),
		func(ctx context.Context, event domain.EventEnvelope[any]) error { return nil })
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- sub.Run(context.Background()) }()
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrGlobalOrderingNotSupported) {
			t.Fatalf("expected ErrGlobalOrderingNotSupported, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not fail fast on a store without an ordered feed")
	}
}

func TestNewSubscriber_Validation(t *testing.T) {
	t.Parallel()

	store := infrastructure.NewMemoryStore()
	checkpoints := subscriptions.NewMemoryCheckpointStore()
	handler := func(ctx context.Context, event domain.EventEnvelope[any]) error { return nil }

	tests := []struct {
		name string
		make func() (*subscriptions.Subscriber, error)
	}{
		{"empty name", func() (*subscriptions.Subscriber, error) {
			return subscriptions.NewSubscriber("", store, checkpoints, handler)
		}},
		{"nil event store", func() (*subscriptions.Subscriber, error) {
			return subscriptions.NewSubscriber("s", nil, checkpoints, handler)
		}},
		{"nil checkpoint store", func() (*subscriptions.Subscriber, error) {
			return subscriptions.NewSubscriber("s", store, nil, handler)
		}},
		{"nil handler", func() (*subscriptions.Subscriber, error) {
			return subscriptions.NewSubscriber("s", store, checkpoints, nil)
		}},
		{"non-positive batch size", func() (*subscriptions.Subscriber, error) {
			return subscriptions.NewSubscriber("s", store, checkpoints, handler, subscriptions.WithBatchSize(0))
		}},
		{"non-positive poll interval", func() (*subscriptions.Subscriber, error) {
			return subscriptions.NewSubscriber("s", store, checkpoints, handler, subscriptions.WithPollInterval(0))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.make(); err == nil {
				t.Fatal("expected a validation error, got nil")
			}
		})
	}
}
