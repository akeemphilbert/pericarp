package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

const (
	// DefaultBatchSize is the number of events read per cycle when
	// WithBatchSize is not supplied.
	DefaultBatchSize = 100

	// DefaultPollInterval is how long an idle subscriber waits before
	// checking the feed again when WithPollInterval is not supplied.
	DefaultPollInterval = time.Second
)

// Subscriber runs a Handler as a crash-safe background worker over the event
// store's global ordered feed. Each cycle acquires the subscriber's
// checkpoint, reads a batch of events past it via EventStore.ReadAfter,
// invokes the handler for each event, and advances the checkpoint. Because
// the checkpoint is the only state, crash recovery and normal startup are the
// same code path.
//
// A handler error abandons the whole batch — the checkpoint stays put, any
// handler writes made through the batch transaction roll back, and the batch
// is retried after the poll interval. Events are therefore delivered
// at-least-once; handlers writing through TxFromContext get exactly-once.
type Subscriber struct {
	name         string
	events       domain.EventStore
	checkpoints  CheckpointStore
	handler      Handler
	batchSize    int
	pollInterval time.Duration
	logger       *slog.Logger
}

// SubscriberOption configures a Subscriber.
type SubscriberOption func(*Subscriber)

// WithBatchSize sets how many events are read per cycle (default
// DefaultBatchSize).
func WithBatchSize(n int) SubscriberOption {
	return func(s *Subscriber) { s.batchSize = n }
}

// WithPollInterval sets how long an idle subscriber waits before checking the
// feed again (default DefaultPollInterval). It is also the retry delay after
// a failed batch.
func WithPollInterval(d time.Duration) SubscriberOption {
	return func(s *Subscriber) { s.pollInterval = d }
}

// WithLogger sets the logger for batch failures and lifecycle events. The
// default discards logs.
func WithLogger(logger *slog.Logger) SubscriberOption {
	return func(s *Subscriber) { s.logger = logger }
}

// NewSubscriber creates a subscriber. name identifies the checkpoint —
// processes using the same name share one position. handler is invoked for
// every event in feed order; EventDispatcher.Dispatch satisfies the Handler
// signature for pattern-based routing.
func NewSubscriber(name string, events domain.EventStore, checkpoints CheckpointStore, handler Handler, opts ...SubscriberOption) (*Subscriber, error) {
	if name == "" {
		return nil, errors.New("subscriber name must not be empty")
	}
	if events == nil {
		return nil, errors.New("event store must not be nil")
	}
	if checkpoints == nil {
		return nil, errors.New("checkpoint store must not be nil")
	}
	if handler == nil {
		return nil, errors.New("handler must not be nil")
	}

	s := &Subscriber{
		name:         name,
		events:       events,
		checkpoints:  checkpoints,
		handler:      handler,
		batchSize:    DefaultBatchSize,
		pollInterval: DefaultPollInterval,
		logger:       slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.batchSize <= 0 {
		return nil, fmt.Errorf("batch size must be positive, got %d", s.batchSize)
	}
	if s.pollInterval <= 0 {
		return nil, fmt.Errorf("poll interval must be positive, got %v", s.pollInterval)
	}
	return s, nil
}

// Name returns the subscriber's checkpoint name.
func (s *Subscriber) Name() string { return s.name }

// Run processes the feed until ctx is cancelled. A batch that is already in
// flight when cancellation arrives is drained — handlers finish and the
// checkpoint advances — before Run returns nil.
//
// Transient errors (database hiccups, handler failures) are logged and
// retried after the poll interval; they never stop the subscriber. The only
// error Run returns is fatal misconfiguration: an event store without a
// global ordered feed (ErrGlobalOrderingNotSupported).
func (s *Subscriber) Run(ctx context.Context) error {
	s.logger.Info("subscriber started", "subscriber", s.name)
	defer s.logger.Info("subscriber stopped", "subscriber", s.name)

	for {
		processed, err := s.processBatch(ctx)
		if err != nil {
			if errors.Is(err, domain.ErrGlobalOrderingNotSupported) {
				return fmt.Errorf("subscriber %q cannot run: %w", s.name, err)
			}
			if ctx.Err() == nil {
				s.logger.Error("subscriber batch failed",
					"subscriber", s.name, "error", err)
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		if processed > 0 && err == nil {
			// There may be more backlog; read again immediately.
			continue
		}

		timer := time.NewTimer(s.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// processBatch runs one cycle: acquire checkpoint, read, handle, advance.
// It returns the number of events processed. Once a batch has been read,
// processing and the checkpoint commit are shielded from ctx cancellation so
// shutdown drains the in-flight batch instead of abandoning it.
func (s *Subscriber) processBatch(ctx context.Context) (int, error) {
	batch, acquired, err := s.checkpoints.Acquire(ctx, s.name)
	if err != nil {
		return 0, fmt.Errorf("failed to acquire checkpoint: %w", err)
	}
	if !acquired {
		return 0, nil
	}

	events, err := s.events.ReadAfter(ctx, batch.Position(), s.batchSize)
	if err != nil {
		_ = batch.Rollback()
		return 0, fmt.Errorf("failed to read feed after position %d: %w", batch.Position(), err)
	}
	if len(events) == 0 {
		_ = batch.Rollback()
		return 0, nil
	}

	// From here on the batch is drained even if ctx is cancelled.
	drainCtx := context.WithoutCancel(ctx)
	handlerCtx := batch.HandlerContext(drainCtx)
	for _, event := range events {
		if err := s.handler(handlerCtx, event); err != nil {
			_ = batch.Rollback()
			return 0, fmt.Errorf("handler failed at position %d (event %s, type %s): %w",
				event.Position, event.ID, event.EventType, err)
		}
	}

	last := events[len(events)-1].Position
	if err := batch.Commit(drainCtx, last); err != nil {
		return 0, fmt.Errorf("failed to commit batch at position %d: %w", last, err)
	}
	return len(events), nil
}

// Lag returns how far the subscriber's committed checkpoint trails the feed
// head (0 when caught up). Consumers log or meter it; pericarp does not.
func (s *Subscriber) Lag(ctx context.Context) (int64, error) {
	head, err := s.events.HeadPosition(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to read head position: %w", err)
	}
	position, err := s.checkpoints.Position(ctx, s.name)
	if err != nil {
		return 0, fmt.Errorf("failed to read checkpoint: %w", err)
	}
	return max(head-position, 0), nil
}

// ResetCheckpoint sets the subscriber's checkpoint. Resetting to 0 replays
// all history — incrementally and resumably, since replay uses the same
// batch/checkpoint cycle as live processing. Call it while the subscriber is
// stopped, or rely on the checkpoint store to serialize with in-flight
// batches (the GORM store blocks until the current batch finishes).
func (s *Subscriber) ResetCheckpoint(ctx context.Context, position int64) error {
	return s.checkpoints.Reset(ctx, s.name, position)
}
