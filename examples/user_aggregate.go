package examples

import (
	"errors"
	"time"

	"github.com/example/pericarp/pkg/domain"
)

// User demonstrates how to use the Entity struct as a base for concrete aggregates.
// It embeds the Entity struct to inherit event sourcing capabilities.
type User struct {
	domain.Entity
	email    string
	name     string
	isActive bool
}

// NewUser creates a new user aggregate with the given details.
// It demonstrates the typical pattern of creating an aggregate and generating
// the initial domain event.
func NewUser(id, email, name string) (*User, error) {
	if id == "" {
		return nil, errors.New("user ID cannot be empty")
	}
	if email == "" {
		return nil, errors.New("email cannot be empty")
	}
	if name == "" {
		return nil, errors.New("name cannot be empty")
	}

	user := &User{
		Entity:   domain.NewEntity(id),
		email:    email,
		name:     name,
		isActive: true,
	}

	// Generate the initial domain event
	event := UserCreatedEvent{
		UserID:    id,
		Email:     email,
		Name:      name,
		CreatedAt: time.Now(),
	}

	user.AddEvent(event)
	return user, nil
}

// Email returns the user's email address
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

// ChangeEmail changes the user's email address and generates a domain event.
// This demonstrates how to implement business logic that generates events.
func (u *User) ChangeEmail(newEmail string) error {
	if newEmail == "" {
		return errors.New("email cannot be empty")
	}

	if newEmail == u.email {
		return nil // No change needed
	}

	// Apply the business logic
	oldEmail := u.email
	u.email = newEmail

	// Generate domain event
	event := UserEmailChangedEvent{
		UserID:    u.ID(),
		OldEmail:  oldEmail,
		NewEmail:  newEmail,
		ChangedAt: time.Now(),
	}

	u.AddEvent(event)
	return nil
}

// ChangeName changes the user's name and generates a domain event.
func (u *User) ChangeName(newName string) error {
	if newName == "" {
		return errors.New("name cannot be empty")
	}

	if newName == u.name {
		return nil // No change needed
	}

	// Apply the business logic
	oldName := u.name
	u.name = newName

	// Generate domain event
	event := UserNameChangedEvent{
		UserID:    u.ID(),
		OldName:   oldName,
		NewName:   newName,
		ChangedAt: time.Now(),
	}

	u.AddEvent(event)
	return nil
}

// Deactivate deactivates the user account.
func (u *User) Deactivate() error {
	if !u.isActive {
		return nil // Already deactivated
	}

	u.isActive = false

	event := UserDeactivatedEvent{
		UserID:        u.ID(),
		DeactivatedAt: time.Now(),
	}

	u.AddEvent(event)
	return nil
}

// Activate activates the user account.
func (u *User) Activate() error {
	if u.isActive {
		return nil // Already active
	}

	u.isActive = true

	event := UserActivatedEvent{
		UserID:      u.ID(),
		ActivatedAt: time.Now(),
	}

	u.AddEvent(event)
	return nil
}

// LoadFromHistory reconstructs the user aggregate from historical events.
// This overrides the base Entity.LoadFromHistory to apply domain-specific logic.
func (u *User) LoadFromHistory(events []domain.Event) {
	for _, event := range events {
		u.applyEvent(event)
	}

	// Call the base implementation to update version and sequence
	u.Entity.LoadFromHistory(events)
}

// applyEvent applies a single event to the user aggregate.
// This is used during aggregate reconstruction from event history.
func (u *User) applyEvent(event domain.Event) {
	switch e := event.(type) {
	case UserCreatedEvent:
		u.email = e.Email
		u.name = e.Name
		u.isActive = true

	case UserEmailChangedEvent:
		u.email = e.NewEmail

	case UserNameChangedEvent:
		u.name = e.NewName

	case UserDeactivatedEvent:
		u.isActive = false

	case UserActivatedEvent:
		u.isActive = true

	default:
		// Unknown event type - this is normal for forward compatibility
		// The aggregate should gracefully handle unknown events
	}
}

// Domain Events

// UserCreatedEvent represents the creation of a new user
type UserCreatedEvent struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (e UserCreatedEvent) EventType() string     { return "UserCreated" }
func (e UserCreatedEvent) AggregateID() string   { return e.UserID }
func (e UserCreatedEvent) Version() int          { return 1 }
func (e UserCreatedEvent) OccurredAt() time.Time { return e.CreatedAt }

// UserEmailChangedEvent represents a change in user's email
type UserEmailChangedEvent struct {
	UserID    string    `json:"user_id"`
	OldEmail  string    `json:"old_email"`
	NewEmail  string    `json:"new_email"`
	ChangedAt time.Time `json:"changed_at"`
}

func (e UserEmailChangedEvent) EventType() string     { return "UserEmailChanged" }
func (e UserEmailChangedEvent) AggregateID() string   { return e.UserID }
func (e UserEmailChangedEvent) Version() int          { return 1 }
func (e UserEmailChangedEvent) OccurredAt() time.Time { return e.ChangedAt }

// UserNameChangedEvent represents a change in user's name
type UserNameChangedEvent struct {
	UserID    string    `json:"user_id"`
	OldName   string    `json:"old_name"`
	NewName   string    `json:"new_name"`
	ChangedAt time.Time `json:"changed_at"`
}

func (e UserNameChangedEvent) EventType() string     { return "UserNameChanged" }
func (e UserNameChangedEvent) AggregateID() string   { return e.UserID }
func (e UserNameChangedEvent) Version() int          { return 1 }
func (e UserNameChangedEvent) OccurredAt() time.Time { return e.ChangedAt }

// UserDeactivatedEvent represents user deactivation
type UserDeactivatedEvent struct {
	UserID        string    `json:"user_id"`
	DeactivatedAt time.Time `json:"deactivated_at"`
}

func (e UserDeactivatedEvent) EventType() string     { return "UserDeactivated" }
func (e UserDeactivatedEvent) AggregateID() string   { return e.UserID }
func (e UserDeactivatedEvent) Version() int          { return 1 }
func (e UserDeactivatedEvent) OccurredAt() time.Time { return e.DeactivatedAt }

// UserActivatedEvent represents user activation
type UserActivatedEvent struct {
	UserID      string    `json:"user_id"`
	ActivatedAt time.Time `json:"activated_at"`
}

func (e UserActivatedEvent) EventType() string     { return "UserActivated" }
func (e UserActivatedEvent) AggregateID() string   { return e.UserID }
func (e UserActivatedEvent) Version() int          { return 1 }
func (e UserActivatedEvent) OccurredAt() time.Time { return e.ActivatedAt }
