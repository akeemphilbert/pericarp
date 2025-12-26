package domain

import (
	"encoding/json"
)

// WrapEvent wraps a typed payload in a generic EventEnvelope.
// If the payload implements the Event interface, the AggregateID is extracted from it.
// Otherwise, the provided aggregateID parameter is used.
func WrapEvent[T any](payload T, aggregateID, eventType string) (EventEnvelope[T], error) {
	return NewEventEnvelope(payload, aggregateID, eventType), nil
}

// MarshalEventToJSON marshals a generic EventEnvelope to JSON.
func MarshalEventToJSON[T any](envelope EventEnvelope[T]) ([]byte, error) {
	return json.Marshal(envelope)
}

// UnmarshalEventFromJSON unmarshals an EventEnvelope from JSON.
// The type parameter T must match the payload type in the JSON data.
func UnmarshalEventFromJSON[T any](data []byte) (EventEnvelope[T], error) {
	var envelope EventEnvelope[T]
	if err := json.Unmarshal(data, &envelope); err != nil {
		return EventEnvelope[T]{}, err
	}
	return envelope, nil
}
