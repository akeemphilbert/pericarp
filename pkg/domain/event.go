// Package domain provides core domain layer interfaces and types for implementing
// Domain-Driven Design (DDD) patterns with Event Sourcing and CQRS.
//
// This package defines the fundamental abstractions for:
//   - Domain events and event handling
//   - Aggregate roots and repositories
//   - Event sourcing infrastructure
//   - Domain services and value objects
//
// The domain layer is kept pure with no external dependencies, following
// clean architecture principles.
package domain

//go:generate moq -out mocks/event_store_mock.go . EventStore
//go:generate moq -out mocks/event_dispatcher_mock.go . EventDispatcher
//go:generate moq -out mocks/event_handler_mock.go . EventHandler
//go:generate moq -out mocks/unit_of_work_mock.go . UnitOfWork
//go:generate moq -out mocks/event_mock.go . Event
//go:generate moq -out mocks/envelope_mock.go . Envelope

import (
	"context"
	"time"
)

// Event represents a domain event that captures something significant that happened
// in the business domain. Events are immutable facts about what occurred and are
// used for event sourcing, integration, and building read models.
//
// Events should:
//   - Use past tense names (UserCreated, OrderShipped)
//   - Contain all necessary data to understand what happened
//   - Be immutable once created
//   - Include business context, not just technical details
//
// Example implementation:
//
//	type UserCreatedEvent struct {
//	    UserID     string    `json:"user_id"`
//	    Email      string    `json:"email"`
//	    Name       string    `json:"name"`
//	    CreatedAt  time.Time `json:"created_at"`
//	    SequenceNo int64     `json:"sequence_no"`
//	}
//
//	func (e UserCreatedEvent) EventType() string { return "UserCreated" }
//	func (e UserCreatedEvent) AggregateID() string { return e.UserID }
//	func (e UserCreatedEvent) SequenceNo() int64 { return e.SequenceNo }
//	func (e UserCreatedEvent) CreatedAt() time.Time { return e.CreatedAt }
type Event interface {
	// EventType returns the type identifier for this event.
	// This should be a stable string that uniquely identifies the event type
	// across versions (e.g., "UserCreated", "OrderShipped").
	EventType() string

	// AggregateID returns the ID of the aggregate that generated this event.
	// This is used to group events by aggregate for event sourcing.
	AggregateID() string

	// SequenceNo returns the sequence number of the aggregate when this event occurred.
	// This is used for optimistic concurrency control and event ordering.
	SequenceNo() int64

	// CreatedAt returns the timestamp when this event was created in the domain.
	// This should represent the business time, not the technical persistence time.
	CreatedAt() time.Time

	// User returns the user ID associated with this event.
	// This is useful for auditing and tracking which user performed the action.
	User() string

	// Account returns the account ID associated with this event.
	// This is useful for multi-tenant systems to track which account the event belongs to.
	Account() string

	// Payload returns the event-specific data payload.
	// This contains the actual business data associated with the event.
	Payload() any
}

// Envelope wraps domain events with additional metadata for transport and processing.
// The envelope pattern separates the pure domain event from infrastructure concerns
// like correlation IDs, causation tracking, and technical timestamps.
//
// Envelopes are created by the event store when events are persisted and provide
// a consistent way to handle event metadata across the system.
type Envelope interface {
	// Event returns the wrapped domain event.
	// This is the pure business event without infrastructure concerns.
	Event() Event

	// Metadata returns additional metadata associated with the event.
	// This may include correlation IDs, causation IDs, user context,
	// or other cross-cutting concerns.
	Metadata() map[string]interface{}

	// EventID returns the unique identifier for this event envelope.
	// This is different from the domain event's aggregate ID and is used
	// for deduplication and tracking at the infrastructure level.
	EventID() string

	// Timestamp returns when this envelope was created (typically when
	// the event was persisted). This is different from the event's
	// OccurredAt timestamp which represents business time.
	Timestamp() time.Time
}

// EventStore provides persistent storage for domain events, implementing the
// event sourcing pattern. The event store is append-only and maintains the
// complete history of all domain events.
//
// The event store is responsible for:
//   - Persisting events in order
//   - Ensuring event immutability
//   - Providing efficient aggregate reconstruction
//   - Maintaining event metadata through envelopes
//
// Implementation considerations:
//   - Events should be stored in append-only fashion
//   - Concurrent writes to the same aggregate should be handled with optimistic locking
//   - Event ordering within an aggregate must be maintained
//   - Consider partitioning strategies for scalability
type EventStore interface {
	// Save persists a batch of events atomically and returns envelopes with metadata.
	// The events are wrapped in envelopes that include technical metadata like
	// event IDs, timestamps, and correlation information.
	//
	// This method should:
	//   - Persist events atomically (all or none)
	//   - Assign unique event IDs
	//   - Maintain event ordering within aggregates
	//   - Handle optimistic concurrency conflicts
	//
	// Returns the persisted events wrapped in envelopes for further processing.
	Save(ctx context.Context, events []Event) ([]Envelope, error)

	// Load retrieves all events for a specific aggregate, ordered by version.
	// This is used to reconstruct aggregate state from its complete event history.
	//
	// The returned envelopes should be ordered by the event version to ensure
	// correct aggregate reconstruction.
	Load(ctx context.Context, aggregateID string) ([]Envelope, error)

	// LoadFromSequence retrieves events for an aggregate starting from a specific sequence number.
	// This is useful for incremental updates or when you already have a snapshot
	// of the aggregate at a certain sequence number.
	//
	// The sequenceNo parameter is inclusive - events with sequenceNo >= sequenceNo will be returned.
	LoadFromSequence(ctx context.Context, aggregateID string, sequenceNo int64) ([]Envelope, error)
}

// EventDispatcher handles the distribution of events to registered handlers,
// implementing the publish-subscribe pattern for event-driven architecture.
//
// The dispatcher enables:
//   - Decoupled event processing
//   - Multiple handlers per event type
//   - Asynchronous event processing
//   - Integration with external systems
//
// Common use cases:
//   - Updating read models (projections)
//   - Triggering business processes (sagas)
//   - Sending notifications
//   - Integrating with external systems
type EventDispatcher interface {
	// Dispatch sends a batch of event envelopes to all registered handlers
	// that are subscribed to the respective event types.
	//
	// The dispatcher should:
	//   - Route events to appropriate handlers based on event type
	//   - Handle errors gracefully (retry, dead letter, etc.)
	//   - Support concurrent processing where appropriate
	//   - Maintain event ordering guarantees where required
	//
	// Error handling strategy depends on implementation but should consider:
	//   - Partial failures (some handlers succeed, others fail)
	//   - Retry mechanisms for transient failures
	//   - Dead letter queues for persistent failures
	Dispatch(ctx context.Context, envelopes []Envelope) error

	// Subscribe registers an event handler to receive events of a specific type.
	// Multiple handlers can be registered for the same event type.
	//
	// The eventType parameter should match the EventType() returned by events.
	// Handlers will receive all events of the specified type through their
	// Handle method.
	Subscribe(eventType string, handler EventHandler) error
}

// EventHandler processes domain events to implement various event-driven patterns.
// Handlers are the primary mechanism for reacting to domain events and can
// implement different patterns:
//
//   - Projectors: Build and maintain read models from events
//   - Sagas: Coordinate long-running business processes
//   - Integration handlers: Integrate with external systems
//   - Notification handlers: Send notifications based on events
//
// Example projector implementation:
//
//	type UserProjector struct {
//	    readModelRepo UserReadModelRepository
//	}
//
//	func (p *UserProjector) Handle(ctx context.Context, envelope Envelope) error {
//	    switch event := envelope.Event().(type) {
//	    case UserCreatedEvent:
//	        return p.readModelRepo.Create(ctx, UserReadModel{
//	            ID:    event.UserID,
//	            Email: event.Email,
//	            Name:  event.Name,
//	        })
//	    case UserEmailUpdatedEvent:
//	        return p.readModelRepo.UpdateEmail(ctx, event.UserID, event.NewEmail)
//	    }
//	    return nil
//	}
//
//	func (p *UserProjector) EventTypes() []string {
//	    return []string{"UserCreated", "UserEmailUpdated"}
//	}
type EventHandler interface {
	// Handle processes a single event envelope.
	// The handler should:
	//   - Check if it can handle the event type
	//   - Extract the event from the envelope
	//   - Perform the appropriate processing
	//   - Handle errors gracefully
	//
	// Handlers should be idempotent where possible, as events may be
	// delivered more than once in failure scenarios.
	Handle(ctx context.Context, envelope Envelope) error

	// EventTypes returns the list of event types this handler can process.
	// This is used by the event dispatcher to route events to appropriate handlers.
	// The strings should match the EventType() values returned by events.
	EventTypes() []string
}

// UnitOfWork manages transactional boundaries for event persistence and dispatch,
// implementing the Unit of Work pattern for event sourcing.
//
// The UnitOfWork ensures that:
//   - Multiple aggregates can be saved in a single transaction
//   - Events are persisted before being dispatched (persist-then-dispatch)
//   - All operations succeed or fail together
//   - Event ordering is maintained
//
// Typical usage pattern:
//
//	// Register events from multiple aggregates
//	uow.RegisterEvents(user.UncommittedEvents())
//	uow.RegisterEvents(order.UncommittedEvents())
//
//	// Commit persists events and dispatches them
//	envelopes, err := uow.Commit(ctx)
//	if err != nil {
//	    return err
//	}
//
//	// Mark events as committed on aggregates
//	user.MarkEventsAsCommitted()
//	order.MarkEventsAsCommitted()
type UnitOfWork interface {
	// RegisterEvents adds events to be persisted in the current transaction.
	// Events from multiple aggregates can be registered in the same unit of work
	// to ensure they are persisted atomically.
	//
	// The events will be persisted when Commit() is called.
	RegisterEvents(events []Event)

	// Commit persists all registered events atomically and then dispatches them.
	// This implements the persist-then-dispatch pattern to ensure events are
	// safely stored before being processed by handlers.
	//
	// The method:
	//   1. Persists all registered events to the event store
	//   2. Dispatches the resulting envelopes to event handlers
	//   3. Returns the envelopes for further processing if needed
	//
	// If any step fails, the entire operation should be rolled back.
	Commit(ctx context.Context) ([]Envelope, error)

	// Rollback discards all registered events without persisting them.
	// This is used when an error occurs before commit or when explicitly
	// canceling a unit of work.
	//
	// After rollback, the unit of work can be reused for new operations.
	Rollback() error
}

// EntityEvent provides a basic implementation of the Event interface that can be
// embedded in concrete event types. It handles common event concerns like
// entity type, event type, sequence numbers, and metadata.
//
// The EventType() method returns a concatenation of EntityType and Type
// in the format "entitytype.eventtype" (e.g., "user.created", "order.shipped").
//
// Example usage:
//
//	type UserCreatedEvent struct {
//	    EntityEvent
//	    Email string `json:"email"`
//	    Name  string `json:"name"`
//	}
//
//	func NewUserCreatedEvent(userID, email, name, userID, accountID string) UserCreatedEvent {
//	    return UserCreatedEvent{
//	        EntityEvent: NewEntityEvent("user", "created", userID, userID, accountID),
//	        Email:       email,
//	        Name:        name,
//	    }
//	}
type EntityEvent struct {
	EntityType  string    `json:"entity_type"`
	Type        string    `json:"type"`
	AggregateId string    `json:"aggregate_id"`
	SequenceNum int64     `json:"sequence_no"`
	CreatedTime time.Time `json:"created_at"`
	UserId      string    `json:"user_id"`
	AccountId   string    `json:"account_id"`
	Data        any       `json:"payload"`
}

// NewEntityEvent creates a new EntityEvent with the specified parameters.
// The entityType and eventType are combined to form the full event type.
//
// Parameters:
//   - entityType: The type of entity (e.g., "user", "order", "product")
//   - eventType: The type of event (e.g., "created", "updated", "deleted")
//   - aggregateID: The ID of the aggregate that generated this event
//   - userID: The ID of the user who triggered this event
//   - accountID: The ID of the account this event belongs to
//   - payload: The event-specific data payload
//
// Example:
//
//	payload := map[string]interface{}{"email": "user@example.com", "name": "John Doe"}
//	event := NewEntityEvent("user", "created", "user-123", "admin-456", "account-789", payload)
//	// event.EventType() returns "user.created"
func NewEntityEvent(entityType, eventType, aggregateID, userID, accountID string, payload any) EntityEvent {
	return EntityEvent{
		EntityType:  entityType,
		Type:        eventType,
		AggregateId: aggregateID,
		SequenceNum: 0, // Will be set by the entity when the event is added
		CreatedTime: time.Now(),
		UserId:      userID,
		AccountId:   accountID,
		Data:        payload,
	}
}

// EventType returns the full event type as a concatenation of EntityType and Type.
// Format: "entitytype.eventtype" (e.g., "user.created", "order.shipped")
func (e EntityEvent) EventType() string {
	return e.EntityType + "." + e.Type
}

// AggregateID returns the ID of the aggregate that generated this event.
func (e EntityEvent) AggregateID() string {
	return e.AggregateId
}

// SequenceNo returns the sequence number of this event.
func (e EntityEvent) SequenceNo() int64 {
	return e.SequenceNum
}

// CreatedAt returns the timestamp when this event was created.
func (e EntityEvent) CreatedAt() time.Time {
	return e.CreatedTime
}

// User returns the user ID associated with this event.
func (e EntityEvent) User() string {
	return e.UserId
}

// Account returns the account ID associated with this event.
func (e EntityEvent) Account() string {
	return e.AccountId
}

// Payload returns the event-specific data payload.
func (e EntityEvent) Payload() any {
	return e.Data
}
