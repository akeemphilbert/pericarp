package domain

import (
	"regexp"
	"strings"
)

// User represents a user aggregate in the domain
type User struct {
	id                string
	email             string
	name              string
	version           int
	uncommittedEvents []Event
}

// NewUser creates a new User aggregate
func NewUser(id, email, name string) (*User, error) {
	user := &User{
		id:                id,
		version:           0,
		uncommittedEvents: make([]Event, 0),
	}

	// Validate and set email
	if err := user.validateEmail(email); err != nil {
		return nil, err
	}
	user.email = email

	// Validate and set name
	if err := user.validateName(name); err != nil {
		return nil, err
	}
	user.name = name

	// Generate UserCreated event
	event := NewUserCreatedEvent(id, email, name, user.version+1)
	user.applyEvent(event)

	return user, nil
}

// CreateUser business method for creating a user
func (u *User) CreateUser(email, name string) error {
	// Validate email
	if err := u.validateEmail(email); err != nil {
		return err
	}

	// Validate name
	if err := u.validateName(name); err != nil {
		return err
	}

	// Generate UserCreated event
	event := NewUserCreatedEvent(u.id, email, name, u.version+1)
	u.applyEvent(event)

	return nil
}

// UpdateUserEmail business method for updating user email
func (u *User) UpdateUserEmail(newEmail string) error {
	// Validate new email
	if err := u.validateEmail(newEmail); err != nil {
		return err
	}

	// Check if email is actually changing
	if u.email == newEmail {
		return NewValidationError("email", "new email must be different from current email", newEmail)
	}

	// Generate UserEmailUpdated event
	event := NewUserEmailUpdatedEvent(u.id, u.email, newEmail, u.version+1)
	u.applyEvent(event)

	return nil
}

// ID returns the user's unique identifier
func (u *User) ID() string {
	return u.id
}

// Email returns the user's email address
func (u *User) Email() string {
	return u.email
}

// Name returns the user's name
func (u *User) Name() string {
	return u.name
}

// Version returns the current version of the aggregate
func (u *User) Version() int {
	return u.version
}

// UncommittedEvents returns the list of uncommitted events
func (u *User) UncommittedEvents() []Event {
	return u.uncommittedEvents
}

// MarkEventsAsCommitted clears the uncommitted events
func (u *User) MarkEventsAsCommitted() {
	u.uncommittedEvents = make([]Event, 0)
}

// LoadFromHistory reconstructs the aggregate from events
func (u *User) LoadFromHistory(events []Event) {
	for _, event := range events {
		u.applyEventFromHistory(event)
	}
}

// applyEvent applies an event and adds it to uncommitted events
func (u *User) applyEvent(event Event) {
	u.applyEventFromHistory(event)
	u.uncommittedEvents = append(u.uncommittedEvents, event)
}

// applyEventFromHistory applies an event without adding to uncommitted events
func (u *User) applyEventFromHistory(event Event) {
	switch e := event.(type) {
	case *UserCreatedEvent:
		u.email = e.Email
		u.name = e.Name
		u.version = e.Version()
	case *UserEmailUpdatedEvent:
		u.email = e.NewEmail
		u.version = e.Version()
	}
}

// validateEmail validates email format and business rules
func (u *User) validateEmail(email string) error {
	if email == "" {
		return NewValidationError("email", "email cannot be empty", email)
	}

	email = strings.TrimSpace(email)
	if len(email) > 254 {
		return NewValidationError("email", "email cannot exceed 254 characters", email)
	}

	// Basic email regex validation
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return NewValidationError("email", "invalid email format", email)
	}

	return nil
}

// validateName validates name format and business rules
func (u *User) validateName(name string) error {
	if name == "" {
		return NewValidationError("name", "name cannot be empty", name)
	}

	name = strings.TrimSpace(name)
	if len(name) < 2 {
		return NewValidationError("name", "name must be at least 2 characters long", name)
	}

	if len(name) > 100 {
		return NewValidationError("name", "name cannot exceed 100 characters", name)
	}

	return nil
}

// LoadUserFromHistory creates a User aggregate from historical events
func LoadUserFromHistory(id string, events []Event) *User {
	user := &User{
		id:                id,
		version:           0,
		uncommittedEvents: make([]Event, 0),
	}

	user.LoadFromHistory(events)
	return user
}
