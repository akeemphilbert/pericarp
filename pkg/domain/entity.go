package domain

import (
	"fmt"
	"sync"
)

type Entity interface {
	GetID() string
	GetSequenceNo() int64
	UncommittedEvents() []Event
	MarkEventsAsCommitted()
	LoadFromHistory(events []Event)
	AddEvent(event Event)
	HasUncommittedEvents() bool
	UncommittedEventCount() int
	MergeEventsFrom(source Entity) error
	AddError(err error)
	Errors() []error
	IsValid() bool
	Reset()
	Clone() Entity
	String() string
}

// BasicEntity provides a concrete implementation of AggregateRoot that can be embedded
// in other aggregate types. It handles the common concerns of event sourcing:
// identity, versioning, event management, and sequence tracking.
//
// Usage example:
//
//	type User struct {
//	    BasicEntity
//	    email string
//	    name  string
//	}
//
//	func NewUser(id, email, name string) *User {
//	    user := &User{
//	        BasicEntity: NewEntity(id),
//	        email:  email,
//	        name:   name,
//	    }
//
//	    event := UserCreatedEvent{
//	        UserID: id,
//	        Email:  email,
//	        Name:   name,
//	    }
//
//	    user.AddEvent(event)
//	    return user
//	}
//
//	func (u *User) ChangeEmail(newEmail string) error {
//	    if newEmail == u.email {
//	        return nil
//	    }
//
//	    event := UserEmailChangedEvent{
//	        UserID:   u.GetID(),
//	        OldEmail: u.email,
//	        NewEmail: newEmail,
//	    }
//
//	    u.email = newEmail
//	    u.AddEvent(event)
//	    return nil
//	}
type BasicEntity struct {
	ID                string `json:"id,omitempty"` // Unique identifier
	SequenceNo        int64
	events            []Event // Committed events (full history)
	uncommittedEvents []Event // Events pending persistence
	errors            []error
	mu                sync.RWMutex // Protects concurrent access to entity state
}

// NewEntity creates a new entity with the given GetID.
// The entity starts with sequence number 0.
func NewEntity(id string) *BasicEntity {
	return &BasicEntity{
		ID:                id,
		SequenceNo:        0,
		events:            []Event{},
		uncommittedEvents: []Event{},
		errors:            []error{},
	}
}

// WithID sets the ID of the entity and returns a pointer to the entity.
// This allows for fluent initialization: new(Entity).WithID("some-id")
//
// Example usage:
//
//	entity := new(Entity).WithID("user-123")
//	// or
//	var entity Entity
//	entity.WithID("user-123")
func (e *BasicEntity) WithID(id string) *BasicEntity {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ID = id
	e.SequenceNo = 0
	e.events = []Event{}
	e.uncommittedEvents = []Event{}
	e.errors = []error{}

	return e
}

// ID returns the unique identifier of the entity.
// This implements the AggregateRoot interface.
func (e *BasicEntity) GetID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ID
}

// SequenceNo returns the current sequence number of the entity.
// The sequence number is incremented each time an event is added and can be used
// for ordering events within the same aggregate or for optimistic concurrency control.
func (e *BasicEntity) GetSequenceNo() int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.SequenceNo
}

// Version returns the current version of the entity.
// This is an alias for SequenceNo() to implement the AggregateRoot interface.
func (e *BasicEntity) Version() int {
	return int(e.GetSequenceNo())
}

// UncommittedEvents returns a copy of the events that have been generated
// but not yet persisted to the event store.
// This implements the AggregateRoot interface.
func (e *BasicEntity) UncommittedEvents() []Event {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return a copy to prevent external modification
	events := make([]Event, len(e.uncommittedEvents))
	copy(events, e.uncommittedEvents)
	return events
}

// MarkEventsAsCommitted clears the uncommitted events after they have
// been successfully persisted to the event store.
// This implements the AggregateRoot interface.
func (e *BasicEntity) MarkEventsAsCommitted() {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Move uncommitted events to committed events
	e.events = append(e.events, e.uncommittedEvents...)

	// Clear the uncommitted events slice but keep the underlying array for reuse
	e.uncommittedEvents = e.uncommittedEvents[:0]
}

// LoadFromHistory reconstructs the entity state from a sequence of events.
// This method should be called by concrete aggregate implementations to
// apply historical events during aggregate reconstruction.
// This implements the AggregateRoot interface.
//
// Note: This method only updates the sequence number.
// Concrete aggregates should override this method to apply domain-specific
// event handling while calling this base implementation.
//
// Example:
//
//	func (u *User) LoadFromHistory(events []Event) {
//	    for _, event := range events {
//	        switch e := event.(type) {
//	        case UserCreatedEvent:
//	            u.email = e.Email
//	            u.name = e.Name
//	        case UserEmailChangedEvent:
//	            u.email = e.NewEmail
//	        }
//	    }
//	    // Call base implementation to update sequence number
//	    u.Entity.LoadFromHistory(events)
//	}
func (e *BasicEntity) LoadFromHistory(events []Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Update sequence number based on events
	e.SequenceNo = int64(len(events))

	// Load events into committed events and clear uncommitted events
	e.events = make([]Event, len(events))
	copy(e.events, events)
	e.uncommittedEvents = e.uncommittedEvents[:0]
	e.errors = e.errors[:0]
}

// AddEvent adds a new event to the entity's uncommitted events list.
// This method automatically increments the sequence number.
//
// This method is thread-safe and can be called concurrently.
//
// Example usage:
//
//	func (u *User) ChangeEmail(newEmail string) error {
//	    // Validate business rules
//	    if newEmail == u.email {
//	        return nil
//	    }
//
//	    // Apply the change
//	    oldEmail := u.email
//	    u.email = newEmail
//
//	    // Create and add the event
//	    event := UserEmailChangedEvent{
//	        UserID:   u.GetID(),
//	        OldEmail: oldEmail,
//	        NewEmail: newEmail,
//	        ChangedAt: time.Now(),
//	    }
//
//	    u.AddEvent(event)
//	    return nil
//	}
func (e *BasicEntity) AddEvent(event Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Increment sequence number
	e.SequenceNo++
	event.SetSequenceNo(e.SequenceNo)

	// Add the event to uncommitted events only
	e.uncommittedEvents = append(e.uncommittedEvents, event)
}

// HasUncommittedEvents returns true if the entity has events that haven't been persisted.
// This is useful for checking if the entity needs to be saved.
func (e *BasicEntity) HasUncommittedEvents() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.uncommittedEvents) > 0
}

// UncommittedEventCount returns the number of uncommitted events.
// This is useful for monitoring and debugging purposes.
func (e *BasicEntity) UncommittedEventCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.uncommittedEvents)
}

// MergeEventsFrom merges the uncommitted events from another entity into this entity.
// Only uncommitted events from the source entity are merged; committed events are ignored.
// The merged events preserve their original sequence numbers and references from the source.
//
// This method is useful for aggregate root entities that need to collect events from
// child entities without modifying the event identity or sequence numbers.
// The source entity remains unchanged after the merge operation.
//
// Example usage:
//
//	entity1 := NewEntity("entity-1")        // Aggregate root
//	entity1.AddEvent(event1)                // sequence: 1
//	entity1.MarkEventsAsCommitted()
//	entity1.AddEvent(event2)                // sequence: 2 (uncommitted)
//
//	childEntity := NewEntity("child-1")     // Child entity
//	childEntity.AddEvent(event3)            // sequence: 1 (uncommitted)
//	childEntity.AddEvent(event4)            // sequence: 2 (uncommitted)
//
//	err := entity1.MergeEventsFrom(childEntity)
//	// entity1 now has: event2 (seq: 2), event3 (seq: 1), event4 (seq: 2) as uncommitted
//	// Original sequence numbers from childEntity are preserved
//	// childEntity remains unchanged
//
// Note: The source must be a BasicEntity implementation. If a different Entity
// implementation is passed, an error will be returned.
func (e *BasicEntity) MergeEventsFrom(source Entity) error {
	if source == nil {
		return fmt.Errorf("source entity cannot be nil")
	}

	// Get uncommitted events from source using the interface method
	sourceUncommittedEvents := source.UncommittedEvents()
	if len(sourceUncommittedEvents) == 0 {
		// No uncommitted events to merge - this is a no-op
		return nil
	}

	// Merge the events preserving their original sequence numbers and references
	for _, sourceEvent := range sourceUncommittedEvents {
		// Add the event to our uncommitted events without modifying it
		// This preserves the original sequence number and event identity from the source
		e.uncommittedEvents = append(e.uncommittedEvents, sourceEvent)
	}

	return nil
}

// AddError adds an error to the entity's error collection.
// This is useful for collecting validation errors or business rule violations.
//
// Example usage:
//
//	func (u *User) ChangeEmail(newEmail string) error {
//	    if newEmail == "" {
//	        err := errors.New("email cannot be empty")
//	        u.AddError(err)
//	        return err
//	    }
//	    // ... rest of the logic
//	}
func (e *BasicEntity) AddError(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errors = append(e.errors, err)
}

// Errors returns a copy of all errors collected by the entity.
// This prevents external modification of the internal errors slice.
func (e *BasicEntity) Errors() []error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	errors := make([]error, len(e.errors))
	copy(errors, e.errors)
	return errors
}

// IsValid returns true if the entity has no errors.
func (e *BasicEntity) IsValid() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.errors) == 0
}

// Reset resets the entity to its initial state.
// This is primarily useful for testing or when reusing entity instances.
//
// Warning: This method clears all state including uncommitted events and errors.
// Use with caution in production code.
func (e *BasicEntity) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.SequenceNo = 0
	e.events = e.events[:0]
	e.uncommittedEvents = e.uncommittedEvents[:0]
	e.errors = e.errors[:0]
}

// Clone creates a deep copy of the entity's metadata (ID, sequence).
// The events and errors slices are also copied to prevent shared state.
//
// Note: This only clones the Entity struct itself. Concrete aggregates
// that embed Entity should implement their own Clone method if needed.
func (e *BasicEntity) Clone() Entity {
	e.mu.RLock()
	defer e.mu.RUnlock()

	events := make([]Event, len(e.events))
	copy(events, e.events)

	uncommittedEvents := make([]Event, len(e.uncommittedEvents))
	copy(uncommittedEvents, e.uncommittedEvents)

	errors := make([]error, len(e.errors))
	copy(errors, e.errors)

	return &BasicEntity{
		ID:                e.ID,
		SequenceNo:        e.SequenceNo,
		events:            events,
		uncommittedEvents: uncommittedEvents,
		errors:            errors,
	}
}

// String returns a string representation of the entity for debugging.
func (e *BasicEntity) String() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return fmt.Sprintf("Entity{GetID: %s, GetSequenceNo: %d, UncommittedEvents: %d, Errors: %d}",
		e.ID, e.SequenceNo, len(e.uncommittedEvents), len(e.errors))
}
