package domain

//go:generate moq -out mocks/aggregate_root_mock.go . AggregateRoot
//go:generate moq -out mocks/repository_mock.go . Repository

import "context"

// AggregateRoot defines the interface for domain aggregates in event sourcing.
// An aggregate is a cluster of domain objects that can be treated as a single unit
// for data changes. It ensures consistency boundaries and encapsulates business logic.
//
// Key principles:
//   - Aggregates are consistency boundaries
//   - Only aggregate roots can be referenced from outside
//   - Aggregates generate events when their state changes
//   - State changes are applied through business methods, not direct field access
//
// Example implementation:
//
//	type User struct {
//	    id      string
//	    email   string
//	    name    string
//	    version int
//	    events  []Event
//	}
//
//	func (u *User) ChangeEmail(newEmail string) error {
//	    if newEmail == u.email {
//	        return nil // No change needed
//	    }
//
//	    // Validate business rules
//	    if !isValidEmail(newEmail) {
//	        return errors.New("invalid email format")
//	    }
//
//	    // Apply change and generate event
//	    oldEmail := u.email
//	    u.email = newEmail
//	    u.version++
//
//	    event := UserEmailChangedEvent{
//	        UserID:   u.id,
//	        OldEmail: oldEmail,
//	        NewEmail: newEmail,
//	        ChangedAt: time.Now(),
//	    }
//	    u.events = append(u.events, event)
//
//	    return nil
//	}
type AggregateRoot interface {
	// ID returns the unique identifier of the aggregate.
	// This ID should be immutable and unique across all instances
	// of this aggregate type.
	ID() string

	// Version returns the current version of the aggregate.
	// The version is incremented each time the aggregate's state changes
	// and is used for optimistic concurrency control.
	Version() int

	// UncommittedEvents returns the list of events that have been generated
	// by business operations but not yet persisted to the event store.
	//
	// These events represent the changes that will be persisted when the
	// aggregate is saved through a repository.
	UncommittedEvents() []Event

	// MarkEventsAsCommitted clears the uncommitted events after they have
	// been successfully persisted to the event store.
	//
	// This method should be called by the repository after successful
	// persistence to prevent events from being persisted multiple times.
	MarkEventsAsCommitted()

	// LoadFromHistory reconstructs the aggregate state from a sequence of events.
	// This is the core of event sourcing - instead of loading current state
	// from a database, the aggregate rebuilds its state by applying historical events.
	//
	// The method should:
	//   - Apply events in the order they occurred
	//   - Update the aggregate's version based on the events
	//   - Not generate new events during reconstruction
	//   - Handle unknown event types gracefully (for forward compatibility)
	//
	// Example implementation:
	//
	//	func (u *User) LoadFromHistory(events []Event) {
	//	    for _, event := range events {
	//	        switch e := event.(type) {
	//	        case UserCreatedEvent:
	//	            u.id = e.UserID
	//	            u.email = e.Email
	//	            u.name = e.Name
	//	        case UserEmailChangedEvent:
	//	            u.email = e.NewEmail
	//	        }
	//	        u.version = event.Version()
	//	    }
	//	    u.events = nil // Clear events after loading
	//	}
	LoadFromHistory(events []Event)
}

// Repository defines the interface for aggregate persistence using event sourcing.
// The repository abstracts the complexity of event storage and aggregate
// reconstruction, providing a simple interface for saving and loading aggregates.
//
// The repository pattern provides:
//   - Abstraction over event storage details
//   - Consistent interface for aggregate persistence
//   - Separation between domain logic and infrastructure
//   - Support for different storage implementations
//
// Example usage:
//
//	// Save an aggregate
//	user, err := NewUser("user@example.com", "John Doe")
//	if err != nil {
//	    return err
//	}
//	err = userRepo.Save(ctx, user)
//
//	// Load an aggregate
//	user, err := userRepo.Load(ctx, "user-123")
//	if err != nil {
//	    return err
//	}
//	err = user.ChangeEmail("newemail@example.com")
//	err = userRepo.Save(ctx, user)
type Repository[T AggregateRoot] interface {
	// Save persists the aggregate by storing its uncommitted events.
	// The repository should:
	//   - Extract uncommitted events from the aggregate
	//   - Persist events to the event store
	//   - Handle concurrency conflicts (optimistic locking)
	//   - Mark events as committed on successful persistence
	//
	// Typical implementation:
	//   1. Get uncommitted events from aggregate
	//   2. Use UnitOfWork to persist events
	//   3. Mark events as committed on aggregate
	//   4. Handle any concurrency or persistence errors
	//
	// The method should handle optimistic concurrency control by checking
	// the aggregate version against the latest version in the event store.
	Save(ctx context.Context, aggregate T) error

	// Load retrieves an aggregate by its ID, reconstructing it from stored events.
	// The repository should:
	//   - Load all events for the aggregate from the event store
	//   - Create a new instance of the aggregate
	//   - Reconstruct state by calling LoadFromHistory with the events
	//   - Return the fully reconstructed aggregate
	//
	// If no events exist for the given ID, the repository should return
	// an appropriate "not found" error.
	//
	// The reconstructed aggregate should have:
	//   - Current state based on all historical events
	//   - Correct version number
	//   - No uncommitted events (clean state)
	Load(ctx context.Context, id string) (T, error)
}
