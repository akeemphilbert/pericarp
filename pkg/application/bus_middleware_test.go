package application

import (
	"context"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// Test middleware that tracks execution order
type testMiddleware struct {
	name      string
	execOrder *[]string
}

func newTestMiddleware(name string, execOrder *[]string) Middleware[Command, any] {
	return func(next Handler[Command, any]) Handler[Command, any] {
		return func(ctx context.Context, logger domain.Logger, eventStore domain.EventStore, eventDispatcher domain.EventDispatcher, p Payload[Command]) (Response[any], error) {
			*execOrder = append(*execOrder, name+"-before")
			response, err := next(ctx, logger, eventStore, eventDispatcher, p)
			*execOrder = append(*execOrder, name+"-after")
			return response, err
		}
	}
}

func newTestQueryMiddleware(name string, execOrder *[]string) Middleware[Query, any] {
	return func(next Handler[Query, any]) Handler[Query, any] {
		return func(ctx context.Context, logger domain.Logger, eventStore domain.EventStore, eventDispatcher domain.EventDispatcher, p Payload[Query]) (Response[any], error) {
			*execOrder = append(*execOrder, name+"-before")
			response, err := next(ctx, logger, eventStore, eventDispatcher, p)
			*execOrder = append(*execOrder, name+"-after")
			return response, err
		}
	}
}

// MockEventStore provides a simple mock implementation for testing
type MockEventStore struct {
	events []domain.Event
}

func NewMockEventStore() *MockEventStore {
	return &MockEventStore{
		events: make([]domain.Event, 0),
	}
}

func (m *MockEventStore) Save(ctx context.Context, events []domain.Event) ([]domain.Envelope, error) {
	m.events = append(m.events, events...)
	envelopes := make([]domain.Envelope, len(events))
	for i, event := range events {
		envelopes[i] = NewMockEnvelope(event)
	}
	return envelopes, nil
}

func (m *MockEventStore) Load(ctx context.Context, aggregateID string) ([]domain.Envelope, error) {
	return []domain.Envelope{}, nil
}

func (m *MockEventStore) LoadFromSequence(ctx context.Context, aggregateID string, sequenceNo int64) ([]domain.Envelope, error) {
	return []domain.Envelope{}, nil
}

func (m *MockEventStore) NewUnitOfWork() domain.UnitOfWork {
	return nil // Not needed for these tests
}

// Test command and handler
type testCommand struct{}

func (c testCommand) CommandType() string { return "TestCommand" }

// Additional test commands for different handler tests
type testCommand1 struct{}

func (c testCommand1) CommandType() string { return "TestCommand1" }

type testCommand2 struct{}

func (c testCommand2) CommandType() string { return "TestCommand2" }

// Test query and handler
type testQuery struct{}

func (q testQuery) QueryType() string { return "TestQuery" }

// Helper functions to create test handlers
func createTestCommandHandler(execOrder *[]string) Handler[Command, any] {
	return func(ctx context.Context, logger domain.Logger, eventStore domain.EventStore, eventDispatcher domain.EventDispatcher, p Payload[Command]) (Response[any], error) {
		*execOrder = append(*execOrder, "handler")
		return Response[any]{Data: struct{}{}}, nil
	}
}

func createTestQueryHandler(execOrder *[]string) Handler[Query, any] {
	return func(ctx context.Context, logger domain.Logger, eventStore domain.EventStore, eventDispatcher domain.EventDispatcher, p Payload[Query]) (Response[any], error) {
		*execOrder = append(*execOrder, "handler")
		return Response[any]{Data: "result"}, nil
	}
}

func TestCommandBus_PerHandlerMiddleware_ExecutionOrder(t *testing.T) {
	// Arrange
	bus := NewCommandBus()
	execOrder := make([]string, 0)
	logger := NewMockLogger()
	eventStore := NewMockEventStore()
	eventDispatcher := NewMockEventDispatcher()

	handler := createTestCommandHandler(&execOrder)

	// Register handler with middleware in specific order
	bus.Register("TestCommand", handler,
		newTestMiddleware("first", &execOrder),
		newTestMiddleware("second", &execOrder),
		newTestMiddleware("third", &execOrder),
	)

	// Act
	err := bus.Handle(context.Background(), logger, eventStore, eventDispatcher, testCommand{})

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedOrder := []string{
		"first-before",
		"second-before",
		"third-before",
		"handler",
		"third-after",
		"second-after",
		"first-after",
	}

	if len(execOrder) != len(expectedOrder) {
		t.Fatalf("Expected %d execution steps, got %d", len(expectedOrder), len(execOrder))
	}

	for i, expected := range expectedOrder {
		if execOrder[i] != expected {
			t.Errorf("Expected step %d to be '%s', got '%s'", i, expected, execOrder[i])
		}
	}
}

func TestQueryBus_PerHandlerMiddleware_ExecutionOrder(t *testing.T) {
	// Arrange
	bus := NewQueryBus()
	execOrder := make([]string, 0)
	logger := NewMockLogger()
	eventStore := NewMockEventStore()
	eventDispatcher := NewMockEventDispatcher()

	handler := createTestQueryHandler(&execOrder)

	// Register handler with middleware in specific order
	bus.Register("TestQuery", handler,
		newTestQueryMiddleware("first", &execOrder),
		newTestQueryMiddleware("second", &execOrder),
		newTestQueryMiddleware("third", &execOrder),
	)

	// Act
	result, err := bus.Handle(context.Background(), logger, eventStore, eventDispatcher, testQuery{})

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result != "result" {
		t.Errorf("Expected result 'result', got %v", result)
	}

	expectedOrder := []string{
		"first-before",
		"second-before",
		"third-before",
		"handler",
		"third-after",
		"second-after",
		"first-after",
	}

	if len(execOrder) != len(expectedOrder) {
		t.Fatalf("Expected %d execution steps, got %d", len(expectedOrder), len(execOrder))
	}

	for i, expected := range expectedOrder {
		if execOrder[i] != expected {
			t.Errorf("Expected step %d to be '%s', got '%s'", i, expected, execOrder[i])
		}
	}
}

func TestCommandBus_DifferentMiddlewarePerHandler(t *testing.T) {
	// Arrange
	bus := NewCommandBus()
	execOrder1 := make([]string, 0)
	execOrder2 := make([]string, 0)
	logger := NewMockLogger()
	eventStore := NewMockEventStore()
	eventDispatcher := NewMockEventDispatcher()

	handler1 := createTestCommandHandler(&execOrder1)
	handler2 := createTestCommandHandler(&execOrder2)

	// Register first handler with one middleware
	bus.Register("TestCommand1", handler1,
		newTestMiddleware("middleware1", &execOrder1),
	)

	// Register second handler with different middleware
	bus.Register("TestCommand2", handler2,
		newTestMiddleware("middlewareA", &execOrder2),
		newTestMiddleware("middlewareB", &execOrder2),
	)

	// Act
	err1 := bus.Handle(context.Background(), logger, eventStore, eventDispatcher, testCommand1{})
	err2 := bus.Handle(context.Background(), logger, eventStore, eventDispatcher, testCommand2{})

	// Assert
	if err1 != nil {
		t.Errorf("Expected no error for command 1, got: %v", err1)
	}
	if err2 != nil {
		t.Errorf("Expected no error for command 2, got: %v", err2)
	}

	// Verify middleware execution for first handler
	expectedOrder1 := []string{"middleware1-before", "handler", "middleware1-after"}
	if len(execOrder1) != len(expectedOrder1) {
		t.Errorf("Expected %d execution steps for handler 1, got %d", len(expectedOrder1), len(execOrder1))
	}

	// Verify middleware execution for second handler
	expectedOrder2 := []string{"middlewareA-before", "middlewareB-before", "handler", "middlewareB-after", "middlewareA-after"}
	if len(execOrder2) != len(expectedOrder2) {
		t.Errorf("Expected %d execution steps for handler 2, got %d", len(expectedOrder2), len(execOrder2))
	}
}

func TestCommandBus_NoMiddleware(t *testing.T) {
	// Arrange
	bus := NewCommandBus()
	execOrder := make([]string, 0)
	logger := NewMockLogger()
	eventStore := NewMockEventStore()
	eventDispatcher := NewMockEventDispatcher()

	handler := createTestCommandHandler(&execOrder)

	// Register handler without any middleware
	bus.Register("TestCommand", handler)

	// Act
	err := bus.Handle(context.Background(), logger, eventStore, eventDispatcher, testCommand{})

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedOrder := []string{"handler"}

	if len(execOrder) != len(expectedOrder) {
		t.Fatalf("Expected %d execution steps, got %d", len(expectedOrder), len(execOrder))
	}

	if execOrder[0] != "handler" {
		t.Errorf("Expected 'handler', got '%s'", execOrder[0])
	}
}
