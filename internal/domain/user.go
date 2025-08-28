// Package domain contains the internal domain models used for examples and testing
package domain

//go:generate moq -out mocks/user_repository_mock.go -pkg mocks . UserRepository

import (
	"errors"

	"github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/segmentio/ksuid"
)

// User represents a user aggregate for internal examples and testing
type User struct {
	id                ksuid.KSUID
	email             string
	name              string
	isActive          bool
	version           int
	uncommittedEvents []domain.Event
}

// NewUser creates a new user aggregate
func NewUser(email, name string) (*User, error) {
	if email == "" {
		return nil, errors.New("email cannot be empty")
	}
	if name == "" {
		return nil, errors.New("name cannot be empty")
	}

	id := ksuid.New()
	user := &User{
		id:                id,
		email:             email,
		name:              name,
		isActive:          true,
		version:           1,
		uncommittedEvents: make([]domain.Event, 0),
	}

	event := NewUserCreatedEvent(id, email, name, id.String(), user.version)
	user.recordEvent(event)

	return user, nil
}

// ID returns the user's ID as a string (implements AggregateRoot)
func (u *User) ID() string {
	return u.id.String()
}

// UserID returns the user's ID as KSUID
func (u *User) UserID() ksuid.KSUID {
	return u.id
}

// Version returns the current version of the aggregate
func (u *User) Version() int {
	return u.version
}

// UncommittedEvents returns the list of events that have been generated but not yet persisted
func (u *User) UncommittedEvents() []domain.Event {
	return u.uncommittedEvents
}

// MarkEventsAsCommitted clears the uncommitted events after they have been successfully persisted
func (u *User) MarkEventsAsCommitted() {
	u.uncommittedEvents = make([]domain.Event, 0)
}

// LoadFromHistory reconstructs the aggregate state from a sequence of events
func (u *User) LoadFromHistory(events []domain.Event) {
	for _, event := range events {
		u.applyEvent(event)
		u.version = event.Version()
	}
}

// recordEvent adds an event to the uncommitted events list
func (u *User) recordEvent(event domain.Event) {
	u.uncommittedEvents = append(u.uncommittedEvents, event)
}

// applyEvent applies an event to the aggregate state
func (u *User) applyEvent(event domain.Event) {
	switch e := event.(type) {
	case UserCreatedEvent:
		u.id = e.UserID
		u.email = e.Email
		u.name = e.Name
		u.isActive = true
	case UserEmailUpdatedEvent:
		u.email = e.NewEmail
	case UserNameUpdatedEvent:
		u.name = e.NewName
	case UserDeactivatedEvent:
		u.isActive = false
	case UserActivatedEvent:
		u.isActive = true
	}
}

// Email returns the user's email
func (u *User) Email() string {
	return u.email
}

// Name returns the user's name
func (u *User) Name() string {
	return u.name
}

// IsActive returns whether the user is active
func (u *User) IsActive() bool {
	return u.isActive
}

// UpdateEmail updates the user's email
func (u *User) UpdateEmail(newEmail string) error {
	if newEmail == "" {
		return errors.New("email cannot be empty")
	}
	if newEmail == u.email {
		return nil // No change needed
	}

	oldEmail := u.email
	u.version++

	event := NewUserEmailUpdatedEvent(u.id, oldEmail, newEmail, u.id.String(), u.version)
	u.recordEvent(event)
	u.applyEvent(event)

	return nil
}

// UpdateName updates the user's name
func (u *User) UpdateName(newName string) error {
	if newName == "" {
		return errors.New("name cannot be empty")
	}
	if newName == u.name {
		return nil // No change needed
	}

	oldName := u.name
	u.version++

	event := NewUserNameUpdatedEvent(u.id, oldName, newName, u.id.String(), u.version)
	u.recordEvent(event)
	u.applyEvent(event)

	return nil
}

// Deactivate deactivates the user
func (u *User) Deactivate() error {
	if !u.isActive {
		return nil // Already deactivated
	}

	u.version++

	event := NewUserDeactivatedEvent(u.id, u.id.String(), u.version)
	u.recordEvent(event)
	u.applyEvent(event)

	return nil
}

// Activate activates the user
func (u *User) Activate() error {
	if u.isActive {
		return nil // Already active
	}

	u.version++

	event := NewUserActivatedEvent(u.id, u.id.String(), u.version)
	u.recordEvent(event)
	u.applyEvent(event)

	return nil
}

// UserRepository defines the repository interface for users
type UserRepository interface {
	Save(user *User) error
	FindByID(id ksuid.KSUID) (*User, error)
	FindByEmail(email string) (*User, error)
	Delete(id ksuid.KSUID) error
	LoadFromVersion(id ksuid.KSUID, version int) (*User, error)
}
