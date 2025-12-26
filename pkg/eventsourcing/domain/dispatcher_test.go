package domain_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Test event types for testing (using different names to avoid conflicts with event_test.go)
type DispatcherTestUserCreatedEvent struct {
	AggregateID string    `json:"aggregate_id"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	OccurredAt  time.Time `json:"occurred_at"`
}

func (e *DispatcherTestUserCreatedEvent) GetAggregateID() string {
	return e.AggregateID
}

type DispatcherTestOrderPlacedEvent struct {
	OrderID     string    `json:"order_id"`
	CustomerID  string    `json:"customer_id"`
	TotalAmount float64   `json:"total_amount"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type DispatcherTestAccountCreatedEvent struct {
	AccountID string `json:"account_id"`
	Email     string `json:"email"`
}

func TestNewEventDispatcher(t *testing.T) {
	t.Parallel()

	t.Run("creates new dispatcher", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()
		if d == nil {
			t.Fatal("Expected non-nil EventDispatcher")
		}
	})
}

func TestSubscribe(t *testing.T) {
	t.Parallel()

	t.Run("valid handler registration", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()
		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			return nil
		}

		err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("empty event type returns error", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()
		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			return nil
		}

		err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "", handler)
		if err == nil {
			t.Error("Expected error for empty event type")
		}
		if err.Error() != "event type cannot be empty" {
			t.Errorf("Expected 'event type cannot be empty', got %q", err.Error())
		}
	})

	t.Run("nil handler returns error", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", nil)
		if err == nil {
			t.Error("Expected error for nil handler")
		}
		if err.Error() != "handler cannot be nil" {
			t.Errorf("Expected 'handler cannot be nil', got %q", err.Error())
		}
	})

	t.Run("multiple handlers for same event type", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		callCount1 := 0
		handler1 := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			callCount1++
			return nil
		}

		callCount2 := 0
		handler2 := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			callCount2++
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler1); err != nil {
			t.Fatalf("Failed to register handler1: %v", err)
		}
		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler2); err != nil {
			t.Fatalf("Failed to register handler2: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{
			AggregateID: "user-123",
			UserID:      "user-123",
			Email:       "test@example.com",
		}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		if err := d.Dispatch(ctx, anyEnvelope); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if callCount1 != 1 {
			t.Errorf("Expected handler1 to be called once, got %d", callCount1)
		}
		if callCount2 != 1 {
			t.Errorf("Expected handler2 to be called once, got %d", callCount2)
		}
	})
}

func TestDispatch(t *testing.T) {
	t.Parallel()

	t.Run("type-safe handler dispatch", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		var receivedEvent *DispatcherTestUserCreatedEvent
		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			receivedEvent = &env.Payload
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{
			AggregateID: "user-123",
			UserID:      "user-123",
			Email:       "john@example.com",
			Name:        "John Doe",
		}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		if err := d.Dispatch(ctx, anyEnvelope); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if receivedEvent == nil {
			t.Fatal("Handler was not called")
		}
		if receivedEvent.Email != "john@example.com" {
			t.Errorf("Expected Email 'john@example.com', got %q", receivedEvent.Email)
		}
		if receivedEvent.Name != "John Doe" {
			t.Errorf("Expected Name 'John Doe', got %q", receivedEvent.Name)
		}
	})

	t.Run("type assertion error handling", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		// Dispatch wrong event type
		wrongEvent := &DispatcherTestOrderPlacedEvent{
			OrderID:     "order-123",
			CustomerID:  "customer-456",
			TotalAmount: 99.99,
		}
		wrongEnvelope := domain.NewEventEnvelope(wrongEvent, "order-123", "user.created", 1)

		// Convert to EventEnvelope[any]
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          wrongEnvelope.ID,
			AggregateID: wrongEnvelope.AggregateID,
			EventType:   wrongEnvelope.EventType,
			Payload:     wrongEnvelope.Payload,
			Created:     wrongEnvelope.Created,
			SequenceNo:  wrongEnvelope.SequenceNo,
			Metadata:    wrongEnvelope.Metadata,
		}

		ctx := context.Background()
		err := d.Dispatch(ctx, anyEnvelope)
		if err == nil {
			t.Error("Expected error for type mismatch")
		}
		if err.Error() == "" {
			t.Error("Expected non-empty error message")
		}
	})

	t.Run("handler error collection", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		handler1 := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			return errors.New("handler1 error")
		}

		handler2 := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			return errors.New("handler2 error")
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler1); err != nil {
			t.Fatalf("Failed to subscribe handler1: %v", err)
		}
		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler2); err != nil {
			t.Fatalf("Failed to subscribe handler2: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		err := d.Dispatch(ctx, anyEnvelope)
		if err == nil {
			t.Error("Expected error from handlers")
		}
		// Both errors should be collected
		if err.Error() == "" {
			t.Error("Expected non-empty error message")
		}
	})

	t.Run("no handlers for event type", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)

		// Convert to EventEnvelope[any] for Dispatch
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		err := d.Dispatch(ctx, anyEnvelope)
		if err != nil {
			t.Errorf("Expected no error when no handlers registered, got %v", err)
		}
	})

	t.Run("context propagation", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		var receivedCtx context.Context
		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			receivedCtx = ctx
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)

		// Convert to EventEnvelope[any] for Dispatch
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.WithValue(context.Background(), "test-key", "test-value")
		if err := d.Dispatch(ctx, anyEnvelope); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if receivedCtx == nil {
			t.Fatal("Context was not propagated")
		}
		if receivedCtx.Value("test-key") != "test-value" {
			t.Error("Context value was not propagated correctly")
		}
	})
}

func TestSubscribeWildcard(t *testing.T) {
	t.Parallel()

	t.Run("wildcard handler called for all events", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		callCount := 0
		wildcardHandler := func(ctx context.Context, env domain.EventEnvelope[any]) error {
			callCount++
			return nil
		}

		if err := d.SubscribeWildcard(wildcardHandler); err != nil {
			t.Fatalf("Failed to subscribe wildcard: %v", err)
		}

		event1 := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope1 := domain.NewEventEnvelope(event1, "", "user.created", 1)
		anyEnvelope1 := domain.EventEnvelope[any]{
			ID:          envelope1.ID,
			AggregateID: envelope1.AggregateID,
			EventType:   envelope1.EventType,
			Payload:     envelope1.Payload,
			Created:     envelope1.Created,
			SequenceNo:  envelope1.SequenceNo,
			Metadata:    envelope1.Metadata,
		}

		event2 := &DispatcherTestOrderPlacedEvent{OrderID: "order-123"}
		envelope2 := domain.NewEventEnvelope(event2, "order-123", "order.placed", 1)
		anyEnvelope2 := domain.EventEnvelope[any]{
			ID:          envelope2.ID,
			AggregateID: envelope2.AggregateID,
			EventType:   envelope2.EventType,
			Payload:     envelope2.Payload,
			Created:     envelope2.Created,
			SequenceNo:  envelope2.SequenceNo,
			Metadata:    envelope2.Metadata,
		}

		ctx := context.Background()
		if err := d.Dispatch(ctx, anyEnvelope1); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}
		if err := d.Dispatch(ctx, anyEnvelope2); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if callCount != 2 {
			t.Errorf("Expected wildcard handler to be called 2 times, got %d", callCount)
		}
	})

	t.Run("wildcard handler called after event-specific handlers", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		callOrder := make([]string, 0)
		specificHandler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			callOrder = append(callOrder, "specific")
			return nil
		}

		wildcardHandler := func(ctx context.Context, env domain.EventEnvelope[any]) error {
			callOrder = append(callOrder, "wildcard")
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", specificHandler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}
		if err := d.SubscribeWildcard(wildcardHandler); err != nil {
			t.Fatalf("Failed to subscribe wildcard: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		if err := d.Dispatch(ctx, anyEnvelope); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if len(callOrder) != 2 {
			t.Fatalf("Expected 2 handler calls, got %d", len(callOrder))
		}
		if callOrder[0] != "specific" {
			t.Errorf("Expected specific handler first, got %q", callOrder[0])
		}
		if callOrder[1] != "wildcard" {
			t.Errorf("Expected wildcard handler second, got %q", callOrder[1])
		}
	})

	t.Run("nil wildcard handler returns error", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		err := d.SubscribeWildcard(nil)
		if err == nil {
			t.Error("Expected error for nil handler")
		}
		if err.Error() != "handler cannot be nil" {
			t.Errorf("Expected 'handler cannot be nil', got %q", err.Error())
		}
	})
}

func TestConcurrentDispatch(t *testing.T) {
	t.Parallel()

	t.Run("thread-safe concurrent dispatch", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		var mu sync.Mutex
		callCount := 0
		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			mu.Lock()
			callCount++
			mu.Unlock()
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		var wg sync.WaitGroup
		concurrency := 10
		wg.Add(concurrency)

		ctx := context.Background()
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				if err := d.Dispatch(ctx, anyEnvelope); err != nil {
					t.Errorf("Dispatch failed: %v", err)
				}
			}()
		}

		wg.Wait()

		mu.Lock()
		defer mu.Unlock()
		if callCount != concurrency {
			t.Errorf("Expected %d handler calls, got %d", concurrency, callCount)
		}
	})

	t.Run("thread-safe concurrent registration and dispatch", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		var wg sync.WaitGroup
		concurrency := 5

		// Concurrent registration
		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func(id int) {
				defer wg.Done()
				handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
					return nil
				}
				if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler); err != nil {
					t.Errorf("Failed to subscribe: %v", err)
				}
			}(i)
		}

		// Concurrent dispatch
		wg.Add(concurrency)
		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}
		ctx := context.Background()

		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				if err := d.Dispatch(ctx, anyEnvelope); err != nil {
					t.Errorf("Dispatch failed: %v", err)
				}
			}()
		}

		wg.Wait()
	})
}

func TestRegisterType(t *testing.T) {
	t.Parallel()

	t.Run("register type factory", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		factory := func() DispatcherTestUserCreatedEvent {
			return DispatcherTestUserCreatedEvent{}
		}

		err := domain.RegisterType[DispatcherTestUserCreatedEvent](d, "user.created", factory)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("empty event type returns error", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		factory := func() DispatcherTestUserCreatedEvent {
			return DispatcherTestUserCreatedEvent{}
		}

		err := domain.RegisterType[DispatcherTestUserCreatedEvent](d, "", factory)
		if err == nil {
			t.Error("Expected error for empty event type")
		}
		if err.Error() != "event type cannot be empty" {
			t.Errorf("Expected 'event type cannot be empty', got %q", err.Error())
		}
	})

	t.Run("nil factory returns error", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		err := domain.RegisterType[DispatcherTestUserCreatedEvent](d, "user.created", nil)
		if err == nil {
			t.Error("Expected error for nil factory")
		}
		if err.Error() != "factory cannot be nil" {
			t.Errorf("Expected 'factory cannot be nil', got %q", err.Error())
		}
	})
}

func TestUnmarshalEvent(t *testing.T) {
	t.Parallel()

	t.Run("unmarshal event with registered type", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		factory := func() DispatcherTestUserCreatedEvent {
			return DispatcherTestUserCreatedEvent{}
		}

		if err := domain.RegisterType[DispatcherTestUserCreatedEvent](d, "user.created", factory); err != nil {
			t.Fatalf("Failed to register type: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{
			AggregateID: "user-123",
			UserID:      "user-123",
			Email:       "test@example.com",
			Name:        "Test User",
		}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)

		data, err := json.Marshal(envelope)
		if err != nil {
			t.Fatalf("Failed to marshal envelope: %v", err)
		}

		ctx := context.Background()
		unmarshaled, err := d.UnmarshalEvent(ctx, data, "user.created")
		if err != nil {
			t.Fatalf("Failed to unmarshal event: %v", err)
		}

		if unmarshaled.EventType != "user.created" {
			t.Errorf("Expected EventType 'user.created', got %q", unmarshaled.EventType)
		}
		if unmarshaled.AggregateID != "user-123" {
			t.Errorf("Expected AggregateID 'user-123', got %q", unmarshaled.AggregateID)
		}

		// Type assert payload
		payload, ok := unmarshaled.Payload.(*DispatcherTestUserCreatedEvent)
		if !ok {
			t.Fatalf("Expected *DispatcherTestUserCreatedEvent, got %T", unmarshaled.Payload)
		}
		if payload.Email != "test@example.com" {
			t.Errorf("Expected Email 'test@example.com', got %q", payload.Email)
		}
	})

	t.Run("unmarshal unregistered type returns error", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)

		data, err := json.Marshal(envelope)
		if err != nil {
			t.Fatalf("Failed to marshal envelope: %v", err)
		}

		ctx := context.Background()
		_, err = d.UnmarshalEvent(ctx, data, "user.created")
		if err == nil {
			t.Error("Expected error for unregistered type")
		}
		if err.Error() != "type not registered for event type \"user.created\"" {
			t.Errorf("Expected 'type not registered' error, got %q", err.Error())
		}
	})

	t.Run("unmarshal invalid JSON returns error", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		factory := func() DispatcherTestUserCreatedEvent {
			return DispatcherTestUserCreatedEvent{}
		}

		if err := domain.RegisterType[DispatcherTestUserCreatedEvent](d, "user.created", factory); err != nil {
			t.Fatalf("Failed to register type: %v", err)
		}

		invalidJSON := []byte("{invalid json}")
		ctx := context.Background()
		_, err := d.UnmarshalEvent(ctx, invalidJSON, "user.created")
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})
}

func TestTypeRegistryFromSubscribe(t *testing.T) {
	t.Parallel()

	t.Run("subscribe automatically registers type", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		// Type should now be registered and unmarshal should work
		event := &DispatcherTestUserCreatedEvent{
			AggregateID: "user-123",
			UserID:      "user-123",
			Email:       "test@example.com",
		}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)

		data, err := json.Marshal(envelope)
		if err != nil {
			t.Fatalf("Failed to marshal envelope: %v", err)
		}

		ctx := context.Background()
		unmarshaled, err := d.UnmarshalEvent(ctx, data, "user.created")
		if err != nil {
			t.Fatalf("Failed to unmarshal event: %v", err)
		}

		if unmarshaled.EventType != "user.created" {
			t.Errorf("Expected EventType 'user.created', got %q", unmarshaled.EventType)
		}
	})
}

func TestPatternMatching(t *testing.T) {
	t.Parallel()

	t.Run("exact match handler is called", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		callCount := 0
		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			callCount++
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", handler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		if err := d.Dispatch(ctx, anyEnvelope); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if callCount != 1 {
			t.Errorf("Expected handler to be called once, got %d", callCount)
		}
	})

	t.Run("entity.* pattern matches", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		callCount := 0
		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			callCount++
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.*", handler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		if err := d.Dispatch(ctx, anyEnvelope); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if callCount != 1 {
			t.Errorf("Expected handler to be called once, got %d", callCount)
		}
	})

	t.Run("*.action pattern matches", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		callCount := 0
		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			callCount++
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "*.created", handler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		if err := d.Dispatch(ctx, anyEnvelope); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if callCount != 1 {
			t.Errorf("Expected handler to be called once, got %d", callCount)
		}
	})

	t.Run("*.* pattern matches", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		callCount := 0
		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			callCount++
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "*.*", handler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		if err := d.Dispatch(ctx, anyEnvelope); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if callCount != 1 {
			t.Errorf("Expected handler to be called once, got %d", callCount)
		}
	})

	t.Run("multiple patterns match same event", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		exactCount := 0
		entityWildcardCount := 0
		actionWildcardCount := 0
		allWildcardCount := 0

		exactHandler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			exactCount++
			return nil
		}
		entityWildcardHandler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			entityWildcardCount++
			return nil
		}
		actionWildcardHandler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			actionWildcardCount++
			return nil
		}
		allWildcardHandler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			allWildcardCount++
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.created", exactHandler); err != nil {
			t.Fatalf("Failed to subscribe exact: %v", err)
		}
		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.*", entityWildcardHandler); err != nil {
			t.Fatalf("Failed to subscribe entity.*: %v", err)
		}
		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "*.created", actionWildcardHandler); err != nil {
			t.Fatalf("Failed to subscribe *.created: %v", err)
		}
		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "*.*", allWildcardHandler); err != nil {
			t.Fatalf("Failed to subscribe *.*: %v", err)
		}

		event := &DispatcherTestUserCreatedEvent{AggregateID: "user-123"}
		envelope := domain.NewEventEnvelope(event, "", "user.created", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		if err := d.Dispatch(ctx, anyEnvelope); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if exactCount != 1 {
			t.Errorf("Expected exact handler to be called once, got %d", exactCount)
		}
		if entityWildcardCount != 1 {
			t.Errorf("Expected entity.* handler to be called once, got %d", entityWildcardCount)
		}
		if actionWildcardCount != 1 {
			t.Errorf("Expected *.created handler to be called once, got %d", actionWildcardCount)
		}
		if allWildcardCount != 1 {
			t.Errorf("Expected *.* handler to be called once, got %d", allWildcardCount)
		}
	})

	t.Run("pattern does not match different event", func(t *testing.T) {
		t.Parallel()
		d := domain.NewEventDispatcher()

		callCount := 0
		handler := func(ctx context.Context, env domain.EventEnvelope[DispatcherTestUserCreatedEvent]) error {
			callCount++
			return nil
		}

		if err := domain.Subscribe[DispatcherTestUserCreatedEvent](d, "user.*", handler); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		// Dispatch a different event type
		event := &DispatcherTestOrderPlacedEvent{OrderID: "order-123"}
		envelope := domain.NewEventEnvelope(event, "order-123", "order.placed", 1)
		anyEnvelope := domain.EventEnvelope[any]{
			ID:          envelope.ID,
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			Payload:     envelope.Payload,
			Created:     envelope.Created,
			SequenceNo:  envelope.SequenceNo,
			Metadata:    envelope.Metadata,
		}

		ctx := context.Background()
		if err := d.Dispatch(ctx, anyEnvelope); err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}

		if callCount != 0 {
			t.Errorf("Expected handler not to be called, got %d", callCount)
		}
	})
}
