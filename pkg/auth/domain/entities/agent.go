package entities

import (
	"context"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Agent represents an authenticated party (person, organization, or software agent)
// aligned with the FOAF ontology. Agents are the subjects of authorization decisions.
type Agent struct {
	*ddd.BaseEntity
	name      string
	agentType string
	status    string
	createdAt time.Time
}

// With initializes a new Agent with the given ID, name, and FOAF agent type.
// If agentType is empty, it defaults to AgentTypePerson.
func (a *Agent) With(id, name, agentType string) (*Agent, error) {
	if id == "" {
		return nil, fmt.Errorf("agent ID cannot be empty")
	}
	if name == "" {
		return nil, fmt.Errorf("agent name cannot be empty")
	}
	if agentType == "" {
		agentType = AgentTypePerson
	}

	a.BaseEntity = ddd.NewBaseEntity(id)
	a.name = name
	a.agentType = agentType
	a.status = AgentStatusActive
	a.createdAt = time.Now()

	event := new(AgentCreated).With(name, agentType)
	if err := a.BaseEntity.RecordEvent(event, event.EventType()); err != nil {
		return nil, fmt.Errorf("failed to record Agent.Created event: %w", err)
	}

	return a, nil
}

// WithInvite creates a skeleton invited agent identified only by email.
// The agent starts with status "invited" and uses the email as a placeholder name.
func (a *Agent) WithInvite(id, email string) (*Agent, error) {
	if id == "" {
		return nil, fmt.Errorf("agent ID cannot be empty")
	}
	if email == "" {
		return nil, fmt.Errorf("email cannot be empty")
	}

	a.BaseEntity = ddd.NewBaseEntity(id)
	a.name = email
	a.agentType = AgentTypePerson
	a.status = AgentStatusInvited
	a.createdAt = time.Now()

	event := new(AgentInvited).With(email)
	if err := a.BaseEntity.RecordEvent(event, event.EventType()); err != nil {
		return nil, fmt.Errorf("failed to record Agent.Invited event: %w", err)
	}

	return a, nil
}

// Name returns the agent's display name.
func (a *Agent) Name() string {
	return a.name
}

// AgentType returns the FOAF agent type IRI.
func (a *Agent) AgentType() string {
	return a.agentType
}

// Active returns whether the agent is currently active.
func (a *Agent) Active() bool {
	return a.status == AgentStatusActive
}

// Status returns the agent's current status.
func (a *Agent) Status() string {
	return a.status
}

// CreatedAt returns when the agent was created.
func (a *Agent) CreatedAt() time.Time {
	return a.createdAt
}

// Restore restores an Agent from database values without recording events.
func (a *Agent) Restore(id, name, agentType, status string, createdAt time.Time) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	if name == "" {
		return fmt.Errorf("agent name cannot be empty")
	}
	a.BaseEntity = ddd.NewBaseEntity(id)
	a.name = name
	a.agentType = agentType
	a.status = status
	a.createdAt = createdAt
	return nil
}

// Deactivate marks the agent as inactive.
func (a *Agent) Deactivate() error {
	if a.status == AgentStatusDeactivated {
		return nil
	}
	a.status = AgentStatusDeactivated
	event := new(AgentDeactivated).With()
	return a.BaseEntity.RecordEvent(event, event.EventType())
}

// Activate marks the agent as active.
func (a *Agent) Activate() error {
	if a.status == AgentStatusActive {
		return nil
	}
	a.status = AgentStatusActive
	event := new(AgentActivated).With()
	return a.BaseEntity.RecordEvent(event, event.EventType())
}

// UpdateName changes the agent's display name.
func (a *Agent) UpdateName(name string) error {
	if name == "" {
		return fmt.Errorf("agent name cannot be empty")
	}
	if a.name == name {
		return nil
	}
	a.name = name
	event := new(AgentNameUpdated).With(name)
	return a.BaseEntity.RecordEvent(event, event.EventType())
}

// AssignRole assigns a role to this agent.
// Records a triple event: (Agent, org:hasRole, Role)
func (a *Agent) AssignRole(roleID string) error {
	if roleID == "" {
		return fmt.Errorf("role ID cannot be empty")
	}
	event := new(AgentRoleAssigned).With(a.GetID(), roleID)
	return a.BaseEntity.RecordEvent(event, event.EventType())
}

// RevokeRole revokes a role from this agent.
// Records a triple event: (Agent, org:hadRole, Role)
func (a *Agent) RevokeRole(roleID string) error {
	if roleID == "" {
		return fmt.Errorf("role ID cannot be empty")
	}
	event := new(AgentRoleRevoked).With(a.GetID(), roleID)
	return a.BaseEntity.RecordEvent(event, event.EventType())
}

// AddToGroup adds this agent to a group.
// Records a triple event: (Agent, foaf:member, Group)
func (a *Agent) AddToGroup(groupID string) error {
	if groupID == "" {
		return fmt.Errorf("group ID cannot be empty")
	}
	event := new(AgentGroupMembershipAdded).With(a.GetID(), groupID)
	return a.BaseEntity.RecordEvent(event, event.EventType())
}

// RemoveFromGroup removes this agent from a group.
func (a *Agent) RemoveFromGroup(groupID string) error {
	if groupID == "" {
		return fmt.Errorf("group ID cannot be empty")
	}
	event := new(AgentGroupMembershipRemoved).With(a.GetID(), groupID)
	return a.BaseEntity.RecordEvent(event, event.EventType())
}

// ApplyEvent applies a domain event to reconstruct the aggregate state.
func (a *Agent) ApplyEvent(ctx context.Context, envelope domain.EventEnvelope[any]) error {
	if err := a.BaseEntity.ApplyEvent(ctx, envelope); err != nil {
		return fmt.Errorf("base entity apply event failed: %w", err)
	}

	switch payload := envelope.Payload.(type) {
	case AgentCreated:
		a.name = payload.Name
		a.agentType = payload.AgentType
		a.status = AgentStatusActive
		a.createdAt = payload.Timestamp
	case AgentInvited:
		a.name = payload.Email
		a.agentType = AgentTypePerson
		a.status = AgentStatusInvited
		a.createdAt = payload.Timestamp
	case AgentNameUpdated:
		a.name = payload.Name
	case AgentDeactivated:
		a.status = AgentStatusDeactivated
	case AgentActivated:
		a.status = AgentStatusActive
	case AgentRoleAssigned, AgentRoleRevoked,
		AgentGroupMembershipAdded, AgentGroupMembershipRemoved:
		// Triple events — relationships stored in event store, not entity state
	default:
		return fmt.Errorf("unknown event type: %T", envelope.Payload)
	}
	return nil
}
