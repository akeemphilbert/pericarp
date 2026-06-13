package subscriptions

import (
	"context"
	"sync"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// InProcessNotifier fans a commit signal out to subscribers in the same
// process — the SQLite / single-process counterpart of Postgres
// LISTEN/NOTIFY. Wire it by wrapping the event store in a
// NotifyingEventStore and passing each subscriber a Subscribe() channel via
// WithWakeSignal.
//
// Signals are best-effort wake-ups, never load-bearing: sends never block
// (a subscriber that is busy keeps at most one pending signal), and a missed
// signal costs at most one poll interval.
type InProcessNotifier struct {
	mu          sync.Mutex
	subscribers []chan struct{}
}

// NewInProcessNotifier creates a notifier with no subscribers.
func NewInProcessNotifier() *InProcessNotifier {
	return &InProcessNotifier{}
}

// Subscribe returns a channel that receives after each Notify. Pass it to a
// Subscriber via WithWakeSignal.
func (n *InProcessNotifier) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	n.subscribers = append(n.subscribers, ch)
	n.mu.Unlock()
	return ch
}

// Notify wakes all subscribers without blocking.
func (n *InProcessNotifier) Notify() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, ch := range n.subscribers {
		select {
		case ch <- struct{}{}:
		default: // a wake is already pending; one is enough
		}
	}
}

// NotifyingEventStore decorates an EventStore so every successful Append
// fires a callback — typically InProcessNotifier.Notify — after the events
// are durably stored. Wrap the store handed to the UnitOfWork (or used
// directly) so background subscribers wake on new commits instead of waiting
// out their poll interval.
//
// Postgres deployments don't need this: the GORM event store NOTIFYs on
// commit by itself (see infrastructure.PostgresNotifyChannel) and
// PostgresListener picks it up across processes.
type NotifyingEventStore struct {
	domain.EventStore
	notify func()
}

// NewNotifyingEventStore wraps store; notify runs after every successful
// Append.
func NewNotifyingEventStore(store domain.EventStore, notify func()) *NotifyingEventStore {
	return &NotifyingEventStore{EventStore: store, notify: notify}
}

// Append delegates to the wrapped store and signals on success.
func (s *NotifyingEventStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	if err := s.EventStore.Append(ctx, aggregateID, expectedVersion, events...); err != nil {
		return err
	}
	if len(events) > 0 && s.notify != nil {
		s.notify()
	}
	return nil
}
