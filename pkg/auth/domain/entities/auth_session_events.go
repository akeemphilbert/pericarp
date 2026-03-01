package entities

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// SessionCreated represents the creation of an authenticated session.
// Triple: (Agent, schema:session, Session)
type SessionCreated struct {
	domain.BasicTripleEvent
	CredentialID string    `json:"credential_id"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	ExpiresAt    time.Time `json:"expires_at"`
	Timestamp    time.Time `json:"timestamp"`
}

// With creates a new SessionCreated event.
func (e SessionCreated) With(agentID, sessionID, credentialID, ipAddress, userAgent string, expiresAt time.Time) SessionCreated {
	return SessionCreated{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   agentID,
			Predicate: PredicateSession,
			Object:    sessionID,
		},
		CredentialID: credentialID,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		ExpiresAt:    expiresAt,
		Timestamp:    time.Now(),
	}
}

// EventType returns the event type name.
func (e SessionCreated) EventType() string {
	return EventTypeSessionCreated
}

// SessionTouched represents an update to the session's last accessed time.
type SessionTouched struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new SessionTouched event.
func (e SessionTouched) With() SessionTouched {
	return SessionTouched{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e SessionTouched) EventType() string {
	return EventTypeSessionTouched
}

// SessionRevoked represents the revocation of an authenticated session.
type SessionRevoked struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new SessionRevoked event.
func (e SessionRevoked) With() SessionRevoked {
	return SessionRevoked{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e SessionRevoked) EventType() string {
	return EventTypeSessionRevoked
}

// SessionAccountScoped represents scoping a session to a specific account.
// Triple: (Session, schema:authenticator, Account)
type SessionAccountScoped struct {
	domain.BasicTripleEvent
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new SessionAccountScoped event.
func (e SessionAccountScoped) With(sessionID, accountID string) SessionAccountScoped {
	return SessionAccountScoped{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   sessionID,
			Predicate: PredicateAuthenticator,
			Object:    accountID,
		},
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e SessionAccountScoped) EventType() string {
	return EventTypeSessionAccountScoped
}
