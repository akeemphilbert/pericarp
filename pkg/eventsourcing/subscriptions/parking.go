package subscriptions

import (
	"context"
	"errors"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// ErrEventNotParked is returned by Replay when the event is not parked for
// the subscriber.
var ErrEventNotParked = errors.New("event is not parked")

// ParkedEvent records an event a subscriber could not process after
// exhausting its retries.
type ParkedEvent struct {
	Subscriber string    `json:"subscriber"`
	EventID    string    `json:"event_id"`
	EventType  string    `json:"event_type"`
	Position   int64     `json:"position"`
	Error      string    `json:"error"`
	Attempts   int       `json:"attempts"`
	ParkedAt   time.Time `json:"parked_at"`
}

// ParkingLot stores poison events so one unprocessable event never blocks the
// events behind it: the subscriber parks it and advances the checkpoint past
// it. CLI or HTTP surfaces over this API are the consumer's job.
type ParkingLot interface {
	// Park records a poison event. When ctx carries a batch transaction
	// (TxFromContext), the row is written through it so the parking and the
	// checkpoint advance past the event commit atomically.
	Park(ctx context.Context, parked ParkedEvent) error

	// List returns the subscriber's parked events ordered by position.
	List(ctx context.Context, subscriber string) ([]ParkedEvent, error)

	// Replay re-runs handler for a parked event and clears the row on
	// success. Database-backed implementations run both in one transaction
	// (with the transaction exposed to the handler via TxFromContext), so a
	// failed replay leaves the row — and any handler writes — untouched.
	// Returns ErrEventNotParked when the event is not parked.
	Replay(ctx context.Context, subscriber, eventID string, event domain.EventEnvelope[any], handler Handler) error
}
