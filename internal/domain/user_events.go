package domain

import (
	"time"

	"github.com/google/uuid"
)

// UserCreatedEvent represents the event when a user is created
type UserCreatedEvent struct {
	UserID      uuid.UUID
	Email       string
	Name        string
	aggregateID string
	version     int
	occurredAt  time.Time
}

// NewUserCreatedEvent creates a new UserCreatedEvent
func NewUserCreatedEvent(userID uuid.UUID, email, name string, aggregateID string, version int) UserCreatedEvent {
	return UserCreatedEvent{
		UserID:      userID,
		Email:       email,
		Name:        name,
		aggregateID: aggregateID,
		version:     version,
		occurredAt:  time.Now(),
	}
}

// EventType returns the event type identifier
func (e UserCreatedEvent) EventType() string {
	return "UserCreated"
}

// AggregateID returns the ID of the aggregate that generated this event
func (e UserCreatedEvent) AggregateID() string {
	return e.aggregateID
}

// Version returns the version of the aggregate when this event occurred
func (e UserCreatedEvent) Version() int {
	return e.version
}

// OccurredAt returns the timestamp when this event occurred
func (e UserCreatedEvent) OccurredAt() time.Time {
	return e.occurredAt
}

// UserEmailUpdatedEvent represents the event when a user's email is updated
type UserEmailUpdatedEvent struct {
	UserID      uuid.UUID
	OldEmail    string
	NewEmail    string
	aggregateID string
	version     int
	occurredAt  time.Time
}

// NewUserEmailUpdatedEvent creates a new UserEmailUpdatedEvent
func NewUserEmailUpdatedEvent(userID uuid.UUID, oldEmail, newEmail string, aggregateID string, version int) UserEmailUpdatedEvent {
	return UserEmailUpdatedEvent{
		UserID:      userID,
		OldEmail:    oldEmail,
		NewEmail:    newEmail,
		aggregateID: aggregateID,
		version:     version,
		occurredAt:  time.Now(),
	}
}

// EventType returns the event type identifier
func (e UserEmailUpdatedEvent) EventType() string {
	return "UserEmailUpdated"
}

// AggregateID returns the ID of the aggregate that generated this event
func (e UserEmailUpdatedEvent) AggregateID() string {
	return e.aggregateID
}

// Version returns the version of the aggregate when this event occurred
func (e UserEmailUpdatedEvent) Version() int {
	return e.version
}

// OccurredAt returns the timestamp when this event occurred
func (e UserEmailUpdatedEvent) OccurredAt() time.Time {
	return e.occurredAt
}

// UserNameUpdatedEvent represents the event when a user's name is updated
type UserNameUpdatedEvent struct {
	UserID      uuid.UUID
	OldName     string
	NewName     string
	aggregateID string
	version     int
	occurredAt  time.Time
}

// NewUserNameUpdatedEvent creates a new UserNameUpdatedEvent
func NewUserNameUpdatedEvent(userID uuid.UUID, oldName, newName string, aggregateID string, version int) UserNameUpdatedEvent {
	return UserNameUpdatedEvent{
		UserID:      userID,
		OldName:     oldName,
		NewName:     newName,
		aggregateID: aggregateID,
		version:     version,
		occurredAt:  time.Now(),
	}
}

// EventType returns the event type identifier
func (e UserNameUpdatedEvent) EventType() string {
	return "UserNameUpdated"
}

// AggregateID returns the ID of the aggregate that generated this event
func (e UserNameUpdatedEvent) AggregateID() string {
	return e.aggregateID
}

// Version returns the version of the aggregate when this event occurred
func (e UserNameUpdatedEvent) Version() int {
	return e.version
}

// OccurredAt returns the timestamp when this event occurred
func (e UserNameUpdatedEvent) OccurredAt() time.Time {
	return e.occurredAt
}

// UserDeactivatedEvent represents the event when a user is deactivated
type UserDeactivatedEvent struct {
	UserID      uuid.UUID
	aggregateID string
	version     int
	occurredAt  time.Time
}

// NewUserDeactivatedEvent creates a new UserDeactivatedEvent
func NewUserDeactivatedEvent(userID uuid.UUID, aggregateID string, version int) UserDeactivatedEvent {
	return UserDeactivatedEvent{
		UserID:      userID,
		aggregateID: aggregateID,
		version:     version,
		occurredAt:  time.Now(),
	}
}

// EventType returns the event type identifier
func (e UserDeactivatedEvent) EventType() string {
	return "UserDeactivated"
}

// AggregateID returns the ID of the aggregate that generated this event
func (e UserDeactivatedEvent) AggregateID() string {
	return e.aggregateID
}

// Version returns the version of the aggregate when this event occurred
func (e UserDeactivatedEvent) Version() int {
	return e.version
}

// OccurredAt returns the timestamp when this event occurred
func (e UserDeactivatedEvent) OccurredAt() time.Time {
	return e.occurredAt
}

// UserActivatedEvent represents the event when a user is activated
type UserActivatedEvent struct {
	UserID      uuid.UUID
	aggregateID string
	version     int
	occurredAt  time.Time
}

// NewUserActivatedEvent creates a new UserActivatedEvent
func NewUserActivatedEvent(userID uuid.UUID, aggregateID string, version int) UserActivatedEvent {
	return UserActivatedEvent{
		UserID:      userID,
		aggregateID: aggregateID,
		version:     version,
		occurredAt:  time.Now(),
	}
}

// EventType returns the event type identifier
func (e UserActivatedEvent) EventType() string {
	return "UserActivated"
}

// AggregateID returns the ID of the aggregate that generated this event
func (e UserActivatedEvent) AggregateID() string {
	return e.aggregateID
}

// Version returns the version of the aggregate when this event occurred
func (e UserActivatedEvent) Version() int {
	return e.version
}

// OccurredAt returns the timestamp when this event occurred
func (e UserActivatedEvent) OccurredAt() time.Time {
	return e.occurredAt
}
