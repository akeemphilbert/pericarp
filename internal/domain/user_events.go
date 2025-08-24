package domain

import (
	"time"

	"github.com/google/uuid"
)

// UserCreatedEvent represents the event when a user is created
type UserCreatedEvent struct {
	UserID uuid.UUID
	Email  string
	Name   string
}

// EventType returns the event type identifier
func (e UserCreatedEvent) EventType() string {
	return "UserCreated"
}

// UserEmailUpdatedEvent represents the event when a user's email is updated
type UserEmailUpdatedEvent struct {
	UserID   uuid.UUID
	OldEmail string
	NewEmail string
}

// EventType returns the event type identifier
func (e UserEmailUpdatedEvent) EventType() string {
	return "UserEmailUpdated"
}

// UserNameUpdatedEvent represents the event when a user's name is updated
type UserNameUpdatedEvent struct {
	UserID  uuid.UUID
	OldName string
	NewName string
}

// EventType returns the event type identifier
func (e UserNameUpdatedEvent) EventType() string {
	return "UserNameUpdated"
}

// UserDeactivatedEvent represents the event when a user is deactivated
type UserDeactivatedEvent struct {
	UserID uuid.UUID
}

// EventType returns the event type identifier
func (e UserDeactivatedEvent) EventType() string {
	return "UserDeactivated"
}

// UserActivatedEvent represents the event when a user is activated
type UserActivatedEvent struct {
	UserID uuid.UUID
}

// EventType returns the event type identifier
func (e UserActivatedEvent) EventType() string {
	return "UserActivated"
}
