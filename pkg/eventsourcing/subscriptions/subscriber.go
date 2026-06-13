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
// default is slog.Default() — a permanently failing subscriber must be
// visible somewhere out of the box.
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
		logger:       slog.Default(),
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
			// Cancellation noise from Acquire/ReadAfter during shutdown is
			// expected; anything else — including a failed drain of the final
			// batch — is a real error and must be visible.
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
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
func (s *Subscriber) processBatch(ctx context.Context) (processed int, err error) {
	batch, acquired, err := s.checkpoints.Acquire(ctx, s.name)
	if err != nil {
		return 0, fmt.Errorf("failed to acquire checkpoint: %w", err)
	}
	if !acquired {
		s.logger.Debug("checkpoint held by another process; skipping cycle",
			"subscriber", s.name)
		return 0, nil
	}

	// The batch must end no matter how this function exits — including a
	// handler panic. A leaked batch would hold the checkpoint (and, for
	// database-backed stores, its transaction and row lock) forever,
	// silently starving every replica of the subscriber.
	batchFinished := false
	defer func() {
		if batchFinished {
			return
		}
		if rbErr := batch.Rollback(); rbErr != nil {
			rbErr = fmt.Errorf("failed to roll back batch: %w", rbErr)
			if err == nil {
				err = rbErr
			} else {
				err = errors.Join(err, rbErr)
			}
		}
	}()

	events, err := s.events.ReadAfter(ctx, batch.Position(), s.batchSize)
	if err != nil {
		return 0, fmt.Errorf("failed to read feed after position %d: %w", batch.Position(), err)
	}
	if len(events) == 0 {
		return 0, nil
	}

	// From here on the batch is drained even if ctx is cancelled. A handler
	// panic propagates to the caller after the deferred rollback runs.
	drainCtx := context.WithoutCancel(ctx)
	handlerCtx := batch.HandlerContext(drainCtx)
	for _, event := range events {
		if err := s.handler(handlerCtx, event); err != nil {
			return 0, fmt.Errorf("handler failed at position %d (event %s, type %s): %w",
				event.Position, event.ID, event.EventType, err)
		}
	}

	last := events[len(events)-1].Position
	err = batch.Commit(drainCtx, last)
	batchFinished = true // Commit owns the batch outcome either way; never double-finish.
	if err != nil {
		return 0, fmt.Errorf("failed to commit batch at position %d: %w", last, err)
	}
	return len(events), nil
}

// Lag returns how far the subscriber's committed checkpoint trails the feed
// head (0 when caught up). Consumers log or meter it; pericarp does not.
//
// A checkpoint ahead of the head is reported as an error rather than clamped
// to 0: it means the events table was truncated or the checkpoint was reset
// past the head, and the subscriber would otherwise look caught up while
// silently skipping everything until positions catch back up.
func (s *Subscriber) Lag(ctx context.Context) (int64, error) {
	head, err := s.events.HeadPosition(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to read head position: %w", err)
	}
	position, err := s.checkpoints.Position(ctx, s.name)
	if err != nil {
		return 0, fmt.Errorf("failed to read checkpoint: %w", err)
	}
	if position > head {
		return 0, fmt.Errorf("checkpoint %d is ahead of feed head %d: the event store was truncated or the checkpoint was reset past the head", position, head)
	}
	return head - position, nil
}

// ResetCheckpoint sets the subscriber's checkpoint. Resetting to 0 replays
// all history — incrementally and resumably, since replay uses the same
// batch/checkpoint cycle as live processing. It is safe while the subscriber
// runs: the checkpoint store serializes with in-flight batches (see
// CheckpointStore.Reset), so the reset is never silently overwritten.
func (s *Subscriber) ResetCheckpoint(ctx context.Context, position int64) error {
	return s.checkpoints.Reset(ctx, s.name, position)
}
