package domain

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestEvent is a simple event implementation for testing
type TestEvent struct {
	eventType   string
	aggregateID string
	sequenceNo  int64
	createdAt   time.Time
	userID      string
	accountID   string
	data        []byte
}

func (e TestEvent) EventType() string               { return e.eventType }
func (e TestEvent) AggregateID() string             { return e.aggregateID }
func (e TestEvent) SequenceNo() int64               { return e.sequenceNo }
func (e TestEvent) CreatedAt() time.Time            { return e.createdAt }
func (e TestEvent) User() string                    { return e.userID }
func (e TestEvent) Account() string                 { return e.accountID }
func (e TestEvent) Payload() []byte                 { return e.data }
func (e *TestEvent) SetSequenceNo(sequenceNo int64) { e.sequenceNo = sequenceNo }

func TestEvent_SetSequenceNo(t *testing.T) {
	event := &TestEvent{
		eventType:   "TestEvent",
		aggregateID: "test-123",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data"),
	}

	// Initially sequence number should be 0
	if event.SequenceNo() != 0 {
		t.Errorf("Expected initial sequence number 0, got %d", event.SequenceNo())
	}

	// Set sequence number
	event.SetSequenceNo(42)

	// Verify sequence number was set
	if event.SequenceNo() != 42 {
		t.Errorf("Expected sequence number 42, got %d", event.SequenceNo())
	}

	// Test with Event interface
	var e Event = event
	e.SetSequenceNo(99)
	if e.SequenceNo() != 99 {
		t.Errorf("Expected sequence number 99 via interface, got %d", e.SequenceNo())
	}
}

func TestEntity_AddEvent_SetSequenceNo(t *testing.T) {
	entity := NewEntity("test-123")

	event := &TestEvent{
		eventType:   "TestEvent",
		aggregateID: "test-123",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data"),
	}

	// Verify initial state
	if event.SequenceNo() != 0 {
		t.Errorf("Expected initial sequence number 0, got %d", event.SequenceNo())
	}

	// Add event to entity
	entity.AddEvent(event)

	// Verify sequence number was set
	if event.SequenceNo() != 1 {
		t.Errorf("Expected sequence number 1 after AddEvent, got %d", event.SequenceNo())
	}

	// Verify entity state
	if entity.SequenceNo() != 1 {
		t.Errorf("Expected entity sequence number 1, got %d", entity.SequenceNo())
	}
}

func TestNewEntity(t *testing.T) {
	id := "test-entity-123"
	entity := NewEntity(id)

	if entity.ID() != id {
		t.Errorf("Expected ID %s, got %s", id, entity.ID())
	}

	if entity.SequenceNo() != 0 {
		t.Errorf("Expected sequence number 0, got %d", entity.SequenceNo())
	}

	if entity.SequenceNo() != 0 {
		t.Errorf("Expected sequence number 0, got %d", entity.SequenceNo())
	}

	if entity.HasUncommittedEvents() {
		t.Error("New entity should not have uncommitted events")
	}

	if entity.UncommittedEventCount() != 0 {
		t.Errorf("Expected 0 uncommitted events, got %d", entity.UncommittedEventCount())
	}
}

func TestEntity_AddEvent(t *testing.T) {
	entity := NewEntity("test-123")

	event1 := &TestEvent{
		eventType:   "TestEvent1",
		aggregateID: "test-123",
		sequenceNo:  0, // Should be set by AddEvent
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 1"),
	}

	entity.AddEvent(event1)

	if entity.SequenceNo() != 1 {
		t.Errorf("Expected sequence number 1, got %d", entity.SequenceNo())
	}

	if entity.SequenceNo() != 1 {
		t.Errorf("Expected sequence number 1, got %d", entity.SequenceNo())
	}

	if !entity.HasUncommittedEvents() {
		t.Error("Entity should have uncommitted events")
	}

	if entity.UncommittedEventCount() != 1 {
		t.Errorf("Expected 1 uncommitted event, got %d", entity.UncommittedEventCount())
	}

	// Add second event
	event2 := &TestEvent{
		eventType:   "TestEvent2",
		aggregateID: "test-123",
		sequenceNo:  0, // Should be set by AddEvent
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 2"),
	}

	entity.AddEvent(event2)

	if entity.SequenceNo() != 2 {
		t.Errorf("Expected sequence number 2, got %d", entity.SequenceNo())
	}

	if entity.SequenceNo() != 2 {
		t.Errorf("Expected sequence number 2, got %d", entity.SequenceNo())
	}

	// Verify that events have their sequence numbers set correctly
	if event1.SequenceNo() != 1 {
		t.Errorf("Expected event1 sequence number 1, got %d", event1.SequenceNo())
	}

	if event2.SequenceNo() != 2 {
		t.Errorf("Expected event2 sequence number 2, got %d", event2.SequenceNo())
	}

	if entity.UncommittedEventCount() != 2 {
		t.Errorf("Expected 2 uncommitted events, got %d", entity.UncommittedEventCount())
	}
}

func TestEntity_UncommittedEvents(t *testing.T) {
	entity := NewEntity("test-123")

	event1 := &TestEvent{
		eventType:   "TestEvent1",
		aggregateID: "test-123",
		sequenceNo:  1,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 1"),
	}

	event2 := &TestEvent{
		eventType:   "TestEvent2",
		aggregateID: "test-123",
		sequenceNo:  2,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 2"),
	}

	entity.AddEvent(event1)
	entity.AddEvent(event2)

	events := entity.UncommittedEvents()

	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	// Verify events are returned in order
	if events[0].EventType() != "TestEvent1" {
		t.Errorf("Expected first event type TestEvent1, got %s", events[0].EventType())
	}

	if events[1].EventType() != "TestEvent2" {
		t.Errorf("Expected second event type TestEvent2, got %s", events[1].EventType())
	}

	// Verify that modifying returned slice doesn't affect entity
	events[0] = &TestEvent{
		eventType:   "Modified",
		aggregateID: "test-123",
		sequenceNo:  1,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("modified data"),
	}
	originalEvents := entity.UncommittedEvents()
	if originalEvents[0].EventType() == "Modified" {
		t.Error("Modifying returned events slice should not affect entity")
	}
}

func TestEntity_MarkEventsAsCommitted(t *testing.T) {
	entity := NewEntity("test-123")

	event := &TestEvent{
		eventType:   "TestEvent",
		aggregateID: "test-123",
		sequenceNo:  1,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data"),
	}

	entity.AddEvent(event)

	// Verify event is uncommitted
	if !entity.HasUncommittedEvents() {
		t.Error("Entity should have uncommitted events before marking as committed")
	}

	// Verify uncommitted events are returned correctly
	uncommittedEvents := entity.UncommittedEvents()
	if len(uncommittedEvents) != 1 {
		t.Errorf("Expected 1 uncommitted event, got %d", len(uncommittedEvents))
	}

	entity.MarkEventsAsCommitted()

	// Verify uncommitted events are cleared
	if entity.HasUncommittedEvents() {
		t.Error("Entity should not have uncommitted events after marking as committed")
	}

	if entity.UncommittedEventCount() != 0 {
		t.Errorf("Expected 0 uncommitted events, got %d", entity.UncommittedEventCount())
	}

	// Verify events are now in the committed events (internal events slice)
	// We can't directly access the events slice, but we can verify through behavior
	// when we add a new event after committing
	newEvent := &TestEvent{
		eventType:   "TestEvent2",
		aggregateID: "test-123",
		sequenceNo:  2,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 2"),
	}

	entity.AddEvent(newEvent)

	// Should now have 1 uncommitted event (the new one)
	if entity.UncommittedEventCount() != 1 {
		t.Errorf("Expected 1 uncommitted event after adding new event, got %d", entity.UncommittedEventCount())
	}

	// Verify sequence number is preserved and incremented
	if entity.SequenceNo() != 2 {
		t.Errorf("Expected sequence number 2, got %d", entity.SequenceNo())
	}
}

func TestEntity_CommittedVsUncommittedEvents(t *testing.T) {
	entity := NewEntity("test-123")

	// Add first event
	event1 := &TestEvent{
		eventType:   "TestEvent1",
		aggregateID: "test-123",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 1"),
	}

	entity.AddEvent(event1)

	// Should have 1 uncommitted event
	if entity.UncommittedEventCount() != 1 {
		t.Errorf("Expected 1 uncommitted event, got %d", entity.UncommittedEventCount())
	}

	// Commit the events
	entity.MarkEventsAsCommitted()

	// Should have 0 uncommitted events
	if entity.UncommittedEventCount() != 0 {
		t.Errorf("Expected 0 uncommitted events after commit, got %d", entity.UncommittedEventCount())
	}

	// Add second event
	event2 := &TestEvent{
		eventType:   "TestEvent2",
		aggregateID: "test-123",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 2"),
	}

	entity.AddEvent(event2)

	// Should have 1 uncommitted event
	if entity.UncommittedEventCount() != 1 {
		t.Errorf("Expected 1 uncommitted event after adding second event, got %d", entity.UncommittedEventCount())
	}

	// Verify the uncommitted event is the second one
	uncommittedEvents := entity.UncommittedEvents()
	if len(uncommittedEvents) != 1 {
		t.Errorf("Expected 1 uncommitted event in slice, got %d", len(uncommittedEvents))
	}

	if uncommittedEvents[0].EventType() != "TestEvent2" {
		t.Errorf("Expected uncommitted event to be TestEvent2, got %s", uncommittedEvents[0].EventType())
	}

	// Verify sequence number is 2
	if entity.SequenceNo() != 2 {
		t.Errorf("Expected sequence number 2, got %d", entity.SequenceNo())
	}

	// Commit the second event
	entity.MarkEventsAsCommitted()

	// Should have 0 uncommitted events
	if entity.UncommittedEventCount() != 0 {
		t.Errorf("Expected 0 uncommitted events after second commit, got %d", entity.UncommittedEventCount())
	}
}

func TestEntity_MergeEventsFrom(t *testing.T) {
	entity1 := NewEntity("entity-1")
	entity2 := NewEntity("entity-2")

	// Add and commit an event to entity1
	event1 := &TestEvent{
		eventType:   "TestEvent1",
		aggregateID: "entity-1",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 1"),
	}

	entity1.AddEvent(event1)
	entity1.MarkEventsAsCommitted()

	// Add another uncommitted event to entity1
	event2 := &TestEvent{
		eventType:   "TestEvent2",
		aggregateID: "entity-1",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 2"),
	}

	entity1.AddEvent(event2)

	// Add events to entity2 (some committed, some uncommitted)
	event3 := &TestEvent{
		eventType:   "TestEvent3",
		aggregateID: "entity-2",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 3"),
	}

	event4 := &TestEvent{
		eventType:   "TestEvent4",
		aggregateID: "entity-2",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 4"),
	}

	event5 := &TestEvent{
		eventType:   "TestEvent5",
		aggregateID: "entity-2",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 5"),
	}

	entity2.AddEvent(event3)
	entity2.MarkEventsAsCommitted() // event3 is now committed
	entity2.AddEvent(event4)        // event4 is uncommitted
	entity2.AddEvent(event5)        // event5 is uncommitted

	// Verify initial state
	if entity1.UncommittedEventCount() != 1 {
		t.Errorf("Expected entity1 to have 1 uncommitted event, got %d", entity1.UncommittedEventCount())
	}

	if entity2.UncommittedEventCount() != 2 {
		t.Errorf("Expected entity2 to have 2 uncommitted events, got %d", entity2.UncommittedEventCount())
	}

	if entity1.SequenceNo() != 2 {
		t.Errorf("Expected entity1 sequence number 2, got %d", entity1.SequenceNo())
	}

	if entity2.SequenceNo() != 3 {
		t.Errorf("Expected entity2 sequence number 3, got %d", entity2.SequenceNo())
	}

	// Merge uncommitted events from entity2 into entity1
	err := entity1.MergeEventsFrom(entity2)
	if err != nil {
		t.Errorf("MergeEventsFrom returned unexpected error: %v", err)
	}

	// Verify entity1 after merge
	if entity1.UncommittedEventCount() != 3 {
		t.Errorf("Expected entity1 to have 3 uncommitted events after merge, got %d", entity1.UncommittedEventCount())
	}

	// Verify entity1's sequence number remains unchanged (merge doesn't affect it)
	if entity1.SequenceNo() != 2 {
		t.Errorf("Expected entity1 sequence number to remain 2 after merge, got %d", entity1.SequenceNo())
	}

	// Verify entity2 is unchanged
	if entity2.UncommittedEventCount() != 2 {
		t.Errorf("Expected entity2 to still have 2 uncommitted events, got %d", entity2.UncommittedEventCount())
	}

	if entity2.SequenceNo() != 3 {
		t.Errorf("Expected entity2 sequence number to remain 3, got %d", entity2.SequenceNo())
	}

	// Verify the events in entity1 preserve their original sequence numbers
	uncommittedEvents := entity1.UncommittedEvents()
	if len(uncommittedEvents) != 3 {
		t.Errorf("Expected 3 uncommitted events, got %d", len(uncommittedEvents))
	}

	// First event should be event2 with original sequence 2
	if uncommittedEvents[0].EventType() != "TestEvent2" || uncommittedEvents[0].SequenceNo() != 2 {
		t.Errorf("Expected first event to be TestEvent2 with sequence 2, got %s with sequence %d",
			uncommittedEvents[0].EventType(), uncommittedEvents[0].SequenceNo())
	}

	// Second event should be event4 with original sequence 2 (from entity2)
	if uncommittedEvents[1].EventType() != "TestEvent4" || uncommittedEvents[1].SequenceNo() != 2 {
		t.Errorf("Expected second event to be TestEvent4 with sequence 2, got %s with sequence %d",
			uncommittedEvents[1].EventType(), uncommittedEvents[1].SequenceNo())
	}

	// Third event should be event5 with original sequence 3 (from entity2)
	if uncommittedEvents[2].EventType() != "TestEvent5" || uncommittedEvents[2].SequenceNo() != 3 {
		t.Errorf("Expected third event to be TestEvent5 with sequence 3, got %s with sequence %d",
			uncommittedEvents[2].EventType(), uncommittedEvents[2].SequenceNo())
	}
}

func TestEntity_MergeEventsFrom_NilSource(t *testing.T) {
	entity := NewEntity("test-123")

	err := entity.MergeEventsFrom(nil)
	if err == nil {
		t.Error("Expected error when merging from nil source, got nil")
	}

	if err.Error() != "source entity cannot be nil" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

// MockEntity implements Entity interface but is not a BasicEntity
type MockEntity struct{}

func (m *MockEntity) ID() string                          { return "mock-id" }
func (m *MockEntity) SequenceNo() int64                   { return 0 }
func (m *MockEntity) UncommittedEvents() []Event          { return []Event{} }
func (m *MockEntity) MarkEventsAsCommitted()              {}
func (m *MockEntity) LoadFromHistory(events []Event)      {}
func (m *MockEntity) AddEvent(event Event)                {}
func (m *MockEntity) HasUncommittedEvents() bool          { return false }
func (m *MockEntity) UncommittedEventCount() int          { return 0 }
func (m *MockEntity) MergeEventsFrom(source Entity) error { return nil }
func (m *MockEntity) AddError(err error)                  {}
func (m *MockEntity) Errors() []error                     { return []error{} }
func (m *MockEntity) IsValid() bool                       { return true }
func (m *MockEntity) Reset()                              {}
func (m *MockEntity) Clone() Entity                       { return &MockEntity{} }
func (m *MockEntity) String() string                      { return "MockEntity{}" }

func TestEntity_MergeEventsFrom_InvalidType(t *testing.T) {
	entity := NewEntity("test-123")

	// Create a mock Entity that's not a BasicEntity
	mockEntity := &MockEntity{}

	err := entity.MergeEventsFrom(mockEntity)
	if err == nil {
		t.Error("Expected error when merging from non-BasicEntity source, got nil")
	}

	if err.Error() != "source entity must be a BasicEntity" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestEntity_MergeEventsFrom_EmptySource(t *testing.T) {
	entity1 := NewEntity("entity-1")
	entity2 := NewEntity("entity-2")

	// Add an event to entity1
	event1 := &TestEvent{
		eventType:   "TestEvent1",
		aggregateID: "entity-1",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 1"),
	}

	entity1.AddEvent(event1)

	// entity2 has no uncommitted events
	if entity2.HasUncommittedEvents() {
		t.Error("entity2 should have no uncommitted events")
	}

	// Merge from empty entity2
	err := entity1.MergeEventsFrom(entity2)
	if err != nil {
		t.Errorf("MergeEventsFrom returned unexpected error: %v", err)
	}

	// entity1 should be unchanged
	if entity1.UncommittedEventCount() != 1 {
		t.Errorf("Expected entity1 to still have 1 uncommitted event, got %d", entity1.UncommittedEventCount())
	}

	if entity1.SequenceNo() != 1 {
		t.Errorf("Expected entity1 sequence number to remain 1, got %d", entity1.SequenceNo())
	}
}

func TestEntity_MergeEventsFrom_OnlyCommittedEventsInSource(t *testing.T) {
	entity1 := NewEntity("entity-1")
	entity2 := NewEntity("entity-2")

	// Add event to entity2 and commit it
	event1 := &TestEvent{
		eventType:   "TestEvent1",
		aggregateID: "entity-2",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 1"),
	}

	entity2.AddEvent(event1)
	entity2.MarkEventsAsCommitted()

	// entity2 should have no uncommitted events
	if entity2.HasUncommittedEvents() {
		t.Error("entity2 should have no uncommitted events after commit")
	}

	// Merge from entity2 (which has only committed events)
	err := entity1.MergeEventsFrom(entity2)
	if err != nil {
		t.Errorf("MergeEventsFrom returned unexpected error: %v", err)
	}

	// entity1 should be unchanged
	if entity1.HasUncommittedEvents() {
		t.Error("entity1 should have no uncommitted events after merge")
	}

	if entity1.SequenceNo() != 0 {
		t.Errorf("Expected entity1 sequence number to remain 0, got %d", entity1.SequenceNo())
	}
}

func TestEntity_MergeEventsFrom_IntoEmptyEntity(t *testing.T) {
	entity1 := NewEntity("entity-1")
	entity2 := NewEntity("entity-2")

	// Add uncommitted events to entity2
	event1 := &TestEvent{
		eventType:   "TestEvent1",
		aggregateID: "entity-2",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 1"),
	}

	event2 := &TestEvent{
		eventType:   "TestEvent2",
		aggregateID: "entity-2",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 2"),
	}

	entity2.AddEvent(event1)
	entity2.AddEvent(event2)

	// entity1 is empty
	if entity1.HasUncommittedEvents() {
		t.Error("entity1 should be empty")
	}

	// Merge into empty entity1
	err := entity1.MergeEventsFrom(entity2)
	if err != nil {
		t.Errorf("MergeEventsFrom returned unexpected error: %v", err)
	}

	// Verify entity1 after merge
	if entity1.UncommittedEventCount() != 2 {
		t.Errorf("Expected entity1 to have 2 uncommitted events, got %d", entity1.UncommittedEventCount())
	}

	// entity1's sequence number should remain 0 (no events added to entity1)
	if entity1.SequenceNo() != 0 {
		t.Errorf("Expected entity1 sequence number to remain 0, got %d", entity1.SequenceNo())
	}

	// Verify events preserve their original sequence numbers from entity2
	uncommittedEvents := entity1.UncommittedEvents()
	if uncommittedEvents[0].SequenceNo() != 1 {
		t.Errorf("Expected first event sequence number 1 (original from entity2), got %d", uncommittedEvents[0].SequenceNo())
	}

	if uncommittedEvents[1].SequenceNo() != 2 {
		t.Errorf("Expected second event sequence number 2 (original from entity2), got %d", uncommittedEvents[1].SequenceNo())
	}
}

func TestEntity_LoadFromHistory(t *testing.T) {
	entity := NewEntity("test-123")

	events := []Event{
		&TestEvent{
			eventType:   "TestEvent1",
			aggregateID: "test-123",
			sequenceNo:  1,
			createdAt:   time.Now(),
			userID:      "test-user",
			accountID:   "test-account",
			data:        []byte("test data 1"),
		},
		&TestEvent{
			eventType:   "TestEvent2",
			aggregateID: "test-123",
			sequenceNo:  2,
			createdAt:   time.Now(),
			userID:      "test-user",
			accountID:   "test-account",
			data:        []byte("test data 2"),
		},
		&TestEvent{
			eventType:   "TestEvent3",
			aggregateID: "test-123",
			sequenceNo:  3,
			createdAt:   time.Now(),
			userID:      "test-user",
			accountID:   "test-account",
			data:        []byte("test data 3"),
		},
	}

	entity.LoadFromHistory(events)

	if entity.SequenceNo() != 3 {
		t.Errorf("Expected sequence number 3, got %d", entity.SequenceNo())
	}

	// Should not have uncommitted events after loading from history
	if entity.HasUncommittedEvents() {
		t.Error("Entity should not have uncommitted events after loading from history")
	}

	if entity.UncommittedEventCount() != 0 {
		t.Errorf("Expected 0 uncommitted events after loading from history, got %d", entity.UncommittedEventCount())
	}

	// Verify that we can still add new events after loading from history
	newEvent := &TestEvent{
		eventType:   "NewEvent",
		aggregateID: "test-123",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("new event data"),
	}

	entity.AddEvent(newEvent)

	// Should now have 1 uncommitted event
	if entity.UncommittedEventCount() != 1 {
		t.Errorf("Expected 1 uncommitted event after adding new event, got %d", entity.UncommittedEventCount())
	}

	// Sequence number should be 4
	if entity.SequenceNo() != 4 {
		t.Errorf("Expected sequence number 4 after adding new event, got %d", entity.SequenceNo())
	}
}

func TestEntity_LoadFromHistoryEmptyEvents(t *testing.T) {
	entity := NewEntity("test-123")

	// Add an event first to have some state
	event := &TestEvent{
		eventType:   "TestEvent",
		aggregateID: "test-123",
		sequenceNo:  1,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data"),
	}
	entity.AddEvent(event)

	// Verify we have state before loading empty history
	if entity.SequenceNo() != 1 {
		t.Errorf("Expected sequence number 1 before loading empty history, got %d", entity.SequenceNo())
	}

	// Load empty history - this should reset the entity to initial state
	entity.LoadFromHistory([]Event{})

	if entity.SequenceNo() != 0 {
		t.Errorf("Expected sequence number 0 after loading empty history, got %d", entity.SequenceNo())
	}

	if entity.HasUncommittedEvents() {
		t.Error("Entity should not have uncommitted events after loading empty history")
	}
}

func TestEntity_Reset(t *testing.T) {
	entity := NewEntity("test-123")

	// Add some events
	event1 := &TestEvent{
		eventType:   "TestEvent1",
		aggregateID: "test-123",
		sequenceNo:  1,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 1"),
	}

	event2 := &TestEvent{
		eventType:   "TestEvent2",
		aggregateID: "test-123",
		sequenceNo:  2,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data 2"),
	}

	entity.AddEvent(event1)
	entity.AddEvent(event2)

	// Verify state before reset
	if entity.SequenceNo() != 2 {
		t.Errorf("Expected sequence number 2 before reset, got %d", entity.SequenceNo())
	}

	if entity.SequenceNo() != 2 {
		t.Errorf("Expected sequence number 2 before reset, got %d", entity.SequenceNo())
	}

	if !entity.HasUncommittedEvents() {
		t.Error("Entity should have uncommitted events before reset")
	}

	// Reset the entity
	entity.Reset()

	// Verify state after reset
	if entity.SequenceNo() != 0 {
		t.Errorf("Expected sequence number 0 after reset, got %d", entity.SequenceNo())
	}

	if entity.SequenceNo() != 0 {
		t.Errorf("Expected sequence number 0 after reset, got %d", entity.SequenceNo())
	}

	if entity.HasUncommittedEvents() {
		t.Error("Entity should not have uncommitted events after reset")
	}

	// ID should be preserved
	if entity.ID() != "test-123" {
		t.Errorf("Expected ID to be preserved after reset, got %s", entity.ID())
	}
}

func TestEntity_Clone(t *testing.T) {
	entity := NewEntity("test-123")

	event := &TestEvent{
		eventType:   "TestEvent",
		aggregateID: "test-123",
		sequenceNo:  1,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data"),
	}

	entity.AddEvent(event)

	clone := entity.Clone()

	// Verify clone has same state
	if clone.ID() != entity.ID() {
		t.Errorf("Expected clone ID %s, got %s", entity.ID(), clone.ID())
	}

	if clone.SequenceNo() != entity.SequenceNo() {
		t.Errorf("Expected clone sequence number %d, got %d", entity.SequenceNo(), clone.SequenceNo())
	}

	if clone.UncommittedEventCount() != entity.UncommittedEventCount() {
		t.Errorf("Expected clone event count %d, got %d", entity.UncommittedEventCount(), clone.UncommittedEventCount())
	}

	// Verify independence - modifying clone shouldn't affect original
	clone.AddEvent(&TestEvent{
		eventType:   "CloneEvent",
		aggregateID: "test-123",
		sequenceNo:  2,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("clone data"),
	})

	if entity.UncommittedEventCount() == clone.UncommittedEventCount() {
		t.Error("Clone and original should be independent")
	}
}

func TestEntity_String(t *testing.T) {
	entity := NewEntity("test-123")

	event := &TestEvent{
		eventType:   "TestEvent",
		aggregateID: "test-123",
		sequenceNo:  1,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("test data"),
	}

	entity.AddEvent(event)

	str := entity.String()
	expected := "Entity{ID: test-123, SequenceNo: 1, UncommittedEvents: 1, Errors: 0}"

	if str != expected {
		t.Errorf("Expected string %s, got %s", expected, str)
	}
}

func TestEntity_ConcurrentAccess(t *testing.T) {
	entity := NewEntity("test-123")
	numGoroutines := 100
	eventsPerGoroutine := 10

	var wg sync.WaitGroup

	// Concurrent event addition
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < eventsPerGoroutine; j++ {
				event := &TestEvent{
					eventType:   "ConcurrentEvent",
					aggregateID: "test-123",
					sequenceNo:  int64(goroutineID*eventsPerGoroutine + j + 1),
					createdAt:   time.Now(),
					userID:      "test-user",
					accountID:   "test-account",
					data:        []byte(fmt.Sprintf("goroutine-%d-event-%d", goroutineID, j)),
				}

				entity.AddEvent(event)
			}
		}(i)
	}

	wg.Wait()

	expectedEventCount := numGoroutines * eventsPerGoroutine
	if entity.UncommittedEventCount() != int(expectedEventCount) {
		t.Errorf("Expected %d events, got %d", expectedEventCount, entity.UncommittedEventCount())
	}

	if entity.SequenceNo() != int64(expectedEventCount) {
		t.Errorf("Expected sequence number %d, got %d", expectedEventCount, entity.SequenceNo())
	}

	if entity.SequenceNo() != int64(expectedEventCount) {
		t.Errorf("Expected sequence number %d, got %d", expectedEventCount, entity.SequenceNo())
	}
}

func TestEntity_ConcurrentReadWrite(t *testing.T) {
	entity := NewEntity("test-123")
	duration := 100 * time.Millisecond
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				event := &TestEvent{
					eventType:   "ConcurrentEvent",
					aggregateID: "test-123",
					sequenceNo:  1,
					createdAt:   time.Now(),
					userID:      "test-user",
					accountID:   "test-account",
					data:        []byte("concurrent data"),
				}
				entity.AddEvent(event)
				entity.MarkEventsAsCommitted()
			}
		}
	}()

	// Reader goroutines
	for i := 0; i < 5; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					_ = entity.ID()
					_ = entity.SequenceNo()
					_ = entity.HasUncommittedEvents()
					_ = entity.UncommittedEvents()
				}
			}
		}()
	}

	time.Sleep(duration)
	close(done)

	// Test should complete without race conditions or deadlocks
}

// Benchmark tests
func BenchmarkEntity_AddEvent(b *testing.B) {
	entity := NewEntity("bench-test")
	event := &TestEvent{
		eventType:   "BenchEvent",
		aggregateID: "bench-test",
		sequenceNo:  1,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        []byte("benchmark data"),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		entity.AddEvent(event)
	}
}

func BenchmarkEntity_UncommittedEvents(b *testing.B) {
	entity := NewEntity("bench-test")

	// Add some events
	for i := 0; i < 100; i++ {
		event := &TestEvent{
			eventType:   "BenchEvent",
			aggregateID: "bench-test",
			sequenceNo:  int64(i + 1),
			createdAt:   time.Now(),
			userID:      "test-user",
			accountID:   "test-account",
			data:        []byte(fmt.Sprintf("benchmark data %d", i)),
		}
		entity.AddEvent(event)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = entity.UncommittedEvents()
	}
}

func BenchmarkEntity_LoadFromHistory(b *testing.B) {
	events := make([]Event, 1000)
	for i := 0; i < 1000; i++ {
		events[i] = &TestEvent{
			eventType:   "BenchEvent",
			aggregateID: "bench-test",
			sequenceNo:  int64(i + 1),
			createdAt:   time.Now(),
			userID:      "test-user",
			accountID:   "test-account",
			data:        []byte(fmt.Sprintf("benchmark data %d", i)),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		entity := NewEntity("bench-test")
		entity.LoadFromHistory(events)
	}
}
