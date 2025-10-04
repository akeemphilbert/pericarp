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

//go:generate moq -out mocks/event_store_mock.go -pkg mocks . EventStore
//go:generate moq -out mocks/event_dispatcher_mock.go -pkg mocks . EventDispatcher
//go:generate moq -out mocks/event_handler_mock.go -pkg mocks . EventHandler
//go:generate moq -out mocks/unit_of_work_mock.go -pkg mocks . UnitOfWork
//go:generate moq -out mocks/event_mock.go -pkg mocks . Event
//go:generate moq -out mocks/envelope_mock.go -pkg mocks . Envelope

import (
	"context"
	"encoding/json"
	"time"
)

type ContextKey string

const (
	UserID    ContextKey = "user_id"
	AccountID ContextKey = "account_id"
	Source    ContextKey = "source"
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
//	    GetSequenceNo int64     `json:"sequence_no"`
//	}
//
//	func (e UserCreatedEvent) EventType() string { return "UserCreated" }
//	func (e UserCreatedEvent) AggregateID() string { return e.UserID }
//	func (e UserCreatedEvent) GetSequenceNo() int64 { return e.GetSequenceNo }
//	func (e UserCreatedEvent) CreatedAt() time.Time { return e.CreatedAt }
type Event interface {
	// EventType returns the type identifier for this event.
	// This should be a stable string that uniquely identifies the event type
	// across versions (e.g., "UserCreated", "OrderShipped").
	EventType() string

	// AggregateID returns the GetID of the aggregate that generated this event.
	// This is used to group events by aggregate for event sourcing.
	AggregateID() string

	// SequenceNo returns the sequence number of the aggregate when this event occurred.
	// This is used for optimistic concurrency control and event ordering.
	SequenceNo() int64

	// CreatedAt returns the timestamp when this event was created in the domain.
	// This should represent the business time, not the technical persistence time.
	CreatedAt() time.Time

	// User returns the user GetID associated with this event.
	// This is useful for auditing and tracking which user performed the action.
	User() string

	// Account returns the account GetID associated with this event.
	// This is useful for multi-tenant systems to track which account the event belongs to.
	Account() string

	// Payload returns the event-specific data payload as a byte slice.
	// This contains the actual business data associated with the event.
	Payload() []byte

	// SetSequenceNo sets the sequence number for this event.
	// This is typically called by the aggregate when the event is added
	// to ensure proper ordering and concurrency control.
	SetSequenceNo(sequenceNo int64)
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
	// This is different from the domain event's aggregate GetID and is used
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
	// Start initializes the dispatcher, setting up any necessary resources
	Start() error
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
//	            GetID:    event.UserID,
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

// EntityEvent provides a flexible implementation of the Event interface that can be
// used for all domain events. It handles common event concerns like entity type,
// event type, sequence numbers, and metadata with a JSON payload.
//
// The EventType() method returns a concatenation of EntityType and Type
// in the format "entitytype.eventtype" (e.g., "user.created", "order.shipped").
//
// Example usage:
//
//	// Create a user created event
//	user := &User{GetID: "user-123", Email: "john@example.com", Name: "John Doe"}
//	event := NewEntityEvent("user", "created", "user-123", "admin-456", "account-789", user)
//
//	// Access event data through the payload
//	var userData User
//	json.Unmarshal(event.Payload(), &userData)
//	email := userData.Email
//	name := userData.Name
type EntityEvent struct {
	EntityType  string                 `json:"entity_type"`
	Type        string                 `json:"type"`
	AggregateId string                 `json:"aggregate_id"`
	SequenceNum int64                  `json:"sequence_no"`
	CreatedTime time.Time              `json:"created_at"`
	UserId      string                 `json:"user_id"`
	AccountId   string                 `json:"account_id"`
	Metadata    map[string]interface{} `json:"metadata"`
	PayloadData []byte                 `json:"payload"`
}

// NewEntityEvent creates a new EntityEvent with the specified parameters.
// The entityType and eventType are combined to form the full event type.
//
// Parameters:
//   - entityType: The type of entity (e.g., "user", "order", "product")
//   - eventType: The type of event (e.g., "created", "updated", "deleted")
//   - aggregateID: The GetID of the aggregate that generated this event
//   - userID: The GetID of the user who triggered this event
//   - accountID: The GetID of the account this event belongs to
//   - data: Any serializable data to include as the event payload
//
// Example:
//
//	user := &User{GetID: "user-123", Email: "user@example.com", Name: "John Doe"}
//	event := NewEntityEvent("user", "created", "user-123", "admin-456", "account-789", user)
//	// event.EventType() returns "user.created"
func NewEntityEvent(ctx context.Context, logger Logger, entityType, eventType, aggregateID string, data interface{}) *EntityEvent {
	// Marshal data to JSON bytes for the payload
	payload, err := json.Marshal(data)
	if err != nil {
		payload = []byte{}
	}

	var userID, accountID string
	var ok bool
	if ctx != nil {
		if userID, ok = ctx.Value(UserID).(string); !ok {
			logger.Warn(
				"User GetID not found in context",
				"entity_type", entityType,
				"event_type", eventType,
				"aggregate_id", aggregateID,
				"user_id", userID)
		}
		if accountID, ok = ctx.Value(AccountID).(string); !ok {
			logger.Warn("Account GetID not found in context", "entity_type", entityType, "event_type", eventType, "aggregate_id", aggregateID)
		}
	}

	return &EntityEvent{
		EntityType:  entityType,
		Type:        eventType,
		AggregateId: aggregateID,
		SequenceNum: 0, // Will be set by the entity when the event is added
		CreatedTime: time.Now(),
		UserId:      userID,
		AccountId:   accountID,
		Metadata:    make(map[string]interface{}),
		PayloadData: payload,
	}
}

// EventType returns the full event type as a concatenation of EntityType and Type.
// Format: "entitytype.eventtype" (e.g., "user.created", "order.shipped")
func (e EntityEvent) EventType() string {
	return e.EntityType + "." + e.Type
}

// AggregateID returns the GetID of the aggregate that generated this event.
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

// User returns the user GetID associated with this event.
func (e EntityEvent) User() string {
	return e.UserId
}

// Account returns the account GetID associated with this event.
func (e EntityEvent) Account() string {
	return e.AccountId
}

// Payload returns the event-specific data payload as a byte slice.
func (e EntityEvent) Payload() []byte {
	return e.PayloadData
}

// SetSequenceNo sets the sequence number for this event.
// This method modifies the event, so it should be called before the event
// is considered immutable (typically when it's added to an aggregate).
func (e *EntityEvent) SetSequenceNo(sequenceNo int64) {
	e.SequenceNum = sequenceNo
}

// SetMetadata sets a metadata value.
func (e *EntityEvent) SetMetadata(key string, value interface{}) {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
}

// GetMetadata returns a specific metadata value.
func (e *EntityEvent) GetMetadata(key string) interface{} {
	return e.Metadata[key]
}

// StandardEvent provides a generic implementation of the Event interface that can be
// created from a map of data. This is useful for creating events from external sources
// like API payloads, database records, or other map-based data structures.
//
// The StandardEvent uses a map to store all event data, making it very flexible for
// handling various event structures without requiring specific type definitions.
//
// Example usage:
//
//	data := map[string]interface{}{
//		"event_type": "user.created",
//		"aggregate_id": "user-123",
//		"user_id": "admin-456",
//		"account_id": "account-789",
//		"email": "john@example.com",
//		"name": "John Doe",
//		"created_at": time.Now(),
//	}
//	event := NewStandardEventFromMap(data)
//
//	// Access data using helper methods
//	email, _ := event.GetString("email")
//	name, _ := event.GetString("name")
type StandardEvent struct {
	data map[string]interface{}
}

// NewStandardEventFromMap creates a new StandardEvent from a map of data.
// The map should contain at minimum the required fields for an event:
//   - event_type: The type of the event (e.g., "user.created")
//   - aggregate_id: The GetID of the aggregate that generated this event
//   - user_id: The GetID of the user who triggered this event
//   - account_id: The GetID of the account this event belongs to
//
// Optional fields:
//   - sequence_no: The sequence number (defaults to 0)
//   - created_at: The creation timestamp (defaults to time.Now())
//   - Any other custom fields will be stored in the event data
//
// The function will set default values for missing required fields and will
// marshal the entire map as the event payload.
func NewStandardEventFromMap(data map[string]interface{}) *StandardEvent {
	// Ensure we have a valid map
	if data == nil {
		data = make(map[string]interface{})
	}

	// Set defaults for required fields if not present
	if _, exists := data["sequence_no"]; !exists {
		data["sequence_no"] = int64(0)
	}
	if _, exists := data["created_at"]; !exists {
		data["created_at"] = time.Now()
	}

	return &StandardEvent{
		data: data,
	}
}

// EventType returns the event type from the data map.
func (e StandardEvent) EventType() string {
	if eventType, exists := e.data["event_type"]; exists {
		if str, ok := eventType.(string); ok {
			return str
		}
	}
	return ""
}

// AggregateID returns the aggregate GetID from the data map.
func (e StandardEvent) AggregateID() string {
	if aggregateID, exists := e.data["aggregate_id"]; exists {
		if str, ok := aggregateID.(string); ok {
			return str
		}
	}
	return ""
}

// SequenceNo returns the sequence number from the data map.
func (e StandardEvent) SequenceNo() int64 {
	if seqNo, exists := e.data["sequence_no"]; exists {
		switch v := seqNo.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		}
	}
	return 0
}

// CreatedAt returns the creation timestamp from the data map.
func (e StandardEvent) CreatedAt() time.Time {
	if createdAt, exists := e.data["created_at"]; exists {
		if t, ok := createdAt.(time.Time); ok {
			return t
		}
		// Handle string timestamps
		if str, ok := createdAt.(string); ok {
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				return t
			}
		}
	}
	return time.Now()
}

// User returns the user GetID from the data map.
func (e StandardEvent) User() string {
	if userID, exists := e.data["user_id"]; exists {
		if str, ok := userID.(string); ok {
			return str
		}
	}
	return ""
}

// Account returns the account GetID from the data map.
func (e StandardEvent) Account() string {
	if accountID, exists := e.data["account_id"]; exists {
		if str, ok := accountID.(string); ok {
			return str
		}
	}
	return ""
}

// Payload returns the entire data map as JSON bytes.
func (e StandardEvent) Payload() []byte {
	payload, err := json.Marshal(e.data)
	if err != nil {
		return []byte{}
	}
	return payload
}

// SetSequenceNo sets the sequence number in the data map.
func (e *StandardEvent) SetSequenceNo(sequenceNo int64) {
	e.data["sequence_no"] = sequenceNo
}

// GetString retrieves a string value from the event data.
// Returns the value and a boolean indicating if the key exists and is a string.
func (e StandardEvent) GetString(key string) (string, bool) {
	if value, exists := e.data[key]; exists {
		if str, ok := value.(string); ok {
			return str, true
		}
	}
	return "", false
}

// GetInt retrieves an integer value from the event data.
// Returns the value and a boolean indicating if the key exists and is an integer.
func (e StandardEvent) GetInt(key string) (int, bool) {
	if value, exists := e.data[key]; exists {
		switch v := value.(type) {
		case int:
			return v, true
		case int64:
			return int(v), true
		case float64:
			return int(v), true
		}
	}
	return 0, false
}

// GetInt64 retrieves an int64 value from the event data.
// Returns the value and a boolean indicating if the key exists and is an int64.
func (e StandardEvent) GetInt64(key string) (int64, bool) {
	if value, exists := e.data[key]; exists {
		switch v := value.(type) {
		case int64:
			return v, true
		case int:
			return int64(v), true
		case float64:
			return int64(v), true
		}
	}
	return 0, false
}

// GetFloat64 retrieves a float64 value from the event data.
// Returns the value and a boolean indicating if the key exists and is a float64.
func (e StandardEvent) GetFloat64(key string) (float64, bool) {
	if value, exists := e.data[key]; exists {
		if f, ok := value.(float64); ok {
			return f, true
		}
	}
	return 0, false
}

// GetBool retrieves a boolean value from the event data.
// Returns the value and a boolean indicating if the key exists and is a boolean.
func (e StandardEvent) GetBool(key string) (bool, bool) {
	if value, exists := e.data[key]; exists {
		if b, ok := value.(bool); ok {
			return b, true
		}
	}
	return false, false
}

// GetTime retrieves a time.Time value from the event data.
// Returns the value and a boolean indicating if the key exists and is a time.Time.
func (e StandardEvent) GetTime(key string) (time.Time, bool) {
	if value, exists := e.data[key]; exists {
		if t, ok := value.(time.Time); ok {
			return t, true
		}
		// Handle string timestamps
		if str, ok := value.(string); ok {
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

// GetInterface retrieves any value from the event data.
// Returns the value and a boolean indicating if the key exists.
func (e StandardEvent) GetInterface(key string) (interface{}, bool) {
	value, exists := e.data[key]
	return value, exists
}

// SetData sets a value in the event data map.
func (e *StandardEvent) SetData(key string, value interface{}) {
	e.data[key] = value
}

// GetAllData returns a copy of the entire data map.
func (e StandardEvent) GetAllData() map[string]interface{} {
	// Return a copy to prevent external modification
	result := make(map[string]interface{})
	for k, v := range e.data {
		result[k] = v
	}
	return result
}
