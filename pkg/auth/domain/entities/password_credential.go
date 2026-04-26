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
	credentialID string
	algorithm    string
	hash         string
	// salt is a plaintext suffix appended to the user-supplied password
	// before verification. Non-empty only for legacy hashes imported from
	// systems that applied an extra application-layer salt suffix on top
	// of bcrypt (i.e. bcrypted plaintext+salt rather than plaintext
	// alone). New pericarp-issued credentials always carry an empty salt;
	// rotating a password via Update() clears it.
	salt           string
	createdAt      time.Time
	updatedAt      time.Time
	lastVerifiedAt time.Time
}

// With initializes a new PasswordCredential with the given parameters.
// The hash must already be computed by the caller. New credentials carry
// no salt suffix — bcrypt's own per-hash salt is sufficient. Use WithSalt
// to import a legacy hash whose plaintext was suffixed before hashing.
func (p *PasswordCredential) With(id, credentialID, algorithm, hash string) (*PasswordCredential, error) {
	return p.WithSalt(id, credentialID, algorithm, hash, "")
}

// MaxSaltLength is the upper bound on the legacy salt suffix. Mirrors
// the GORM column width on PasswordCredentialModel — keeps construction
// and projection in lockstep so a salt that fits in the aggregate is
// guaranteed to fit in the row.
const MaxSaltLength = 64

// WithSalt initializes a new PasswordCredential with an additional
// plaintext salt suffix. The salt is appended to the user-supplied
// plaintext before bcrypt comparison; pass an empty salt to behave
// identically to With. Used for importing legacy hashes whose plaintext
// was suffixed with a per-credential value before bcrypt hashing.
func (p *PasswordCredential) WithSalt(id, credentialID, algorithm, hash, salt string) (*PasswordCredential, error) {
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
	if len(salt) > MaxSaltLength {
		return nil, fmt.Errorf("salt suffix too long: %d > %d", len(salt), MaxSaltLength)
	}
	// Salt-suffix verification is a bcrypt-only concept here. Reject
	// non-bcrypt + non-empty salt so a future Argon2 import (or similar)
	// does not silently persist a salt that verifyPassword would never
	// consume — a record that always fails to verify is worse than a
	// loud import failure.
	if salt != "" && algorithm != PasswordAlgorithmBcrypt {
		return nil, fmt.Errorf("salt suffix is only supported for %q, got %q", PasswordAlgorithmBcrypt, algorithm)
	}

	p.BaseEntity = ddd.NewBaseEntity(id)
	p.credentialID = credentialID
	p.algorithm = algorithm
	p.hash = hash
	p.salt = salt
	p.createdAt = time.Now()
	p.updatedAt = p.createdAt

	event := new(PasswordCredentialCreated).With(id, credentialID, algorithm, p.createdAt)
	if err := p.RecordEvent(event, event.EventType()); err != nil {
		return nil, fmt.Errorf("failed to record PasswordCredential.Created event: %w", err)
	}
	return p, nil
}

// String redacts the Hash and Salt so the aggregate is safe to log via
// %v / %+v. Callers needing the raw values must use Hash() / Salt()
// explicitly. Salt is redacted because, paired with the hash, it is
// exactly what an offline attacker needs to brute-force the plaintext
// — keep the two values out of the same log line.
func (p *PasswordCredential) String() string {
	if p == nil {
		return "<nil>"
	}
	saltState := "[EMPTY]"
	if p.salt != "" {
		saltState = "[REDACTED]"
	}
	return fmt.Sprintf(
		"PasswordCredential{ID:%s CredentialID:%s Algorithm:%s Hash:[REDACTED] Salt:%s}",
		p.GetID(), p.credentialID, p.algorithm, saltState,
	)
}

// GoString mirrors String so %#v also redacts the hash.
func (p *PasswordCredential) GoString() string { return p.String() }

// CredentialID returns the ID of the parent Credential aggregate.
func (p *PasswordCredential) CredentialID() string { return p.credentialID }

// Algorithm returns the hashing algorithm identifier (e.g. "bcrypt").
func (p *PasswordCredential) Algorithm() string { return p.algorithm }

// Hash returns the stored hash. Callers should never log or expose this
// value; it exists only so persistence and verification helpers can read
// the row contents.
func (p *PasswordCredential) Hash() string { return p.hash }

// Salt returns the plaintext salt suffix appended to the user-supplied
// plaintext before verification. Empty for credentials created through
// pericarp's own RegisterPassword flow; non-empty only for legacy
// imports.
func (p *PasswordCredential) Salt() string { return p.salt }

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
func (p *PasswordCredential) Restore(id, credentialID, algorithm, hash, salt string, createdAt, updatedAt, lastVerifiedAt time.Time) error {
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
	p.salt = salt
	p.createdAt = createdAt
	p.updatedAt = updatedAt
	p.lastVerifiedAt = lastVerifiedAt
	return nil
}

// Update rotates the stored hash and emits a PasswordUpdated event.
// Any legacy salt suffix is cleared as part of rotation: pericarp issues
// the new hash directly via bcrypt.GenerateFromPassword(plaintext), so a
// rotated credential is always on the modern, suffix-free scheme.
func (p *PasswordCredential) Update(algorithm, hash string) error {
	if algorithm == "" {
		return fmt.Errorf("algorithm cannot be empty")
	}
	if hash == "" {
		return fmt.Errorf("hash cannot be empty")
	}
	p.algorithm = algorithm
	p.hash = hash
	p.salt = ""
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
// Note: salt is intentionally not carried in event payloads (same
// convention as hash), so an aggregate hydrated purely from events will
// have salt="" regardless of whether the credential was originally
// imported with a non-empty salt. The projection (via Restore) is the
// canonical hydration source for verification — event replay alone is
// safe only for audit and dispatch, never as a precursor to
// VerifyPassword.
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
