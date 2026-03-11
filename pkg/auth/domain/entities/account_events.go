package entities

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// AccountCreated represents the creation of an account.
type AccountCreated struct {
	Name        string    `json:"name"`
	AccountType string    `json:"account_type"`
	Timestamp   time.Time `json:"timestamp"`
}

// With creates a new AccountCreated event.
func (e AccountCreated) With(name, accountType string) AccountCreated {
	return AccountCreated{
		Name:        name,
		AccountType: accountType,
		Timestamp:   time.Now(),
	}
}

// EventType returns the event type name.
func (e AccountCreated) EventType() string {
	return EventTypeAccountCreated
}

// AccountActivated represents the activation of an account.
type AccountActivated struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AccountActivated event.
func (e AccountActivated) With() AccountActivated {
	return AccountActivated{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e AccountActivated) EventType() string {
	return EventTypeAccountActivated
}

// AccountDeactivated represents the deactivation of an account.
type AccountDeactivated struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AccountDeactivated event.
func (e AccountDeactivated) With() AccountDeactivated {
	return AccountDeactivated{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e AccountDeactivated) EventType() string {
	return EventTypeAccountDeactivated
}

// AccountMemberAdded represents adding an agent as a member of an account with a role.
// Enriched triple: (Account, org:hasMember, Agent) with role metadata.
type AccountMemberAdded struct {
	domain.BasicTripleEvent
	Role      string    `json:"role"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AccountMemberAdded event.
func (e AccountMemberAdded) With(accountID, agentID, roleID string) AccountMemberAdded {
	return AccountMemberAdded{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   accountID,
			Predicate: PredicateHasMember,
			Object:    agentID,
		},
		Role:      roleID,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e AccountMemberAdded) EventType() string {
	return EventTypeAccountMemberAdded
}

// AccountMemberRemoved represents removing an agent from an account.
// Triple: (Account, org:hadMember, Agent)
type AccountMemberRemoved struct {
	domain.BasicTripleEvent
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AccountMemberRemoved event.
func (e AccountMemberRemoved) With(accountID, agentID string) AccountMemberRemoved {
	return AccountMemberRemoved{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   accountID,
			Predicate: PredicateHadMember,
			Object:    agentID,
		},
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e AccountMemberRemoved) EventType() string {
	return EventTypeAccountMemberRemoved
}

// AccountMemberRoleChanged represents changing a member's role within an account.
// Enriched triple: (Account, org:hasMember, Agent) with the new role.
type AccountMemberRoleChanged struct {
	domain.BasicTripleEvent
	Role      string    `json:"role"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new AccountMemberRoleChanged event.
func (e AccountMemberRoleChanged) With(accountID, agentID, newRoleID string) AccountMemberRoleChanged {
	return AccountMemberRoleChanged{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   accountID,
			Predicate: PredicateHasMember,
			Object:    agentID,
		},
		Role:      newRoleID,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e AccountMemberRoleChanged) EventType() string {
	return EventTypeAccountMemberRoleChanged
}
