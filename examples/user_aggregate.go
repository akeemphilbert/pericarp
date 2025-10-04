package examples

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// User demonstrates how to use the Entity struct as a base for concrete aggregates.
// It embeds the Entity struct to inherit event sourcing capabilities.
type User struct {
	domain.BasicEntity
	email    string
	name     string
	isActive bool
}

// NewUser creates a new user aggregate with the given details.
// It demonstrates the typical pattern of creating an aggregate and generating
// the initial domain event.
func NewUser(id, email, name string) (*User, error) {
	if id == "" {
		return nil, errors.New("user GetID cannot be empty")
	}
	if email == "" {
		return nil, errors.New("email cannot be empty")
	}
	if name == "" {
		return nil, errors.New("name cannot be empty")
	}

	user := &User{
		BasicEntity: *domain.NewEntity(id),
		email:       email,
		name:        name,
		isActive:    true,
	}

	// Generate the initial domain event
	eventData := struct {
		Email     string    `json:"email"`
		Name      string    `json:"name"`
		IsActive  bool      `json:"is_active"`
		CreatedAt time.Time `json:"created_at"`
	}{
		Email:     email,
		Name:      name,
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	event := domain.NewEntityEvent(nil, nil, "user", "created", id, eventData)

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
	eventData := struct {
		OldEmail string `json:"old_email"`
		NewEmail string `json:"new_email"`
	}{
		OldEmail: oldEmail,
		NewEmail: newEmail,
	}
	event := domain.NewEntityEvent(nil, nil, "user", "email_changed", u.GetID(), eventData)

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
	eventData := struct {
		OldName string `json:"old_name"`
		NewName string `json:"new_name"`
	}{
		OldName: oldName,
		NewName: newName,
	}
	event := domain.NewEntityEvent(nil, nil, "user", "name_changed", u.GetID(), eventData)

	u.AddEvent(event)
	return nil
}

// Deactivate deactivates the user account.
func (u *User) Deactivate() error {
	if !u.isActive {
		return nil // Already deactivated
	}

	u.isActive = false

	eventData := struct {
		DeactivatedAt time.Time `json:"deactivated_at"`
		Reason        string    `json:"reason"`
	}{
		DeactivatedAt: time.Now(),
		Reason:        "user_requested",
	}
	event := domain.NewEntityEvent(nil, nil, "user", "deactivated", u.GetID(), eventData)

	u.AddEvent(event)
	return nil
}

// Activate activates the user account.
func (u *User) Activate() error {
	if u.isActive {
		return nil // Already active
	}

	u.isActive = true

	eventData := struct {
		ActivatedAt time.Time `json:"activated_at"`
		Reason      string    `json:"reason"`
	}{
		ActivatedAt: time.Now(),
		Reason:      "admin_action",
	}
	event := domain.NewEntityEvent(nil, nil, "user", "activated", u.GetID(), eventData)

	u.AddEvent(event)
	return nil
}

// LoadFromHistory reconstructs the user aggregate from historical events.
// This overrides the base Entity.LoadFromHistory to apply domain-specific logic.
func (u *User) LoadFromHistory(events []domain.Event) {
	for _, event := range events {
		u.applyEvent(event)
	}

	// Call the base implementation to update sequence number
	u.BasicEntity.LoadFromHistory(events)
}

// applyEvent applies a single event to the user aggregate.
// This is used during aggregate reconstruction from event history.
func (u *User) applyEvent(event domain.Event) {
	// Cast to EntityEvent to access data
	entityEvent, ok := event.(*domain.EntityEvent)
	if !ok {
		// Unknown event type - this is normal for forward compatibility
		// The aggregate should gracefully handle unknown events
		return
	}

	switch entityEvent.Type {
	case "created":
		// For created events, the payload contains the full user data
		var userData User
		if err := json.Unmarshal(entityEvent.Payload(), &userData); err == nil {
			u.email = userData.email
			u.name = userData.name
			u.isActive = userData.isActive
		}

	case "email_changed":
		// For email changed events, the payload contains the change data
		var changeData struct {
			OldEmail string `json:"old_email"`
			NewEmail string `json:"new_email"`
		}
		if err := json.Unmarshal(entityEvent.Payload(), &changeData); err == nil {
			u.email = changeData.NewEmail
		}

	case "name_changed":
		// For name changed events, the payload contains the change data
		var changeData struct {
			OldName string `json:"old_name"`
			NewName string `json:"new_name"`
		}
		if err := json.Unmarshal(entityEvent.Payload(), &changeData); err == nil {
			u.name = changeData.NewName
		}

	case "deactivated":
		u.isActive = false

	case "activated":
		u.isActive = true

	default:
		// Unknown event type - this is normal for forward compatibility
		// The aggregate should gracefully handle unknown events
	}
}

// Domain Events - Using EntityEvent for all events
