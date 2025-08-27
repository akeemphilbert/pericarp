//go:build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	internaldomain "github.com/akeemphilbert/pericarp/internal/domain"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	pkginfra "github.com/akeemphilbert/pericarp/pkg/infrastructure"
)

// TestEventDispatcherIntegration tests the EventDispatcher with Watermill channels
func TestEventDispatcherIntegration(t *testing.T) {
	t.Run("BasicEventDispatch", func(t *testing.T) {
		testBasicEventDispatch(t)
	})

	t.Run("MultipleHandlers", func(t *testing.T) {
		testMultipleHandlers(t)
	})

	t.Run("ConcurrentDispatch", func(t *testing.T) {
		testConcurrentDispatch(t)
	})

	t.Run("HandlerErrors", func(t *testing.T) {
		testHandlerErrors(t)
	})

	t.Run("EventFiltering", func(t *testing.T) {
		testEventFiltering(t)
	})

	t.Run("PerformanceTest", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping performance test in short mode")
		}
		testDispatcherPerformance(t)
	})
}

func testBasicEventDispatch(t *testing.T) {
	dispatcher := pkginfra.NewEventDispatcher()
	ctx := context.Background()

	// Create a test handler
	var receivedEvents []pkgdomain.Event
	var mu sync.Mutex

	handler := &TestEventHandler{
		eventTypes: []string{"UserCreated"},
		handleFunc: func(ctx context.Context, envelope pkgdomain.Envelope) error {
			mu.Lock()
			defer mu.Unlock()
			receivedEvents = append(receivedEvents, envelope.Event())
			return nil
		},
	}

	// Subscribe handler
	err := dispatcher.Subscribe("UserCreated", handler)
	if err != nil {
		t.Fatalf("failed to subscribe handler: %v", err)
	}

	// Create test event
	event := internaldomain.NewUserCreatedEvent(
		uuid.New(),
		"test@example.com",
		"Test User",
		uuid.New().String(),
		1,
	)

	// Create envelope
	envelope := &TestEnvelope{
		event:     event,
		eventID:   uuid.New().String(),
		timestamp: time.Now(),
		metadata: map[string]interface{}{
			"aggregate_id": event.AggregateID(),
			"event_type":   event.EventType(),
		},
	}

	// Dispatch event
	err = dispatcher.Dispatch(ctx, []pkgdomain.Envelope{envelope})
	if err != nil {
		t.Fatalf("failed to dispatch event: %v", err)
	}

	// Wait for event processing
	time.Sleep(100 * time.Millisecond)

	// Verify event was received
	mu.Lock()
	defer mu.Unlock()

	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 received event, got %d", len(receivedEvents))
	}

	receivedEvent := receivedEvents[0]
	if receivedEvent.EventType() != event.EventType() {
		t.Errorf("expected event type %s, got %s", event.EventType(), receivedEvent.EventType())
	}

	if receivedEvent.AggregateID() != event.AggregateID() {
		t.Errorf("expected aggregate ID %s, got %s", event.AggregateID(), receivedEvent.AggregateID())
	}
}

func testMultipleHandlers(t *testing.T) {
	dispatcher := pkginfra.NewEventDispatcher()
	ctx := context.Background()

	// Create multiple handlers for the same event type
	var handler1Events, handler2Events []pkgdomain.Event
	var mu1, mu2 sync.Mutex

	handler1 := &TestEventHandler{
		eventTypes: []string{"UserCreated"},
		handleFunc: func(ctx context.Context, envelope pkgdomain.Envelope) error {
			mu1.Lock()
			defer mu1.Unlock()
			handler1Events = append(handler1Events, envelope.Event())
			return nil
		},
	}

	handler2 := &TestEventHandler{
		eventTypes: []string{"UserCreated"},
		handleFunc: func(ctx context.Context, envelope pkgdomain.Envelope) error {
			mu2.Lock()
			defer mu2.Unlock()
			handler2Events = append(handler2Events, envelope.Event())
			return nil
		},
	}

	// Subscribe both handlers
	err := dispatcher.Subscribe("UserCreated", handler1)
	if err != nil {
		t.Fatalf("failed to subscribe handler1: %v", err)
	}

	err = dispatcher.Subscribe("UserCreated", handler2)
	if err != nil {
		t.Fatalf("failed to subscribe handler2: %v", err)
	}

	// Create and dispatch event
	event := internaldomain.NewUserCreatedEvent(
		uuid.New(),
		"test@example.com",
		"Test User",
		uuid.New().String(),
		1,
	)

	envelope := &TestEnvelope{
		event:     event,
		eventID:   uuid.New().String(),
		timestamp: time.Now(),
		metadata:  map[string]interface{}{},
	}

	err = dispatcher.Dispatch(ctx, []pkgdomain.Envelope{envelope})
	if err != nil {
		t.Fatalf("failed to dispatch event: %v", err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify both handlers received the event
	mu1.Lock()
	handler1Count := len(handler1Events)
	mu1.Unlock()

	mu2.Lock()
	handler2Count := len(handler2Events)
	mu2.Unlock()

	if handler1Count != 1 {
		t.Errorf("handler1 expected 1 event, got %d", handler1Count)
	}

	if handler2Count != 1 {
		t.Errorf("handler2 expected 1 event, got %d", handler2Count)
	}
}

func testConcurrentDispatch(t *testing.T) {
	dispatcher := pkginfra.NewEventDispatcher()
	ctx := context.Background()

	// Create handler that tracks concurrent executions
	var receivedEvents []pkgdomain.Event
	var mu sync.Mutex
	var concurrentCount int32
	var maxConcurrent int32

	handler := &TestEventHandler{
		eventTypes: []string{"UserCreated", "UserEmailUpdated"},
		handleFunc: func(ctx context.Context, envelope pkgdomain.Envelope) error {
			// Track concurrent executions
			current := atomic.AddInt32(&concurrentCount, 1)
			defer atomic.AddInt32(&concurrentCount, -1)

			// Update max concurrent
			for {
				max := atomic.LoadInt32(&maxConcurrent)
				if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
					break
				}
			}

			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			receivedEvents = append(receivedEvents, envelope.Event())
			mu.Unlock()

			return nil
		},
	}

	// Subscribe handler
	err := dispatcher.Subscribe("UserCreated", handler)
	if err != nil {
		t.Fatalf("failed to subscribe to UserCreated: %v", err)
	}

	err = dispatcher.Subscribe("UserEmailUpdated", handler)
	if err != nil {
		t.Fatalf("failed to subscribe to UserEmailUpdated: %v", err)
	}

	// Create multiple events
	numEvents := 20
	envelopes := make([]pkgdomain.Envelope, numEvents)

	for i := 0; i < numEvents; i++ {
		var event pkgdomain.Event
		if i%2 == 0 {
			event = internaldomain.NewUserCreatedEvent(
				uuid.New(),
				"test@example.com",
				"Test User",
				uuid.New().String(),
				1,
			)
		} else {
			event = internaldomain.NewUserEmailUpdatedEvent(
				uuid.New(),
				"old@example.com",
				"new@example.com",
				uuid.New().String(),
				2,
			)
		}

		envelopes[i] = &TestEnvelope{
			event:     event,
			eventID:   uuid.New().String(),
			timestamp: time.Now(),
			metadata:  map[string]interface{}{},
		}
	}

	// Dispatch all events concurrently
	start := time.Now()
	err = dispatcher.Dispatch(ctx, envelopes)
	if err != nil {
		t.Fatalf("failed to dispatch events: %v", err)
	}

	// Wait for all events to be processed
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for events to be processed")
		case <-ticker.C:
			mu.Lock()
			count := len(receivedEvents)
			mu.Unlock()

			if count == numEvents {
				goto done
			}
		}
	}

done:
	duration := time.Since(start)
	t.Logf("Processed %d events in %v with max concurrency %d", numEvents, duration, maxConcurrent)

	// Verify all events were processed
	mu.Lock()
	finalCount := len(receivedEvents)
	mu.Unlock()

	if finalCount != numEvents {
		t.Errorf("expected %d events processed, got %d", numEvents, finalCount)
	}

	// Verify concurrent processing occurred
	if maxConcurrent < 2 {
		t.Errorf("expected concurrent processing, max concurrent was %d", maxConcurrent)
	}
}

func testHandlerErrors(t *testing.T) {
	dispatcher := pkginfra.NewEventDispatcher()
	ctx := context.Background()

	// Create handler that fails on specific events
	var processedEvents []pkgdomain.Event
	var mu sync.Mutex

	handler := &TestEventHandler{
		eventTypes: []string{"UserCreated"},
		handleFunc: func(ctx context.Context, envelope pkgdomain.Envelope) error {
			event := envelope.Event()

			// Fail if aggregate ID contains "fail"
			if contains(event.AggregateID(), "fail") {
				return fmt.Errorf("simulated handler error")
			}

			mu.Lock()
			processedEvents = append(processedEvents, event)
			mu.Unlock()

			return nil
		},
	}

	// Subscribe handler
	err := dispatcher.Subscribe("UserCreated", handler)
	if err != nil {
		t.Fatalf("failed to subscribe handler: %v", err)
	}

	// Create events - some that will succeed, some that will fail
	successEvent := internaldomain.NewUserCreatedEvent(
		uuid.New(),
		"success@example.com",
		"Success User",
		"success-aggregate",
		1,
	)

	failEvent := internaldomain.NewUserCreatedEvent(
		uuid.New(),
		"fail@example.com",
		"Fail User",
		"fail-aggregate",
		1,
	)

	envelopes := []pkgdomain.Envelope{
		&TestEnvelope{
			event:     successEvent,
			eventID:   uuid.New().String(),
			timestamp: time.Now(),
			metadata:  map[string]interface{}{},
		},
		&TestEnvelope{
			event:     failEvent,
			eventID:   uuid.New().String(),
			timestamp: time.Now(),
			metadata:  map[string]interface{}{},
		},
	}

	// Dispatch events
	err = dispatcher.Dispatch(ctx, envelopes)
	// Note: The dispatcher should handle errors gracefully and not fail the entire dispatch
	if err != nil {
		t.Logf("dispatch returned error (may be expected): %v", err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify only successful event was processed
	mu.Lock()
	count := len(processedEvents)
	mu.Unlock()

	// Should have processed only the successful event
	if count != 1 {
		t.Errorf("expected 1 processed event, got %d", count)
	}
}

func testEventFiltering(t *testing.T) {
	dispatcher := pkginfra.NewEventDispatcher()
	ctx := context.Background()

	// Create handlers for different event types
	var userCreatedEvents, userEmailUpdatedEvents []pkgdomain.Event
	var mu1, mu2 sync.Mutex

	userCreatedHandler := &TestEventHandler{
		eventTypes: []string{"UserCreated"},
		handleFunc: func(ctx context.Context, envelope pkgdomain.Envelope) error {
			mu1.Lock()
			defer mu1.Unlock()
			userCreatedEvents = append(userCreatedEvents, envelope.Event())
			return nil
		},
	}

	userEmailUpdatedHandler := &TestEventHandler{
		eventTypes: []string{"UserEmailUpdated"},
		handleFunc: func(ctx context.Context, envelope pkgdomain.Envelope) error {
			mu2.Lock()
			defer mu2.Unlock()
			userEmailUpdatedEvents = append(userEmailUpdatedEvents, envelope.Event())
			return nil
		},
	}

	// Subscribe handlers
	err := dispatcher.Subscribe("UserCreated", userCreatedHandler)
	if err != nil {
		t.Fatalf("failed to subscribe UserCreated handler: %v", err)
	}

	err = dispatcher.Subscribe("UserEmailUpdated", userEmailUpdatedHandler)
	if err != nil {
		t.Fatalf("failed to subscribe UserEmailUpdated handler: %v", err)
	}

	// Create mixed events
	events := []pkgdomain.Event{
		internaldomain.NewUserCreatedEvent(
			uuid.New(),
			"user1@example.com",
			"User 1",
			uuid.New().String(),
			1,
		),
		internaldomain.NewUserEmailUpdatedEvent(
			uuid.New(),
			"old@example.com",
			"new@example.com",
			uuid.New().String(),
			2,
		),
		internaldomain.NewUserCreatedEvent(
			uuid.New(),
			"user2@example.com",
			"User 2",
			uuid.New().String(),
			1,
		),
	}

	envelopes := make([]pkgdomain.Envelope, len(events))
	for i, event := range events {
		envelopes[i] = &TestEnvelope{
			event:     event,
			eventID:   uuid.New().String(),
			timestamp: time.Now(),
			metadata:  map[string]interface{}{},
		}
	}

	// Dispatch events
	err = dispatcher.Dispatch(ctx, envelopes)
	if err != nil {
		t.Fatalf("failed to dispatch events: %v", err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify filtering
	mu1.Lock()
	createdCount := len(userCreatedEvents)
	mu1.Unlock()

	mu2.Lock()
	emailUpdatedCount := len(userEmailUpdatedEvents)
	mu2.Unlock()

	if createdCount != 2 {
		t.Errorf("expected 2 UserCreated events, got %d", createdCount)
	}

	if emailUpdatedCount != 1 {
		t.Errorf("expected 1 UserEmailUpdated event, got %d", emailUpdatedCount)
	}

	// Verify event types
	mu1.Lock()
	for i, event := range userCreatedEvents {
		if event.EventType() != "UserCreated" {
			t.Errorf("userCreatedEvents[%d] has wrong type: %s", i, event.EventType())
		}
	}
	mu1.Unlock()

	mu2.Lock()
	for i, event := range userEmailUpdatedEvents {
		if event.EventType() != "UserEmailUpdated" {
			t.Errorf("userEmailUpdatedEvents[%d] has wrong type: %s", i, event.EventType())
		}
	}
	mu2.Unlock()
}

func testDispatcherPerformance(t *testing.T) {
	dispatcher := pkginfra.NewEventDispatcher()
	ctx := context.Background()

	// Create high-throughput handler
	var processedCount int64
	handler := &TestEventHandler{
		eventTypes: []string{"UserCreated"},
		handleFunc: func(ctx context.Context, envelope pkgdomain.Envelope) error {
			atomic.AddInt64(&processedCount, 1)
			return nil
		},
	}

	err := dispatcher.Subscribe("UserCreated", handler)
	if err != nil {
		t.Fatalf("failed to subscribe handler: %v", err)
	}

	// Create large number of events
	numEvents := 10000
	envelopes := make([]pkgdomain.Envelope, numEvents)

	for i := 0; i < numEvents; i++ {
		event := internaldomain.NewUserCreatedEvent(
			uuid.New(),
			fmt.Sprintf("user%d@example.com", i),
			fmt.Sprintf("User %d", i),
			uuid.New().String(),
			1,
		)

		envelopes[i] = &TestEnvelope{
			event:     event,
			eventID:   uuid.New().String(),
			timestamp: time.Now(),
			metadata:  map[string]interface{}{},
		}
	}

	// Measure dispatch performance
	start := time.Now()
	err = dispatcher.Dispatch(ctx, envelopes)
	if err != nil {
		t.Fatalf("failed to dispatch events: %v", err)
	}

	// Wait for all events to be processed
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			processed := atomic.LoadInt64(&processedCount)
			t.Fatalf("timeout: processed %d/%d events", processed, numEvents)
		case <-ticker.C:
			processed := atomic.LoadInt64(&processedCount)
			if processed >= int64(numEvents) {
				goto done
			}
		}
	}

done:
	duration := time.Since(start)
	throughput := float64(numEvents) / duration.Seconds()

	t.Logf("Processed %d events in %v (%.2f events/sec)", numEvents, duration, throughput)

	// Performance assertions
	maxDuration := 10 * time.Second
	minThroughput := 1000.0 // events per second

	if duration > maxDuration {
		t.Errorf("dispatch too slow: %v (max: %v)", duration, maxDuration)
	}

	if throughput < minThroughput {
		t.Errorf("throughput too low: %.2f events/sec (min: %.2f)", throughput, minThroughput)
	}
}

// TestEventHandler is a test implementation of EventHandler
type TestEventHandler struct {
	eventTypes []string
	handleFunc func(context.Context, pkgdomain.Envelope) error
}

func (h *TestEventHandler) Handle(ctx context.Context, envelope pkgdomain.Envelope) error {
	return h.handleFunc(ctx, envelope)
}

func (h *TestEventHandler) EventTypes() []string {
	return h.eventTypes
}

// TestEnvelope is a test implementation of Envelope
type TestEnvelope struct {
	event     pkgdomain.Event
	eventID   string
	timestamp time.Time
	metadata  map[string]interface{}
}

func (e *TestEnvelope) Event() pkgdomain.Event {
	return e.event
}

func (e *TestEnvelope) EventID() string {
	return e.eventID
}

func (e *TestEnvelope) Timestamp() time.Time {
	return e.timestamp
}

func (e *TestEnvelope) Metadata() map[string]interface{} {
	return e.metadata
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
