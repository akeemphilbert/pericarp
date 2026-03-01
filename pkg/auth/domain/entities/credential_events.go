package entities

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// CredentialCreated represents the creation of a credential linking an external identity provider to an agent.
// Triple: (Agent, schema:credential, Credential)
type CredentialCreated struct {
	domain.BasicTripleEvent
	Provider       string    `json:"provider"`
	ProviderUserID string    `json:"provider_user_id"`
	Email          string    `json:"email"`
	DisplayName    string    `json:"display_name"`
	Timestamp      time.Time `json:"timestamp"`
}

// With creates a new CredentialCreated event.
func (e CredentialCreated) With(agentID, credentialID, provider, providerUserID, email, displayName string) CredentialCreated {
	return CredentialCreated{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   agentID,
			Predicate: PredicateCredential,
			Object:    credentialID,
		},
		Provider:       provider,
		ProviderUserID: providerUserID,
		Email:          email,
		DisplayName:    displayName,
		Timestamp:      time.Now(),
	}
}

// EventType returns the event type name.
func (e CredentialCreated) EventType() string {
	return EventTypeCredentialCreated
}

// CredentialUsed represents a credential being used for authentication.
type CredentialUsed struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new CredentialUsed event.
func (e CredentialUsed) With() CredentialUsed {
	return CredentialUsed{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e CredentialUsed) EventType() string {
	return EventTypeCredentialUsed
}

// CredentialDeactivated represents the deactivation of a credential.
type CredentialDeactivated struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new CredentialDeactivated event.
func (e CredentialDeactivated) With() CredentialDeactivated {
	return CredentialDeactivated{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e CredentialDeactivated) EventType() string {
	return EventTypeCredentialDeactivated
}

// CredentialReactivated represents the reactivation of a credential.
type CredentialReactivated struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new CredentialReactivated event.
func (e CredentialReactivated) With() CredentialReactivated {
	return CredentialReactivated{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e CredentialReactivated) EventType() string {
	return EventTypeCredentialReactivated
}
