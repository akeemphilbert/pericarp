package examples

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// UserWithEntityEvents demonstrates how to use the EntityEvent with the Entity struct
// to avoid creating many specific event types. This approach is more flexible and
// reduces boilerplate code.
type UserWithEntityEvents struct {
	domain.BasicEntity
	email    string
	name     string
	isActive bool
}

// NewUserWithEntityEvents creates a new user aggregate using EntityEvent.
func NewUserWithEntityEvents(id, email, name string) (*UserWithEntityEvents, error) {
	if id == "" {
		return nil, errors.New("user ID cannot be empty")
	}
	if email == "" {
		return nil, errors.New("email cannot be empty")
	}
	if name == "" {
		return nil, errors.New("name cannot be empty")
	}

	user := &UserWithEntityEvents{
		BasicEntity: *domain.NewEntity(id),
		email:       email,
		name:        name,
		isActive:    true,
	}

	// Create a standard "Created" event
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
func (u *UserWithEntityEvents) Email() string {
	return u.email
}

// Name returns the user's name
func (u *UserWithEntityEvents) Name() string {
	return u.name
}

// IsActive returns whether the user is active
func (u *UserWithEntityEvents) IsActive() bool {
	return u.isActive
}

// ChangeEmail changes the user's email address using a standard field update event.
func (u *UserWithEntityEvents) ChangeEmail(newEmail string) error {
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
	eventData := struct {
		Field    string `json:"field"`
		OldValue string `json:"old_value"`
		NewValue string `json:"new_value"`
	}{
		Field:    "email",
		OldValue: oldEmail,
		NewValue: newEmail,
	}
	event := domain.NewEntityEvent(nil, nil, "user", "email_updated", u.ID(), eventData)

	// Add additional context
	event.SetMetadata("updated_by", "system")
	event.SetMetadata("updated_at", time.Now())

	u.AddEvent(event)
	return nil
}

// ChangeName changes the user's name using a standard field update event.
func (u *UserWithEntityEvents) ChangeName(newName string) error {
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
	eventData := struct {
		Field    string `json:"field"`
		OldValue string `json:"old_value"`
		NewValue string `json:"new_value"`
	}{
		Field:    "name",
		OldValue: oldName,
		NewValue: newName,
	}
	event := domain.NewEntityEvent(nil, nil, "user", "name_updated", u.ID(), eventData)

	// Add additional context
	event.SetMetadata("updated_by", "system")
	event.SetMetadata("updated_at", time.Now())

	u.AddEvent(event)
	return nil
}

// Deactivate deactivates the user account using a status change event.
func (u *UserWithEntityEvents) Deactivate() error {
	if !u.isActive {
		return nil // Already deactivated
	}

	u.isActive = false

	// Generate standard status change event
	eventData := struct {
		OldStatus     string    `json:"old_status"`
		NewStatus     string    `json:"new_status"`
		Reason        string    `json:"reason"`
		DeactivatedAt time.Time `json:"deactivated_at"`
	}{
		OldStatus:     "active",
		NewStatus:     "inactive",
		Reason:        "user_requested",
		DeactivatedAt: time.Now(),
	}
	event := domain.NewEntityEvent(nil, nil, "user", "status_changed", u.ID(), eventData)

	u.AddEvent(event)
	return nil
}

// Activate activates the user account using a status change event.
func (u *UserWithEntityEvents) Activate() error {
	if u.isActive {
		return nil // Already active
	}

	u.isActive = true

	// Generate standard status change event
	eventData := struct {
		OldStatus   string    `json:"old_status"`
		NewStatus   string    `json:"new_status"`
		Reason      string    `json:"reason"`
		ActivatedAt time.Time `json:"activated_at"`
	}{
		OldStatus:   "inactive",
		NewStatus:   "active",
		Reason:      "admin_action",
		ActivatedAt: time.Now(),
	}
	event := domain.NewEntityEvent(nil, nil, "user", "status_changed", u.ID(), eventData)

	u.AddEvent(event)
	return nil
}

// UpdateProfile updates multiple user fields in a single operation.
func (u *UserWithEntityEvents) UpdateProfile(newEmail, newName string) error {
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
	eventData := struct {
		UpdatedAt time.Time `json:"updated_at"`
		OldEmail  string    `json:"old_email,omitempty"`
		NewEmail  string    `json:"new_email,omitempty"`
		OldName   string    `json:"old_name,omitempty"`
		NewName   string    `json:"new_name,omitempty"`
	}{
		UpdatedAt: time.Now(),
	}

	if emailChanged {
		eventData.OldEmail = oldEmail
		eventData.NewEmail = newEmail
	}

	if nameChanged {
		eventData.OldName = oldName
		eventData.NewName = newName
	}

	event := domain.NewEntityEvent(nil, nil, "user", "updated", u.ID(), eventData)
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
func (u *UserWithEntityEvents) Delete(reason string) error {
	// Generate standard delete event
	eventData := struct {
		Reason     string    `json:"reason"`
		DeletedAt  time.Time `json:"deleted_at"`
		SoftDelete bool      `json:"soft_delete"`
	}{
		Reason:     reason,
		DeletedAt:  time.Now(),
		SoftDelete: true,
	}
	event := domain.NewEntityEvent(nil, nil, "user", "deleted", u.ID(), eventData)

	u.AddEvent(event)
	return nil
}

// LoadFromHistory reconstructs the user aggregate from EntityEvents.
func (u *UserWithEntityEvents) LoadFromHistory(events []domain.Event) {
	for _, event := range events {
		u.applyEntityEvent(event)
	}

	// Call the base implementation to update sequence number
	u.BasicEntity.LoadFromHistory(events)
}

// applyEntityEvent applies an EntityEvent to the user aggregate.
func (u *UserWithEntityEvents) applyEntityEvent(event domain.Event) {
	// Try to cast to EntityEvent
	entityEvent, ok := event.(*domain.EntityEvent)
	if !ok {
		// Handle other event types if needed
		return
	}

	// Handle events based on entity type
	if entityEvent.EntityType != "user" {
		return // Not a user event
	}

	// Parse the payload to access event data
	var data map[string]interface{}
	if err := json.Unmarshal(entityEvent.Payload(), &data); err != nil {
		return
	}

	switch entityEvent.Type {
	case "created":
		if email, ok := data["email"].(string); ok {
			u.email = email
		}
		if name, ok := data["name"].(string); ok {
			u.name = name
		}
		if isActive, ok := data["is_active"].(bool); ok {
			u.isActive = isActive
		}

	case "email_updated":
		if newValue, ok := data["new_value"].(string); ok {
			u.email = newValue
		}

	case "name_updated":
		if newValue, ok := data["new_value"].(string); ok {
			u.name = newValue
		}

	case "updated":
		// Handle bulk updates
		if newEmail, ok := data["new_email"].(string); ok && newEmail != "" {
			u.email = newEmail
		}
		if newName, ok := data["new_name"].(string); ok && newName != "" {
			u.name = newName
		}

	case "status_changed":
		if newStatus, ok := data["new_status"].(string); ok {
			u.isActive = newStatus == "active"
		}

	case "deleted":
		// Handle soft delete - could set a deleted flag if needed
		// For this example, we don't change the state

	default:
		// Unknown action type - gracefully ignore for forward compatibility
	}
}

// GetEventSummary returns a summary of all uncommitted events for debugging.
func (u *UserWithEntityEvents) GetEventSummary() []string {
	events := u.UncommittedEvents()
	summary := make([]string, len(events))

	for i, event := range events {
		summary[i] = event.EventType()
	}

	return summary
}
