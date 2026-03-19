package entities

import (
	"context"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Invite represents an invitation for a user to join an account with a pre-assigned role.
type Invite struct {
	*ddd.BaseEntity
	accountID      string
	email          string
	roleID         string
	inviterAgentID string
	inviteeAgentID string
	status         string
	expiresAt      time.Time
	acceptedAt     time.Time
	createdAt      time.Time
}

// With initializes a new Invite with the given parameters.
func (i *Invite) With(id, accountID, email, roleID, inviterAgentID, inviteeAgentID string, expiresAt time.Time) (*Invite, error) {
	if id == "" {
		return nil, fmt.Errorf("invite ID cannot be empty")
	}
	if accountID == "" {
		return nil, fmt.Errorf("account ID cannot be empty")
	}
	if email == "" {
		return nil, fmt.Errorf("email cannot be empty")
	}
	if roleID == "" {
		return nil, fmt.Errorf("role ID cannot be empty")
	}
	if inviterAgentID == "" {
		return nil, fmt.Errorf("inviter agent ID cannot be empty")
	}
	if inviteeAgentID == "" {
		return nil, fmt.Errorf("invitee agent ID cannot be empty")
	}

	i.BaseEntity = ddd.NewBaseEntity(id)
	i.accountID = accountID
	i.email = email
	i.roleID = roleID
	i.inviterAgentID = inviterAgentID
	i.inviteeAgentID = inviteeAgentID
	i.status = InviteStatusPending
	i.expiresAt = expiresAt
	i.createdAt = time.Now()

	event := InviteCreated{
		AccountID:      accountID,
		Email:          email,
		RoleID:         roleID,
		InviterAgentID: inviterAgentID,
		InviteeAgentID: inviteeAgentID,
		ExpiresAt:      expiresAt,
		Timestamp:      i.createdAt,
	}
	if err := i.BaseEntity.RecordEvent(event, event.EventType()); err != nil {
		return nil, fmt.Errorf("failed to record Invite.Created event: %w", err)
	}

	return i, nil
}

// Accept marks the invite as accepted.
func (i *Invite) Accept() error {
	if i.status != InviteStatusPending {
		return fmt.Errorf("invite cannot be accepted: current status is %q", i.status)
	}
	if i.IsExpired() {
		return fmt.Errorf("invite has expired")
	}
	i.status = InviteStatusAccepted
	i.acceptedAt = time.Now()
	event := InviteAccepted{Timestamp: i.acceptedAt}
	return i.BaseEntity.RecordEvent(event, event.EventType())
}

// Revoke marks the invite as revoked.
func (i *Invite) Revoke() error {
	if i.status != InviteStatusPending {
		return fmt.Errorf("invite cannot be revoked: current status is %q", i.status)
	}
	i.status = InviteStatusRevoked
	event := InviteRevoked{Timestamp: time.Now()}
	return i.BaseEntity.RecordEvent(event, event.EventType())
}

// IsExpired returns whether the invite has expired.
func (i *Invite) IsExpired() bool {
	return time.Now().After(i.expiresAt)
}

// Restore restores an Invite from database values without recording events.
func (i *Invite) Restore(id, accountID, email, roleID, inviterAgentID, inviteeAgentID, status string, expiresAt, acceptedAt, createdAt time.Time) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	i.BaseEntity = ddd.NewBaseEntity(id)
	i.accountID = accountID
	i.email = email
	i.roleID = roleID
	i.inviterAgentID = inviterAgentID
	i.inviteeAgentID = inviteeAgentID
	i.status = status
	i.expiresAt = expiresAt
	i.acceptedAt = acceptedAt
	i.createdAt = createdAt
	return nil
}

// ApplyEvent applies a domain event to reconstruct the aggregate state.
func (i *Invite) ApplyEvent(ctx context.Context, envelope domain.EventEnvelope[any]) error {
	if err := i.BaseEntity.ApplyEvent(ctx, envelope); err != nil {
		return fmt.Errorf("base entity apply event failed: %w", err)
	}

	switch payload := envelope.Payload.(type) {
	case InviteCreated:
		i.accountID = payload.AccountID
		i.email = payload.Email
		i.roleID = payload.RoleID
		i.inviterAgentID = payload.InviterAgentID
		i.inviteeAgentID = payload.InviteeAgentID
		i.expiresAt = payload.ExpiresAt
		i.status = InviteStatusPending
		i.createdAt = payload.Timestamp
	case InviteAccepted:
		i.status = InviteStatusAccepted
		i.acceptedAt = payload.Timestamp
	case InviteRevoked:
		i.status = InviteStatusRevoked
	default:
		return fmt.Errorf("unknown event type: %T", envelope.Payload)
	}
	return nil
}

// AccountID returns the account this invite is for.
func (i *Invite) AccountID() string { return i.accountID }

// Email returns the email address of the invitee.
func (i *Invite) Email() string { return i.email }

// RoleID returns the pre-assigned role for the invitee.
func (i *Invite) RoleID() string { return i.roleID }

// InviterAgentID returns the ID of the agent who created the invite.
func (i *Invite) InviterAgentID() string { return i.inviterAgentID }

// InviteeAgentID returns the ID of the skeleton agent created for the invitee.
func (i *Invite) InviteeAgentID() string { return i.inviteeAgentID }

// Status returns the current invite status.
func (i *Invite) Status() string { return i.status }

// ExpiresAt returns the invite expiration time.
func (i *Invite) ExpiresAt() time.Time { return i.expiresAt }

// AcceptedAt returns when the invite was accepted.
func (i *Invite) AcceptedAt() time.Time { return i.acceptedAt }

// CreatedAt returns when the invite was created.
func (i *Invite) CreatedAt() time.Time { return i.createdAt }
