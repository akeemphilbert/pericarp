package domain_test

import (
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Test event structs for testing
type UserCreatedEvent struct {
	AggregateID string    `json:"aggregate_id"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// Implement Event interface
func (e *UserCreatedEvent) GetAggregateID() string {
	return e.AggregateID
}

type OrderPlacedEvent struct {
	OrderID     string    `json:"order_id"`
	CustomerID  string    `json:"customer_id"`
	TotalAmount float64   `json:"total_amount"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// Does not implement Event interface

func TestEventInterface(t *testing.T) {
	t.Parallel()

	t.Run("event implements interface", func(t *testing.T) {
		t.Parallel()
		event := &UserCreatedEvent{
			AggregateID: "user-123",
		}

		var e domain.Event = event
		if e.GetAggregateID() != "user-123" {
			t.Errorf("Expected AggregateID 'user-123', got %q", e.GetAggregateID())
		}
	})

	t.Run("non-event struct does not implement interface", func(t *testing.T) {
		t.Parallel()
		event := &OrderPlacedEvent{
			OrderID: "order-123",
		}

		// This should compile - interface is optional
		_ = event
	})
}

func TestNewEventEnvelope(t *testing.T) {
	t.Parallel()

	t.Run("creates envelope with event implementing interface", func(t *testing.T) {
		t.Parallel()
		event := &UserCreatedEvent{
			AggregateID: "user-123",
			UserID:      "user-123",
			Email:       "john@example.com",
			Name:        "John Doe",
		}

		envelope := domain.NewEventEnvelope(event, "", "user.created")

		if envelope.AggregateID != "user-123" {
			t.Errorf("Expected AggregateID 'user-123', got %q", envelope.AggregateID)
		}
		if envelope.EventType != "user.created" {
			t.Errorf("Expected EventType 'user.created', got %q", envelope.EventType)
		}
		if envelope.ID == "" {
			t.Error("Expected non-empty ID (KSUID)")
		}
		if envelope.Payload.UserID != "user-123" {
			t.Errorf("Expected Payload.UserID 'user-123', got %q", envelope.Payload.UserID)
		}
		if envelope.Created.IsZero() {
			t.Error("Expected non-zero timestamp")
		}
		if envelope.Version != 1 {
			t.Errorf("Expected Version 1, got %d", envelope.Version)
		}
		if envelope.Metadata == nil {
			t.Error("Expected non-nil Metadata map")
		}
	})

	t.Run("creates envelope with event not implementing interface", func(t *testing.T) {
		t.Parallel()
		event := &OrderPlacedEvent{
			OrderID:     "order-123",
			CustomerID:  "customer-456",
			TotalAmount: 99.99,
		}

		envelope := domain.NewEventEnvelope(event, "order-123", "order.placed")

		if envelope.AggregateID != "order-123" {
			t.Errorf("Expected AggregateID 'order-123', got %q", envelope.AggregateID)
		}
		if envelope.Payload.OrderID != "order-123" {
			t.Errorf("Expected Payload.OrderID 'order-123', got %q", envelope.Payload.OrderID)
		}
	})

	t.Run("uses provided aggregateID when event implements interface", func(t *testing.T) {
		t.Parallel()
		event := &UserCreatedEvent{
			AggregateID: "user-123",
		}

		// When event implements interface, AggregateID from event is used
		envelope := domain.NewEventEnvelope(event, "provided-id", "user.created")
		if envelope.AggregateID != "user-123" {
			t.Errorf("Expected AggregateID from event 'user-123', got %q", envelope.AggregateID)
		}
	})
}

func TestNewEventEnvelopeWithVersion(t *testing.T) {
	t.Parallel()

	event := &UserCreatedEvent{
		AggregateID: "user-123",
	}

	envelope := domain.NewEventEnvelopeWithVersion(event, "", "user.created", 5)

	if envelope.Version != 5 {
		t.Errorf("Expected Version 5, got %d", envelope.Version)
	}
	if envelope.AggregateID != "user-123" {
		t.Errorf("Expected AggregateID 'user-123', got %q", envelope.AggregateID)
	}
}

func TestEventEnvelopeKSUID(t *testing.T) {
	t.Parallel()

	t.Run("generates unique KSUIDs", func(t *testing.T) {
		t.Parallel()
		event1 := &UserCreatedEvent{AggregateID: "user-1"}
		event2 := &UserCreatedEvent{AggregateID: "user-2"}

		envelope1 := domain.NewEventEnvelope(event1, "", "user.created")
		envelope2 := domain.NewEventEnvelope(event2, "", "user.created")

		if envelope1.ID == envelope2.ID {
			t.Error("Expected different KSUIDs for different envelopes")
		}

		if len(envelope1.ID) == 0 {
			t.Error("Expected non-empty KSUID")
		}
	})
}

func TestEventEnvelopeTimestamp(t *testing.T) {
	t.Parallel()

	event := &UserCreatedEvent{AggregateID: "user-123"}
	before := time.Now()
	envelope := domain.NewEventEnvelope(event, "", "user.created")
	after := time.Now()

	if envelope.Created.Before(before) || envelope.Created.After(after) {
		t.Errorf("Expected timestamp between %v and %v, got %v", before, after, envelope.Created)
	}
}

func TestEventEnvelopeTypeSafety(t *testing.T) {
	t.Parallel()

	t.Run("strongly typed payload access", func(t *testing.T) {
		t.Parallel()
		event := &UserCreatedEvent{
			AggregateID: "user-123",
			Email:       "john@example.com",
		}

		envelope := domain.NewEventEnvelope(event, "", "user.created")

		// Type-safe access - no assertion needed
		if envelope.Payload.Email != "john@example.com" {
			t.Errorf("Expected Email 'john@example.com', got %q", envelope.Payload.Email)
		}

		// Compiler ensures type safety
		_ = envelope.Payload.UserID
		_ = envelope.Payload.Name
	})

	t.Run("different event types create different envelope types", func(t *testing.T) {
		t.Parallel()
		userEvent := &UserCreatedEvent{AggregateID: "user-123"}
		orderEvent := &OrderPlacedEvent{OrderID: "order-123"}

		userEnvelope := domain.NewEventEnvelope(userEvent, "", "user.created")
		orderEnvelope := domain.NewEventEnvelope(orderEvent, "order-123", "order.placed")

		// Type safety prevents mixing
		_ = userEnvelope.Payload.UserID
		_ = orderEnvelope.Payload.OrderID

		// These would cause compile errors if uncommented:
		// _ = userEnvelope.Payload.OrderID  // compile error
		// _ = orderEnvelope.Payload.UserID  // compile error
	})
}

func TestEventEnvelopeMetadata(t *testing.T) {
	t.Parallel()

	event := &UserCreatedEvent{AggregateID: "user-123"}
	envelope := domain.NewEventEnvelope(event, "", "user.created")

	if envelope.Metadata == nil {
		t.Error("Expected non-nil Metadata map")
	}

	envelope.Metadata["key"] = "value"
	if envelope.Metadata["key"] != "value" {
		t.Errorf("Expected Metadata['key'] 'value', got %v", envelope.Metadata["key"])
	}
}
