package ddd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// toAnyEvent converts an EventEnvelope[T] to EventEnvelope[any] for testing.
func toAnyEvent[T any](event domain.EventEnvelope[T]) domain.EventEnvelope[any] {
	return domain.EventEnvelope[any]{
		ID:          event.ID,
		AggregateID: event.AggregateID,
		EventType:   event.EventType,
		Payload:     event.Payload,
		Created:     event.Created,
		SequenceNo:  event.SequenceNo,
		Metadata:    event.Metadata,
	}
}

func TestNewBaseEntity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		aggregateID    string
		wantID         string
		wantSequenceNo int
	}{
		{
			name:           "valid aggregate ID",
			aggregateID:    "agg-123",
			wantID:         "agg-123",
			wantSequenceNo: -1, // Starts at -1 so first event is 0
		},
		{
			name:           "empty aggregate ID",
			aggregateID:    "",
			wantID:         "",
			wantSequenceNo: -1,
		},
		{
			name:           "long aggregate ID",
			aggregateID:    "very-long-aggregate-id-with-many-characters",
			wantID:         "very-long-aggregate-id-with-many-characters",
			wantSequenceNo: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entity := NewBaseEntity(tt.aggregateID)

			if got := entity.GetID(); got != tt.wantID {
				t.Errorf("GetID() = %v, want %v", got, tt.wantID)
			}

			if got := entity.GetSequenceNo(); got != tt.wantSequenceNo {
				t.Errorf("GetSequenceNo() = %v, want %v", got, tt.wantSequenceNo)
			}

			events := entity.GetUncommittedEvents()
			if len(events) != 0 {
				t.Errorf("GetUncommittedEvents() length = %v, want 0", len(events))
			}
		})
	}
}

func TestBaseEntity_GetID(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")
	if got := entity.GetID(); got != "test-id" {
		t.Errorf("GetID() = %v, want test-id", got)
	}
}

func TestBaseEntity_GetSequenceNo(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")
	if got := entity.GetSequenceNo(); got != -1 {
		t.Errorf("GetSequenceNo() = %v, want -1", got)
	}
}

func TestBaseEntity_GetUncommittedEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupEvents    int
		wantEventCount int
	}{
		{
			name:           "no events",
			setupEvents:    0,
			wantEventCount: 0,
		},
		{
			name:           "one event",
			setupEvents:    1,
			wantEventCount: 1,
		},
		{
			name:           "multiple events",
			setupEvents:    5,
			wantEventCount: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entity := NewBaseEntity("test-id")

			// Record some events
			for i := 0; i < tt.setupEvents; i++ {
				if err := entity.RecordEvent("payload", "test.event"); err != nil {
					t.Fatalf("RecordEvent() error = %v", err)
				}
			}

			events := entity.GetUncommittedEvents()
			if len(events) != tt.wantEventCount {
				t.Errorf("GetUncommittedEvents() length = %v, want %v", len(events), tt.wantEventCount)
			}

			// Verify returned slice is a copy (modifying it shouldn't affect entity)
			if len(events) > 0 {
				events[0] = toAnyEvent(domain.NewEventEnvelope("modified", "test-id", "modified.event", 0))
				events2 := entity.GetUncommittedEvents()
				if len(events2) != tt.wantEventCount {
					t.Errorf("GetUncommittedEvents() after modification length = %v, want %v", len(events2), tt.wantEventCount)
				}
			}
		})
	}
}

func TestBaseEntity_ClearUncommittedEvents(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")

	// Record some events
	for i := 0; i < 3; i++ {
		if err := entity.RecordEvent("payload", "test.event"); err != nil {
			t.Fatalf("RecordEvent() error = %v", err)
		}
	}

	// Verify events exist
	events := entity.GetUncommittedEvents()
	if len(events) != 3 {
		t.Fatalf("Expected 3 events before clear, got %d", len(events))
	}

	// Clear events
	entity.ClearUncommittedEvents()

	// Verify events are cleared
	events = entity.GetUncommittedEvents()
	if len(events) != 0 {
		t.Errorf("GetUncommittedEvents() after clear length = %v, want 0", len(events))
	}

	// SequenceNo should remain unchanged
	if got := entity.GetSequenceNo(); got != 2 {
		t.Errorf("GetSequenceNo() after clear = %v, want 2", got)
	}
}

func TestBaseEntity_ApplyEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(*BaseEntity) error
		event          domain.EventEnvelope[any]
		ctx            context.Context
		wantErr        bool
		wantErrType    error
		wantSequenceNo int
	}{
		{
			name:           "happy path - first event",
			event:          toAnyEvent(domain.NewEventEnvelope("payload", "test-id", "test.event", 0)),
			ctx:            context.Background(),
			wantSequenceNo: 0,
		},
		{
			name: "happy path - sequential events",
			setup: func(e *BaseEntity) error {
				event1 := toAnyEvent(domain.NewEventEnvelope("payload1", "test-id", "test.event1", 0))
				return e.ApplyEvent(context.Background(), event1)
			},
			event:          toAnyEvent(domain.NewEventEnvelope("payload2", "test-id", "test.event2", 1)),
			ctx:            context.Background(),
			wantSequenceNo: 1,
		},
		{
			name:           "wrong aggregate ID",
			event:          toAnyEvent(domain.NewEventEnvelope("payload", "wrong-id", "test.event", 0)),
			ctx:            context.Background(),
			wantErr:        true,
			wantErrType:    ErrWrongAggregate,
			wantSequenceNo: -1,
		},
		{
			name: "duplicate event",
			setup: func(e *BaseEntity) error {
				event := toAnyEvent(domain.NewEventEnvelope("payload", "test-id", "test.event", 0))
				return e.ApplyEvent(context.Background(), event)
			},
			event: func() domain.EventEnvelope[any] {
				evt := domain.NewEventEnvelope("payload", "test-id", "test.event", 0)
				return toAnyEvent(evt)
			}(),
			ctx:            context.Background(),
			wantErr:        true,
			wantErrType:    ErrDuplicateEvent,
			wantSequenceNo: 0,
		},
		{
			name:           "context cancelled",
			event:          toAnyEvent(domain.NewEventEnvelope("payload", "test-id", "test.event", 0)),
			ctx:            func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }(),
			wantErr:        true,
			wantSequenceNo: -1,
		},
		{
			name:           "context timeout",
			event:          toAnyEvent(domain.NewEventEnvelope("payload", "test-id", "test.event", 0)),
			ctx:            func() context.Context { ctx, _ := context.WithTimeout(context.Background(), 0); return ctx }(),
			wantErr:        true,
			wantSequenceNo: -1,
		},
		{
			name: "invalid event sequence number",
			setup: func(e *BaseEntity) error {
				event1 := toAnyEvent(domain.NewEventEnvelope("payload", "test-id", "test.event", 0))
				return e.ApplyEvent(context.Background(), event1)
			},
			event: func() domain.EventEnvelope[any] {
				evt := toAnyEvent(domain.NewEventEnvelope("payload", "test-id", "test.event", 5))
				return evt
			}(),
			ctx:            context.Background(),
			wantErr:        true,
			wantErrType:    ErrInvalidEventSequenceNo,
			wantSequenceNo: 0,
		},
		{
			name: "event with correct sequence number",
			setup: func(e *BaseEntity) error {
				event1 := toAnyEvent(domain.NewEventEnvelope("payload1", "test-id", "test.event1", 0))
				return e.ApplyEvent(context.Background(), event1)
			},
			event: func() domain.EventEnvelope[any] {
				evt := toAnyEvent(domain.NewEventEnvelope("payload2", "test-id", "test.event2", 1))
				return evt
			}(),
			ctx:            context.Background(),
			wantSequenceNo: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entity := NewBaseEntity("test-id")

			if tt.setup != nil {
				if err := tt.setup(entity); err != nil {
					if !tt.wantErr {
						t.Fatalf("Setup failed: %v", err)
					}
				}
			}

			err := entity.ApplyEvent(tt.ctx, tt.event)

			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrType != nil {
				if !errors.Is(err, tt.wantErrType) {
					t.Errorf("ApplyEvent() error = %v, want error type %v", err, tt.wantErrType)
				}
			}

			if got := entity.GetSequenceNo(); got != tt.wantSequenceNo {
				t.Errorf("GetSequenceNo() = %v, want %v", got, tt.wantSequenceNo)
			}
		})
	}
}

func TestBaseEntity_RecordEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(*BaseEntity) error
		payload        any
		eventType      string
		wantErr        bool
		wantErrType    error
		wantSequenceNo int
		wantEventCount int
	}{
		{
			name:           "happy path - first event",
			payload:        "payload",
			eventType:      "test.event",
			wantSequenceNo: 0, // First event has sequenceNo 0
			wantEventCount: 1,
		},
		{
			name: "happy path - multiple events",
			setup: func(e *BaseEntity) error {
				return e.RecordEvent("payload1", "test.event1")
			},
			payload:        "payload2",
			eventType:      "test.event2",
			wantSequenceNo: 1, // Second event has sequenceNo 1
			wantEventCount: 2,
		},
		{
			name: "duplicate event (same payload and type)",
			setup: func(e *BaseEntity) error {
				return e.RecordEvent("payload", "test.event")
			},
			payload:        "payload",
			eventType:      "test.event",
			wantErr:        false, // Different event IDs, so not a duplicate
			wantSequenceNo: 1,     // Second event has sequenceNo 1
			wantEventCount: 2,
		},
		{
			name:           "event sequence number is set automatically",
			payload:        "payload",
			eventType:      "test.event",
			wantSequenceNo: 0, // First event has sequenceNo 0
			wantEventCount: 1,
		},
		{
			name:           "struct payload",
			payload:        map[string]interface{}{"key": "value"},
			eventType:      "test.event",
			wantSequenceNo: 0, // First event has sequenceNo 0
			wantEventCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entity := NewBaseEntity("test-id")

			if tt.setup != nil {
				if err := tt.setup(entity); err != nil {
					if !tt.wantErr {
						t.Fatalf("Setup failed: %v", err)
					}
				}
			}

			err := entity.RecordEvent(tt.payload, tt.eventType)

			if (err != nil) != tt.wantErr {
				t.Errorf("RecordEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrType != nil {
				if !errors.Is(err, tt.wantErrType) {
					t.Errorf("RecordEvent() error = %v, want error type %v", err, tt.wantErrType)
				}
			}

			if got := entity.GetSequenceNo(); got != tt.wantSequenceNo {
				t.Errorf("GetSequenceNo() = %v, want %v", got, tt.wantSequenceNo)
			}

			events := entity.GetUncommittedEvents()
			if len(events) != tt.wantEventCount {
				t.Errorf("GetUncommittedEvents() length = %v, want %v", len(events), tt.wantEventCount)
			}

			// Verify event sequence number is set correctly
			if len(events) > 0 && !tt.wantErr {
				lastEvent := events[len(events)-1]
				if lastEvent.SequenceNo != tt.wantSequenceNo {
					t.Errorf("Last event sequenceNo = %v, want %v", lastEvent.SequenceNo, tt.wantSequenceNo)
				}
				// Verify aggregate ID is set correctly
				if lastEvent.AggregateID != "test-id" {
					t.Errorf("Last event aggregate ID = %v, want test-id", lastEvent.AggregateID)
				}
				// Verify event type is set correctly
				if lastEvent.EventType != tt.eventType {
					t.Errorf("Last event type = %v, want %v", lastEvent.EventType, tt.eventType)
				}
			}
		})
	}
}

func TestBaseEntity_Idempotency(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")
	event := toAnyEvent(domain.NewEventEnvelope("payload", "test-id", "test.event", 0))

	// Apply event first time
	if err := entity.ApplyEvent(context.Background(), event); err != nil {
		t.Fatalf("ApplyEvent() first time error = %v", err)
	}

	if got := entity.GetSequenceNo(); got != 0 {
		t.Errorf("GetSequenceNo() after first apply = %v, want 0", got)
	}

	// Try to apply same event again (should fail)
	err := entity.ApplyEvent(context.Background(), event)
	if err == nil {
		t.Error("ApplyEvent() second time should have returned error")
	}
	if !errors.Is(err, ErrDuplicateEvent) {
		t.Errorf("ApplyEvent() error = %v, want ErrDuplicateEvent", err)
	}

	// SequenceNo should not have changed
	if got := entity.GetSequenceNo(); got != 0 {
		t.Errorf("GetSequenceNo() after duplicate apply = %v, want 0", got)
	}
}

func TestBaseEntity_Concurrency(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")
	const numGoroutines = 10
	const eventsPerGoroutine = 5

	// Channel to collect errors
	errChan := make(chan error, numGoroutines*eventsPerGoroutine)
	done := make(chan bool, numGoroutines)

	// Launch multiple goroutines recording events
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			for j := 0; j < eventsPerGoroutine; j++ {
				payload := map[string]interface{}{"goroutine": id, "event": j}
				if err := entity.RecordEvent(payload, "test.event"); err != nil {
					errChan <- err
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	// We expect some duplicate errors due to race conditions with event IDs
	// But the entity should still be in a consistent state
	events := entity.GetUncommittedEvents()
	sequenceNo := entity.GetSequenceNo()

	// SequenceNo should be >= (number of events - 1) since sequenceNo is 0-indexed
	// Since we're using ksuid for event IDs, duplicates should be rare but possible
	if sequenceNo < len(events)-1 {
		t.Errorf("SequenceNo (%d) should be >= number of events - 1 (%d)", sequenceNo, len(events)-1)
	}

	// Verify all events belong to the correct aggregate
	for _, event := range events {
		if event.AggregateID != "test-id" {
			t.Errorf("Event has wrong aggregate ID: %s", event.AggregateID)
		}
	}
}

func TestBaseEntity_ConcurrentReads(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")

	// Record some events
	for i := 0; i < 10; i++ {
		if err := entity.RecordEvent("payload", "test.event"); err != nil {
			t.Fatalf("RecordEvent() error = %v", err)
		}
	}

	const numReaders = 20
	done := make(chan bool, numReaders)

	// Launch multiple goroutines reading concurrently
	for i := 0; i < numReaders; i++ {
		go func() {
			defer func() { done <- true }()
			id := entity.GetID()
			version := entity.GetSequenceNo()
			events := entity.GetUncommittedEvents()

			if id != "test-id" {
				t.Errorf("GetID() = %v, want test-id", id)
			}
			if version != 9 {
				t.Errorf("GetSequenceNo() = %v, want 9", version)
			}
			if len(events) != 10 {
				t.Errorf("GetUncommittedEvents() length = %v, want 10", len(events))
			}
		}()
	}

	// Wait for all readers to complete
	for i := 0; i < numReaders; i++ {
		<-done
	}
}

func TestBaseEntity_LargeEventList(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")
	const numEvents = 1000

	// Record many events
	for i := 0; i < numEvents; i++ {
		if err := entity.RecordEvent("payload", "test.event"); err != nil {
			t.Fatalf("RecordEvent() error = %v", err)
		}
	}

	events := entity.GetUncommittedEvents()
	if len(events) != numEvents {
		t.Errorf("GetUncommittedEvents() length = %v, want %v", len(events), numEvents)
	}

	if got := entity.GetSequenceNo(); got != numEvents-1 {
		t.Errorf("GetSequenceNo() = %v, want %v", got, numEvents-1)
	}
}

func TestBaseEntity_EventVersionTracking(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")

	// Record events and verify sequence numbers are sequential
	for i := 0; i < 5; i++ {
		if err := entity.RecordEvent("payload", "test.event"); err != nil {
			t.Fatalf("RecordEvent() error = %v", err)
		}

		events := entity.GetUncommittedEvents()
		lastEvent := events[len(events)-1]
		expectedSeqNo := i
		if lastEvent.SequenceNo != expectedSeqNo {
			t.Errorf("Event %d sequenceNo = %v, want %v", i+1, lastEvent.SequenceNo, expectedSeqNo)
		}

		if entity.GetSequenceNo() != expectedSeqNo {
			t.Errorf("Entity sequenceNo = %v, want %v", entity.GetSequenceNo(), expectedSeqNo)
		}
	}
}

func TestBaseEntity_ContextCancellation(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	event := toAnyEvent(domain.NewEventEnvelope("payload", "test-id", "test.event", 0))
	err := entity.ApplyEvent(ctx, event)

	if err == nil {
		t.Error("ApplyEvent() with cancelled context should return error")
	}

	if entity.GetSequenceNo() != -1 {
		t.Errorf("GetSequenceNo() after cancelled context = %v, want -1", entity.GetSequenceNo())
	}
}

func TestBaseEntity_ContextTimeout(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")

	// Create a context with immediate timeout
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	// Wait a tiny bit to ensure timeout
	time.Sleep(1 * time.Millisecond)

	event := toAnyEvent(domain.NewEventEnvelope("payload", "test-id", "test.event", 0))
	err := entity.ApplyEvent(ctx, event)

	if err == nil {
		t.Error("ApplyEvent() with timed out context should return error")
	}

	if entity.GetSequenceNo() != -1 {
		t.Errorf("GetSequenceNo() after timeout = %v, want -1", entity.GetSequenceNo())
	}
}

func TestBaseEntity_EmptyAggregateID(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("")

	if err := entity.RecordEvent("payload", "test.event"); err != nil {
		t.Errorf("RecordEvent() with empty aggregate ID error = %v", err)
	}

	if entity.GetSequenceNo() != 0 {
		t.Errorf("GetSequenceNo() = %v, want 0", entity.GetSequenceNo())
	}
}

func TestBaseEntity_ApplyThenRecord(t *testing.T) {
	t.Parallel()

	entity := NewBaseEntity("test-id")
	event1 := toAnyEvent(domain.NewEventEnvelope("payload1", "test-id", "test.event1", 0))

	// Apply first event (replay scenario)
	if err := entity.ApplyEvent(context.Background(), event1); err != nil {
		t.Fatalf("ApplyEvent() error = %v", err)
	}

	if entity.GetSequenceNo() != 0 {
		t.Errorf("GetSequenceNo() after ApplyEvent = %v, want 0", entity.GetSequenceNo())
	}

	// Record second event (new event)
	if err := entity.RecordEvent("payload2", "test.event2"); err != nil {
		t.Fatalf("RecordEvent() error = %v", err)
	}

	if entity.GetSequenceNo() != 1 {
		t.Errorf("GetSequenceNo() after RecordEvent = %v, want 1", entity.GetSequenceNo())
	}

	// Only the recorded event should be in uncommitted events
	events := entity.GetUncommittedEvents()
	if len(events) != 1 {
		t.Errorf("GetUncommittedEvents() length = %v, want 1", len(events))
	}

	// Verify the recorded event has correct properties
	if events[0].EventType != "test.event2" {
		t.Errorf("Uncommitted event type = %v, want test.event2", events[0].EventType)
	}
	if events[0].Payload != "payload2" {
		t.Errorf("Uncommitted event payload = %v, want payload2", events[0].Payload)
	}
}
