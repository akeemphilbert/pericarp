package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// defaultSecondaryBufferSize sizes each secondary's ingest channel.
const defaultSecondaryBufferSize = 1024

// ErrCompositeClosed is returned by Append calls that arrive after Close has
// been called. Append calls already in flight when Close is initiated either
// drain normally or return ctx.Err() if the caller's context cancels first —
// sendMu holds Close back until every in-flight Append releases its RLock,
// so an Append cannot observe a half-closed store.
var ErrCompositeClosed = errors.New("composite event store: closed")

var _ domain.EventStore = (*CompositeEventStore)(nil)

// SecondaryErrorHandler is invoked when a secondary store's Append returns an
// error. idx is the secondary's position in the slice passed to the constructor.
// Handlers run on the secondary's own goroutine; panics are recovered and
// written to stderr so replication keeps flowing.
type SecondaryErrorHandler func(idx int, err error, envelopes []domain.EventEnvelope[any])

// Option configures a CompositeEventStore.
type Option func(*compositeConfig)

type compositeConfig struct {
	bufferSize   int
	errorHandler SecondaryErrorHandler
}

// WithErrorHandler registers a callback invoked for every failed secondary Append.
// Without this option, secondary errors are silently dropped.
func WithErrorHandler(h SecondaryErrorHandler) Option {
	return func(c *compositeConfig) { c.errorHandler = h }
}

// WithSecondaryBufferSize overrides the per-secondary channel buffer size.
// A full buffer causes Append to block until a slot opens or the caller's
// context is cancelled.
func WithSecondaryBufferSize(n int) Option {
	return func(c *compositeConfig) {
		if n > 0 {
			c.bufferSize = n
		}
	}
}

// CompositeEventStore wraps a primary EventStore plus zero or more secondaries.
// Writes to the primary are synchronous; writes to each secondary run on a
// dedicated background goroutine so secondary processing never contends with
// the primary. If a secondary's buffered channel is full, Append blocks until
// a slot opens or the caller's context is cancelled.
//
// All read methods forward to the primary — secondaries are write-only replicas.
//
// After Close, Append returns ErrCompositeClosed; it is safe to call Close
// multiple times.
type CompositeEventStore struct {
	primary      domain.EventStore
	secondaries  []domain.EventStore
	queues       []chan secondaryJob
	wg           sync.WaitGroup
	errorHandler SecondaryErrorHandler

	// sendMu serializes Append calls (RLock) against Close (Lock). This makes
	// close-on-queue safe: Close cannot close a channel while any Append holds
	// the RLock, and Append sees `closed == true` only after Close has run.
	sendMu    sync.RWMutex
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

type secondaryJob struct {
	aggregateID string
	events      []domain.EventEnvelope[any]
}

// NewCompositeEventStore constructs a composite wrapping primary and the
// given secondaries. Each secondary gets its own goroutine and buffered
// channel so one slow secondary cannot starve the others. Panics if primary
// or any secondary is nil — that's a programmer error, not a runtime
// condition, and a nil secondary would silently kill its drain goroutine
// and eventually wedge Append once the channel buffer fills.
func NewCompositeEventStore(primary domain.EventStore, secondaries []domain.EventStore, opts ...Option) *CompositeEventStore {
	if primary == nil {
		panic("composite: primary must not be nil")
	}
	for i, s := range secondaries {
		if s == nil {
			panic(fmt.Sprintf("composite: secondaries[%d] must not be nil", i))
		}
	}

	cfg := compositeConfig{bufferSize: defaultSecondaryBufferSize}
	for _, opt := range opts {
		opt(&cfg)
	}

	c := &CompositeEventStore{
		primary:      primary,
		secondaries:  secondaries,
		queues:       make([]chan secondaryJob, len(secondaries)),
		errorHandler: cfg.errorHandler,
	}
	for i := range secondaries {
		c.queues[i] = make(chan secondaryJob, cfg.bufferSize)
		c.wg.Add(1)
		go c.runSecondary(i)
	}
	return c
}

func (c *CompositeEventStore) runSecondary(idx int) {
	defer c.wg.Done()
	secondary := c.secondaries[idx]
	for job := range c.queues[idx] {
		// Secondaries are best-effort replicas; the primary is authoritative for
		// optimistic concurrency. Passing -1 here prevents a missed event from
		// permanently wedging all future secondary writes with ErrConcurrencyConflict.
		// Background context: per-request cancellations must not kill replication;
		// secondaries are expected to honor their own deadlines internally.
		if err := secondary.Append(context.Background(), job.aggregateID, -1, job.events...); err != nil {
			c.reportError(idx, err, job.events)
		}
	}
}

// reportError invokes the caller-supplied handler. A handler panic is
// recovered and written to stderr; losing the stack would turn real bugs
// (nil map, type assertion, …) into silent failures.
func (c *CompositeEventStore) reportError(idx int, err error, envelopes []domain.EventEnvelope[any]) {
	if c.errorHandler == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "composite: error handler panicked: %v\n%s\n", r, debug.Stack())
		}
	}()
	c.errorHandler(idx, err, envelopes)
}

// Append writes to the primary synchronously, then enqueues the same events
// to each secondary. Primary errors short-circuit — nothing is sent to any
// secondary if the primary rejected the write. Returns ErrCompositeClosed if
// Close has been called, or ctx.Err() if a full secondary buffer causes
// enqueue to exceed the caller's deadline.
//
// Note: if ctx expires after the primary has committed but before every
// secondary has been enqueued, the primary write stands (it is authoritative)
// and Append returns ctx.Err(). Secondaries are appended with
// expectedVersion=-1, so a skipped enqueue does not later surface as a
// version conflict — the missed events are silently dropped for those
// secondaries unless the secondary itself errors on some future write.
// Callers that must not lose replication should either use an unbounded
// context for Append or reconcile secondaries out of band.
func (c *CompositeEventStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	c.sendMu.RLock()
	defer c.sendMu.RUnlock()
	if c.closed {
		return ErrCompositeClosed
	}
	if err := c.primary.Append(ctx, aggregateID, expectedVersion, events...); err != nil {
		return err
	}
	if len(events) == 0 || len(c.queues) == 0 {
		return nil
	}
	copied := append([]domain.EventEnvelope[any](nil), events...)
	job := secondaryJob{aggregateID: aggregateID, events: copied}
	for _, q := range c.queues {
		select {
		case q <- job:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (c *CompositeEventStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	return c.primary.GetEvents(ctx, aggregateID)
}

func (c *CompositeEventStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	return c.primary.GetEventsFromVersion(ctx, aggregateID, fromVersion)
}

func (c *CompositeEventStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	return c.primary.GetEventsRange(ctx, aggregateID, fromVersion, toVersion)
}

func (c *CompositeEventStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	return c.primary.GetEventByID(ctx, eventID)
}

func (c *CompositeEventStore) GetEventsByTransactionID(ctx context.Context, transactionID string) ([]domain.EventEnvelope[any], error) {
	return c.primary.GetEventsByTransactionID(ctx, transactionID)
}

func (c *CompositeEventStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	return c.primary.GetCurrentVersion(ctx, aggregateID)
}

// Close marks the store closed so concurrent Appends fail fast with
// ErrCompositeClosed, drains every secondary's queue (each goroutine
// processes remaining jobs before exiting), then closes the primary and
// every secondary. It is safe to call more than once; the cached first-call
// result is returned on subsequent calls. All non-nil underlying close
// errors are joined via errors.Join so no failure is shadowed.
func (c *CompositeEventStore) Close() error {
	c.closeOnce.Do(func() {
		// Wait for any in-flight Appends to finish (they hold RLock). Once we
		// hold the write lock, no new Append can enter and no existing one is
		// mid-send — so closing the channels here is race-free.
		c.sendMu.Lock()
		c.closed = true
		for _, q := range c.queues {
			close(q)
		}
		c.sendMu.Unlock()
		c.wg.Wait()

		var errs []error
		if err := c.primary.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close primary: %w", err))
		}
		for i, s := range c.secondaries {
			if err := s.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close secondary[%d]: %w", i, err))
			}
		}
		c.closeErr = errors.Join(errs...)
	})
	return c.closeErr
}
