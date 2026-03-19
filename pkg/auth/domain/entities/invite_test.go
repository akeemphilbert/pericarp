package entities_test

import (
	"context"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

func TestInvite_With(t *testing.T) {
	t.Parallel()

	expires := time.Now().Add(7 * 24 * time.Hour)

	tests := []struct {
		name           string
		id             string
		accountID      string
		email          string
		roleID         string
		inviterAgentID string
		inviteeAgentID string
		wantErr        bool
	}{
		{
			name:           "creates invite",
			id:             "invite-1",
			accountID:      "account-1",
			email:          "alice@example.com",
			roleID:         entities.RoleMember,
			inviterAgentID: "agent-1",
			inviteeAgentID: "agent-2",
		},
		{
			name:           "fails with empty ID",
			id:             "",
			accountID:      "account-1",
			email:          "alice@example.com",
			roleID:         entities.RoleMember,
			inviterAgentID: "agent-1",
			inviteeAgentID: "agent-2",
			wantErr:        true,
		},
		{
			name:           "fails with empty account ID",
			id:             "invite-1",
			accountID:      "",
			email:          "alice@example.com",
			roleID:         entities.RoleMember,
			inviterAgentID: "agent-1",
			inviteeAgentID: "agent-2",
			wantErr:        true,
		},
		{
			name:           "fails with empty email",
			id:             "invite-1",
			accountID:      "account-1",
			email:          "",
			roleID:         entities.RoleMember,
			inviterAgentID: "agent-1",
			inviteeAgentID: "agent-2",
			wantErr:        true,
		},
		{
			name:           "fails with empty role ID",
			id:             "invite-1",
			accountID:      "account-1",
			email:          "alice@example.com",
			roleID:         "",
			inviterAgentID: "agent-1",
			inviteeAgentID: "agent-2",
			wantErr:        true,
		},
		{
			name:           "fails with empty inviter agent ID",
			id:             "invite-1",
			accountID:      "account-1",
			email:          "alice@example.com",
			roleID:         entities.RoleMember,
			inviterAgentID: "",
			inviteeAgentID: "agent-2",
			wantErr:        true,
		},
		{
			name:           "fails with empty invitee agent ID",
			id:             "invite-1",
			accountID:      "account-1",
			email:          "alice@example.com",
			roleID:         entities.RoleMember,
			inviterAgentID: "agent-1",
			inviteeAgentID: "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			invite, err := new(entities.Invite).With(tt.id, tt.accountID, tt.email, tt.roleID, tt.inviterAgentID, tt.inviteeAgentID, expires)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if invite.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", invite.GetID(), tt.id)
			}
			if invite.AccountID() != tt.accountID {
				t.Errorf("AccountID() = %q, want %q", invite.AccountID(), tt.accountID)
			}
			if invite.Email() != tt.email {
				t.Errorf("Email() = %q, want %q", invite.Email(), tt.email)
			}
			if invite.RoleID() != tt.roleID {
				t.Errorf("RoleID() = %q, want %q", invite.RoleID(), tt.roleID)
			}
			if invite.InviterAgentID() != tt.inviterAgentID {
				t.Errorf("InviterAgentID() = %q, want %q", invite.InviterAgentID(), tt.inviterAgentID)
			}
			if invite.InviteeAgentID() != tt.inviteeAgentID {
				t.Errorf("InviteeAgentID() = %q, want %q", invite.InviteeAgentID(), tt.inviteeAgentID)
			}
			if invite.Status() != entities.InviteStatusPending {
				t.Errorf("Status() = %q, want %q", invite.Status(), entities.InviteStatusPending)
			}

			events := invite.GetUncommittedEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 uncommitted event, got %d", len(events))
			}
			if events[0].EventType != entities.EventTypeInviteCreated {
				t.Errorf("event type = %q, want %q", events[0].EventType, entities.EventTypeInviteCreated)
			}
		})
	}
}

func TestInvite_Accept(t *testing.T) {
	t.Parallel()

	expires := time.Now().Add(7 * 24 * time.Hour)
	invite, err := new(entities.Invite).With("invite-1", "account-1", "alice@example.com", entities.RoleMember, "agent-1", "agent-2", expires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := invite.Accept(); err != nil {
		t.Fatalf("Accept() error: %v", err)
	}

	if invite.Status() != entities.InviteStatusAccepted {
		t.Errorf("Status() = %q, want %q", invite.Status(), entities.InviteStatusAccepted)
	}

	events := invite.GetUncommittedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 uncommitted events, got %d", len(events))
	}
	if events[1].EventType != entities.EventTypeInviteAccepted {
		t.Errorf("event type = %q, want %q", events[1].EventType, entities.EventTypeInviteAccepted)
	}
}

func TestInvite_Accept_Expired(t *testing.T) {
	t.Parallel()

	expires := time.Now().Add(-1 * time.Hour) // already expired
	invite, err := new(entities.Invite).With("invite-1", "account-1", "alice@example.com", entities.RoleMember, "agent-1", "agent-2", expires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = invite.Accept()
	if err == nil {
		t.Fatal("expected error for expired invite, got nil")
	}
}

func TestInvite_Accept_AlreadyAccepted(t *testing.T) {
	t.Parallel()

	expires := time.Now().Add(7 * 24 * time.Hour)
	invite, err := new(entities.Invite).With("invite-1", "account-1", "alice@example.com", entities.RoleMember, "agent-1", "agent-2", expires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := invite.Accept(); err != nil {
		t.Fatalf("first Accept() error: %v", err)
	}

	err = invite.Accept()
	if err == nil {
		t.Fatal("expected error for already accepted invite, got nil")
	}
}

func TestInvite_Revoke(t *testing.T) {
	t.Parallel()

	expires := time.Now().Add(7 * 24 * time.Hour)
	invite, err := new(entities.Invite).With("invite-1", "account-1", "alice@example.com", entities.RoleMember, "agent-1", "agent-2", expires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := invite.Revoke(); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}

	if invite.Status() != entities.InviteStatusRevoked {
		t.Errorf("Status() = %q, want %q", invite.Status(), entities.InviteStatusRevoked)
	}

	events := invite.GetUncommittedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 uncommitted events, got %d", len(events))
	}
	if events[1].EventType != entities.EventTypeInviteRevoked {
		t.Errorf("event type = %q, want %q", events[1].EventType, entities.EventTypeInviteRevoked)
	}
}

func TestInvite_Revoke_AlreadyAccepted(t *testing.T) {
	t.Parallel()

	expires := time.Now().Add(7 * 24 * time.Hour)
	invite, err := new(entities.Invite).With("invite-1", "account-1", "alice@example.com", entities.RoleMember, "agent-1", "agent-2", expires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := invite.Accept(); err != nil {
		t.Fatalf("Accept() error: %v", err)
	}

	err = invite.Revoke()
	if err == nil {
		t.Fatal("expected error for revoking accepted invite, got nil")
	}
}

func TestInvite_ApplyEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	expires := time.Now().Add(7 * 24 * time.Hour)
	invite, err := new(entities.Invite).With("invite-1", "account-1", "alice@example.com", entities.RoleMember, "agent-1", "agent-2", expires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := invite.GetUncommittedEvents()
	invite.ClearUncommittedEvents()

	// Reconstruct from events
	restored := &entities.Invite{}
	if err := restored.Restore("invite-1", "", "", "", "", "", entities.InviteStatusPending, time.Time{}, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	if err := restored.ApplyEvent(ctx, events[0]); err != nil {
		t.Fatalf("ApplyEvent() error: %v", err)
	}
	if restored.AccountID() != "account-1" {
		t.Errorf("AccountID() = %q, want %q", restored.AccountID(), "account-1")
	}
	if restored.Email() != "alice@example.com" {
		t.Errorf("Email() = %q, want %q", restored.Email(), "alice@example.com")
	}
	if restored.Status() != entities.InviteStatusPending {
		t.Errorf("Status() = %q, want %q", restored.Status(), entities.InviteStatusPending)
	}
}

func TestAgent_WithInvite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		email   string
		wantErr bool
	}{
		{
			name:  "creates invited agent",
			id:    "agent-1",
			email: "alice@example.com",
		},
		{
			name:    "fails with empty ID",
			id:      "",
			email:   "alice@example.com",
			wantErr: true,
		},
		{
			name:    "fails with empty email",
			id:      "agent-1",
			email:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			agent, err := new(entities.Agent).WithInvite(tt.id, tt.email)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if agent.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", agent.GetID(), tt.id)
			}
			if agent.Name() != tt.email {
				t.Errorf("Name() = %q, want %q", agent.Name(), tt.email)
			}
			if agent.Status() != entities.AgentStatusInvited {
				t.Errorf("Status() = %q, want %q", agent.Status(), entities.AgentStatusInvited)
			}
			if agent.Active() {
				t.Error("expected invited agent to not be active")
			}

			events := agent.GetUncommittedEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 uncommitted event, got %d", len(events))
			}
			if events[0].EventType != entities.EventTypeAgentInvited {
				t.Errorf("event type = %q, want %q", events[0].EventType, entities.EventTypeAgentInvited)
			}
		})
	}
}

func TestAgent_StatusBasedActive(t *testing.T) {
	t.Parallel()

	agent, err := new(entities.Agent).With("agent-1", "Alice", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agent.Status() != entities.AgentStatusActive {
		t.Errorf("Status() = %q, want %q", agent.Status(), entities.AgentStatusActive)
	}
	if !agent.Active() {
		t.Error("expected active agent")
	}

	if err := agent.Deactivate(); err != nil {
		t.Fatalf("Deactivate() error: %v", err)
	}
	if agent.Status() != entities.AgentStatusDeactivated {
		t.Errorf("Status() = %q, want %q", agent.Status(), entities.AgentStatusDeactivated)
	}
	if agent.Active() {
		t.Error("expected inactive agent")
	}

	if err := agent.Activate(); err != nil {
		t.Fatalf("Activate() error: %v", err)
	}
	if agent.Status() != entities.AgentStatusActive {
		t.Errorf("Status() = %q, want %q", agent.Status(), entities.AgentStatusActive)
	}
	if !agent.Active() {
		t.Error("expected active agent after re-activation")
	}
}
