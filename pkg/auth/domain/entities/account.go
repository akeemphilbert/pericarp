package entities

import (
	"context"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Well-known account types.
const (
	AccountTypePersonal     = "personal"
	AccountTypeTeam         = "team"
	AccountTypeOrganization = "organization"
)

// Account represents a tenant or workspace that agents belong to.
// Agents are members of accounts and hold account-scoped roles.
type Account struct {
	*ddd.BaseEntity
	name        string
	accountType string
	active      bool
	createdAt   time.Time
}

// With initializes a new Account with the given ID, name, and account type.
func (a *Account) With(id, name, accountType string) (*Account, error) {
	if id == "" {
		return nil, fmt.Errorf("account ID cannot be empty")
	}
	if name == "" {
		return nil, fmt.Errorf("account name cannot be empty")
	}
	if !validAccountType(accountType) {
		return nil, fmt.Errorf("invalid account type: %q", accountType)
	}

	a.BaseEntity = ddd.NewBaseEntity(id)
	a.name = name
	a.accountType = accountType
	a.active = true
	a.createdAt = time.Now()

	event := new(AccountCreated).With(name, accountType)
	if err := a.RecordEvent(event, event.EventType()); err != nil {
		return nil, fmt.Errorf("failed to record Account.Created event: %w", err)
	}

	return a, nil
}

// Name returns the account name.
func (a *Account) Name() string {
	return a.name
}

// AccountType returns the account type (e.g. "personal", "team", "organization").
func (a *Account) AccountType() string {
	return a.accountType
}

// Active returns whether the account is currently active.
func (a *Account) Active() bool {
	return a.active
}

// CreatedAt returns when the account was created.
func (a *Account) CreatedAt() time.Time {
	return a.createdAt
}

// Restore restores an Account from database values without recording events.
func (a *Account) Restore(id, name, accountType string, active bool, createdAt time.Time) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	if name == "" {
		return fmt.Errorf("account name cannot be empty")
	}
	a.BaseEntity = ddd.NewBaseEntity(id)
	a.name = name
	a.accountType = accountType
	a.active = active
	a.createdAt = createdAt
	return nil
}

// Activate activates the account.
func (a *Account) Activate() error {
	if a.active {
		return nil
	}
	a.active = true
	event := new(AccountActivated).With()
	return a.RecordEvent(event, event.EventType())
}

// Deactivate deactivates the account.
func (a *Account) Deactivate() error {
	if !a.active {
		return nil
	}
	a.active = false
	event := new(AccountDeactivated).With()
	return a.RecordEvent(event, event.EventType())
}

// AddMember adds an agent as a member of this account with the given role.
// Records an enriched triple: (Account, org:hasMember, Agent) with role metadata.
func (a *Account) AddMember(agentID, roleID string) error {
	if agentID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}
	if roleID == "" {
		return fmt.Errorf("role ID cannot be empty")
	}

	event := new(AccountMemberAdded).With(a.GetID(), agentID, roleID)
	return a.RecordEvent(event, event.EventType())
}

// RemoveMember removes an agent from this account.
// Records a triple: (Account, org:hadMember, Agent)
func (a *Account) RemoveMember(agentID string) error {
	if agentID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}

	event := new(AccountMemberRemoved).With(a.GetID(), agentID)
	return a.RecordEvent(event, event.EventType())
}

// ChangeMemberRole changes the role of an existing member within this account.
// Records an enriched triple: (Account, org:hasMember, Agent) with the new role.
func (a *Account) ChangeMemberRole(agentID, newRoleID string) error {
	if agentID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}
	if newRoleID == "" {
		return fmt.Errorf("role ID cannot be empty")
	}

	event := new(AccountMemberRoleChanged).With(a.GetID(), agentID, newRoleID)
	return a.RecordEvent(event, event.EventType())
}

// ApplyEvent applies a domain event to reconstruct the aggregate state.
func (a *Account) ApplyEvent(ctx context.Context, envelope domain.EventEnvelope[any]) error {
	if err := a.BaseEntity.ApplyEvent(ctx, envelope); err != nil {
		return fmt.Errorf("base entity apply event failed: %w", err)
	}

	switch payload := envelope.Payload.(type) {
	case AccountCreated:
		a.name = payload.Name
		a.accountType = payload.AccountType
		a.active = true
		a.createdAt = payload.Timestamp
	case AccountActivated:
		a.active = true
	case AccountDeactivated:
		a.active = false
	case AccountMemberAdded, AccountMemberRemoved, AccountMemberRoleChanged:
		// Triple events — membership stored in event store, not entity state
	default:
		return fmt.Errorf("unknown event type: %T", envelope.Payload)
	}
	return nil
}

func validAccountType(t string) bool {
	switch t {
	case AccountTypePersonal, AccountTypeTeam, AccountTypeOrganization:
		return true
	default:
		return false
	}
}
