package entities_test

import (
	"context"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

func TestAccount_With(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		id          string
		accountName string
		wantErr     bool
	}{
		{
			name:        "creates account",
			id:          "account-1",
			accountName: "Acme Corp",
		},
		{
			name:        "fails with empty ID",
			id:          "",
			accountName: "Acme Corp",
			wantErr:     true,
		},
		{
			name:        "fails with empty name",
			id:          "account-1",
			accountName: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			account, err := new(entities.Account).With(tt.id, tt.accountName)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if account.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", account.GetID(), tt.id)
			}
			if account.Name() != tt.accountName {
				t.Errorf("Name() = %q, want %q", account.Name(), tt.accountName)
			}
			if !account.Active() {
				t.Error("expected account to be active")
			}
			if account.CreatedAt().IsZero() {
				t.Error("expected non-zero CreatedAt")
			}

			events := account.GetUncommittedEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 uncommitted event, got %d", len(events))
			}
			if events[0].EventType != entities.EventTypeAccountCreated {
				t.Errorf("event type = %q, want %q", events[0].EventType, entities.EventTypeAccountCreated)
			}
		})
	}
}

func TestAccount_ActivateDeactivate(t *testing.T) {
	t.Parallel()

	account, err := new(entities.Account).With("account-1", "Acme Corp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Deactivate
	if err := account.Deactivate(); err != nil {
		t.Fatalf("Deactivate() error: %v", err)
	}
	if account.Active() {
		t.Error("expected account to be inactive")
	}

	// Deactivate again (no-op)
	eventsBefore := len(account.GetUncommittedEvents())
	if err := account.Deactivate(); err != nil {
		t.Fatalf("Deactivate() error: %v", err)
	}
	if len(account.GetUncommittedEvents()) != eventsBefore {
		t.Error("expected no new event from duplicate Deactivate")
	}

	// Activate
	if err := account.Activate(); err != nil {
		t.Fatalf("Activate() error: %v", err)
	}
	if !account.Active() {
		t.Error("expected account to be active")
	}
}

func TestAccount_AddMember(t *testing.T) {
	t.Parallel()

	account, err := new(entities.Account).With("account-1", "Acme Corp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := account.AddMember("agent-1", "role-admin"); err != nil {
		t.Fatalf("AddMember() error: %v", err)
	}

	events := account.GetUncommittedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	memberEvent := events[1]
	if memberEvent.EventType != entities.EventTypeAccountMemberAdded {
		t.Errorf("event type = %q, want %q", memberEvent.EventType, entities.EventTypeAccountMemberAdded)
	}

	payload, ok := memberEvent.Payload.(entities.AccountMemberAdded)
	if !ok {
		t.Fatalf("expected AccountMemberAdded payload, got %T", memberEvent.Payload)
	}
	if payload.Subject != "account-1" {
		t.Errorf("Subject = %q, want %q", payload.Subject, "account-1")
	}
	if payload.Predicate != entities.PredicateHasMember {
		t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateHasMember)
	}
	if payload.Object != "agent-1" {
		t.Errorf("Object = %q, want %q", payload.Object, "agent-1")
	}
	if payload.Role != "role-admin" {
		t.Errorf("Role = %q, want %q", payload.Role, "role-admin")
	}
}

func TestAccount_RemoveMember(t *testing.T) {
	t.Parallel()

	account, err := new(entities.Account).With("account-1", "Acme Corp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := account.RemoveMember("agent-1"); err != nil {
		t.Fatalf("RemoveMember() error: %v", err)
	}

	events := account.GetUncommittedEvents()
	lastEvent := events[len(events)-1]
	if lastEvent.EventType != entities.EventTypeAccountMemberRemoved {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypeAccountMemberRemoved)
	}

	payload, ok := lastEvent.Payload.(entities.AccountMemberRemoved)
	if !ok {
		t.Fatalf("expected AccountMemberRemoved payload, got %T", lastEvent.Payload)
	}
	if payload.Predicate != entities.PredicateHadMember {
		t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateHadMember)
	}
}

func TestAccount_ChangeMemberRole(t *testing.T) {
	t.Parallel()

	account, err := new(entities.Account).With("account-1", "Acme Corp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := account.ChangeMemberRole("agent-1", "role-editor"); err != nil {
		t.Fatalf("ChangeMemberRole() error: %v", err)
	}

	events := account.GetUncommittedEvents()
	lastEvent := events[len(events)-1]
	if lastEvent.EventType != entities.EventTypeAccountMemberRoleChanged {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypeAccountMemberRoleChanged)
	}

	payload, ok := lastEvent.Payload.(entities.AccountMemberRoleChanged)
	if !ok {
		t.Fatalf("expected AccountMemberRoleChanged payload, got %T", lastEvent.Payload)
	}
	if payload.Subject != "account-1" {
		t.Errorf("Subject = %q, want %q", payload.Subject, "account-1")
	}
	if payload.Object != "agent-1" {
		t.Errorf("Object = %q, want %q", payload.Object, "agent-1")
	}
	if payload.Role != "role-editor" {
		t.Errorf("Role = %q, want %q", payload.Role, "role-editor")
	}
}

func TestAccount_ValidationErrors(t *testing.T) {
	t.Parallel()

	account, err := new(entities.Account).With("account-1", "Acme Corp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := account.AddMember("", "role-admin"); err == nil {
		t.Error("expected error for empty agent ID")
	}
	if err := account.AddMember("agent-1", ""); err == nil {
		t.Error("expected error for empty role ID")
	}
	if err := account.RemoveMember(""); err == nil {
		t.Error("expected error for empty agent ID")
	}
	if err := account.ChangeMemberRole("", "role-editor"); err == nil {
		t.Error("expected error for empty agent ID")
	}
	if err := account.ChangeMemberRole("agent-1", ""); err == nil {
		t.Error("expected error for empty role ID")
	}
}

func TestAccount_ApplyEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	account, err := new(entities.Account).With("account-1", "Acme Corp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := account.GetUncommittedEvents()
	account.ClearUncommittedEvents()

	// Reconstruct from events
	restored := &entities.Account{}
	if err := restored.Restore("account-1", "placeholder", true, events[0].Created); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	if err := restored.ApplyEvent(ctx, events[0]); err != nil {
		t.Fatalf("ApplyEvent() error: %v", err)
	}
	if restored.Name() != "Acme Corp" {
		t.Errorf("Name() = %q, want %q", restored.Name(), "Acme Corp")
	}
}
