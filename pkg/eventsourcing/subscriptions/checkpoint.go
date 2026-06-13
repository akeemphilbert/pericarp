// Package subscriptions provides an opt-in runtime for running event handlers
// as crash-safe background subscribers. A Subscriber treats the event store's
// global ordered feed (EventStore.ReadAfter) as a durable queue: it remembers
// exactly one number — its checkpoint — so crash recovery and normal startup
// are the same code path.
//
// Synchronous in-commit dispatch via the UnitOfWork's EventDispatcher is
// unaffected; this package is for consumers that need resumable, transactional
// background processing.
package subscriptions

import (
	"context"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Handler processes a single event from the feed. EventDispatcher.Dispatch
// satisfies this signature, so a dispatcher with registered pattern handlers
// can be wired directly as a Subscriber's handler.
//
// When the subscriber's CheckpointStore is database-backed, the context
// passed to the handler carries the batch's transaction (see TxFromContext):
// writes made through it commit atomically with the checkpoint advance,
// giving exactly-once processing for same-database handlers. Handlers with
// side effects outside that transaction must tolerate at-least-once delivery.
type Handler func(ctx context.Context, event domain.EventEnvelope[any]) error

// CheckpointStore persists the position each named subscriber has processed
// up to. Implementations coordinate concurrent access: at most one Batch per
// subscriber name is active at a time.
type CheckpointStore interface {
	// Acquire begins a processing cycle for the named subscriber, creating
	// the checkpoint at position 0 if it does not exist. It returns
	// acquired=false (and no Batch) when another process currently holds the
	// subscriber's checkpoint; the caller should skip the cycle.
	Acquire(ctx context.Context, subscriber string) (batch Batch, acquired bool, err error)

	// Position returns the subscriber's committed checkpoint (0 if the
	// subscriber is unknown).
	Position(ctx context.Context, subscriber string) (int64, error)

	// Reset sets the subscriber's committed checkpoint, creating it if
	// needed. Resetting to 0 makes the subscriber replay all history,
	// incrementally and resumably, on its next cycles. Implementations
	// serialize Reset against in-flight batches: either Reset waits for the
	// batch (or fails), or the batch's Commit detects the moved checkpoint
	// and aborts — a reset is never silently overwritten.
	Reset(ctx context.Context, subscriber string, position int64) error
}

// Batch is one acquired processing cycle for a subscriber. Exactly one of
// Commit or Rollback must be called.
type Batch interface {
	// Position is the committed checkpoint at acquisition time; the cycle
	// processes events after it.
	Position() int64

	// HandlerContext derives the context handlers run with. Database-backed
	// implementations attach the batch transaction so handlers can join it
	// via TxFromContext.
	HandlerContext(ctx context.Context) context.Context

	// Commit advances the checkpoint to position and commits the batch
	// (including any handler writes made through the batch transaction).
	Commit(ctx context.Context, position int64) error

	// Rollback abandons the cycle: the checkpoint stays at Position() and
	// any handler writes made through the batch transaction are discarded.
	Rollback() error
}
