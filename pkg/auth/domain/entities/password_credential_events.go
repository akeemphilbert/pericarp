package entities

import "time"

// PasswordCredentialCreated represents the creation of a password credential
// linked to a Credential aggregate. The hash and plaintext are deliberately
// excluded; only metadata is carried so events can be safely persisted and
// dispatched.
type PasswordCredentialCreated struct {
	PasswordCredentialID string    `json:"password_credential_id"`
	CredentialID         string    `json:"credential_id"`
	Algorithm            string    `json:"algorithm"`
	CreatedAt            time.Time `json:"created_at"`
}

// With creates a new PasswordCredentialCreated event.
func (e PasswordCredentialCreated) With(passwordCredentialID, credentialID, algorithm string, createdAt time.Time) PasswordCredentialCreated {
	return PasswordCredentialCreated{
		PasswordCredentialID: passwordCredentialID,
		CredentialID:         credentialID,
		Algorithm:            algorithm,
		CreatedAt:            createdAt,
	}
}

// EventType returns the event type name.
func (e PasswordCredentialCreated) EventType() string {
	return EventTypePasswordCredentialCreated
}

// PasswordUpdated represents a password rotation on a PasswordCredential.
// Carries only metadata — never the hash or plaintext.
type PasswordUpdated struct {
	PasswordCredentialID string    `json:"password_credential_id"`
	CredentialID         string    `json:"credential_id"`
	Algorithm            string    `json:"algorithm"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// With creates a new PasswordUpdated event.
func (e PasswordUpdated) With(passwordCredentialID, credentialID, algorithm string, updatedAt time.Time) PasswordUpdated {
	return PasswordUpdated{
		PasswordCredentialID: passwordCredentialID,
		CredentialID:         credentialID,
		Algorithm:            algorithm,
		UpdatedAt:            updatedAt,
	}
}

// EventType returns the event type name.
func (e PasswordUpdated) EventType() string {
	return EventTypePasswordUpdated
}
