package infrastructure

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// mockEventStore implements domain.EventStore for testing
type mockEventStore struct {
	savedEvents []domain.Event
	saveError   error
	mu          sync.Mutex
}

func (m *mockEventStore) Save(ctx context.Context, events []domain.Event) ([]domain.Envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.saveError != nil {
		return nil, m.saveError
	}

	m.savedEvents = append(m.savedEvents, events...)

	// Create envelopes for the saved events
	envelopes := make([]domain.Envelope, len(events))
	for i, event := range events {
		envelopes[i] = &eventEnvelope{
			event:     event,
			metadata:  map[string]interface{}{"saved": true},
			eventID:   "test-envelope-" + event.EventType(),
			timestamp: time.Now(),
		}
	}

	return envelopes, nil
}

func (m *mockEventStore) Load(ctx context.Context, aggregateID string) ([]domain.Envelope, error) {
	return nil, nil // Not used in UoW tests
}

func (m *mockEventStore) LoadFromVersion(ctx context.Context, aggregateID string, version int) ([]domain.Envelope, error) {
	return nil, nil // Not used in UoW tests
}

func (m *mockEventStore) LoadFromSequence(ctx context.Context, aggregateID string, sequenceNo int64) ([]domain.Envelope, error) {
	return nil, nil // Not used in UoW tests
}

func (m *mockEventStore) NewUnitOfWork() domain.UnitOfWork {
	return NewUnitOfWork(m, &mockEventDispatcher{})
}

func (m *mockEventStore) GetSavedEvents() []domain.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	events := make([]domain.Event, len(m.savedEvents))
	copy(events, m.savedEvents)
	return events
}

func (m *mockEventStore) SetSaveError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveError = err
}

// mockEventDispatcher implements domain.EventDispatcher for testing
type mockEventDispatcher struct {
	dispatchedEnvelopes []domain.Envelope
	dispatchError       error
	handlers            map[string][]domain.EventHandler
	mu                  sync.Mutex
}

func (m *mockEventDispatcher) Dispatch(ctx context.Context, envelopes []domain.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.dispatchError != nil {
		return m.dispatchError
	}

	m.dispatchedEnvelopes = append(m.dispatchedEnvelopes, envelopes...)
	return nil
}

func (m *mockEventDispatcher) Subscribe(eventType string, handler domain.EventHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.handlers == nil {
		m.handlers = make(map[string][]domain.EventHandler)
	}
	m.handlers[eventType] = append(m.handlers[eventType], handler)
	return nil
}

func (m *mockEventDispatcher) GetDispatchedEnvelopes() []domain.Envelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	envelopes := make([]domain.Envelope, len(m.dispatchedEnvelopes))
	copy(envelopes, m.dispatchedEnvelopes)
	return envelopes
}

func (m *mockEventDispatcher) SetDispatchError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dispatchError = err
}

// testEvent implements domain.Event for testing
type testEvent struct {
	eventType   string
	aggregateID string
	version     int64
	occurredAt  time.Time
	user        string
	account     string
	payload     any
}

func (e *testEvent) EventType() string {
	return e.eventType
}

func (e *testEvent) AggregateID() string {
	return e.aggregateID
}

func (e *testEvent) SequenceNo() int64 {
	return e.version
}

func (e *testEvent) CreatedAt() time.Time {
	return e.occurredAt
}

func (e *testEvent) User() string {
	return e.user
}

func (e *testEvent) Account() string {
	return e.account
}

func (e *testEvent) Payload() any {
	return e.payload
}

func (e *testEvent) SetSequenceNo(sequenceNo int64) {
	e.version = sequenceNo
}

func TestUnitOfWork_RegisterAndCommit(t *testing.T) {
	eventStore := &mockEventStore{}
	eventDispatcher := &mockEventDispatcher{}
	uow := NewUnitOfWork(eventStore, eventDispatcher)

	// Create test events
	event1 := &testEvent{
		eventType:   "TestEvent1",
		aggregateID: "aggregate-1",
		version:     1,
		occurredAt:  time.Now(),
	}

	event2 := &testEvent{
		eventType:   "TestEvent2",
		aggregateID: "aggregate-1",
		version:     2,
		occurredAt:  time.Now(),
	}

	// Register events
	uow.RegisterEvents([]domain.Event{event1})
	uow.RegisterEvents([]domain.Event{event2})

	// Verify events are registered
	if uow.EventCount() != 2 {
		t.Errorf("Expected 2 registered events, got %d", uow.EventCount())
	}

	registeredEvents := uow.GetRegisteredEvents()
	if len(registeredEvents) != 2 {
		t.Errorf("Expected 2 registered events, got %d", len(registeredEvents))
	}

	// Commit the unit of work
	ctx := context.Background()
	envelopes, err := uow.Commit(ctx)
	if err != nil {
		t.Fatalf("Failed to commit unit of work: %v", err)
	}

	// Verify envelopes returned
	if len(envelopes) != 2 {
		t.Errorf("Expected 2 envelopes, got %d", len(envelopes))
	}

	// Verify events were saved to event store
	savedEvents := eventStore.GetSavedEvents()
	if len(savedEvents) != 2 {
		t.Errorf("Expected 2 saved events, got %d", len(savedEvents))
	}

	// Verify events were dispatched
	dispatchedEnvelopes := eventDispatcher.GetDispatchedEnvelopes()
	if len(dispatchedEnvelopes) != 2 {
		t.Errorf("Expected 2 dispatched envelopes, got %d", len(dispatchedEnvelopes))
	}

	// Verify unit of work is committed
	if !uow.IsCommitted() {
		t.Error("Unit of work should be committed")
	}
}

func TestUnitOfWork_EmptyCommit(t *testing.T) {
	eventStore := &mockEventStore{}
	eventDispatcher := &mockEventDispatcher{}
	uow := NewUnitOfWork(eventStore, eventDispatcher)

	// Commit without registering any events
	ctx := context.Background()
	envelopes, err := uow.Commit(ctx)
	if err != nil {
		t.Fatalf("Failed to commit empty unit of work: %v", err)
	}

	// Verify empty envelopes returned
	if len(envelopes) != 0 {
		t.Errorf("Expected 0 envelopes for empty commit, got %d", len(envelopes))
	}

	// Verify no events were saved or dispatched
	savedEvents := eventStore.GetSavedEvents()
	if len(savedEvents) != 0 {
		t.Errorf("Expected 0 saved events, got %d", len(savedEvents))
	}

	dispatchedEnvelopes := eventDispatcher.GetDispatchedEnvelopes()
	if len(dispatchedEnvelopes) != 0 {
		t.Errorf("Expected 0 dispatched envelopes, got %d", len(dispatchedEnvelopes))
	}

	// Verify unit of work is committed
	if !uow.IsCommitted() {
		t.Error("Unit of work should be committed")
	}
}

func TestUnitOfWork_Rollback(t *testing.T) {
	eventStore := &mockEventStore{}
	eventDispatcher := &mockEventDispatcher{}
	uow := NewUnitOfWork(eventStore, eventDispatcher)

	// Register events
	event := &testEvent{
		eventType:   "TestEvent",
		aggregateID: "aggregate-1",
		version:     1,
		occurredAt:  time.Now(),
	}

	uow.RegisterEvents([]domain.Event{event})

	// Verify event is registered
	if uow.EventCount() != 1 {
		t.Errorf("Expected 1 registered event, got %d", uow.EventCount())
	}

	// Rollback
	err := uow.Rollback()
	if err != nil {
		t.Fatalf("Failed to rollback: %v", err)
	}

	// Verify events are cleared
	if uow.EventCount() != 0 {
		t.Errorf("Expected 0 events after rollback, got %d", uow.EventCount())
	}

	// Verify no events were saved or dispatched
	savedEvents := eventStore.GetSavedEvents()
	if len(savedEvents) != 0 {
		t.Errorf("Expected 0 saved events after rollback, got %d", len(savedEvents))
	}

	dispatchedEnvelopes := eventDispatcher.GetDispatchedEnvelopes()
	if len(dispatchedEnvelopes) != 0 {
		t.Errorf("Expected 0 dispatched envelopes after rollback, got %d", len(dispatchedEnvelopes))
	}
}

func TestUnitOfWork_SaveError(t *testing.T) {
	eventStore := &mockEventStore{}
	eventDispatcher := &mockEventDispatcher{}
	uow := NewUnitOfWork(eventStore, eventDispatcher)

	// Set up event store to return error
	saveError := errors.New("save failed")
	eventStore.SetSaveError(saveError)

	// Register event
	event := &testEvent{
		eventType:   "TestEvent",
		aggregateID: "aggregate-1",
		version:     1,
		occurredAt:  time.Now(),
	}

	uow.RegisterEvents([]domain.Event{event})

	// Commit should fail
	ctx := context.Background()
	envelopes, err := uow.Commit(ctx)
	if err == nil {
		t.Fatal("Expected commit to fail due to save error")
	}

	if envelopes != nil {
		t.Error("Expected nil envelopes when save fails")
	}

	// Verify no events were dispatched
	dispatchedEnvelopes := eventDispatcher.GetDispatchedEnvelopes()
	if len(dispatchedEnvelopes) != 0 {
		t.Errorf("Expected 0 dispatched envelopes when save fails, got %d", len(dispatchedEnvelopes))
	}

	// Unit of work should not be committed
	if uow.IsCommitted() {
		t.Error("Unit of work should not be committed when save fails")
	}
}

func TestUnitOfWork_DispatchError(t *testing.T) {
	eventStore := &mockEventStore{}
	eventDispatcher := &mockEventDispatcher{}
	uow := NewUnitOfWork(eventStore, eventDispatcher)

	// Set up event dispatcher to return error
	dispatchError := errors.New("dispatch failed")
	eventDispatcher.SetDispatchError(dispatchError)

	// Register event
	event := &testEvent{
		eventType:   "TestEvent",
		aggregateID: "aggregate-1",
		version:     1,
		occurredAt:  time.Now(),
	}

	uow.RegisterEvents([]domain.Event{event})

	// Commit should succeed but return dispatch error
	ctx := context.Background()
	envelopes, err := uow.Commit(ctx)
	if err == nil {
		t.Fatal("Expected commit to return dispatch error")
	}

	// Events should still be persisted and envelopes returned
	if len(envelopes) != 1 {
		t.Errorf("Expected 1 envelope even with dispatch error, got %d", len(envelopes))
	}

	// Verify events were saved
	savedEvents := eventStore.GetSavedEvents()
	if len(savedEvents) != 1 {
		t.Errorf("Expected 1 saved event even with dispatch error, got %d", len(savedEvents))
	}

	// Unit of work should be committed (events are persisted)
	if !uow.IsCommitted() {
		t.Error("Unit of work should be committed even with dispatch error")
	}
}

func TestUnitOfWork_DoubleCommit(t *testing.T) {
	eventStore := &mockEventStore{}
	eventDispatcher := &mockEventDispatcher{}
	uow := NewUnitOfWork(eventStore, eventDispatcher)

	// Register event
	event := &testEvent{
		eventType:   "TestEvent",
		aggregateID: "aggregate-1",
		version:     1,
		occurredAt:  time.Now(),
	}

	uow.RegisterEvents([]domain.Event{event})

	// First commit should succeed
	ctx := context.Background()
	_, err := uow.Commit(ctx)
	if err != nil {
		t.Fatalf("First commit failed: %v", err)
	}

	// Second commit should fail
	_, err = uow.Commit(ctx)
	if err == nil {
		t.Fatal("Expected second commit to fail")
	}
}

func TestUnitOfWork_RegisterAfterCommit(t *testing.T) {
	eventStore := &mockEventStore{}
	eventDispatcher := &mockEventDispatcher{}
	uow := NewUnitOfWork(eventStore, eventDispatcher)

	// Commit empty unit of work
	ctx := context.Background()
	_, err := uow.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Registering events after commit should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when registering events after commit")
		}
	}()

	event := &testEvent{
		eventType:   "TestEvent",
		aggregateID: "aggregate-1",
		version:     1,
		occurredAt:  time.Now(),
	}

	uow.RegisterEvents([]domain.Event{event})
}

func TestUnitOfWork_RollbackAfterCommit(t *testing.T) {
	eventStore := &mockEventStore{}
	eventDispatcher := &mockEventDispatcher{}
	uow := NewUnitOfWork(eventStore, eventDispatcher)

	// Commit empty unit of work
	ctx := context.Background()
	_, err := uow.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Rollback after commit should fail
	err = uow.Rollback()
	if err == nil {
		t.Fatal("Expected rollback after commit to fail")
	}
}
