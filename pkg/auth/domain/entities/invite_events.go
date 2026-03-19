package entities

import "time"

// InviteCreated represents the creation of an invite.
type InviteCreated struct {
	AccountID      string    `json:"account_id"`
	Email          string    `json:"email"`
	RoleID         string    `json:"role_id"`
	InviterAgentID string    `json:"inviter_agent_id"`
	InviteeAgentID string    `json:"invitee_agent_id"`
	ExpiresAt      time.Time `json:"expires_at"`
	Timestamp      time.Time `json:"timestamp"`
}

// EventType returns the event type name.
func (e InviteCreated) EventType() string {
	return EventTypeInviteCreated
}

// InviteAccepted represents the acceptance of an invite.
type InviteAccepted struct {
	Timestamp time.Time `json:"timestamp"`
}

// EventType returns the event type name.
func (e InviteAccepted) EventType() string {
	return EventTypeInviteAccepted
}

// InviteRevoked represents the revocation of an invite.
type InviteRevoked struct {
	Timestamp time.Time `json:"timestamp"`
}

// EventType returns the event type name.
func (e InviteRevoked) EventType() string {
	return EventTypeInviteRevoked
}
