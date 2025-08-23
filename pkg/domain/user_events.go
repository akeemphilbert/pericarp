package domain

import "time"

// UserCreatedEvent represents the event when a user is created
type UserCreatedEvent struct {
	aggregateID string
	Email       string
	Name        string
	version     int
	occurredAt  time.Time
}

// NewUserCreatedEvent creates a new UserCreatedEvent
func NewUserCreatedEvent(aggregateID, email, name string, version int) *UserCreatedEvent {
	return &UserCreatedEvent{
		aggregateID: aggregateID,
		Email:       email,
		Name:        name,
		version:     version,
		occurredAt:  time.Now(),
	}
}

// EventType returns the event type identifier
func (e *UserCreatedEvent) EventType() string {
	return "UserCreated"
}

// AggregateID returns the ID of the aggregate that generated this event
func (e *UserCreatedEvent) AggregateID() string {
	return e.aggregateID
}

// Version returns the version of the aggregate when this event occurred
func (e *UserCreatedEvent) Version() int {
	return e.version
}

// OccurredAt returns the timestamp when this event occurred
func (e *UserCreatedEvent) OccurredAt() time.Time {
	return e.occurredAt
}

// UserEmailUpdatedEvent represents the event when a user's email is updated
type UserEmailUpdatedEvent struct {
	aggregateID string
	OldEmail    string
	NewEmail    string
	version     int
	occurredAt  time.Time
}

// NewUserEmailUpdatedEvent creates a new UserEmailUpdatedEvent
func NewUserEmailUpdatedEvent(aggregateID, oldEmail, newEmail string, version int) *UserEmailUpdatedEvent {
	return &UserEmailUpdatedEvent{
		aggregateID: aggregateID,
		OldEmail:    oldEmail,
		NewEmail:    newEmail,
		version:     version,
		occurredAt:  time.Now(),
	}
}

// EventType returns the event type identifier
func (e *UserEmailUpdatedEvent) EventType() string {
	return "UserEmailUpdated"
}

// AggregateID returns the ID of the aggregate that generated this event
func (e *UserEmailUpdatedEvent) AggregateID() string {
	return e.aggregateID
}

// Version returns the version of the aggregate when this event occurred
func (e *UserEmailUpdatedEvent) Version() int {
	return e.version
}

// OccurredAt returns the timestamp when this event occurred
func (e *UserEmailUpdatedEvent) OccurredAt() time.Time {
	return e.occurredAt
}
