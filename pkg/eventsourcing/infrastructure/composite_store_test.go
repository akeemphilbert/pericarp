package infrastructure_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

// recordingStore wraps an EventStore, counts method calls, and captures the
// order events arrive in so ordering tests can assert on it.
type recordingStore struct {
	inner       domain.EventStore
	appendCalls atomic.Int64
	readCalls   atomic.Int64

	orderMu sync.Mutex
	eventIDs []string
}

func newRecordingStore() *recordingStore {
	return &recordingStore{inner: infrastructure.NewMemoryStore()}
}

func (r *recordingStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	r.appendCalls.Add(1)
	r.orderMu.Lock()
	for _, e := range events {
		r.eventIDs = append(r.eventIDs, e.ID)
	}
	r.orderMu.Unlock()
	return r.inner.Append(ctx, aggregateID, expectedVersion, events...)
}

func (r *recordingStore) observedOrder() []string {
	r.orderMu.Lock()
	defer r.orderMu.Unlock()
	out := make([]string, len(r.eventIDs))
	copy(out, r.eventIDs)
	return out
}

func (r *recordingStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	r.readCalls.Add(1)
	return r.inner.GetEvents(ctx, aggregateID)
}
func (r *recordingStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	r.readCalls.Add(1)
	return r.inner.GetEventsFromVersion(ctx, aggregateID, fromVersion)
}
func (r *recordingStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	r.readCalls.Add(1)
	return r.inner.GetEventsRange(ctx, aggregateID, fromVersion, toVersion)
}
func (r *recordingStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	r.readCalls.Add(1)
	return r.inner.GetEventByID(ctx, eventID)
}
func (r *recordingStore) GetEventsByTransactionID(ctx context.Context, transactionID string) ([]domain.EventEnvelope[any], error) {
	r.readCalls.Add(1)
	return r.inner.GetEventsByTransactionID(ctx, transactionID)
}
func (r *recordingStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	r.readCalls.Add(1)
	return r.inner.GetCurrentVersion(ctx, aggregateID)
}
func (r *recordingStore) Close() error { return r.inner.Close() }

// slowStore delays every Append call by a fixed duration.
type slowStore struct {
	inner domain.EventStore
	delay time.Duration
}

func (s *slowStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	time.Sleep(s.delay)
	return s.inner.Append(ctx, aggregateID, expectedVersion, events...)
}
func (s *slowStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEvents(ctx, aggregateID)
}
func (s *slowStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEventsFromVersion(ctx, aggregateID, fromVersion)
}
func (s *slowStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEventsRange(ctx, aggregateID, fromVersion, toVersion)
}
func (s *slowStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	return s.inner.GetEventByID(ctx, eventID)
}
func (s *slowStore) GetEventsByTransactionID(ctx context.Context, transactionID string) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEventsByTransactionID(ctx, transactionID)
}
func (s *slowStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	return s.inner.GetCurrentVersion(ctx, aggregateID)
}
func (s *slowStore) Close() error { return s.inner.Close() }

// failingAppendStore returns a fixed error on every Append.
type failingAppendStore struct {
	inner domain.EventStore
	err   error
}

func (f *failingAppendStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	return f.err
}
func (f *failingAppendStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	return f.inner.GetEvents(ctx, aggregateID)
}
func (f *failingAppendStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	return f.inner.GetEventsFromVersion(ctx, aggregateID, fromVersion)
}
func (f *failingAppendStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	return f.inner.GetEventsRange(ctx, aggregateID, fromVersion, toVersion)
}
func (f *failingAppendStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	return f.inner.GetEventByID(ctx, eventID)
}
func (f *failingAppendStore) GetEventsByTransactionID(ctx context.Context, transactionID string) ([]domain.EventEnvelope[any], error) {
	return f.inner.GetEventsByTransactionID(ctx, transactionID)
}
func (f *failingAppendStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	return f.inner.GetCurrentVersion(ctx, aggregateID)
}
func (f *failingAppendStore) Close() error { return f.inner.Close() }

// closeFailsStore returns a fixed error from Close(); delegates everything else.
type closeFailsStore struct {
	inner    domain.EventStore
	closeErr error
}

func (s *closeFailsStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	return s.inner.Append(ctx, aggregateID, expectedVersion, events...)
}
func (s *closeFailsStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEvents(ctx, aggregateID)
}
func (s *closeFailsStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEventsFromVersion(ctx, aggregateID, fromVersion)
}
func (s *closeFailsStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEventsRange(ctx, aggregateID, fromVersion, toVersion)
}
func (s *closeFailsStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	return s.inner.GetEventByID(ctx, eventID)
}
func (s *closeFailsStore) GetEventsByTransactionID(ctx context.Context, transactionID string) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEventsByTransactionID(ctx, transactionID)
}
func (s *closeFailsStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	return s.inner.GetCurrentVersion(ctx, aggregateID)
}
func (s *closeFailsStore) Close() error { return s.closeErr }

// waitForAppendCount polls r.appendCalls until it reaches want or the deadline expires.
func waitForAppendCount(t *testing.T, r *recordingStore, want int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if r.appendCalls.Load() >= want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("secondary never reached %d appends; got %d", want, r.appendCalls.Load())
}

func TestComposite_AppendForwardsToPrimaryAndSecondaries(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	secondary := newRecordingStore()
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{secondary})

	ctx := context.Background()
	event := createTestEvent("agg-1", "evt-1", "test.created", 1)
	if err := composite.Append(ctx, "agg-1", -1, event); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := primary.GetEvents(ctx, "agg-1")
	if err != nil || len(got) != 1 {
		t.Fatalf("primary missing event: len=%d err=%v", len(got), err)
	}

	waitForAppendCount(t, secondary, 1, time.Second)

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_MultipleSecondariesAllReceive(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	sec1 := newRecordingStore()
	sec2 := newRecordingStore()
	sec3 := newRecordingStore()
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{sec1, sec2, sec3})

	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		ev := createTestEvent("agg-1", uniqueID("evt", i), "test.created", i)
		if err := composite.Append(ctx, "agg-1", i-1, ev); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	waitForAppendCount(t, sec1, 5, time.Second)
	waitForAppendCount(t, sec2, 5, time.Second)
	waitForAppendCount(t, sec3, 5, time.Second)

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_PrimaryFailureShortCircuitsSecondaries(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("primary exploded")
	primary := &failingAppendStore{inner: infrastructure.NewMemoryStore(), err: sentinel}
	secondary := newRecordingStore()
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{secondary})

	ctx := context.Background()
	err := composite.Append(ctx, "agg-1", -1, createTestEvent("agg-1", "evt-1", "test.created", 1))
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected primary error, got %v", err)
	}

	// Give any (erroneous) secondary work a chance to run before asserting zero.
	time.Sleep(20 * time.Millisecond)
	if got := secondary.appendCalls.Load(); got != 0 {
		t.Fatalf("secondary must not receive events when primary fails; got %d calls", got)
	}

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_AppendLatencyBoundedByPrimary(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	slow := &slowStore{inner: infrastructure.NewMemoryStore(), delay: 500 * time.Millisecond}
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{slow})

	ctx := context.Background()
	start := time.Now()
	if err := composite.Append(ctx, "agg-1", -1, createTestEvent("agg-1", "evt-1", "test.created", 1)); err != nil {
		t.Fatalf("append: %v", err)
	}
	elapsed := time.Since(start)

	// Primary is in-memory; composite should return in well under half the
	// secondary's delay. Generous threshold keeps CI runners happy.
	if elapsed > slow.delay/2 {
		t.Fatalf("append latency %v leaked secondary delay (expected < %v)", elapsed, slow.delay/2)
	}

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_ReadsForwardToPrimaryOnly(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	secondary := newRecordingStore()
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{secondary})

	ctx := context.Background()
	_ = composite.Append(ctx, "agg-1", -1, createTestEvent("agg-1", "evt-1", "test.created", 1))
	waitForAppendCount(t, secondary, 1, time.Second)

	_, _ = composite.GetEvents(ctx, "agg-1")
	_, _ = composite.GetEventsFromVersion(ctx, "agg-1", 1)
	_, _ = composite.GetEventsRange(ctx, "agg-1", -1, -1)
	_, _ = composite.GetEventByID(ctx, "evt-1")
	_, _ = composite.GetEventsByTransactionID(ctx, "tx-1")
	_, _ = composite.GetCurrentVersion(ctx, "agg-1")

	if got := secondary.readCalls.Load(); got != 0 {
		t.Fatalf("reads must forward to primary only; secondary saw %d read calls", got)
	}

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_NoSecondariesIsValid(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	composite := infrastructure.NewCompositeEventStore(primary, nil)

	ctx := context.Background()
	if err := composite.Append(ctx, "agg-1", -1, createTestEvent("agg-1", "evt-1", "test.created", 1)); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_SecondaryFailureInvokesHandler(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	sentinel := errors.New("secondary down")
	failing := &failingAppendStore{inner: infrastructure.NewMemoryStore(), err: sentinel}

	var handlerCalls atomic.Int64
	var gotIdx atomic.Int64
	gotIdx.Store(-1)
	handler := func(idx int, err error, envelopes []domain.EventEnvelope[any]) {
		handlerCalls.Add(1)
		gotIdx.Store(int64(idx))
		if !errors.Is(err, sentinel) {
			t.Errorf("handler got unexpected err: %v", err)
		}
		if len(envelopes) != 1 {
			t.Errorf("handler got %d envelopes, want 1", len(envelopes))
		}
	}
	composite := infrastructure.NewCompositeEventStore(
		primary,
		[]domain.EventStore{failing},
		infrastructure.WithErrorHandler(handler),
	)

	ctx := context.Background()
	const commits = 4
	for i := 1; i <= commits; i++ {
		ev := createTestEvent("agg-1", uniqueID("evt", i), "test.created", i)
		if err := composite.Append(ctx, "agg-1", i-1, ev); err != nil {
			t.Fatalf("commit %d should succeed on primary, got: %v", i, err)
		}
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && handlerCalls.Load() < commits {
		time.Sleep(2 * time.Millisecond)
	}
	if got := handlerCalls.Load(); got != commits {
		t.Fatalf("handler called %d times, want %d", got, commits)
	}
	if got := gotIdx.Load(); got != 0 {
		t.Fatalf("handler got idx %d, want 0", got)
	}

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_HandlerIdentifiesFailingSecondary(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	good := newRecordingStore()
	bad := &failingAppendStore{inner: infrastructure.NewMemoryStore(), err: errors.New("boom")}

	var failingIdx atomic.Int64
	failingIdx.Store(-1)
	handler := func(idx int, err error, envelopes []domain.EventEnvelope[any]) {
		failingIdx.Store(int64(idx))
	}

	// Order: [good, bad] → failing idx should be 1.
	composite := infrastructure.NewCompositeEventStore(
		primary,
		[]domain.EventStore{good, bad},
		infrastructure.WithErrorHandler(handler),
	)

	ctx := context.Background()
	if err := composite.Append(ctx, "agg-1", -1, createTestEvent("agg-1", "evt-1", "test.created", 1)); err != nil {
		t.Fatalf("append: %v", err)
	}
	waitForAppendCount(t, good, 1, time.Second)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && failingIdx.Load() != 1 {
		time.Sleep(2 * time.Millisecond)
	}
	if got := failingIdx.Load(); got != 1 {
		t.Fatalf("failing secondary idx = %d, want 1", got)
	}

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_OrderingAcrossAggregates(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	// Two secondaries — fan-out must deliver the same global order to each,
	// independently. A regression introducing per-secondary reordering (e.g.,
	// a shared worker pool or map iteration) would break this test.
	sec1 := newRecordingStore()
	sec2 := newRecordingStore()
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{sec1, sec2})

	ctx := context.Background()
	type step struct {
		agg     string
		seq     int
		eventID string
	}
	steps := []step{
		{"agg-a", 1, "a-1"},
		{"agg-b", 1, "b-1"},
		{"agg-a", 2, "a-2"},
		{"agg-c", 1, "c-1"},
		{"agg-b", 2, "b-2"},
		{"agg-a", 3, "a-3"},
		{"agg-c", 2, "c-2"},
	}

	versions := map[string]int{}
	wantOrder := make([]string, 0, len(steps))
	for _, s := range steps {
		ev := createTestEvent(s.agg, s.eventID, "test.created", s.seq)
		if err := composite.Append(ctx, s.agg, versions[s.agg], ev); err != nil {
			t.Fatalf("append %s: %v", s.eventID, err)
		}
		versions[s.agg] = s.seq
		wantOrder = append(wantOrder, s.eventID)
	}

	for _, sec := range []*recordingStore{sec1, sec2} {
		waitForAppendCount(t, sec, int64(len(steps)), time.Second)
		got := sec.observedOrder()
		if len(got) != len(wantOrder) {
			t.Fatalf("secondary saw %d events, want %d", len(got), len(wantOrder))
		}
		for i := range wantOrder {
			if got[i] != wantOrder[i] {
				t.Fatalf("order mismatch at %d: got %s want %s (full: %v)", i, got[i], wantOrder[i], got)
			}
		}
	}

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_PerAggregateOrderUnderConcurrentCallers(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	secondary := newRecordingStore()
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{secondary})
	t.Cleanup(func() { _ = composite.Close() })

	const aggregates = 4
	const perAgg = 25
	var wg sync.WaitGroup
	for a := range aggregates {
		wg.Add(1)
		go func(a int) {
			defer wg.Done()
			agg := "agg-" + strconv.Itoa(a)
			for seq := 1; seq <= perAgg; seq++ {
				id := agg + "-" + strconv.Itoa(seq)
				ev := createTestEvent(agg, id, "test.created", seq)
				if err := composite.Append(context.Background(), agg, seq-1, ev); err != nil {
					t.Errorf("append %s: %v", id, err)
					return
				}
			}
		}(a)
	}
	wg.Wait()

	waitForAppendCount(t, secondary, int64(aggregates*perAgg), 3*time.Second)

	observedSeqs := map[string][]int{}
	for _, id := range secondary.observedOrder() {
		// id = "agg-N-seq"; split on last '-'.
		cut := -1
		for i := len(id) - 1; i >= 0; i-- {
			if id[i] == '-' {
				cut = i
				break
			}
		}
		if cut < 0 {
			t.Fatalf("unexpected event id: %q", id)
		}
		agg := id[:cut]
		seq, err := strconv.Atoi(id[cut+1:])
		if err != nil {
			t.Fatalf("bad seq in %q: %v", id, err)
		}
		observedSeqs[agg] = append(observedSeqs[agg], seq)
	}

	for agg, seqs := range observedSeqs {
		for i := 1; i < len(seqs); i++ {
			if seqs[i] != seqs[i-1]+1 {
				t.Fatalf("%s out of order: %v", agg, seqs)
			}
		}
		if len(seqs) != perAgg {
			t.Fatalf("%s saw %d events, want %d", agg, len(seqs), perAgg)
		}
	}
}

func TestComposite_PanickingHandlerDoesNotStopReplication(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	failing := &failingAppendStore{inner: infrastructure.NewMemoryStore(), err: errors.New("always")}

	var seen atomic.Int64
	handler := func(idx int, err error, envelopes []domain.EventEnvelope[any]) {
		seen.Add(1)
		panic("handler panics")
	}
	composite := infrastructure.NewCompositeEventStore(
		primary,
		[]domain.EventStore{failing},
		infrastructure.WithErrorHandler(handler),
	)

	ctx := context.Background()
	const commits = 3
	for i := 1; i <= commits; i++ {
		ev := createTestEvent("agg-1", uniqueID("evt", i), "test.created", i)
		if err := composite.Append(ctx, "agg-1", i-1, ev); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && seen.Load() < commits {
		time.Sleep(2 * time.Millisecond)
	}
	if got := seen.Load(); got != commits {
		t.Fatalf("handler called %d times, want %d — panic must not stall goroutine", got, commits)
	}

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_NoHandlerSwallowsErrors(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	failing := &failingAppendStore{inner: infrastructure.NewMemoryStore(), err: errors.New("boom")}
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{failing})

	ctx := context.Background()
	if err := composite.Append(ctx, "agg-1", -1, createTestEvent("agg-1", "evt-1", "test.created", 1)); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestComposite_CloseDrainsPendingSecondaryWrites(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	// Slow inner secondary so queued jobs pile up and drain happens during Close.
	slowInner := &slowStore{inner: infrastructure.NewMemoryStore(), delay: 10 * time.Millisecond}
	secondary := &recordingStore{inner: slowInner}

	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{secondary})

	ctx := context.Background()
	const commits = 20
	for i := 1; i <= commits; i++ {
		ev := createTestEvent("agg-1", uniqueID("evt", i), "test.created", i)
		if err := composite.Append(ctx, "agg-1", i-1, ev); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}

	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := secondary.appendCalls.Load(); got != commits {
		t.Fatalf("secondary saw %d appends after Close, want %d", got, commits)
	}
}

func TestComposite_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	secondary := newRecordingStore()
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{secondary})

	if err := composite.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// A second Close must not panic (double-close channel) or error.
	if err := composite.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	// A third for good measure.
	if err := composite.Close(); err != nil {
		t.Fatalf("third close: %v", err)
	}
}

func TestComposite_ClosePropagatesPrimaryError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("primary close failed")
	primary := &closeFailsStore{inner: infrastructure.NewMemoryStore(), closeErr: sentinel}
	secondary := newRecordingStore()
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{secondary})

	err := composite.Close()
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped primary close error, got: %v", err)
	}

	// Idempotent: second call returns same cached error.
	if err := composite.Close(); !errors.Is(err, sentinel) {
		t.Fatalf("second close returned %v, want wrapped primary error", err)
	}
}

func TestComposite_ClosePropagatesSecondaryError(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	sentinel := errors.New("secondary close failed")
	secondary := &closeFailsStore{inner: infrastructure.NewMemoryStore(), closeErr: sentinel}
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{secondary})

	err := composite.Close()
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped secondary close error, got: %v", err)
	}
}

func TestComposite_EmptyCloseReturnsFast(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	secondary := newRecordingStore()
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{secondary})

	start := time.Now()
	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("empty close took %v, expected < 50ms", elapsed)
	}
}

func TestComposite_ConcurrentAppendAndClose(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	// A few secondaries to make the Close fan-out visible to the race detector.
	sec1 := newRecordingStore()
	sec2 := newRecordingStore()
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{sec1, sec2})

	const writers = 8
	var started sync.WaitGroup
	var done sync.WaitGroup
	errCh := make(chan error, writers*64)

	for w := range writers {
		started.Add(1)
		done.Add(1)
		go func(w int) {
			defer done.Done()
			started.Done()
			agg := "agg-" + strconv.Itoa(w)
			for i := 1; i <= 64; i++ {
				ev := createTestEvent(agg, agg+"-"+strconv.Itoa(i), "test.created", i)
				err := composite.Append(context.Background(), agg, -1, ev)
				if err != nil && !errors.Is(err, infrastructure.ErrCompositeClosed) {
					errCh <- err
					return
				}
				if err != nil {
					return
				}
			}
		}(w)
	}

	started.Wait()
	// Concurrently close while writers are active. No panic; writers either
	// succeed or see ErrCompositeClosed.
	if err := composite.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	done.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("unexpected append error: %v", err)
	}

	// Subsequent Appends fail fast.
	err := composite.Append(context.Background(), "agg-late", -1, createTestEvent("agg-late", "late-1", "test.created", 1))
	if !errors.Is(err, infrastructure.ErrCompositeClosed) {
		t.Fatalf("Append after Close returned %v, want ErrCompositeClosed", err)
	}
}

func TestComposite_FullBufferRespectsContext(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	// Very slow secondary + tiny buffer guarantees the third Append's send
	// blocks; the caller's context deadline must unblock it.
	slow := &slowStore{inner: infrastructure.NewMemoryStore(), delay: time.Second}
	composite := infrastructure.NewCompositeEventStore(
		primary,
		[]domain.EventStore{slow},
		infrastructure.WithSecondaryBufferSize(2),
	)
	t.Cleanup(func() { _ = composite.Close() })

	// Fill the buffer (2 slots) + inflight (1 sleeping in secondary Append).
	for i := 1; i <= 3; i++ {
		ctx := context.Background()
		if err := composite.Append(ctx, "agg-fill", -1, createTestEvent("agg-fill", "fill-"+strconv.Itoa(i), "test.created", i)); err != nil {
			t.Fatalf("prefill %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := composite.Append(ctx, "agg-fill", -1, createTestEvent("agg-fill", "fill-4", "test.created", 4))
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	// The ctx unblocks Append promptly — within a small multiple of the deadline,
	// not the secondary's 1s processing time.
	if elapsed > 200*time.Millisecond {
		t.Fatalf("ctx timeout honored too late (%v)", elapsed)
	}
	// Primary is authoritative: it committed fill-4 before the enqueue timed out.
	// This is documented Append behavior — the ctx error signals secondary-side
	// incompleteness, not primary rollback.
}

// transientFailStore fails the first failures Appends, then delegates to inner.
type transientFailStore struct {
	inner    domain.EventStore
	failures int64
	calls    atomic.Int64
	err      error
}

func (s *transientFailStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	n := s.calls.Add(1)
	if n <= s.failures {
		return s.err
	}
	return s.inner.Append(ctx, aggregateID, expectedVersion, events...)
}
func (s *transientFailStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEvents(ctx, aggregateID)
}
func (s *transientFailStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEventsFromVersion(ctx, aggregateID, fromVersion)
}
func (s *transientFailStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEventsRange(ctx, aggregateID, fromVersion, toVersion)
}
func (s *transientFailStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	return s.inner.GetEventByID(ctx, eventID)
}
func (s *transientFailStore) GetEventsByTransactionID(ctx context.Context, transactionID string) ([]domain.EventEnvelope[any], error) {
	return s.inner.GetEventsByTransactionID(ctx, transactionID)
}
func (s *transientFailStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	return s.inner.GetCurrentVersion(ctx, aggregateID)
}
func (s *transientFailStore) Close() error { return s.inner.Close() }

func TestComposite_SecondaryRecoversAfterTransientFailure(t *testing.T) {
	t.Parallel()

	primary := infrastructure.NewMemoryStore()
	secondaryInner := infrastructure.NewMemoryStore()
	secondary := &transientFailStore{inner: secondaryInner, failures: 1, err: errors.New("transient")}
	composite := infrastructure.NewCompositeEventStore(primary, []domain.EventStore{secondary})
	t.Cleanup(func() { _ = composite.Close() })

	ctx := context.Background()
	const events = 5
	for i := 1; i <= events; i++ {
		ev := createTestEvent("agg-1", uniqueID("evt", i), "test.created", i)
		if err := composite.Append(ctx, "agg-1", i-1, ev); err != nil {
			t.Fatalf("primary append %d: %v", i, err)
		}
	}

	// Give goroutine time to process all five.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && secondary.calls.Load() < events {
		time.Sleep(5 * time.Millisecond)
	}

	// Event 1 failed on secondary; events 2..5 must have landed — proving
	// secondaries don't wedge after a miss (primary version not replayed).
	landed, err := secondaryInner.GetEvents(ctx, "agg-1")
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(landed) != events-1 {
		t.Fatalf("secondary has %d events, want %d", len(landed), events-1)
	}
	if landed[0].ID != uniqueID("evt", 2) {
		t.Fatalf("secondary first event ID = %s, want %s", landed[0].ID, uniqueID("evt", 2))
	}
}

func TestComposite_NilPrimaryPanics(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil primary")
		}
	}()
	_ = infrastructure.NewCompositeEventStore(nil, nil)
}

func uniqueID(prefix string, i int) string {
	return prefix + "-" + strconv.Itoa(i)
}

var (
	_ domain.EventStore = (*recordingStore)(nil)
	_ domain.EventStore = (*slowStore)(nil)
	_ domain.EventStore = (*failingAppendStore)(nil)
	_ domain.EventStore = (*closeFailsStore)(nil)
	_ domain.EventStore = (*transientFailStore)(nil)
)
