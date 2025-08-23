package infrastructure

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/example/pericarp/pkg/domain"
)

// testEventHandler implements domain.EventHandler for testing
type testEventHandler struct {
	handledEvents []domain.Envelope
	eventTypes    []string
	mu            sync.Mutex
}

func (h *testEventHandler) Handle(ctx context.Context, envelope domain.Envelope) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handledEvents = append(h.handledEvents, envelope)
	return nil
}

func (h *testEventHandler) EventTypes() []string {
	return h.eventTypes
}

func (h *testEventHandler) GetHandledEvents() []domain.Envelope {
	h.mu.Lock()
	defer h.mu.Unlock()
	events := make([]domain.Envelope, len(h.handledEvents))
	copy(events, h.handledEvents)
	return events
}

func TestWatermillEventDispatcher_SubscribeAndDispatch(t *testing.T) {
	dispatcher, err := NewWatermillEventDispatcher(watermill.NopLogger{})
	if err != nil {
		t.Fatalf("Failed to create event dispatcher: %v", err)
	}
	defer dispatcher.Close()

	// Create test handler
	handler := &testEventHandler{
		eventTypes: []string{"TestEvent"},
	}

	// Subscribe handler
	err = dispatcher.Subscribe("TestEvent", handler)
	if err != nil {
		t.Fatalf("Failed to subscribe handler: %v", err)
	}

	// Verify handler is registered
	handlers := dispatcher.GetHandlers("TestEvent")
	if len(handlers) != 1 {
		t.Errorf("Expected 1 handler, got %d", len(handlers))
	}

	// Create test event and envelope
	event := &testEvent{
		eventType:   "TestEvent",
		aggregateID: "test-123",
		version:     1,
		occurredAt:  time.Now(),
	}

	envelope := &eventEnvelope{
		event:     event,
		metadata:  map[string]interface{}{"test": "metadata"},
		eventID:   "envelope-123",
		timestamp: time.Now(),
	}

	// Dispatch event
	ctx := context.Background()
	err = dispatcher.Dispatch(ctx, []domain.Envelope{envelope})
	if err != nil {
		t.Fatalf("Failed to dispatch event: %v", err)
	}

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify handler received the event
	handledEvents := handler.GetHandledEvents()
	if len(handledEvents) != 1 {
		t.Errorf("Expected 1 handled event, got %d", len(handledEvents))
	}

	if len(handledEvents) > 0 {
		handledEvent := handledEvents[0]
		if handledEvent.EventID() != envelope.EventID() {
			t.Errorf("Expected event ID %s, got %s", envelope.EventID(), handledEvent.EventID())
		}
		if handledEvent.Event().EventType() != event.EventType() {
			t.Errorf("Expected event type %s, got %s", event.EventType(), handledEvent.Event().EventType())
		}
	}
}

func TestWatermillEventDispatcher_MultipleHandlers(t *testing.T) {
	dispatcher, err := NewWatermillEventDispatcher(watermill.NopLogger{})
	if err != nil {
		t.Fatalf("Failed to create event dispatcher: %v", err)
	}
	defer dispatcher.Close()

	// Create multiple handlers for the same event type
	handler1 := &testEventHandler{eventTypes: []string{"TestEvent"}}
	handler2 := &testEventHandler{eventTypes: []string{"TestEvent"}}

	// Subscribe both handlers
	err = dispatcher.Subscribe("TestEvent", handler1)
	if err != nil {
		t.Fatalf("Failed to subscribe handler1: %v", err)
	}

	err = dispatcher.Subscribe("TestEvent", handler2)
	if err != nil {
		t.Fatalf("Failed to subscribe handler2: %v", err)
	}

	// Give the router time to set up the handlers
	time.Sleep(10 * time.Millisecond)

	// Verify both handlers are registered
	handlers := dispatcher.GetHandlers("TestEvent")
	if len(handlers) != 2 {
		t.Errorf("Expected 2 handlers, got %d", len(handlers))
	}

	// Create and dispatch test event
	event := &testEvent{
		eventType:   "TestEvent",
		aggregateID: "test-456",
		version:     1,
		occurredAt:  time.Now(),
	}

	envelope := &eventEnvelope{
		event:     event,
		metadata:  map[string]interface{}{},
		eventID:   "envelope-456",
		timestamp: time.Now(),
	}

	ctx := context.Background()
	err = dispatcher.Dispatch(ctx, []domain.Envelope{envelope})
	if err != nil {
		t.Fatalf("Failed to dispatch event: %v", err)
	}

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify both handlers received the event
	handler1Events := handler1.GetHandledEvents()
	handler2Events := handler2.GetHandledEvents()

	if len(handler1Events) != 1 {
		t.Errorf("Expected handler1 to receive 1 event, got %d", len(handler1Events))
	}

	if len(handler2Events) != 1 {
		t.Errorf("Expected handler2 to receive 1 event, got %d", len(handler2Events))
	}
}

func TestWatermillEventDispatcher_DifferentEventTypes(t *testing.T) {
	dispatcher, err := NewWatermillEventDispatcher(watermill.NopLogger{})
	if err != nil {
		t.Fatalf("Failed to create event dispatcher: %v", err)
	}
	defer dispatcher.Close()

	// Create handlers for different event types
	handler1 := &testEventHandler{eventTypes: []string{"EventType1"}}
	handler2 := &testEventHandler{eventTypes: []string{"EventType2"}}

	// Subscribe handlers
	err = dispatcher.Subscribe("EventType1", handler1)
	if err != nil {
		t.Fatalf("Failed to subscribe handler1: %v", err)
	}

	err = dispatcher.Subscribe("EventType2", handler2)
	if err != nil {
		t.Fatalf("Failed to subscribe handler2: %v", err)
	}

	// Give the router time to set up the handlers
	time.Sleep(10 * time.Millisecond)

	// Create events of different types
	event1 := &testEvent{eventType: "EventType1", aggregateID: "test-1", version: 1, occurredAt: time.Now()}
	event2 := &testEvent{eventType: "EventType2", aggregateID: "test-2", version: 1, occurredAt: time.Now()}

	envelope1 := &eventEnvelope{event: event1, eventID: "env-1", timestamp: time.Now(), metadata: map[string]interface{}{}}
	envelope2 := &eventEnvelope{event: event2, eventID: "env-2", timestamp: time.Now(), metadata: map[string]interface{}{}}

	// Dispatch both events
	ctx := context.Background()
	err = dispatcher.Dispatch(ctx, []domain.Envelope{envelope1, envelope2})
	if err != nil {
		t.Fatalf("Failed to dispatch events: %v", err)
	}

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify each handler only received its event type
	handler1Events := handler1.GetHandledEvents()
	handler2Events := handler2.GetHandledEvents()

	if len(handler1Events) != 1 {
		t.Errorf("Expected handler1 to receive 1 event, got %d", len(handler1Events))
	}

	if len(handler2Events) != 1 {
		t.Errorf("Expected handler2 to receive 1 event, got %d", len(handler2Events))
	}

	if len(handler1Events) > 0 && handler1Events[0].Event().EventType() != "EventType1" {
		t.Errorf("Handler1 received wrong event type: %s", handler1Events[0].Event().EventType())
	}

	if len(handler2Events) > 0 && handler2Events[0].Event().EventType() != "EventType2" {
		t.Errorf("Handler2 received wrong event type: %s", handler2Events[0].Event().EventType())
	}
}