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
// development setups. Parking is not transactional with checkpoint advances.
type MemoryParkingLot struct {
	mu     sync.Mutex
	parked map[string]map[string]ParkedEvent // subscriber -> eventID -> entry
}

var _ ParkingLot = (*MemoryParkingLot)(nil)

// NewMemoryParkingLot creates an empty in-memory parking lot.
func NewMemoryParkingLot() *MemoryParkingLot {
	return &MemoryParkingLot{parked: make(map[string]map[string]ParkedEvent)}
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

// Replay re-runs the handler and clears the parked entry on success.
func (m *MemoryParkingLot) Replay(ctx context.Context, subscriber, eventID string, event domain.EventEnvelope[any], handler Handler) error {
	m.mu.Lock()
	_, exists := m.parked[subscriber][eventID]
	m.mu.Unlock()
	if !exists {
		return fmt.Errorf("%w: event %s for subscriber %q", ErrEventNotParked, eventID, subscriber)
	}

	if err := handler(ctx, event); err != nil {
		return fmt.Errorf("handler failed during replay of event %s: %w", eventID, err)
	}

	m.mu.Lock()
	delete(m.parked[subscriber], eventID)
	m.mu.Unlock()
	return nil
}
