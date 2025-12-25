package domain

import (
	"encoding/json"
	"time"

	"github.com/segmentio/ksuid"
)

// Event is an optional interface that events can implement for convenience.
// It provides a way to extract the aggregate ID from an event.
type Event interface {
	GetAggregateID() string
}

// EventEnvelope is a generic struct that wraps event payloads with metadata
// for transport and persistence. The type parameter T represents the strongly-typed
// event payload.
type EventEnvelope[T any] struct {
	ID          string                 `json:"id"`
	AggregateID string                 `json:"aggregate_id"`
	EventType   string                 `json:"event_type"`
	Payload     T                      `json:"payload"`
	Created     time.Time              `json:"timestamp"`
	Version     int                    `json:"version"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// NewEventEnvelope creates a new EventEnvelope with the given payload and metadata.
// If the payload implements the Event interface, the AggregateID is extracted from it.
// Otherwise, the provided aggregateID parameter is used.
func NewEventEnvelope[T any](payload T, aggregateID, eventType string) *EventEnvelope[T] {
	// Extract AggregateID from payload if it implements Event interface
	if event, ok := any(payload).(Event); ok {
		aggregateID = event.GetAggregateID()
	}

	id := ksuid.New().String()
	return &EventEnvelope[T]{
		ID:          id,
		AggregateID: aggregateID,
		EventType:   eventType,
		Payload:     payload,
		Created:     time.Now(),
		Version:     1,
		Metadata:    make(map[string]interface{}),
	}
}

// NewEventEnvelopeWithVersion creates a new EventEnvelope with a specific version.
func NewEventEnvelopeWithVersion[T any](payload T, aggregateID, eventType string, version int) *EventEnvelope[T] {
	envelope := NewEventEnvelope(payload, aggregateID, eventType)
	envelope.Version = version
	return envelope
}

// MarshalJSON implements json.Marshaler for EventEnvelope.
// This custom implementation ensures the generic type is properly serialized.
func (e *EventEnvelope[T]) MarshalJSON() ([]byte, error) {
	type Alias EventEnvelope[T]
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	})
}

// UnmarshalJSON implements json.Unmarshaler for EventEnvelope.
// This custom implementation ensures the generic type is properly deserialized.
func (e *EventEnvelope[T]) UnmarshalJSON(data []byte) error {
	type Alias EventEnvelope[T]
	aux := &struct {
		*Alias
	}{}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	*e = EventEnvelope[T](*aux.Alias)
	return nil
}
