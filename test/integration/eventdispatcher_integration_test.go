//go:build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/examples"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	pkginfra "github.com/akeemphilbert/pericarp/pkg/infrastructure"
	"github.com/google/uuid"
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
	// Setup
	logger := pkgdomain.NewLogger("test")
	dispatcher := pkginfra.NewEventDispatcher(logger)

	// Track handler calls
	var handlerCalled int32
	handler := func(ctx context.Context, event pkgdomain.Event) error {
		atomic.AddInt32(&handlerCalled, 1)
		return nil
	}

	// Subscribe handler
	if err := dispatcher.RegisterHandler("user", "created", handler); err != nil {
		t.Fatalf("failed to subscribe handler: %v", err)
	}

	// Create test event using examples.User
	userID := uuid.New().String()
	user, err := examples.NewUser(userID, "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	events := user.GetEvents()
	if len(events) == 0 {
		t.Fatal("no events generated")
	}
	event := events[0]

	// Dispatch event
	ctx := context.Background()
	if err := dispatcher.Dispatch(ctx, event); err != nil {
		t.Fatalf("failed to dispatch event: %v", err)
	}

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify handler was called
	if atomic.LoadInt32(&handlerCalled) != 1 {
		t.Errorf("expected handler to be called 1 time, got %d", atomic.LoadInt32(&handlerCalled))
	}
}

func testMultipleHandlers(t *testing.T) {
	// Setup
	logger := pkgdomain.NewLogger("test")
	dispatcher := pkginfra.NewEventDispatcher(logger)

	// Track handler calls
	var handler1Called, handler2Called int32
	handler1 := func(ctx context.Context, event pkgdomain.Event) error {
		atomic.AddInt32(&handler1Called, 1)
		return nil
	}
	handler2 := func(ctx context.Context, event pkgdomain.Event) error {
		atomic.AddInt32(&handler2Called, 1)
		return nil
	}

	// Subscribe multiple handlers
	if err := dispatcher.RegisterHandler("user", "created", handler1); err != nil {
		t.Fatalf("failed to subscribe handler1: %v", err)
	}
	if err := dispatcher.RegisterHandler("user", "created", handler2); err != nil {
		t.Fatalf("failed to subscribe handler2: %v", err)
	}

	// Create test event
	userID := uuid.New().String()
	user, err := examples.NewUser(userID, "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	events := user.GetEvents()
	if len(events) == 0 {
		t.Fatal("no events generated")
	}
	event := events[0]

	// Dispatch event
	ctx := context.Background()
	if err := dispatcher.Dispatch(ctx, event); err != nil {
		t.Fatalf("failed to dispatch event: %v", err)
	}

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify both handlers were called
	if atomic.LoadInt32(&handler1Called) != 1 {
		t.Errorf("expected handler1 to be called 1 time, got %d", atomic.LoadInt32(&handler1Called))
	}
	if atomic.LoadInt32(&handler2Called) != 1 {
		t.Errorf("expected handler2 to be called 1 time, got %d", atomic.LoadInt32(&handler2Called))
	}
}

func testConcurrentDispatch(t *testing.T) {
	// Setup
	logger := pkgdomain.NewLogger("test")
	dispatcher := pkginfra.NewEventDispatcher(logger)

	// Track handler calls
	var handlerCalled int32
	handler := func(ctx context.Context, event pkgdomain.Event) error {
		atomic.AddInt32(&handlerCalled, 1)
		return nil
	}

	// Subscribe handler
	if err := dispatcher.RegisterHandler("user", "created", handler); err != nil {
		t.Fatalf("failed to subscribe handler: %v", err)
	}

	// Create multiple events concurrently
	numEvents := 10
	var wg sync.WaitGroup
	ctx := context.Background()

	for i := 0; i < numEvents; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			userID := uuid.New().String()
			user, err := examples.NewUser(userID, fmt.Sprintf("user%d@example.com", i), fmt.Sprintf("User %d", i))
			if err != nil {
				t.Errorf("failed to create user %d: %v", i, err)
				return
			}

			events := user.GetEvents()
			if len(events) == 0 {
				t.Errorf("no events generated for user %d", i)
				return
			}

			event := events[0]
			if err := dispatcher.Dispatch(ctx, event); err != nil {
				t.Errorf("failed to dispatch event %d: %v", i, err)
			}
		}(i)
	}

	wg.Wait()

	// Wait a bit for async processing
	time.Sleep(200 * time.Millisecond)

	// Verify all handlers were called
	if int(atomic.LoadInt32(&handlerCalled)) != numEvents {
		t.Errorf("expected handler to be called %d times, got %d", numEvents, atomic.LoadInt32(&handlerCalled))
	}
}

func testHandlerErrors(t *testing.T) {
	// Setup
	logger := pkgdomain.NewLogger("test")
	dispatcher := pkginfra.NewEventDispatcher(logger)

	// Create handler that returns error
	handler := func(ctx context.Context, event pkgdomain.Event) error {
		return fmt.Errorf("handler error")
	}

	// Subscribe handler
	if err := dispatcher.RegisterHandler("user", "created", handler); err != nil {
		t.Fatalf("failed to subscribe handler: %v", err)
	}

	// Create test event
	userID := uuid.New().String()
	user, err := examples.NewUser(userID, "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	events := user.GetEvents()
	if len(events) == 0 {
		t.Fatal("no events generated")
	}
	event := events[0]

	// Dispatch event - should not return error even if handler fails
	ctx := context.Background()
	if err := dispatcher.Dispatch(ctx, event); err != nil {
		t.Fatalf("dispatch should not return error even if handler fails: %v", err)
	}

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)
}

func testEventFiltering(t *testing.T) {
	// Setup
	logger := pkgdomain.NewLogger("test")
	dispatcher := pkginfra.NewEventDispatcher(logger)

	// Track handler calls
	var createdHandlerCalled, updatedHandlerCalled int32
	createdHandler := func(ctx context.Context, event pkgdomain.Event) error {
		atomic.AddInt32(&createdHandlerCalled, 1)
		return nil
	}
	updatedHandler := func(ctx context.Context, event pkgdomain.Event) error {
		atomic.AddInt32(&updatedHandlerCalled, 1)
		return nil
	}

	// Subscribe handlers for different event types
	if err := dispatcher.RegisterHandler("user", "created", createdHandler); err != nil {
		t.Fatalf("failed to subscribe created handler: %v", err)
	}
	if err := dispatcher.RegisterHandler("user", "email_changed", updatedHandler); err != nil {
		t.Fatalf("failed to subscribe updated handler: %v", err)
	}

	// Create user (generates created event)
	userID := uuid.New().String()
	user, err := examples.NewUser(userID, "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Dispatch created event
	createdEvents := user.GetEvents()
	if len(createdEvents) == 0 {
		t.Fatal("no created events generated")
	}

	ctx := context.Background()
	if err := dispatcher.Dispatch(ctx, createdEvents[0]); err != nil {
		t.Fatalf("failed to dispatch created event: %v", err)
	}

	// Change email (generates email_changed event)
	if err := user.ChangeEmail("newemail@example.com"); err != nil {
		t.Fatalf("failed to change email: %v", err)
	}

	// Dispatch email changed event
	emailEvents := user.GetEvents()
	if len(emailEvents) == 0 {
		t.Fatal("no email change events generated")
	}

	if err := dispatcher.Dispatch(ctx, emailEvents[0]); err != nil {
		t.Fatalf("failed to dispatch email changed event: %v", err)
	}

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify correct handlers were called
	if atomic.LoadInt32(&createdHandlerCalled) != 1 {
		t.Errorf("expected created handler to be called 1 time, got %d", atomic.LoadInt32(&createdHandlerCalled))
	}
	if atomic.LoadInt32(&updatedHandlerCalled) != 1 {
		t.Errorf("expected updated handler to be called 1 time, got %d", atomic.LoadInt32(&updatedHandlerCalled))
	}
}

func testDispatcherPerformance(t *testing.T) {
	// Setup
	logger := pkgdomain.NewLogger("test")
	dispatcher := pkginfra.NewEventDispatcher(logger)

	// Track handler calls
	var handlerCalled int32
	handler := func(ctx context.Context, event pkgdomain.Event) error {
		atomic.AddInt32(&handlerCalled, 1)
		return nil
	}

	// Subscribe handler
	if err := dispatcher.RegisterHandler("user", "created", handler); err != nil {
		t.Fatalf("failed to subscribe handler: %v", err)
	}

	// Performance test
	numEvents := 1000
	start := time.Now()

	ctx := context.Background()
	for i := 0; i < numEvents; i++ {
		userID := uuid.New().String()
		user, err := examples.NewUser(userID, fmt.Sprintf("user%d@example.com", i), fmt.Sprintf("User %d", i))
		if err != nil {
			t.Fatalf("failed to create user %d: %v", i, err)
		}

		events := user.GetEvents()
		if len(events) == 0 {
			t.Fatalf("no events generated for user %d", i)
		}

		event := events[0]
		if err := dispatcher.Dispatch(ctx, event); err != nil {
			t.Fatalf("failed to dispatch event %d: %v", i, err)
		}
	}

	// Wait for all events to be processed
	time.Sleep(1 * time.Second)

	duration := time.Since(start)
	eventsPerSecond := float64(numEvents) / duration.Seconds()

	t.Logf("Dispatched %d events in %v (%.2f events/sec)", numEvents, duration, eventsPerSecond)

	// Verify all handlers were called
	if int(atomic.LoadInt32(&handlerCalled)) != numEvents {
		t.Errorf("expected handler to be called %d times, got %d", numEvents, atomic.LoadInt32(&handlerCalled))
	}

	// Performance assertion (adjust as needed)
	if eventsPerSecond < 100 {
		t.Errorf("performance too low: %.2f events/sec (expected at least 100)", eventsPerSecond)
	}
}
