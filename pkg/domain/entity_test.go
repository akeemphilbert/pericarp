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
	data        string
}

func (e TestEvent) EventType() string               { return e.eventType }
func (e TestEvent) AggregateID() string             { return e.aggregateID }
func (e TestEvent) SequenceNo() int64               { return e.sequenceNo }
func (e TestEvent) CreatedAt() time.Time            { return e.createdAt }
func (e TestEvent) User() string                    { return e.userID }
func (e TestEvent) Account() string                 { return e.accountID }
func (e TestEvent) Payload() any                    { return e.data }
func (e *TestEvent) SetSequenceNo(sequenceNo int64) { e.sequenceNo = sequenceNo }

func TestEvent_SetSequenceNo(t *testing.T) {
	event := &TestEvent{
		eventType:   "TestEvent",
		aggregateID: "test-123",
		sequenceNo:  0,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        "test data",
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
		data:        "test data",
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
		t.Errorf("Expected version 0, got %d", entity.SequenceNo())
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
		data:        "test data 1",
	}

	entity.AddEvent(event1)

	if entity.SequenceNo() != 1 {
		t.Errorf("Expected version 1, got %d", entity.SequenceNo())
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
		data:        "test data 2",
	}

	entity.AddEvent(event2)

	if entity.SequenceNo() != 2 {
		t.Errorf("Expected version 2, got %d", entity.SequenceNo())
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
		data:        "test data 1",
	}

	event2 := &TestEvent{
		eventType:   "TestEvent2",
		aggregateID: "test-123",
		sequenceNo:  2,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        "test data 2",
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
		data:        "modified data",
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
		data:        "test data",
	}

	entity.AddEvent(event)

	// Verify event is uncommitted
	if !entity.HasUncommittedEvents() {
		t.Error("Entity should have uncommitted events before marking as committed")
	}

	entity.MarkEventsAsCommitted()

	// Verify events are cleared
	if entity.HasUncommittedEvents() {
		t.Error("Entity should not have uncommitted events after marking as committed")
	}

	if entity.UncommittedEventCount() != 0 {
		t.Errorf("Expected 0 uncommitted events, got %d", entity.UncommittedEventCount())
	}

	// Verify version and sequence are preserved
	if entity.SequenceNo() != 1 {
		t.Errorf("Expected version 1, got %d", entity.SequenceNo())
	}

	if entity.SequenceNo() != 1 {
		t.Errorf("Expected sequence number 1, got %d", entity.SequenceNo())
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
			data:        "test data 1",
		},
		&TestEvent{
			eventType:   "TestEvent2",
			aggregateID: "test-123",
			sequenceNo:  2,
			createdAt:   time.Now(),
			userID:      "test-user",
			accountID:   "test-account",
			data:        "test data 2",
		},
		&TestEvent{
			eventType:   "TestEvent3",
			aggregateID: "test-123",
			sequenceNo:  3,
			createdAt:   time.Now(),
			userID:      "test-user",
			accountID:   "test-account",
			data:        "test data 3",
		},
	}

	entity.LoadFromHistory(events)

	if entity.SequenceNo() != 3 {
		t.Errorf("Expected version 3, got %d", entity.SequenceNo())
	}

	if entity.SequenceNo() != 3 {
		t.Errorf("Expected sequence number 3, got %d", entity.SequenceNo())
	}

	// Should not have uncommitted events after loading from history
	if entity.HasUncommittedEvents() {
		t.Error("Entity should not have uncommitted events after loading from history")
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
		data:        "test data",
	}
	entity.AddEvent(event)

	// Verify we have state before loading empty history
	if entity.SequenceNo() != 1 {
		t.Errorf("Expected version 1 before loading empty history, got %d", entity.SequenceNo())
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
		data:        "test data 1",
	}

	event2 := &TestEvent{
		eventType:   "TestEvent2",
		aggregateID: "test-123",
		sequenceNo:  2,
		createdAt:   time.Now(),
		userID:      "test-user",
		accountID:   "test-account",
		data:        "test data 2",
	}

	entity.AddEvent(event1)
	entity.AddEvent(event2)

	// Verify state before reset
	if entity.SequenceNo() != 2 {
		t.Errorf("Expected version 2 before reset, got %d", entity.SequenceNo())
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
		t.Errorf("Expected version 0 after reset, got %d", entity.SequenceNo())
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
		data:        "test data",
	}

	entity.AddEvent(event)

	clone := entity.Clone()

	// Verify clone has same state
	if clone.ID() != entity.ID() {
		t.Errorf("Expected clone ID %s, got %s", entity.ID(), clone.ID())
	}

	if clone.SequenceNo() != entity.SequenceNo() {
		t.Errorf("Expected clone version %d, got %d", entity.SequenceNo(), clone.SequenceNo())
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
		data:        "clone data",
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
		data:        "test data",
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
					data:        fmt.Sprintf("goroutine-%d-event-%d", goroutineID, j),
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
		t.Errorf("Expected version %d, got %d", expectedEventCount, entity.SequenceNo())
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
					data:        "concurrent data",
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
		data:        "benchmark data",
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
			data:        fmt.Sprintf("benchmark data %d", i),
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
			data:        fmt.Sprintf("benchmark data %d", i),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		entity := NewEntity("bench-test")
		entity.LoadFromHistory(events)
	}
}
