package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// StandardEvent provides a generic event implementation that can be used across projects
// to avoid creating many specific event types. It contains the essential information
// needed for most domain events.
//
// Usage examples:
//
//	// User creation event
//	event := domain.NewEvent(
//	    "user-123",
//	    "User",
//	    "Created",
//	    map[string]interface{}{
//	        "email": "john@example.com",
//	        "name":  "John Doe",
//	    },
//	)
//
//	// User email update event
//	event := domain.NewEvent(
//	    "user-123",
//	    "User",
//	    "EmailUpdated",
//	    map[string]interface{}{
//	        "oldEmail": "john@example.com",
//	        "newEmail": "john.doe@example.com",
//	    },
//	)
//
//	// Order status change event
//	event := domain.NewEvent(
//	    "order-456",
//	    "Order",
//	    "StatusChanged",
//	    map[string]interface{}{
//	        "oldStatus": "pending",
//	        "newStatus": "confirmed",
//	        "reason":    "payment_received",
//	    },
//	)
type StandardEvent struct {
	// aggregateID is the ID of the aggregate that generated this event
	aggregateID string

	// entityType represents the type of entity (e.g., "User", "Order", "Product")
	entityType string

	// actionType represents the action that occurred (e.g., "Created", "Updated", "Deleted", "StatusChanged")
	actionType string

	// data contains the event-specific data as a flexible map
	data map[string]interface{}

	// version is the version of the aggregate when this event occurred
	version int

	// occurredAt is the timestamp when this event occurred
	occurredAt time.Time

	// metadata contains additional metadata for the event
	metadata map[string]interface{}
}

// NewEvent creates a new standard event with the given parameters.
// The event is automatically timestamped with the current time.
//
// Parameters:
//   - aggregateID: The ID of the aggregate that generated this event
//   - entityType: The type of entity (e.g., "User", "Order", "Product")
//   - actionType: The action that occurred (e.g., "Created", "Updated", "Deleted")
//   - data: Event-specific data as a map
//
// Example:
//
//	event := domain.NewEvent(
//	    "user-123",
//	    "User",
//	    "Created",
//	    map[string]interface{}{
//	        "email": "john@example.com",
//	        "name":  "John Doe",
//	        "active": true,
//	    },
//	)
func NewEvent(aggregateID, entityType, actionType string, data map[string]interface{}) *StandardEvent {
	return &StandardEvent{
		aggregateID: aggregateID,
		entityType:  entityType,
		actionType:  actionType,
		data:        data,
		version:     1, // Default version, can be updated when adding to aggregate
		occurredAt:  time.Now(),
		metadata:    make(map[string]interface{}),
	}
}

// NewEventWithTime creates a new standard event with a specific timestamp.
// This is useful for testing or when you need to set a specific occurrence time.
func NewEventWithTime(aggregateID, entityType, actionType string, data map[string]interface{}, occurredAt time.Time) *StandardEvent {
	return &StandardEvent{
		aggregateID: aggregateID,
		entityType:  entityType,
		actionType:  actionType,
		data:        data,
		version:     1,
		occurredAt:  occurredAt,
		metadata:    make(map[string]interface{}),
	}
}

// EventType returns the event type in the format "EntityType.ActionType".
// This implements the Event interface.
//
// Examples:
//   - "User.Created"
//   - "User.EmailUpdated"
//   - "Order.StatusChanged"
//   - "Product.Deleted"
func (e *StandardEvent) EventType() string {
	return fmt.Sprintf("%s.%s", e.entityType, e.actionType)
}

// AggregateID returns the ID of the aggregate that generated this event.
// This implements the Event interface.
func (e *StandardEvent) AggregateID() string {
	return e.aggregateID
}

// Version returns the version of the aggregate when this event occurred.
// This implements the Event interface.
func (e *StandardEvent) Version() int {
	return e.version
}

// OccurredAt returns the timestamp when this event occurred.
// This implements the Event interface.
func (e *StandardEvent) OccurredAt() time.Time {
	return e.occurredAt
}

// EntityType returns the type of entity that generated this event.
// This is useful for event handlers that need to know the entity type.
func (e *StandardEvent) EntityType() string {
	return e.entityType
}

// ActionType returns the action that occurred.
// This is useful for event handlers that need to know the specific action.
func (e *StandardEvent) ActionType() string {
	return e.actionType
}

// Data returns a copy of the event data to prevent external modification.
// Use GetDataValue() or typed getter methods for accessing specific values.
func (e *StandardEvent) Data() map[string]interface{} {
	// Return a copy to prevent external modification
	dataCopy := make(map[string]interface{}, len(e.data))
	for k, v := range e.data {
		dataCopy[k] = v
	}
	return dataCopy
}

// GetDataValue returns a specific value from the event data.
// Returns nil if the key doesn't exist.
//
// Example:
//
//	email := event.GetDataValue("email")
//	if emailStr, ok := email.(string); ok {
//	    fmt.Println("Email:", emailStr)
//	}
func (e *StandardEvent) GetDataValue(key string) interface{} {
	return e.data[key]
}

// GetDataString returns a string value from the event data.
// Returns empty string if the key doesn't exist or the value is not a string.
func (e *StandardEvent) GetDataString(key string) string {
	if value, exists := e.data[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

// GetDataInt returns an int value from the event data.
// Returns 0 if the key doesn't exist or the value is not an int.
func (e *StandardEvent) GetDataInt(key string) int {
	if value, exists := e.data[key]; exists {
		switch v := value.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

// GetDataBool returns a bool value from the event data.
// Returns false if the key doesn't exist or the value is not a bool.
func (e *StandardEvent) GetDataBool(key string) bool {
	if value, exists := e.data[key]; exists {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return false
}

// GetDataTime returns a time.Time value from the event data.
// Returns zero time if the key doesn't exist or the value is not a time.Time.
func (e *StandardEvent) GetDataTime(key string) time.Time {
	if value, exists := e.data[key]; exists {
		if t, ok := value.(time.Time); ok {
			return t
		}
		// Try to parse string as RFC3339
		if str, ok := value.(string); ok {
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// SetDataValue sets a value in the event data.
// Note: This should only be used during event construction, not after the event is created.
func (e *StandardEvent) SetDataValue(key string, value interface{}) {
	if e.data == nil {
		e.data = make(map[string]interface{})
	}
	e.data[key] = value
}

// Metadata returns a copy of the event metadata.
func (e *StandardEvent) Metadata() map[string]interface{} {
	metadataCopy := make(map[string]interface{}, len(e.metadata))
	for k, v := range e.metadata {
		metadataCopy[k] = v
	}
	return metadataCopy
}

// SetMetadata sets a metadata value.
func (e *StandardEvent) SetMetadata(key string, value interface{}) {
	if e.metadata == nil {
		e.metadata = make(map[string]interface{})
	}
	e.metadata[key] = value
}

// GetMetadata returns a specific metadata value.
func (e *StandardEvent) GetMetadata(key string) interface{} {
	return e.metadata[key]
}

// SetVersion sets the version of the event.
// This is typically called by the Entity when adding the event.
func (e *StandardEvent) SetVersion(version int) {
	e.version = version
}

// WithVersion returns a new StandardEvent with the specified version.
// This is useful for method chaining during event creation.
func (e *StandardEvent) WithVersion(version int) *StandardEvent {
	e.version = version
	return e
}

// WithMetadata returns a new StandardEvent with additional metadata.
// This is useful for method chaining during event creation.
func (e *StandardEvent) WithMetadata(key string, value interface{}) *StandardEvent {
	e.SetMetadata(key, value)
	return e
}

// String returns a string representation of the event for debugging.
func (e *StandardEvent) String() string {
	return fmt.Sprintf("StandardEvent{Type: %s, AggregateID: %s, Version: %d, OccurredAt: %s}",
		e.EventType(), e.aggregateID, e.version, e.occurredAt.Format(time.RFC3339))
}

// MarshalJSON implements json.Marshaler for custom JSON serialization.
func (e *StandardEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"event_type":   e.EventType(),
		"aggregate_id": e.aggregateID,
		"entity_type":  e.entityType,
		"action_type":  e.actionType,
		"version":      e.version,
		"occurred_at":  e.occurredAt.Format(time.RFC3339),
		"data":         e.data,
		"metadata":     e.metadata,
	})
}

// UnmarshalJSON implements json.Unmarshaler for custom JSON deserialization.
func (e *StandardEvent) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Extract basic fields
	if aggregateID, ok := raw["aggregate_id"].(string); ok {
		e.aggregateID = aggregateID
	}

	if entityType, ok := raw["entity_type"].(string); ok {
		e.entityType = entityType
	}

	if actionType, ok := raw["action_type"].(string); ok {
		e.actionType = actionType
	}

	if version, ok := raw["version"].(float64); ok {
		e.version = int(version)
	}

	if occurredAtStr, ok := raw["occurred_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, occurredAtStr); err == nil {
			e.occurredAt = t
		}
	}

	// Extract data
	if eventData, ok := raw["data"].(map[string]interface{}); ok {
		e.data = eventData
	} else {
		e.data = make(map[string]interface{})
	}

	// Extract metadata
	if metadata, ok := raw["metadata"].(map[string]interface{}); ok {
		e.metadata = metadata
	} else {
		e.metadata = make(map[string]interface{})
	}

	return nil
}
