package entities

import (
	"context"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// PasswordCredential is an event-sourced aggregate that stores the hashed
// secret for a Credential of provider="password". It is linked 1:1 to a
// Credential row by CredentialID; the hash itself is persisted on this
// aggregate's row, never on the parent Credential and never on any event
// payload.
type PasswordCredential struct {
	*ddd.BaseEntity
	credentialID   string
	algorithm      string
	hash           string
	createdAt      time.Time
	updatedAt      time.Time
	lastVerifiedAt time.Time
}

// With initializes a new PasswordCredential with the given parameters.
// The hash must already be computed by the caller.
func (p *PasswordCredential) With(id, credentialID, algorithm, hash string) (*PasswordCredential, error) {
	if id == "" {
		return nil, fmt.Errorf("password credential ID cannot be empty")
	}
	if credentialID == "" {
		return nil, fmt.Errorf("credential ID cannot be empty")
	}
	if algorithm == "" {
		return nil, fmt.Errorf("algorithm cannot be empty")
	}
	if hash == "" {
		return nil, fmt.Errorf("hash cannot be empty")
	}

	p.BaseEntity = ddd.NewBaseEntity(id)
	p.credentialID = credentialID
	p.algorithm = algorithm
	p.hash = hash
	p.createdAt = time.Now()
	p.updatedAt = p.createdAt

	event := new(PasswordCredentialCreated).With(id, credentialID, algorithm, p.createdAt)
	if err := p.RecordEvent(event, event.EventType()); err != nil {
		return nil, fmt.Errorf("failed to record PasswordCredential.Created event: %w", err)
	}
	return p, nil
}

// CredentialID returns the ID of the parent Credential aggregate.
func (p *PasswordCredential) CredentialID() string { return p.credentialID }

// Algorithm returns the hashing algorithm identifier (e.g. "bcrypt").
func (p *PasswordCredential) Algorithm() string { return p.algorithm }

// Hash returns the stored hash. Callers should never log or expose this
// value; it exists only so persistence and verification helpers can read
// the row contents.
func (p *PasswordCredential) Hash() string { return p.hash }

// CreatedAt returns when the password credential was created.
func (p *PasswordCredential) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt returns when the password was last rotated (or createdAt if
// never updated).
func (p *PasswordCredential) UpdatedAt() time.Time { return p.updatedAt }

// LastVerifiedAt returns the last successful verification timestamp.
// Zero if the credential has never been verified.
func (p *PasswordCredential) LastVerifiedAt() time.Time { return p.lastVerifiedAt }

// Restore restores a PasswordCredential from database values without
// recording events.
func (p *PasswordCredential) Restore(id, credentialID, algorithm, hash string, createdAt, updatedAt, lastVerifiedAt time.Time) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	if credentialID == "" {
		return fmt.Errorf("credential ID cannot be empty")
	}
	p.BaseEntity = ddd.NewBaseEntity(id)
	p.credentialID = credentialID
	p.algorithm = algorithm
	p.hash = hash
	p.createdAt = createdAt
	p.updatedAt = updatedAt
	p.lastVerifiedAt = lastVerifiedAt
	return nil
}

// Update rotates the stored hash and emits a PasswordUpdated event.
func (p *PasswordCredential) Update(algorithm, hash string) error {
	if algorithm == "" {
		return fmt.Errorf("algorithm cannot be empty")
	}
	if hash == "" {
		return fmt.Errorf("hash cannot be empty")
	}
	p.algorithm = algorithm
	p.hash = hash
	p.updatedAt = time.Now()

	event := new(PasswordUpdated).With(p.GetID(), p.credentialID, algorithm, p.updatedAt)
	return p.RecordEvent(event, event.EventType())
}

// MarkVerified records a successful verification. This is a projection-only
// state mutation and does not emit an event — bumping last_verified_at on
// every login would amplify event volume without proportional value.
func (p *PasswordCredential) MarkVerified() {
	p.lastVerifiedAt = time.Now()
}

// ApplyEvent applies a domain event to reconstruct the aggregate state.
func (p *PasswordCredential) ApplyEvent(ctx context.Context, envelope domain.EventEnvelope[any]) error {
	if err := p.BaseEntity.ApplyEvent(ctx, envelope); err != nil {
		return fmt.Errorf("base entity apply event failed: %w", err)
	}

	switch payload := envelope.Payload.(type) {
	case PasswordCredentialCreated:
		p.credentialID = payload.CredentialID
		p.algorithm = payload.Algorithm
		p.createdAt = payload.CreatedAt
		p.updatedAt = payload.CreatedAt
	case PasswordUpdated:
		p.algorithm = payload.Algorithm
		p.updatedAt = payload.UpdatedAt
	default:
		return fmt.Errorf("unknown event type: %T", envelope.Payload)
	}
	return nil
}
