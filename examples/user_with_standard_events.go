package examples

import (
	"errors"
	"time"

	"github.com/example/pericarp/pkg/domain"
)

// UserWithStandardEvents demonstrates how to use the StandardEvent with the Entity struct
// to avoid creating many specific event types. This approach is more flexible and
// reduces boilerplate code.
type UserWithStandardEvents struct {
	domain.Entity
	email    string
	name     string
	isActive bool
}

// NewUserWithStandardEvents creates a new user aggregate using StandardEvent.
func NewUserWithStandardEvents(id, email, name string) (*UserWithStandardEvents, error) {
	if id == "" {
		return nil, errors.New("user ID cannot be empty")
	}
	if email == "" {
		return nil, errors.New("email cannot be empty")
	}
	if name == "" {
		return nil, errors.New("name cannot be empty")
	}

	user := &UserWithStandardEvents{
		Entity:   domain.NewEntity(id),
		email:    email,
		name:     name,
		isActive: true,
	}

	// Create a standard "Created" event
	event := domain.NewEvent(id, "User", "Created", map[string]interface{}{
		"email":      email,
		"name":       name,
		"is_active":  true,
		"created_at": time.Now(),
	})

	user.AddEvent(event)
	return user, nil
}

// Email returns the user's email address
func (u *UserWithStandardEvents) Email() string {
	return u.email
}

// Name returns the user's name
func (u *UserWithStandardEvents) Name() string {
	return u.name
}

// IsActive returns whether the user is active
func (u *UserWithStandardEvents) IsActive() bool {
	return u.isActive
}

// ChangeEmail changes the user's email address using a standard field update event.
func (u *UserWithStandardEvents) ChangeEmail(newEmail string) error {
	if newEmail == "" {
		return errors.New("email cannot be empty")
	}

	if newEmail == u.email {
		return nil // No change needed
	}

	// Apply the business logic
	oldEmail := u.email
	u.email = newEmail

	// Generate standard field update event
	event := domain.NewEvent(u.ID(), "User", "EmailUpdated", map[string]interface{}{
		"field":     "email",
		"old_value": oldEmail,
		"new_value": newEmail,
	})

	// Add additional context
	event.SetMetadata("updated_by", "system")
	event.SetMetadata("updated_at", time.Now())

	u.AddEvent(event)
	return nil
}

// ChangeName changes the user's name using a standard field update event.
func (u *UserWithStandardEvents) ChangeName(newName string) error {
	if newName == "" {
		return errors.New("name cannot be empty")
	}

	if newName == u.name {
		return nil // No change needed
	}

	// Apply the business logic
	oldName := u.name
	u.name = newName

	// Generate standard field update event
	event := domain.NewEvent(u.ID(), "User", "NameUpdated", map[string]interface{}{
		"field":     "name",
		"old_value": oldName,
		"new_value": newName,
	})

	// Add additional context
	event.SetMetadata("updated_by", "system")
	event.SetMetadata("updated_at", time.Now())

	u.AddEvent(event)
	return nil
}

// Deactivate deactivates the user account using a status change event.
func (u *UserWithStandardEvents) Deactivate() error {
	if !u.isActive {
		return nil // Already deactivated
	}

	u.isActive = false

	// Generate standard status change event
	event := domain.NewEvent(u.ID(), "User", "StatusChanged", map[string]interface{}{
		"old_status":     "active",
		"new_status":     "inactive",
		"reason":         "user_requested",
		"deactivated_at": time.Now(),
	})

	u.AddEvent(event)
	return nil
}

// Activate activates the user account using a status change event.
func (u *UserWithStandardEvents) Activate() error {
	if u.isActive {
		return nil // Already active
	}

	u.isActive = true

	// Generate standard status change event
	event := domain.NewEvent(u.ID(), "User", "StatusChanged", map[string]interface{}{
		"old_status":   "inactive",
		"new_status":   "active",
		"reason":       "admin_action",
		"activated_at": time.Now(),
	})

	u.AddEvent(event)
	return nil
}

// UpdateProfile updates multiple user fields in a single operation.
func (u *UserWithStandardEvents) UpdateProfile(newEmail, newName string) error {
	if newEmail == "" {
		return errors.New("email cannot be empty")
	}
	if newName == "" {
		return errors.New("name cannot be empty")
	}

	// Check if any changes are needed
	emailChanged := newEmail != u.email
	nameChanged := newName != u.name

	if !emailChanged && !nameChanged {
		return nil // No changes needed
	}

	// Apply changes
	oldEmail := u.email
	oldName := u.name
	u.email = newEmail
	u.name = newName

	// Generate a single "Updated" event with all changes
	data := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if emailChanged {
		data["old_email"] = oldEmail
		data["new_email"] = newEmail
	}

	if nameChanged {
		data["old_name"] = oldName
		data["new_name"] = newName
	}

	event := domain.NewEvent(u.ID(), "User", "Updated", data)
	// Build list of updated fields
	var updatedFields []string
	if emailChanged {
		updatedFields = append(updatedFields, "email")
	}
	if nameChanged {
		updatedFields = append(updatedFields, "name")
	}
	event.SetMetadata("fields_updated", updatedFields)

	u.AddEvent(event)
	return nil
}

// Delete marks the user as deleted (soft delete).
func (u *UserWithStandardEvents) Delete(reason string) error {
	// Generate standard delete event
	event := domain.NewEvent(u.ID(), "User", "Deleted", map[string]interface{}{
		"reason":      reason,
		"deleted_at":  time.Now(),
		"soft_delete": true,
	})

	u.AddEvent(event)
	return nil
}

// LoadFromHistory reconstructs the user aggregate from StandardEvents.
func (u *UserWithStandardEvents) LoadFromHistory(events []domain.Event) {
	for _, event := range events {
		u.applyStandardEvent(event)
	}

	// Call the base implementation to update version and sequence
	u.Entity.LoadFromHistory(events)
}

// applyStandardEvent applies a StandardEvent to the user aggregate.
func (u *UserWithStandardEvents) applyStandardEvent(event domain.Event) {
	// Try to cast to StandardEvent
	standardEvent, ok := event.(*domain.StandardEvent)
	if !ok {
		// Handle other event types if needed
		return
	}

	// Handle events based on entity type and action type
	if standardEvent.EntityType() != "User" {
		return // Not a user event
	}

	switch standardEvent.ActionType() {
	case "Created":
		u.email = standardEvent.GetDataString("email")
		u.name = standardEvent.GetDataString("name")
		u.isActive = standardEvent.GetDataBool("is_active")

	case "EmailUpdated":
		u.email = standardEvent.GetDataString("new_value")

	case "NameUpdated":
		u.name = standardEvent.GetDataString("new_value")

	case "Updated":
		// Handle bulk updates
		if newEmail := standardEvent.GetDataString("new_email"); newEmail != "" {
			u.email = newEmail
		}
		if newName := standardEvent.GetDataString("new_name"); newName != "" {
			u.name = newName
		}

	case "StatusChanged":
		newStatus := standardEvent.GetDataString("new_status")
		u.isActive = newStatus == "active"

	case "Deleted":
		// Handle soft delete - could set a deleted flag if needed
		// For this example, we don't change the state

	default:
		// Unknown action type - gracefully ignore for forward compatibility
	}
}

// GetEventSummary returns a summary of all uncommitted events for debugging.
func (u *UserWithStandardEvents) GetEventSummary() []string {
	events := u.UncommittedEvents()
	summary := make([]string, len(events))

	for i, event := range events {
		if standardEvent, ok := event.(*domain.StandardEvent); ok {
			summary[i] = standardEvent.EventType()
		} else {
			summary[i] = event.EventType()
		}
	}

	return summary
}
