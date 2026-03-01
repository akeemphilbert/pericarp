package entities_test

import (
	"context"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

func TestAuthSession_With(t *testing.T) {
	t.Parallel()

	future := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name         string
		id           string
		agentID      string
		credentialID string
		ipAddress    string
		userAgent    string
		expiresAt    time.Time
		wantErr      bool
	}{
		{
			name:         "creates session with all fields",
			id:           "sess-1",
			agentID:      "agent-1",
			credentialID: "cred-1",
			ipAddress:    "192.168.1.1",
			userAgent:    "Mozilla/5.0",
			expiresAt:    future,
		},
		{
			name:         "creates session with empty optional fields",
			id:           "sess-2",
			agentID:      "agent-1",
			credentialID: "cred-1",
			ipAddress:    "",
			userAgent:    "",
			expiresAt:    future,
		},
		{
			name:         "fails with empty ID",
			id:           "",
			agentID:      "agent-1",
			credentialID: "cred-1",
			ipAddress:    "192.168.1.1",
			userAgent:    "Mozilla/5.0",
			expiresAt:    future,
			wantErr:      true,
		},
		{
			name:         "fails with empty agent ID",
			id:           "sess-1",
			agentID:      "",
			credentialID: "cred-1",
			ipAddress:    "192.168.1.1",
			userAgent:    "Mozilla/5.0",
			expiresAt:    future,
			wantErr:      true,
		},
		{
			name:         "fails with empty credential ID",
			id:           "sess-1",
			agentID:      "agent-1",
			credentialID: "",
			ipAddress:    "192.168.1.1",
			userAgent:    "Mozilla/5.0",
			expiresAt:    future,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sess, err := new(entities.AuthSession).With(tt.id, tt.agentID, tt.credentialID, tt.ipAddress, tt.userAgent, tt.expiresAt)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if sess.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", sess.GetID(), tt.id)
			}
			if sess.AgentID() != tt.agentID {
				t.Errorf("AgentID() = %q, want %q", sess.AgentID(), tt.agentID)
			}
			if sess.CredentialID() != tt.credentialID {
				t.Errorf("CredentialID() = %q, want %q", sess.CredentialID(), tt.credentialID)
			}
			if sess.IPAddress() != tt.ipAddress {
				t.Errorf("IPAddress() = %q, want %q", sess.IPAddress(), tt.ipAddress)
			}
			if sess.UserAgent() != tt.userAgent {
				t.Errorf("UserAgent() = %q, want %q", sess.UserAgent(), tt.userAgent)
			}
			if !sess.Active() {
				t.Error("expected session to be active")
			}
			if sess.CreatedAt().IsZero() {
				t.Error("expected non-zero CreatedAt")
			}
			if sess.ExpiresAt().IsZero() {
				t.Error("expected non-zero ExpiresAt")
			}
			if sess.AccountID() != "" {
				t.Errorf("expected empty AccountID, got %q", sess.AccountID())
			}

			events := sess.GetUncommittedEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 uncommitted event, got %d", len(events))
			}
			if events[0].EventType != entities.EventTypeSessionCreated {
				t.Errorf("event type = %q, want %q", events[0].EventType, entities.EventTypeSessionCreated)
			}

			payload, ok := events[0].Payload.(entities.SessionCreated)
			if !ok {
				t.Fatalf("expected SessionCreated payload, got %T", events[0].Payload)
			}
			if payload.Subject != tt.agentID {
				t.Errorf("Subject = %q, want %q", payload.Subject, tt.agentID)
			}
			if payload.Predicate != entities.PredicateSession {
				t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateSession)
			}
			if payload.Object != tt.id {
				t.Errorf("Object = %q, want %q", payload.Object, tt.id)
			}
			if payload.CredentialID != tt.credentialID {
				t.Errorf("CredentialID = %q, want %q", payload.CredentialID, tt.credentialID)
			}
		})
	}
}

func TestAuthSession_Touch(t *testing.T) {
	t.Parallel()

	sess, err := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	beforeTouch := time.Now()
	if err := sess.Touch(); err != nil {
		t.Fatalf("Touch() error: %v", err)
	}

	if sess.LastAccessedAt().Before(beforeTouch) {
		t.Error("expected LastAccessedAt to be updated after Touch")
	}

	events := sess.GetUncommittedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1].EventType != entities.EventTypeSessionTouched {
		t.Errorf("event type = %q, want %q", events[1].EventType, entities.EventTypeSessionTouched)
	}
}

func TestAuthSession_Revoke(t *testing.T) {
	t.Parallel()

	sess, err := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Revoke
	if err := sess.Revoke(); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}
	if sess.Active() {
		t.Error("expected session to be inactive after Revoke")
	}

	events := sess.GetUncommittedEvents()
	lastEvent := events[len(events)-1]
	if lastEvent.EventType != entities.EventTypeSessionRevoked {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypeSessionRevoked)
	}

	// Revoke again (no-op)
	eventsBefore := len(sess.GetUncommittedEvents())
	if err := sess.Revoke(); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}
	if len(sess.GetUncommittedEvents()) != eventsBefore {
		t.Error("expected no new event from duplicate Revoke")
	}
}

func TestAuthSession_IsExpired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expiresAt   time.Time
		wantExpired bool
	}{
		{
			name:        "not expired with future expiry",
			expiresAt:   time.Now().Add(24 * time.Hour),
			wantExpired: false,
		},
		{
			name:        "expired with past expiry",
			expiresAt:   time.Now().Add(-1 * time.Hour),
			wantExpired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sess, err := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", tt.expiresAt)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if sess.IsExpired() != tt.wantExpired {
				t.Errorf("IsExpired() = %v, want %v", sess.IsExpired(), tt.wantExpired)
			}
		})
	}
}

func TestAuthSession_ScopeToAccount(t *testing.T) {
	t.Parallel()

	sess, err := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Scope to account
	if err := sess.ScopeToAccount("account-1"); err != nil {
		t.Fatalf("ScopeToAccount() error: %v", err)
	}
	if sess.AccountID() != "account-1" {
		t.Errorf("AccountID() = %q, want %q", sess.AccountID(), "account-1")
	}

	events := sess.GetUncommittedEvents()
	lastEvent := events[len(events)-1]
	if lastEvent.EventType != entities.EventTypeSessionAccountScoped {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypeSessionAccountScoped)
	}

	payload, ok := lastEvent.Payload.(entities.SessionAccountScoped)
	if !ok {
		t.Fatalf("expected SessionAccountScoped payload, got %T", lastEvent.Payload)
	}
	if payload.Subject != "sess-1" {
		t.Errorf("Subject = %q, want %q", payload.Subject, "sess-1")
	}
	if payload.Predicate != entities.PredicateAuthenticator {
		t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateAuthenticator)
	}
	if payload.Object != "account-1" {
		t.Errorf("Object = %q, want %q", payload.Object, "account-1")
	}
}

func TestAuthSession_ScopeToAccount_EmptyID(t *testing.T) {
	t.Parallel()

	sess, err := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := sess.ScopeToAccount(""); err == nil {
		t.Error("expected error for empty account ID")
	}
}

func TestAuthSession_Restore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		agentID string
		wantErr bool
	}{
		{
			name:    "restores session successfully",
			id:      "sess-1",
			agentID: "agent-1",
		},
		{
			name:    "fails with empty id",
			id:      "",
			agentID: "agent-1",
			wantErr: true,
		},
		{
			name:    "fails with empty agent ID",
			id:      "sess-1",
			agentID: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sess := &entities.AuthSession{}
			now := time.Now()
			err := sess.Restore(tt.id, tt.agentID, "account-1", "cred-1", "192.168.1.1", "Mozilla/5.0", true, now, now.Add(24*time.Hour), now)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if sess.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", sess.GetID(), tt.id)
			}
			if sess.AgentID() != tt.agentID {
				t.Errorf("AgentID() = %q, want %q", sess.AgentID(), tt.agentID)
			}
			if sess.AccountID() != "account-1" {
				t.Errorf("AccountID() = %q, want %q", sess.AccountID(), "account-1")
			}
			if !sess.Active() {
				t.Error("expected session to be active")
			}

			events := sess.GetUncommittedEvents()
			if len(events) != 0 {
				t.Errorf("expected 0 uncommitted events after Restore, got %d", len(events))
			}
		})
	}
}

func TestAuthSession_ApplyEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sess, err := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := sess.GetUncommittedEvents()
	sess.ClearUncommittedEvents()

	// Reconstruct from events
	restored := &entities.AuthSession{}
	if err := restored.Restore("sess-1", "placeholder", "", "", "", "", false, time.Time{}, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	if err := restored.ApplyEvent(ctx, events[0]); err != nil {
		t.Fatalf("ApplyEvent(SessionCreated) error: %v", err)
	}
	if restored.AgentID() != "agent-1" {
		t.Errorf("AgentID() = %q, want %q", restored.AgentID(), "agent-1")
	}
	if restored.CredentialID() != "cred-1" {
		t.Errorf("CredentialID() = %q, want %q", restored.CredentialID(), "cred-1")
	}
	if !restored.Active() {
		t.Error("expected restored session to be active")
	}
}

func TestAuthSession_ApplyEvent_Revoked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sess, err := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := sess.Revoke(); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}

	events := sess.GetUncommittedEvents()
	sess.ClearUncommittedEvents()

	// Reconstruct
	restored := &entities.AuthSession{}
	if err := restored.Restore("sess-1", "placeholder", "", "", "", "", true, time.Time{}, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	for _, evt := range events {
		if err := restored.ApplyEvent(ctx, evt); err != nil {
			t.Fatalf("ApplyEvent() error: %v", err)
		}
	}

	if restored.Active() {
		t.Error("expected restored session to be inactive after applying Revoked event")
	}
}

func TestAuthSession_ApplyEvent_AccountScoped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sess, err := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := sess.ScopeToAccount("account-1"); err != nil {
		t.Fatalf("ScopeToAccount() error: %v", err)
	}

	events := sess.GetUncommittedEvents()
	sess.ClearUncommittedEvents()

	// Reconstruct
	restored := &entities.AuthSession{}
	if err := restored.Restore("sess-1", "placeholder", "", "", "", "", true, time.Time{}, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	for _, evt := range events {
		if err := restored.ApplyEvent(ctx, evt); err != nil {
			t.Fatalf("ApplyEvent() error: %v", err)
		}
	}

	if restored.AccountID() != "account-1" {
		t.Errorf("AccountID() = %q, want %q", restored.AccountID(), "account-1")
	}
}
