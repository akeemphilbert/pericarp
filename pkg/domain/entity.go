package domain

import (
	"fmt"
	"sync"
)

// Entity provides a concrete implementation of AggregateRoot that can be embedded
// in other aggregate types. It handles the common concerns of event sourcing:
// identity, versioning, event management, and sequence tracking.
//
// Usage example:
//
//	type User struct {
//	    Entity
//	    email string
//	    name  string
//	}
//
//	func NewUser(id, email, name string) *User {
//	    user := &User{
//	        Entity: NewEntity(id),
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
//	        UserID:   u.ID(),
//	        OldEmail: u.email,
//	        NewEmail: newEmail,
//	    }
//
//	    u.email = newEmail
//	    u.AddEvent(event)
//	    return nil
//	}
type Entity struct {
	id         string
	version    int
	sequenceNo int
	events     []Event
	mu         sync.RWMutex // Protects concurrent access to entity state
}

// NewEntity creates a new entity with the given ID.
// The entity starts with version 0 and sequence number 0.
func NewEntity(id string) Entity {
	return Entity{
		id:         id,
		version:    0,
		sequenceNo: 0,
		events:     make([]Event, 0, 5), // Pre-allocate with small capacity
	}
}

// ID returns the unique identifier of the entity.
// This implements the AggregateRoot interface.
func (e *Entity) ID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.id
}

// Version returns the current version of the entity.
// The version represents the number of events that have been applied to this entity.
// This implements the AggregateRoot interface.
func (e *Entity) Version() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.version
}

// SequenceNo returns the current sequence number of the entity.
// The sequence number is incremented each time an event is added and can be used
// for ordering events within the same aggregate or for optimistic concurrency control.
func (e *Entity) SequenceNo() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sequenceNo
}

// UncommittedEvents returns a copy of the events that have been generated
// but not yet persisted to the event store.
// This implements the AggregateRoot interface.
func (e *Entity) UncommittedEvents() []Event {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return a copy to prevent external modification
	events := make([]Event, len(e.events))
	copy(events, e.events)
	return events
}

// MarkEventsAsCommitted clears the uncommitted events after they have
// been successfully persisted to the event store.
// This implements the AggregateRoot interface.
func (e *Entity) MarkEventsAsCommitted() {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Clear the events slice but keep the underlying array for reuse
	e.events = e.events[:0]
}

// LoadFromHistory reconstructs the entity state from a sequence of events.
// This method should be called by concrete aggregate implementations to
// apply historical events during aggregate reconstruction.
// This implements the AggregateRoot interface.
//
// Note: This method only updates the version and sequence number.
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
//	    // Call base implementation to update version and sequence
//	    u.Entity.LoadFromHistory(events)
//	}
func (e *Entity) LoadFromHistory(events []Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Update version and sequence number based on events
	e.version = len(events)
	e.sequenceNo = len(events)

	// Clear any uncommitted events during reconstruction
	e.events = e.events[:0]
}

// AddEvent adds a new event to the entity's uncommitted events list.
// This method automatically increments the version and sequence number.
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
//	        UserID:   u.ID(),
//	        OldEmail: oldEmail,
//	        NewEmail: newEmail,
//	        ChangedAt: time.Now(),
//	    }
//
//	    u.AddEvent(event)
//	    return nil
//	}
func (e *Entity) AddEvent(event Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Increment version and sequence number
	e.version++
	e.sequenceNo++

	// Add the event to uncommitted events
	e.events = append(e.events, event)
}

// HasUncommittedEvents returns true if the entity has events that haven't been persisted.
// This is useful for checking if the entity needs to be saved.
func (e *Entity) HasUncommittedEvents() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.events) > 0
}

// UncommittedEventCount returns the number of uncommitted events.
// This is useful for monitoring and debugging purposes.
func (e *Entity) UncommittedEventCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.events)
}

// Reset resets the entity to its initial state.
// This is primarily useful for testing or when reusing entity instances.
//
// Warning: This method clears all state including uncommitted events.
// Use with caution in production code.
func (e *Entity) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.version = 0
	e.sequenceNo = 0
	e.events = e.events[:0]
}

// Clone creates a deep copy of the entity's metadata (ID, version, sequence).
// The events slice is also copied to prevent shared state.
//
// Note: This only clones the Entity struct itself. Concrete aggregates
// that embed Entity should implement their own Clone method if needed.
func (e *Entity) Clone() Entity {
	e.mu.RLock()
	defer e.mu.RUnlock()

	events := make([]Event, len(e.events))
	copy(events, e.events)

	return Entity{
		id:         e.id,
		version:    e.version,
		sequenceNo: e.sequenceNo,
		events:     events,
	}
}

// String returns a string representation of the entity for debugging.
func (e *Entity) String() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return fmt.Sprintf("Entity{ID: %s, Version: %d, SequenceNo: %d, UncommittedEvents: %d}",
		e.id, e.version, e.sequenceNo, len(e.events))
}
