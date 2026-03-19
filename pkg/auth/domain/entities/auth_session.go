package entities

import (
	"context"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// AuthSession represents an authenticated session for an agent.
// Sessions track authentication state including expiration and account scoping.
type AuthSession struct {
	*ddd.BaseEntity
	agentID        string
	accountID      string
	credentialID   string
	active         bool
	createdAt      time.Time
	expiresAt      time.Time
	lastAccessedAt time.Time
	ipAddress      string
	userAgent      string
}

// With initializes a new AuthSession with the given parameters.
func (s *AuthSession) With(id, agentID, credentialID, ipAddress, userAgent string, expiresAt time.Time) (*AuthSession, error) {
	if id == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}
	if agentID == "" {
		return nil, fmt.Errorf("agent ID cannot be empty")
	}
	if credentialID == "" {
		return nil, fmt.Errorf("credential ID cannot be empty")
	}

	s.BaseEntity = ddd.NewBaseEntity(id)
	s.agentID = agentID
	s.credentialID = credentialID
	s.active = true
	s.createdAt = time.Now()
	s.expiresAt = expiresAt
	s.lastAccessedAt = s.createdAt
	s.ipAddress = ipAddress
	s.userAgent = userAgent

	event := new(SessionCreated).With(agentID, id, credentialID, ipAddress, userAgent, expiresAt)
	if err := s.RecordEvent(event, event.EventType()); err != nil {
		return nil, fmt.Errorf("failed to record Session.Created event: %w", err)
	}

	return s, nil
}

// AgentID returns the ID of the agent who owns this session.
func (s *AuthSession) AgentID() string {
	return s.agentID
}

// AccountID returns the account this session is scoped to. Empty if not scoped.
func (s *AuthSession) AccountID() string {
	return s.accountID
}

// CredentialID returns the ID of the credential used to create this session.
func (s *AuthSession) CredentialID() string {
	return s.credentialID
}

// Active returns whether the session is currently active.
func (s *AuthSession) Active() bool {
	return s.active
}

// CreatedAt returns when the session was created.
func (s *AuthSession) CreatedAt() time.Time {
	return s.createdAt
}

// ExpiresAt returns when the session expires.
func (s *AuthSession) ExpiresAt() time.Time {
	return s.expiresAt
}

// LastAccessedAt returns when the session was last accessed.
func (s *AuthSession) LastAccessedAt() time.Time {
	return s.lastAccessedAt
}

// IPAddress returns the IP address from which the session was created.
func (s *AuthSession) IPAddress() string {
	return s.ipAddress
}

// UserAgent returns the user agent string from the session creation request.
func (s *AuthSession) UserAgent() string {
	return s.userAgent
}

// Restore restores an AuthSession from database values without recording events.
func (s *AuthSession) Restore(id, agentID, accountID, credentialID, ipAddress, userAgent string, active bool, createdAt, expiresAt, lastAccessedAt time.Time) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	if agentID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}
	s.BaseEntity = ddd.NewBaseEntity(id)
	s.agentID = agentID
	s.accountID = accountID
	s.credentialID = credentialID
	s.active = active
	s.createdAt = createdAt
	s.expiresAt = expiresAt
	s.lastAccessedAt = lastAccessedAt
	s.ipAddress = ipAddress
	s.userAgent = userAgent
	return nil
}

// Touch updates the session's last accessed time.
func (s *AuthSession) Touch() error {
	s.lastAccessedAt = time.Now()
	event := new(SessionTouched).With()
	return s.RecordEvent(event, event.EventType())
}

// Revoke marks the session as inactive.
func (s *AuthSession) Revoke() error {
	if !s.active {
		return nil
	}
	s.active = false
	event := new(SessionRevoked).With()
	return s.RecordEvent(event, event.EventType())
}

// IsExpired returns whether the session has passed its expiration time.
func (s *AuthSession) IsExpired() bool {
	return time.Now().After(s.expiresAt)
}

// ScopeToAccount scopes this session to a specific account.
func (s *AuthSession) ScopeToAccount(accountID string) error {
	if accountID == "" {
		return fmt.Errorf("account ID cannot be empty")
	}
	s.accountID = accountID
	event := new(SessionAccountScoped).With(s.GetID(), accountID)
	return s.RecordEvent(event, event.EventType())
}

// ApplyEvent applies a domain event to reconstruct the aggregate state.
func (s *AuthSession) ApplyEvent(ctx context.Context, envelope domain.EventEnvelope[any]) error {
	if err := s.BaseEntity.ApplyEvent(ctx, envelope); err != nil {
		return fmt.Errorf("base entity apply event failed: %w", err)
	}

	switch payload := envelope.Payload.(type) {
	case SessionCreated:
		s.agentID = payload.Subject
		s.credentialID = payload.CredentialID
		s.active = true
		s.createdAt = payload.Timestamp
		s.expiresAt = payload.ExpiresAt
		s.lastAccessedAt = payload.Timestamp
		s.ipAddress = payload.IPAddress
		s.userAgent = payload.UserAgent
	case SessionTouched:
		s.lastAccessedAt = payload.Timestamp
	case SessionRevoked:
		s.active = false
	case SessionAccountScoped:
		s.accountID = payload.Object
	default:
		return fmt.Errorf("unknown event type: %T", envelope.Payload)
	}
	return nil
}
