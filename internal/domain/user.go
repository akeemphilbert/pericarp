// Package domain contains the internal domain models used for examples and testing
package domain

//go:generate moq -out mocks/user_repository_mock.go -pkg mocks . UserRepository

import (
	"encoding/json"
	"fmt"

	"github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/segmentio/ksuid"
)

// User represents a user aggregate for internal examples and testing
type User struct {
	*domain.Entity
	Email  string `json:"email"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// WithEmail creates a new user aggregate
func (u *User) WithEmail(email, name string) *User {
	// Initialize Entity first
	u.Entity = new(domain.Entity).WithID(ksuid.New().String())

	if email == "" {
		u.AddError(fmt.Errorf("must specify valid email address"))
		return u
	}
	if name == "" {
		u.AddError(fmt.Errorf("must specify user name"))
		return u
	}

	u.Email = email
	u.Name = name
	u.Active = true
	u.AddEvent(domain.NewEntityEvent("User", "created", u.ID(), "", "", u))

	return u
}

// LoadFromHistory reconstructs the aggregate state from a sequence of events
func (u *User) LoadFromHistory(events []domain.Event) {
	// Call base Entity LoadFromHistory to update sequence number
	u.Entity.LoadFromHistory(events)

	for _, event := range events {
		switch e := event.(type) {
		case *domain.EntityEvent:
			payloadRaw, err := json.Marshal(e.Payload())
			if err != nil {
				u.AddError(err)
				continue
			}
			err = json.Unmarshal(payloadRaw, u)
			if err != nil {
				u.AddError(err)
				continue
			}
		}
	}
}

// UpdateEmail updates the user's email
func (u *User) UpdateEmail(newEmail string) {
	if newEmail == "" {
		u.AddError(fmt.Errorf("invalid email address provided"))
		return
	}
	if newEmail == u.Email {
		u.AddError(fmt.Errorf("no change provided"))
		return
	}

	u.Email = newEmail
	u.AddEvent(domain.NewEntityEvent("User", "updated", u.ID(), "", "", u))
}

// Deactivate deactivates the user
func (u *User) Deactivate() {
	if !u.Active {
		u.AddError(fmt.Errorf("user already deactivated"))
		return
	}
	u.Active = false
	u.AddEvent(domain.NewEntityEvent("User", "updated", u.ID(), "", "", u))
}

// Activate activates the user
func (u *User) Activate() {
	if u.Active {
		u.AddError(fmt.Errorf("user already activated"))
		return
	}
	u.Active = true
	u.AddEvent(domain.NewEntityEvent("User", "updated", u.ID(), "", "", u))
}

// UserRepository defines the repository interface for users
type UserRepository interface {
	Save(user *User) error
	FindByID(id string) (*User, error)
	FindByEmail(email string) (*User, error)
	Delete(id string) error
	LoadFromVersion(id string, version int) (*User, error)
}
