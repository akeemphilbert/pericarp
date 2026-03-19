package entities

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// AgentCreated represents the creation of an agent.
type AgentCreated struct {
	Name      string    `json:"name"`
	AgentType string    `json:"agent_type"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AgentCreated event.
func (e AgentCreated) With(name, agentType string) AgentCreated {
	return AgentCreated{
		Name:      name,
		AgentType: agentType,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e AgentCreated) EventType() string {
	return EventTypeAgentCreated
}

// AgentInvited represents the creation of a skeleton invited agent.
type AgentInvited struct {
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AgentInvited event.
func (e AgentInvited) With(email string) AgentInvited {
	return AgentInvited{
		Email:     email,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e AgentInvited) EventType() string {
	return EventTypeAgentInvited
}

// AgentNameUpdated represents an agent's display name being changed.
type AgentNameUpdated struct {
	Name      string    `json:"name"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AgentNameUpdated event.
func (e AgentNameUpdated) With(name string) AgentNameUpdated {
	return AgentNameUpdated{
		Name:      name,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e AgentNameUpdated) EventType() string {
	return EventTypeAgentNameUpdated
}

// AgentDeactivated represents the deactivation of an agent.
type AgentDeactivated struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AgentDeactivated event.
func (e AgentDeactivated) With() AgentDeactivated {
	return AgentDeactivated{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e AgentDeactivated) EventType() string {
	return EventTypeAgentDeactivated
}

// AgentActivated represents the reactivation of an agent.
type AgentActivated struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AgentActivated event.
func (e AgentActivated) With() AgentActivated {
	return AgentActivated{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e AgentActivated) EventType() string {
	return EventTypeAgentActivated
}

// AgentRoleAssigned represents assigning a role to an agent.
// Triple: (Agent, org:hasRole, Role)
type AgentRoleAssigned struct {
	domain.BasicTripleEvent
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AgentRoleAssigned event.
func (e AgentRoleAssigned) With(agentID, roleID string) AgentRoleAssigned {
	return AgentRoleAssigned{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   agentID,
			Predicate: PredicateHasRole,
			Object:    roleID,
		},
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e AgentRoleAssigned) EventType() string {
	return EventTypeAgentRoleAssigned
}

// AgentRoleRevoked represents revoking a role from an agent.
// Triple: (Agent, org:hadRole, Role) — uses past tense predicate for revocation tracking.
type AgentRoleRevoked struct {
	domain.BasicTripleEvent
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AgentRoleRevoked event.
func (e AgentRoleRevoked) With(agentID, roleID string) AgentRoleRevoked {
	return AgentRoleRevoked{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   agentID,
			Predicate: PredicateHadRole,
			Object:    roleID,
		},
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e AgentRoleRevoked) EventType() string {
	return EventTypeAgentRoleRevoked
}

// AgentGroupMembershipAdded represents adding an agent to a group.
// Triple: (Agent, foaf:member, Group)
type AgentGroupMembershipAdded struct {
	domain.BasicTripleEvent
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AgentGroupMembershipAdded event.
func (e AgentGroupMembershipAdded) With(agentID, groupID string) AgentGroupMembershipAdded {
	return AgentGroupMembershipAdded{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   agentID,
			Predicate: PredicateMember,
			Object:    groupID,
		},
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e AgentGroupMembershipAdded) EventType() string {
	return EventTypeAgentGroupMembershipAdded
}

// AgentGroupMembershipRemoved represents removing an agent from a group.
// Triple: (Agent, foaf:member, Group) — event type distinguishes removal from addition.
type AgentGroupMembershipRemoved struct {
	domain.BasicTripleEvent
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AgentGroupMembershipRemoved event.
func (e AgentGroupMembershipRemoved) With(agentID, groupID string) AgentGroupMembershipRemoved {
	return AgentGroupMembershipRemoved{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   agentID,
			Predicate: PredicateMember,
			Object:    groupID,
		},
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e AgentGroupMembershipRemoved) EventType() string {
	return EventTypeAgentGroupMembershipRemoved
}
