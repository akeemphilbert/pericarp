package subscriptions

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// MemoryParkingLot is an in-memory ParkingLot for tests and single-process
// development setups. Parking is not transactional with checkpoint advances,
// and parked records do not survive a restart — pairing it with a durable
// checkpoint store means a poison event whose park record is lost on restart
// has already been skipped by the durable checkpoint, permanently.
type MemoryParkingLot struct {
	mu        sync.Mutex
	parked    map[string]map[string]ParkedEvent // subscriber -> eventID -> entry
	replaying map[string]bool                   // subscriber+eventID -> replay in flight
}

var _ ParkingLot = (*MemoryParkingLot)(nil)

// NewMemoryParkingLot creates an empty in-memory parking lot.
func NewMemoryParkingLot() *MemoryParkingLot {
	return &MemoryParkingLot{
		parked:    make(map[string]map[string]ParkedEvent),
		replaying: make(map[string]bool),
	}
}

func replayKey(subscriber, eventID string) string {
	return subscriber + "\x00" + eventID
}

// Park records a poison event.
func (m *MemoryParkingLot) Park(ctx context.Context, parked ParkedEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.parked[parked.Subscriber] == nil {
		m.parked[parked.Subscriber] = make(map[string]ParkedEvent)
	}
	m.parked[parked.Subscriber][parked.EventID] = parked
	return nil
}

// List returns the subscriber's parked events ordered by position.
func (m *MemoryParkingLot) List(ctx context.Context, subscriber string) ([]ParkedEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]ParkedEvent, 0, len(m.parked[subscriber]))
	for _, parked := range m.parked[subscriber] {
		result = append(result, parked)
	}
	slices.SortFunc(result, func(a, b ParkedEvent) int {
		return cmp.Compare(a.Position, b.Position)
	})
	return result, nil
}

// Replay re-runs the handler and clears the parked entry on success. The
// handler runs at most once per entry: concurrent replays of the same event
// conflict instead of double-executing, and an entry re-parked while the
// replay was running (a fresh failure) is preserved rather than cleared.
func (m *MemoryParkingLot) Replay(ctx context.Context, subscriber, eventID string, event domain.EventEnvelope[any], handler Handler) error {
	key := replayKey(subscriber, eventID)

	m.mu.Lock()
	observed, exists := m.parked[subscriber][eventID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("%w: event %s for subscriber %q", ErrEventNotParked, eventID, subscriber)
	}
	if m.replaying[key] {
		m.mu.Unlock()
		return fmt.Errorf("replay of event %s for subscriber %q is already in flight", eventID, subscriber)
	}
	m.replaying[key] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.replaying, key)
		m.mu.Unlock()
	}()

	if err := handler(ctx, event); err != nil {
		return fmt.Errorf("handler failed during replay of event %s: %w", eventID, err)
	}

	m.mu.Lock()
	// Clear only the entry we replayed; an entry rewritten mid-replay
	// records a new failure and must survive.
	if current, ok := m.parked[subscriber][eventID]; ok && current == observed {
		delete(m.parked[subscriber], eventID)
	}
	m.mu.Unlock()
	return nil
}
