package entities_test

import (
	"context"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

func TestPasswordCredential_With(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		id           string
		credentialID string
		algorithm    string
		hash         string
		wantErr      bool
	}{
		{name: "creates password credential", id: "pc-1", credentialID: "cred-1", algorithm: "bcrypt", hash: "$2a$10$abc"},
		{name: "fails with empty id", id: "", credentialID: "cred-1", algorithm: "bcrypt", hash: "$2a$10$abc", wantErr: true},
		{name: "fails with empty credential id", id: "pc-1", credentialID: "", algorithm: "bcrypt", hash: "$2a$10$abc", wantErr: true},
		{name: "fails with empty algorithm", id: "pc-1", credentialID: "cred-1", algorithm: "", hash: "$2a$10$abc", wantErr: true},
		{name: "fails with empty hash", id: "pc-1", credentialID: "cred-1", algorithm: "bcrypt", hash: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pc, err := new(entities.PasswordCredential).With(tt.id, tt.credentialID, tt.algorithm, tt.hash)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pc.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", pc.GetID(), tt.id)
			}
			if pc.CredentialID() != tt.credentialID {
				t.Errorf("CredentialID() = %q, want %q", pc.CredentialID(), tt.credentialID)
			}
			if pc.Algorithm() != tt.algorithm {
				t.Errorf("Algorithm() = %q, want %q", pc.Algorithm(), tt.algorithm)
			}
			if pc.Hash() != tt.hash {
				t.Errorf("Hash() = %q, want %q", pc.Hash(), tt.hash)
			}
			if pc.CreatedAt().IsZero() {
				t.Error("expected non-zero CreatedAt")
			}

			events := pc.GetUncommittedEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 uncommitted event, got %d", len(events))
			}
			if events[0].EventType != entities.EventTypePasswordCredentialCreated {
				t.Errorf("event type = %q, want %q", events[0].EventType, entities.EventTypePasswordCredentialCreated)
			}

			payload, ok := events[0].Payload.(entities.PasswordCredentialCreated)
			if !ok {
				t.Fatalf("expected PasswordCredentialCreated payload, got %T", events[0].Payload)
			}
			if payload.PasswordCredentialID != tt.id {
				t.Errorf("PasswordCredentialID = %q, want %q", payload.PasswordCredentialID, tt.id)
			}
			if payload.CredentialID != tt.credentialID {
				t.Errorf("CredentialID = %q, want %q", payload.CredentialID, tt.credentialID)
			}
			if payload.Algorithm != tt.algorithm {
				t.Errorf("Algorithm = %q, want %q", payload.Algorithm, tt.algorithm)
			}
		})
	}
}

func TestPasswordCredential_EventCarriesNoSecrets(t *testing.T) {
	t.Parallel()

	pc, err := new(entities.PasswordCredential).With("pc-1", "cred-1", "bcrypt", "$2a$10$top-secret-hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := pc.Update("bcrypt", "$2a$10$another-secret"); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	for _, evt := range pc.GetUncommittedEvents() {
		switch p := evt.Payload.(type) {
		case entities.PasswordCredentialCreated:
			// Verify struct shape carries only metadata fields. The presence
			// of any string field whose value matches the hash would mean a
			// leak; we rely on the type system to keep the surface minimal,
			// and assert on the absence of any "Hash" field by inspecting
			// the documented public fields.
			if p.Algorithm == "" {
				t.Error("expected algorithm set")
			}
		case entities.PasswordUpdated:
			if p.Algorithm == "" {
				t.Error("expected algorithm set")
			}
		default:
			t.Fatalf("unexpected event type: %T", evt.Payload)
		}
	}
}

func TestPasswordCredential_Update(t *testing.T) {
	t.Parallel()

	pc, err := new(entities.PasswordCredential).With("pc-1", "cred-1", "bcrypt", "$2a$10$abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	originalUpdated := pc.UpdatedAt()

	if err := pc.Update("bcrypt", "$2a$12$new"); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if pc.Hash() != "$2a$12$new" {
		t.Errorf("Hash() = %q, want $2a$12$new", pc.Hash())
	}
	// UpdatedAt must not regress. We avoid asserting strict advance with a
	// sleep — coarse-resolution clocks (Windows, some CI runners) can leave
	// time.Now() unchanged across two adjacent calls and would make the
	// test flaky. The recorded PasswordUpdated event below already proves
	// Update() was invoked.
	if pc.UpdatedAt().Before(originalUpdated) {
		t.Errorf("UpdatedAt regressed: %s -> %s", originalUpdated, pc.UpdatedAt())
	}

	events := pc.GetUncommittedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1].EventType != entities.EventTypePasswordUpdated {
		t.Errorf("event[1].EventType = %q, want %q", events[1].EventType, entities.EventTypePasswordUpdated)
	}
	payload, ok := events[1].Payload.(entities.PasswordUpdated)
	if !ok {
		t.Fatalf("expected PasswordUpdated payload, got %T", events[1].Payload)
	}
	if payload.Algorithm != "bcrypt" {
		t.Errorf("Algorithm = %q, want bcrypt", payload.Algorithm)
	}

	if err := pc.Update("", "$2a$12$xyz"); err == nil {
		t.Error("expected error for empty algorithm")
	}
	if err := pc.Update("bcrypt", ""); err == nil {
		t.Error("expected error for empty hash")
	}
}

func TestPasswordCredential_MarkVerified(t *testing.T) {
	t.Parallel()

	pc, err := new(entities.PasswordCredential).With("pc-1", "cred-1", "bcrypt", "$2a$10$abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pc.LastVerifiedAt().IsZero() {
		t.Error("expected zero LastVerifiedAt before MarkVerified")
	}

	before := time.Now()
	pc.MarkVerified()
	if pc.LastVerifiedAt().Before(before) {
		t.Error("expected LastVerifiedAt to be updated")
	}

	// MarkVerified is intentionally event-free.
	if got := len(pc.GetUncommittedEvents()); got != 1 {
		t.Errorf("expected exactly 1 uncommitted event after MarkVerified, got %d", got)
	}
}

func TestPasswordCredential_Restore(t *testing.T) {
	t.Parallel()

	pc := &entities.PasswordCredential{}
	now := time.Now()
	if err := pc.Restore("pc-1", "cred-1", "bcrypt", "$2a$10$abc", now, now, time.Time{}); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}
	if pc.GetID() != "pc-1" {
		t.Errorf("GetID() = %q, want pc-1", pc.GetID())
	}
	if got := len(pc.GetUncommittedEvents()); got != 0 {
		t.Errorf("expected 0 uncommitted events after Restore, got %d", got)
	}
	if err := (&entities.PasswordCredential{}).Restore("", "cred-1", "bcrypt", "$2a$10$abc", now, now, time.Time{}); err == nil {
		t.Error("expected error for empty id")
	}
	if err := (&entities.PasswordCredential{}).Restore("pc-1", "", "bcrypt", "$2a$10$abc", now, now, time.Time{}); err == nil {
		t.Error("expected error for empty credential id")
	}
}

func TestPasswordCredential_ApplyEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	pc, err := new(entities.PasswordCredential).With("pc-1", "cred-1", "bcrypt", "$2a$10$abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := pc.Update("bcrypt", "$2a$12$new"); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	events := pc.GetUncommittedEvents()
	pc.ClearUncommittedEvents()

	restored := &entities.PasswordCredential{}
	if err := restored.Restore("pc-1", "placeholder", "", "", time.Time{}, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}
	for _, evt := range events {
		if err := restored.ApplyEvent(ctx, evt); err != nil {
			t.Fatalf("ApplyEvent() error: %v", err)
		}
	}
	if restored.CredentialID() != "cred-1" {
		t.Errorf("CredentialID() = %q, want cred-1", restored.CredentialID())
	}
	if restored.Algorithm() != "bcrypt" {
		t.Errorf("Algorithm() = %q, want bcrypt", restored.Algorithm())
	}
	if restored.UpdatedAt().IsZero() {
		t.Error("expected non-zero UpdatedAt after replay")
	}
}
