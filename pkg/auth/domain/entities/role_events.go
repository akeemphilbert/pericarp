package entities

import "time"

// RoleCreated represents the creation of a role.
type RoleCreated struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}

// With creates a new RoleCreated event.
func (e RoleCreated) With(name, description string) RoleCreated {
	return RoleCreated{
		Name:        name,
		Description: description,
		Timestamp:   time.Now(),
	}
}

// EventType returns the event type name.
func (e RoleCreated) EventType() string {
	return EventTypeRoleCreated
}
