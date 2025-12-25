package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

func TestWrapEvent(t *testing.T) {
	t.Parallel()

	t.Run("wraps event implementing interface", func(t *testing.T) {
		t.Parallel()
		event := &UserCreatedEvent{
			AggregateID: "user-123",
			UserID:      "user-123",
			Email:       "john@example.com",
		}

		envelope, err := domain.WrapEvent(event, "", "user.created")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if envelope.AggregateID != "user-123" {
			t.Errorf("Expected AggregateID 'user-123', got %q", envelope.AggregateID)
		}
		if envelope.Payload.Email != "john@example.com" {
			t.Errorf("Expected Payload.Email 'john@example.com', got %q", envelope.Payload.Email)
		}
	})

	t.Run("wraps event not implementing interface", func(t *testing.T) {
		t.Parallel()
		event := &OrderPlacedEvent{
			OrderID:    "order-123",
			CustomerID: "customer-456",
		}

		envelope, err := domain.WrapEvent(event, "order-123", "order.placed")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if envelope.AggregateID != "order-123" {
			t.Errorf("Expected AggregateID 'order-123', got %q", envelope.AggregateID)
		}
		if envelope.Payload.OrderID != "order-123" {
			t.Errorf("Expected Payload.OrderID 'order-123', got %q", envelope.Payload.OrderID)
		}
	})
}

func TestMarshalEventToJSON(t *testing.T) {
	t.Parallel()

	t.Run("marshals envelope to JSON", func(t *testing.T) {
		t.Parallel()
		event := &UserCreatedEvent{
			AggregateID: "user-123",
			UserID:      "user-123",
			Email:       "john@example.com",
			Name:        "John Doe",
		}

		envelope := domain.NewEventEnvelope(event, "", "user.created")
		data, err := domain.MarshalEventToJSON(envelope)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(data) == 0 {
			t.Error("Expected non-empty JSON data")
		}

		// Verify JSON structure
		var decoded map[string]interface{}
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		if decoded["aggregate_id"] != "user-123" {
			t.Errorf("Expected aggregate_id 'user-123', got %v", decoded["aggregate_id"])
		}
		if decoded["event_type"] != "user.created" {
			t.Errorf("Expected event_type 'user.created', got %v", decoded["event_type"])
		}
	})

	t.Run("marshals envelope with metadata", func(t *testing.T) {
		t.Parallel()
		event := &UserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created")
		envelope.Metadata["source"] = "api"
		envelope.Metadata["user_agent"] = "test-agent"

		data, err := domain.MarshalEventToJSON(envelope)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		var decoded map[string]interface{}
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		metadata, ok := decoded["metadata"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected metadata to be a map")
		}

		if metadata["source"] != "api" {
			t.Errorf("Expected metadata['source'] 'api', got %v", metadata["source"])
		}
	})
}

func TestUnmarshalEventFromJSON(t *testing.T) {
	t.Parallel()

	t.Run("unmarshals envelope from JSON", func(t *testing.T) {
		t.Parallel()
		originalEvent := &UserCreatedEvent{
			AggregateID: "user-123",
			UserID:      "user-123",
			Email:       "john@example.com",
			Name:        "John Doe",
		}

		originalEnvelope := domain.NewEventEnvelope(originalEvent, "", "user.created")
		data, err := domain.MarshalEventToJSON(originalEnvelope)
		if err != nil {
			t.Fatalf("Unexpected error marshaling: %v", err)
		}

		unmarshaledEnvelope, err := domain.UnmarshalEventFromJSON[*UserCreatedEvent](data)
		if err != nil {
			t.Fatalf("Unexpected error unmarshaling: %v", err)
		}

		if unmarshaledEnvelope.AggregateID != "user-123" {
			t.Errorf("Expected AggregateID 'user-123', got %q", unmarshaledEnvelope.AggregateID)
		}
		if unmarshaledEnvelope.EventType != "user.created" {
			t.Errorf("Expected EventType 'user.created', got %q", unmarshaledEnvelope.EventType)
		}
		if unmarshaledEnvelope.Payload.Email != "john@example.com" {
			t.Errorf("Expected Payload.Email 'john@example.com', got %q", unmarshaledEnvelope.Payload.Email)
		}
		if unmarshaledEnvelope.Payload.Name != "John Doe" {
			t.Errorf("Expected Payload.Name 'John Doe', got %q", unmarshaledEnvelope.Payload.Name)
		}
	})

	t.Run("handles invalid JSON", func(t *testing.T) {
		t.Parallel()
		invalidJSON := []byte(`{invalid json}`)

		_, err := UnmarshalEventFromJSON[*UserCreatedEvent](invalidJSON)
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})

	t.Run("handles missing fields gracefully", func(t *testing.T) {
		t.Parallel()
		// JSON with missing optional fields
		jsonData := []byte(`{
			"id": "test-id",
			"aggregate_id": "user-123",
			"event_type": "user.created",
			"payload": {
				"aggregate_id": "user-123",
				"user_id": "user-123",
				"email": "john@example.com"
			},
			"timestamp": "2023-01-01T00:00:00Z",
			"version": 1
		}`)

		envelope, err := UnmarshalEventFromJSON[*UserCreatedEvent](jsonData)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if envelope.AggregateID != "user-123" {
			t.Errorf("Expected AggregateID 'user-123', got %q", envelope.AggregateID)
		}
		// Metadata should be nil or empty if not present
		if envelope.Metadata != nil && len(envelope.Metadata) > 0 {
			t.Error("Expected nil or empty metadata when not present in JSON")
		}
	})
}

func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("round trip with UserCreatedEvent", func(t *testing.T) {
		t.Parallel()
		originalEvent := &UserCreatedEvent{
			AggregateID: "user-123",
			UserID:      "user-123",
			Email:       "john@example.com",
			Name:        "John Doe",
			OccurredAt:  time.Now(),
		}

		originalEnvelope := NewEventEnvelope(originalEvent, "", "user.created")
		originalEnvelope.Metadata["source"] = "test"

		// Marshal
		data, err := MarshalEventToJSON(originalEnvelope)
		if err != nil {
			t.Fatalf("Unexpected error marshaling: %v", err)
		}

		// Unmarshal
		unmarshaledEnvelope, err := UnmarshalEventFromJSON[*UserCreatedEvent](data)
		if err != nil {
			t.Fatalf("Unexpected error unmarshaling: %v", err)
		}

		// Compare
		if originalEnvelope.AggregateID != unmarshaledEnvelope.AggregateID {
			t.Errorf("AggregateID mismatch: %q != %q", originalEnvelope.AggregateID, unmarshaledEnvelope.AggregateID)
		}
		if originalEnvelope.EventType != unmarshaledEnvelope.EventType {
			t.Errorf("EventType mismatch: %q != %q", originalEnvelope.EventType, unmarshaledEnvelope.EventType)
		}
		if originalEnvelope.Payload.Email != unmarshaledEnvelope.Payload.Email {
			t.Errorf("Payload.Email mismatch: %q != %q", originalEnvelope.Payload.Email, unmarshaledEnvelope.Payload.Email)
		}
		if originalEnvelope.Payload.Name != unmarshaledEnvelope.Payload.Name {
			t.Errorf("Payload.Name mismatch: %q != %q", originalEnvelope.Payload.Name, unmarshaledEnvelope.Payload.Name)
		}
		if unmarshaledEnvelope.Metadata["source"] != "test" {
			t.Errorf("Expected metadata['source'] 'test', got %v", unmarshaledEnvelope.Metadata["source"])
		}
	})

	t.Run("round trip with OrderPlacedEvent", func(t *testing.T) {
		t.Parallel()
		originalEvent := &OrderPlacedEvent{
			OrderID:     "order-123",
			CustomerID:  "customer-456",
			TotalAmount: 99.99,
		}

		originalEnvelope := NewEventEnvelope(originalEvent, "order-123", "order.placed")

		data, err := MarshalEventToJSON(originalEnvelope)
		if err != nil {
			t.Fatalf("Unexpected error marshaling: %v", err)
		}

		unmarshaledEnvelope, err := UnmarshalEventFromJSON[*OrderPlacedEvent](data)
		if err != nil {
			t.Fatalf("Unexpected error unmarshaling: %v", err)
		}

		if originalEnvelope.Payload.OrderID != unmarshaledEnvelope.Payload.OrderID {
			t.Errorf("Payload.OrderID mismatch: %q != %q", originalEnvelope.Payload.OrderID, unmarshaledEnvelope.Payload.OrderID)
		}
		if originalEnvelope.Payload.TotalAmount != unmarshaledEnvelope.Payload.TotalAmount {
			t.Errorf("Payload.TotalAmount mismatch: %f != %f", originalEnvelope.Payload.TotalAmount, unmarshaledEnvelope.Payload.TotalAmount)
		}
	})
}

func TestMetadataPreservation(t *testing.T) {
	t.Parallel()

	event := &UserCreatedEvent{AggregateID: "user-123"}
	envelope := NewEventEnvelope(event, "", "user.created")
	envelope.Metadata["key1"] = "value1"
	envelope.Metadata["key2"] = 42
	envelope.Metadata["key3"] = true

	data, err := MarshalEventToJSON(envelope)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	unmarshaledEnvelope, err := UnmarshalEventFromJSON[*UserCreatedEvent](data)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if unmarshaledEnvelope.Metadata["key1"] != "value1" {
		t.Errorf("Expected metadata['key1'] 'value1', got %v", unmarshaledEnvelope.Metadata["key1"])
	}
	if unmarshaledEnvelope.Metadata["key2"] != float64(42) {
		t.Errorf("Expected metadata['key2'] 42, got %v", unmarshaledEnvelope.Metadata["key2"])
	}
	if unmarshaledEnvelope.Metadata["key3"] != true {
		t.Errorf("Expected metadata['key3'] true, got %v", unmarshaledEnvelope.Metadata["key3"])
	}
}

func TestTypeSafetyAfterUnmarshaling(t *testing.T) {
	t.Parallel()

	event := &UserCreatedEvent{
		AggregateID: "user-123",
		Email:       "john@example.com",
	}

	envelope := NewEventEnvelope(event, "", "user.created")
	data, err := MarshalEventToJSON(envelope)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	unmarshaledEnvelope, err := UnmarshalEventFromJSON[*UserCreatedEvent](data)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Type-safe access - no assertion needed
	if unmarshaledEnvelope.Payload.Email != "john@example.com" {
		t.Errorf("Expected Payload.Email 'john@example.com', got %q", unmarshaledEnvelope.Payload.Email)
	}

	// Compiler ensures type safety
	_ = unmarshaledEnvelope.Payload.UserID
	_ = unmarshaledEnvelope.Payload.Name
}
