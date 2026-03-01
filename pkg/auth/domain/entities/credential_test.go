package entities_test

import (
	"context"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

func TestCredential_With(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		id             string
		agentID        string
		provider       string
		providerUserID string
		email          string
		displayName    string
		wantErr        bool
	}{
		{
			name:           "creates credential with all fields",
			id:             "cred-1",
			agentID:        "agent-1",
			provider:       "google",
			providerUserID: "google-123",
			email:          "alice@example.com",
			displayName:    "Alice",
		},
		{
			name:           "creates credential with empty email",
			id:             "cred-2",
			agentID:        "agent-1",
			provider:       "github",
			providerUserID: "gh-456",
			email:          "",
			displayName:    "Alice",
		},
		{
			name:           "creates credential with empty display name",
			id:             "cred-3",
			agentID:        "agent-1",
			provider:       "github",
			providerUserID: "gh-789",
			email:          "alice@example.com",
			displayName:    "",
		},
		{
			name:           "fails with empty ID",
			id:             "",
			agentID:        "agent-1",
			provider:       "google",
			providerUserID: "google-123",
			email:          "alice@example.com",
			displayName:    "Alice",
			wantErr:        true,
		},
		{
			name:           "fails with empty agent ID",
			id:             "cred-1",
			agentID:        "",
			provider:       "google",
			providerUserID: "google-123",
			email:          "alice@example.com",
			displayName:    "Alice",
			wantErr:        true,
		},
		{
			name:           "fails with empty provider",
			id:             "cred-1",
			agentID:        "agent-1",
			provider:       "",
			providerUserID: "google-123",
			email:          "alice@example.com",
			displayName:    "Alice",
			wantErr:        true,
		},
		{
			name:           "fails with empty provider user ID",
			id:             "cred-1",
			agentID:        "agent-1",
			provider:       "google",
			providerUserID: "",
			email:          "alice@example.com",
			displayName:    "Alice",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cred, err := new(entities.Credential).With(tt.id, tt.agentID, tt.provider, tt.providerUserID, tt.email, tt.displayName)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cred.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", cred.GetID(), tt.id)
			}
			if cred.AgentID() != tt.agentID {
				t.Errorf("AgentID() = %q, want %q", cred.AgentID(), tt.agentID)
			}
			if cred.Provider() != tt.provider {
				t.Errorf("Provider() = %q, want %q", cred.Provider(), tt.provider)
			}
			if cred.ProviderUserID() != tt.providerUserID {
				t.Errorf("ProviderUserID() = %q, want %q", cred.ProviderUserID(), tt.providerUserID)
			}
			if cred.Email() != tt.email {
				t.Errorf("Email() = %q, want %q", cred.Email(), tt.email)
			}
			if cred.DisplayName() != tt.displayName {
				t.Errorf("DisplayName() = %q, want %q", cred.DisplayName(), tt.displayName)
			}
			if !cred.Active() {
				t.Error("expected credential to be active")
			}
			if cred.CreatedAt().IsZero() {
				t.Error("expected non-zero CreatedAt")
			}

			events := cred.GetUncommittedEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 uncommitted event, got %d", len(events))
			}
			if events[0].EventType != entities.EventTypeCredentialCreated {
				t.Errorf("event type = %q, want %q", events[0].EventType, entities.EventTypeCredentialCreated)
			}

			payload, ok := events[0].Payload.(entities.CredentialCreated)
			if !ok {
				t.Fatalf("expected CredentialCreated payload, got %T", events[0].Payload)
			}
			if payload.Subject != tt.agentID {
				t.Errorf("Subject = %q, want %q", payload.Subject, tt.agentID)
			}
			if payload.Predicate != entities.PredicateCredential {
				t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateCredential)
			}
			if payload.Object != tt.id {
				t.Errorf("Object = %q, want %q", payload.Object, tt.id)
			}
			if payload.Provider != tt.provider {
				t.Errorf("Provider = %q, want %q", payload.Provider, tt.provider)
			}
			if payload.ProviderUserID != tt.providerUserID {
				t.Errorf("ProviderUserID = %q, want %q", payload.ProviderUserID, tt.providerUserID)
			}
		})
	}
}

func TestCredential_MarkUsed(t *testing.T) {
	t.Parallel()

	cred, err := new(entities.Credential).With("cred-1", "agent-1", "google", "google-123", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	beforeMark := time.Now()
	if err := cred.MarkUsed(); err != nil {
		t.Fatalf("MarkUsed() error: %v", err)
	}

	if cred.LastUsedAt().Before(beforeMark) {
		t.Error("expected LastUsedAt to be updated")
	}

	events := cred.GetUncommittedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1].EventType != entities.EventTypeCredentialUsed {
		t.Errorf("event type = %q, want %q", events[1].EventType, entities.EventTypeCredentialUsed)
	}
}

func TestCredential_DeactivateReactivate(t *testing.T) {
	t.Parallel()

	cred, err := new(entities.Credential).With("cred-1", "agent-1", "google", "google-123", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Deactivate
	if err := cred.Deactivate(); err != nil {
		t.Fatalf("Deactivate() error: %v", err)
	}
	if cred.Active() {
		t.Error("expected credential to be inactive after Deactivate")
	}

	events := cred.GetUncommittedEvents()
	lastEvent := events[len(events)-1]
	if lastEvent.EventType != entities.EventTypeCredentialDeactivated {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypeCredentialDeactivated)
	}

	// Deactivate again (no-op)
	eventsBefore := len(cred.GetUncommittedEvents())
	if err := cred.Deactivate(); err != nil {
		t.Fatalf("Deactivate() error: %v", err)
	}
	if len(cred.GetUncommittedEvents()) != eventsBefore {
		t.Error("expected no new event from duplicate Deactivate")
	}

	// Reactivate
	if err := cred.Reactivate(); err != nil {
		t.Fatalf("Reactivate() error: %v", err)
	}
	if !cred.Active() {
		t.Error("expected credential to be active after Reactivate")
	}

	events = cred.GetUncommittedEvents()
	lastEvent = events[len(events)-1]
	if lastEvent.EventType != entities.EventTypeCredentialReactivated {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypeCredentialReactivated)
	}

	// Reactivate again (no-op)
	eventsBefore = len(cred.GetUncommittedEvents())
	if err := cred.Reactivate(); err != nil {
		t.Fatalf("Reactivate() error: %v", err)
	}
	if len(cred.GetUncommittedEvents()) != eventsBefore {
		t.Error("expected no new event from duplicate Reactivate")
	}
}

func TestCredential_Restore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		agentID string
		wantErr bool
	}{
		{
			name:    "restores credential successfully",
			id:      "cred-1",
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
			id:      "cred-1",
			agentID: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cred := &entities.Credential{}
			now := time.Now()
			err := cred.Restore(tt.id, tt.agentID, "google", "google-123", "alice@example.com", "Alice", true, now, now)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cred.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", cred.GetID(), tt.id)
			}
			if cred.AgentID() != tt.agentID {
				t.Errorf("AgentID() = %q, want %q", cred.AgentID(), tt.agentID)
			}
			if cred.Provider() != "google" {
				t.Errorf("Provider() = %q, want %q", cred.Provider(), "google")
			}
			if !cred.Active() {
				t.Error("expected credential to be active")
			}

			// Restore should not record events
			events := cred.GetUncommittedEvents()
			if len(events) != 0 {
				t.Errorf("expected 0 uncommitted events after Restore, got %d", len(events))
			}
		})
	}
}

func TestCredential_ApplyEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Create credential to capture events
	cred, err := new(entities.Credential).With("cred-1", "agent-1", "google", "google-123", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	createdEvents := cred.GetUncommittedEvents()
	cred.ClearUncommittedEvents()

	// Reconstruct from events
	restored := &entities.Credential{}
	if err := restored.Restore("cred-1", "placeholder", "placeholder", "placeholder", "", "", false, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	// Apply creation event
	if err := restored.ApplyEvent(ctx, createdEvents[0]); err != nil {
		t.Fatalf("ApplyEvent(CredentialCreated) error: %v", err)
	}
	if restored.AgentID() != "agent-1" {
		t.Errorf("AgentID() = %q, want %q", restored.AgentID(), "agent-1")
	}
	if restored.Provider() != "google" {
		t.Errorf("Provider() = %q, want %q", restored.Provider(), "google")
	}
	if restored.Email() != "alice@example.com" {
		t.Errorf("Email() = %q, want %q", restored.Email(), "alice@example.com")
	}
	if !restored.Active() {
		t.Error("expected restored credential to be active")
	}
}

func TestCredential_ApplyEvent_Deactivated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cred, err := new(entities.Credential).With("cred-1", "agent-1", "google", "google-123", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := cred.Deactivate(); err != nil {
		t.Fatalf("Deactivate() error: %v", err)
	}

	events := cred.GetUncommittedEvents()
	cred.ClearUncommittedEvents()

	// Reconstruct
	restored := &entities.Credential{}
	if err := restored.Restore("cred-1", "placeholder", "", "", "", "", true, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	for _, evt := range events {
		if err := restored.ApplyEvent(ctx, evt); err != nil {
			t.Fatalf("ApplyEvent() error: %v", err)
		}
	}

	if restored.Active() {
		t.Error("expected restored credential to be inactive after applying Deactivated event")
	}
}

func TestCredential_ApplyEvent_Reactivated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cred, err := new(entities.Credential).With("cred-1", "agent-1", "google", "google-123", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := cred.Deactivate(); err != nil {
		t.Fatalf("Deactivate() error: %v", err)
	}
	if err := cred.Reactivate(); err != nil {
		t.Fatalf("Reactivate() error: %v", err)
	}

	events := cred.GetUncommittedEvents()
	cred.ClearUncommittedEvents()

	// Reconstruct
	restored := &entities.Credential{}
	if err := restored.Restore("cred-1", "placeholder", "", "", "", "", false, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	for _, evt := range events {
		if err := restored.ApplyEvent(ctx, evt); err != nil {
			t.Fatalf("ApplyEvent() error: %v", err)
		}
	}

	if !restored.Active() {
		t.Error("expected restored credential to be active after applying Reactivated event")
	}
}
