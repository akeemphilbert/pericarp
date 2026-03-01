package entities

import (
	"context"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Policy represents an ODRL policy that groups permissions, prohibitions, and duties.
// Policies are the containers for authorization rules.
type Policy struct {
	*ddd.BaseEntity
	name       string
	policyType string
	active     bool
	createdAt  time.Time
}

// With initializes a new Policy with the given ID, name, and ODRL policy type.
// If policyType is empty, it defaults to PolicyTypeSet.
func (p *Policy) With(id, name, policyType string) (*Policy, error) {
	if id == "" {
		return nil, fmt.Errorf("policy ID cannot be empty")
	}
	if name == "" {
		return nil, fmt.Errorf("policy name cannot be empty")
	}
	if policyType == "" {
		policyType = PolicyTypeSet
	}

	p.BaseEntity = ddd.NewBaseEntity(id)
	p.name = name
	p.policyType = policyType
	p.active = true
	p.createdAt = time.Now()

	event := new(PolicyCreated).With(name, policyType)
	if err := p.BaseEntity.RecordEvent(event, event.EventType()); err != nil {
		return nil, fmt.Errorf("failed to record Policy.Created event: %w", err)
	}

	return p, nil
}

// Name returns the policy name.
func (p *Policy) Name() string {
	return p.name
}

// PolicyType returns the ODRL policy type.
func (p *Policy) PolicyType() string {
	return p.policyType
}

// Active returns whether the policy is currently active.
func (p *Policy) Active() bool {
	return p.active
}

// CreatedAt returns when the policy was created.
func (p *Policy) CreatedAt() time.Time {
	return p.createdAt
}

// Restore restores a Policy from database values without recording events.
func (p *Policy) Restore(id, name, policyType string, active bool, createdAt time.Time) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	if name == "" {
		return fmt.Errorf("policy name cannot be empty")
	}
	p.BaseEntity = ddd.NewBaseEntity(id)
	p.name = name
	p.policyType = policyType
	p.active = active
	p.createdAt = createdAt
	return nil
}

// Activate activates the policy.
func (p *Policy) Activate() error {
	if p.active {
		return nil
	}
	p.active = true
	event := new(PolicyActivated).With()
	return p.BaseEntity.RecordEvent(event, event.EventType())
}

// Deactivate deactivates the policy.
func (p *Policy) Deactivate() error {
	if !p.active {
		return nil
	}
	p.active = false
	event := new(PolicyDeactivated).With()
	return p.BaseEntity.RecordEvent(event, event.EventType())
}

// GrantPermission adds a permission rule to this policy.
// Records an enriched triple event: (Assignee, odrl:permission, Target) with action.
func (p *Policy) GrantPermission(assignee, action, target string) error {
	if assignee == "" {
		return fmt.Errorf("assignee cannot be empty")
	}
	if action == "" {
		return fmt.Errorf("action cannot be empty")
	}
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}

	event := new(PermissionGranted).With(assignee, target, action)
	return p.BaseEntity.RecordEvent(event, event.EventType())
}

// RevokePermission revokes a previously granted permission.
func (p *Policy) RevokePermission(assignee, action, target string) error {
	if assignee == "" {
		return fmt.Errorf("assignee cannot be empty")
	}
	if action == "" {
		return fmt.Errorf("action cannot be empty")
	}
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}

	event := new(PermissionRevoked).With(assignee, target, action)
	return p.BaseEntity.RecordEvent(event, event.EventType())
}

// SetProhibition adds a prohibition rule to this policy.
// Records an enriched triple event: (Assignee, odrl:prohibition, Target) with action.
func (p *Policy) SetProhibition(assignee, action, target string) error {
	if assignee == "" {
		return fmt.Errorf("assignee cannot be empty")
	}
	if action == "" {
		return fmt.Errorf("action cannot be empty")
	}
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}

	event := new(ProhibitionSet).With(assignee, target, action)
	return p.BaseEntity.RecordEvent(event, event.EventType())
}

// RevokeProhibition revokes a previously set prohibition.
func (p *Policy) RevokeProhibition(assignee, action, target string) error {
	if assignee == "" {
		return fmt.Errorf("assignee cannot be empty")
	}
	if action == "" {
		return fmt.Errorf("action cannot be empty")
	}
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}

	event := new(ProhibitionRevoked).With(assignee, target, action)
	return p.BaseEntity.RecordEvent(event, event.EventType())
}

// ImposeDuty adds a duty (obligation) to this policy.
// Records an enriched triple event: (Assignee, odrl:duty, Target) with action.
func (p *Policy) ImposeDuty(assignee, action, target string) error {
	if assignee == "" {
		return fmt.Errorf("assignee cannot be empty")
	}
	if action == "" {
		return fmt.Errorf("action cannot be empty")
	}
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}

	event := new(DutyImposed).With(assignee, target, action)
	return p.BaseEntity.RecordEvent(event, event.EventType())
}

// DischargeDuty marks a duty as fulfilled.
func (p *Policy) DischargeDuty(assignee, action, target string) error {
	if assignee == "" {
		return fmt.Errorf("assignee cannot be empty")
	}
	if action == "" {
		return fmt.Errorf("action cannot be empty")
	}
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}

	event := new(DutyDischarged).With(assignee, target, action)
	return p.BaseEntity.RecordEvent(event, event.EventType())
}

// Assign assigns this policy to an agent or role.
// Records a triple event: (Policy, odrl:assignee, Agent/Role)
func (p *Policy) Assign(assigneeID string) error {
	if assigneeID == "" {
		return fmt.Errorf("assignee ID cannot be empty")
	}
	event := new(PolicyAssigned).With(p.GetID(), assigneeID)
	return p.BaseEntity.RecordEvent(event, event.EventType())
}

// ApplyEvent applies a domain event to reconstruct the aggregate state.
func (p *Policy) ApplyEvent(ctx context.Context, envelope domain.EventEnvelope[any]) error {
	if err := p.BaseEntity.ApplyEvent(ctx, envelope); err != nil {
		return fmt.Errorf("base entity apply event failed: %w", err)
	}

	switch payload := envelope.Payload.(type) {
	case PolicyCreated:
		p.name = payload.Name
		p.policyType = payload.PolicyType
		p.active = true
		p.createdAt = payload.Timestamp
	case PolicyActivated:
		p.active = true
	case PolicyDeactivated:
		p.active = false
	case PermissionGranted, PermissionRevoked,
		ProhibitionSet, ProhibitionRevoked,
		DutyImposed, DutyDischarged,
		PolicyAssigned:
		// Triple events — rules and assignments stored in event store, not entity state
	default:
		return fmt.Errorf("unknown event type: %T", envelope.Payload)
	}
	return nil
}
