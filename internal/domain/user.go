// Package domain contains the internal domain models used for examples and testing
package domain

import (
	"errors"
	"time"

	"github.com/example/pericarp/pkg/domain"
)

// User represents a user aggregate for internal examples and testing
type User struct {
	domain.AggregateRoot
	id       uuid.UUID
	email    string
	name     string
	isActive bool
}

// NewUser creates a new user aggregate
func NewUser(email, name string) (*User, error) {
	if email == "" {
		return nil, errors.New("email cannot be empty")
	}
	if name == "" {
		return nil, errors.New("name cannot be empty")
	}

	id := uuid.New()
	user := &User{
		id:       id,
		email:    email,
		name:     name,
		isActive: true,
	}

	user.RecordEvent(UserCreatedEvent{
		UserID: id,
		Email:  email,
		Name:   name,
	})

	return user, nil
}

// ID returns the user's ID
func (u *User) ID() uuid.UUID {
	return u.id
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
	u.email = newEmail

	u.RecordEvent(UserEmailUpdatedEvent{
		UserID:   u.id,
		OldEmail: oldEmail,
		NewEmail: newEmail,
	})

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
	u.name = newName

	u.RecordEvent(UserNameUpdatedEvent{
		UserID:  u.id,
		OldName: oldName,
		NewName: newName,
	})

	return nil
}

// Deactivate deactivates the user
func (u *User) Deactivate() error {
	if !u.isActive {
		return nil // Already deactivated
	}

	u.isActive = false

	u.RecordEvent(UserDeactivatedEvent{
		UserID: u.id,
	})

	return nil
}

// Activate activates the user
func (u *User) Activate() error {
	if u.isActive {
		return nil // Already active
	}

	u.isActive = true

	u.RecordEvent(UserActivatedEvent{
		UserID: u.id,
	})

	return nil
}

// UserRepository defines the repository interface for users
type UserRepository interface {
	Save(user *User) error
	FindByID(id uuid.UUID) (*User, error)
	FindByEmail(email string) (*User, error)
	Delete(id uuid.UUID) error
}
