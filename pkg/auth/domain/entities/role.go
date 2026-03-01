package entities

import (
	"context"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Role represents an organizational role aligned with the W3C ORG ontology.
// Roles are assigned to agents and can be referenced as assignees in ODRL policies.
type Role struct {
	*ddd.BaseEntity
	name        string
	description string
	createdAt   time.Time
}

// With initializes a new Role with the given ID, name, and description.
func (r *Role) With(id, name, description string) (*Role, error) {
	if id == "" {
		return nil, fmt.Errorf("role ID cannot be empty")
	}
	if name == "" {
		return nil, fmt.Errorf("role name cannot be empty")
	}

	r.BaseEntity = ddd.NewBaseEntity(id)
	r.name = name
	r.description = description
	r.createdAt = time.Now()

	event := new(RoleCreated).With(name, description)
	if err := r.BaseEntity.RecordEvent(event, event.EventType()); err != nil {
		return nil, fmt.Errorf("failed to record Role.Created event: %w", err)
	}

	return r, nil
}

// Name returns the role name.
func (r *Role) Name() string {
	return r.name
}

// Description returns the role description.
func (r *Role) Description() string {
	return r.description
}

// CreatedAt returns when the role was created.
func (r *Role) CreatedAt() time.Time {
	return r.createdAt
}

// Restore restores a Role from database values without recording events.
func (r *Role) Restore(id, name, description string, createdAt time.Time) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	if name == "" {
		return fmt.Errorf("role name cannot be empty")
	}
	r.BaseEntity = ddd.NewBaseEntity(id)
	r.name = name
	r.description = description
	r.createdAt = createdAt
	return nil
}

// ApplyEvent applies a domain event to reconstruct the aggregate state.
func (r *Role) ApplyEvent(ctx context.Context, envelope domain.EventEnvelope[any]) error {
	if err := r.BaseEntity.ApplyEvent(ctx, envelope); err != nil {
		return fmt.Errorf("base entity apply event failed: %w", err)
	}

	switch payload := envelope.Payload.(type) {
	case RoleCreated:
		r.name = payload.Name
		r.description = payload.Description
		r.createdAt = payload.Timestamp
	default:
		return fmt.Errorf("unknown event type: %T", envelope.Payload)
	}
	return nil
}
