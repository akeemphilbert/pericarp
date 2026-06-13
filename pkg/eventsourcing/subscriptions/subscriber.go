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

	// DefaultMaxRetries is how many times a failing handler is retried per
	// event (after the initial attempt) before the event is parked, when a
	// ParkingLot is configured and WithMaxRetries is not supplied.
	DefaultMaxRetries = 5

	// DefaultRetryBackoff is the first retry delay; it doubles per attempt.
	DefaultRetryBackoff = 100 * time.Millisecond

	// DefaultMaxRetryBackoff caps the doubling retry delay.
	DefaultMaxRetryBackoff = 5 * time.Second

	// retrySavepoint brackets each handler attempt inside the batch
	// transaction so a failed attempt's partial writes are discarded.
	retrySavepoint = "pericarp_handler_attempt"
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

	// Poison-event handling (only active with a ParkingLot configured).
	parking      ParkingLot
	maxRetries   int
	retryBackoff time.Duration
	maxBackoff   time.Duration

	// wake lets the idle loop react to new commits immediately; nil means
	// poll-only. Polling continues regardless, so lost notifications cost at
	// most one poll interval, never correctness.
	wake <-chan struct{}
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

// WithParkingLot enables poison-event handling: a handler that keeps failing
// on one event has the event parked (with its error) after bounded retries,
// and the checkpoint advances past it so the events behind it keep flowing.
// Without a parking lot, a failing event abandons the batch and is retried
// every poll interval forever.
//
// Failed attempts' writes are discarded only when the batch has a real
// transaction (GormCheckpointStore) and the handler writes through it; with
// MemoryCheckpointStore or out-of-transaction side effects, each retry
// re-applies whatever the handler did before failing.
func WithParkingLot(lot ParkingLot) SubscriberOption {
	return func(s *Subscriber) { s.parking = lot }
}

// WithMaxRetries sets how many times a failing handler is retried per event
// (after the initial attempt) before the event is parked (default
// DefaultMaxRetries). Only meaningful with WithParkingLot.
func WithMaxRetries(n int) SubscriberOption {
	return func(s *Subscriber) { s.maxRetries = n }
}

// WithRetryBackoff sets the first retry delay and its cap; the delay doubles
// per attempt up to the cap (defaults DefaultRetryBackoff and
// DefaultMaxRetryBackoff).
func WithRetryBackoff(initial, maximum time.Duration) SubscriberOption {
	return func(s *Subscriber) {
		s.retryBackoff = initial
		s.maxBackoff = maximum
	}
}

// WithWakeSignal lets the subscriber wake on new commits instead of waiting
// out the poll interval: pass InProcessNotifier.Subscribe() (single-process /
// SQLite) or PostgresListener.Wake() (cross-process via LISTEN/NOTIFY).
// Polling continues as the fallback, so notifications are never load-bearing.
func WithWakeSignal(wake <-chan struct{}) SubscriberOption {
	return func(s *Subscriber) { s.wake = wake }
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
		maxRetries:   DefaultMaxRetries,
		retryBackoff: DefaultRetryBackoff,
		maxBackoff:   DefaultMaxRetryBackoff,
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
	if s.maxRetries < 0 {
		return nil, fmt.Errorf("max retries must not be negative, got %d", s.maxRetries)
	}
	if s.retryBackoff <= 0 || s.maxBackoff < s.retryBackoff {
		return nil, fmt.Errorf("retry backoff must be positive and its cap at least the initial delay, got %v/%v", s.retryBackoff, s.maxBackoff)
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
		case <-s.wake: // nil when unset; a nil channel never fires
			timer.Stop()
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
		if err := s.processEvent(handlerCtx, ctx, batch, event); err != nil {
			return 0, err
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

// processEvent runs the handler for one event. Each attempt is bracketed in a
// batch savepoint so a failed attempt's partial writes are discarded. With a
// ParkingLot configured, failures are retried maxRetries times with doubling
// backoff and then parked — within the batch transaction, so the parking and
// the checkpoint advancing past the event commit atomically. Without one, the
// first failure abandons the batch (retried every poll interval, as before).
//
// runCtx (cancellable) aborts retry backoff on shutdown: a poison event must
// not hold up draining; the batch rolls back and is redelivered on restart.
func (s *Subscriber) processEvent(handlerCtx, runCtx context.Context, batch Batch, event domain.EventEnvelope[any]) error {
	var lastErr error
	for attempt := 0; ; attempt++ {
		if err := batch.Savepoint(handlerCtx, retrySavepoint); err != nil {
			return fmt.Errorf("failed to create savepoint: %w", err)
		}
		lastErr = s.handler(handlerCtx, event)
		if lastErr == nil {
			return nil
		}
		if err := batch.RollbackToSavepoint(handlerCtx, retrySavepoint); err != nil {
			return errors.Join(
				fmt.Errorf("handler failed at position %d (event %s, type %s): %w",
					event.Position, event.ID, event.EventType, lastErr),
				fmt.Errorf("failed to roll back to savepoint: %w", err),
			)
		}
		if s.parking == nil || attempt >= s.maxRetries {
			break
		}

		timer := time.NewTimer(s.backoffDelay(attempt))
		select {
		case <-runCtx.Done():
			timer.Stop()
			// Wrap the context error too so Run recognizes this as
			// shutdown rather than logging a spurious batch failure.
			return errors.Join(runCtx.Err(),
				fmt.Errorf("shutdown while retrying event %s: %w", event.ID, lastErr))
		case <-timer.C:
		}
	}

	if s.parking == nil {
		return fmt.Errorf("handler failed at position %d (event %s, type %s): %w",
			event.Position, event.ID, event.EventType, lastErr)
	}

	parked := ParkedEvent{
		Subscriber: s.name,
		EventID:    event.ID,
		EventType:  event.EventType,
		Position:   event.Position,
		Error:      lastErr.Error(),
		Attempts:   s.maxRetries + 1,
		ParkedAt:   time.Now(),
	}
	if err := s.parking.Park(handlerCtx, parked); err != nil {
		return errors.Join(
			fmt.Errorf("handler failed at position %d (event %s, type %s): %w",
				event.Position, event.ID, event.EventType, lastErr),
			err,
		)
	}
	s.logger.Error("event parked after retries exhausted",
		"subscriber", s.name,
		"event_id", event.ID,
		"event_type", event.EventType,
		"position", event.Position,
		"attempts", parked.Attempts,
		"error", lastErr)
	return nil
}

// backoffDelay doubles per attempt from retryBackoff, capped at maxBackoff.
func (s *Subscriber) backoffDelay(attempt int) time.Duration {
	delay := s.retryBackoff
	for range attempt {
		// Stop before doubling can pass the cap (or overflow into a
		// non-positive duration that would turn backoff into a hot loop).
		if delay > s.maxBackoff/2 {
			return s.maxBackoff
		}
		delay *= 2
	}
	return min(delay, s.maxBackoff)
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

// ListParked returns this subscriber's parked events ordered by position.
// Requires a ParkingLot (WithParkingLot).
func (s *Subscriber) ListParked(ctx context.Context) ([]ParkedEvent, error) {
	if s.parking == nil {
		return nil, fmt.Errorf("subscriber %q has no parking lot configured", s.name)
	}
	return s.parking.List(ctx, s.name)
}

// ReplayParked re-runs the handler for one parked event and clears it on
// success (a failed replay leaves the event parked). The event is loaded
// fresh from the event store; database-backed parking lots run the handler
// and the row deletion in one transaction, exposed via TxFromContext exactly
// like a live batch. Requires a ParkingLot (WithParkingLot).
func (s *Subscriber) ReplayParked(ctx context.Context, eventID string) error {
	if s.parking == nil {
		return fmt.Errorf("subscriber %q has no parking lot configured", s.name)
	}
	event, err := s.events.GetEventByID(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to load parked event %s: %w", eventID, err)
	}
	if err := s.parking.Replay(ctx, s.name, eventID, event, s.handler); err != nil {
		return err
	}
	s.logger.Info("parked event replayed",
		"subscriber", s.name, "event_id", eventID, "event_type", event.EventType)
	return nil
}
