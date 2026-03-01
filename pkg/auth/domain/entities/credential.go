package entities

import (
	"context"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// Credential represents a link between an external identity provider account and an Agent.
// Credentials use Schema.org vocabulary for identity-related predicates.
type Credential struct {
	*ddd.BaseEntity
	agentID        string
	provider       string
	providerUserID string
	email          string
	displayName    string
	active         bool
	createdAt      time.Time
	lastUsedAt     time.Time
}

// With initializes a new Credential with the given parameters.
func (c *Credential) With(id, agentID, provider, providerUserID, email, displayName string) (*Credential, error) {
	if id == "" {
		return nil, fmt.Errorf("credential ID cannot be empty")
	}
	if agentID == "" {
		return nil, fmt.Errorf("agent ID cannot be empty")
	}
	if provider == "" {
		return nil, fmt.Errorf("provider cannot be empty")
	}
	if providerUserID == "" {
		return nil, fmt.Errorf("provider user ID cannot be empty")
	}

	c.BaseEntity = ddd.NewBaseEntity(id)
	c.agentID = agentID
	c.provider = provider
	c.providerUserID = providerUserID
	c.email = email
	c.displayName = displayName
	c.active = true
	c.createdAt = time.Now()

	event := new(CredentialCreated).With(agentID, id, provider, providerUserID, email, displayName)
	if err := c.BaseEntity.RecordEvent(event, event.EventType()); err != nil {
		return nil, fmt.Errorf("failed to record Credential.Created event: %w", err)
	}

	return c, nil
}

// AgentID returns the ID of the agent this credential belongs to.
func (c *Credential) AgentID() string {
	return c.agentID
}

// Provider returns the identity provider name (e.g., "google", "github").
func (c *Credential) Provider() string {
	return c.provider
}

// ProviderUserID returns the user's ID within the identity provider.
func (c *Credential) ProviderUserID() string {
	return c.providerUserID
}

// Email returns the email associated with this credential.
func (c *Credential) Email() string {
	return c.email
}

// DisplayName returns the display name from the identity provider.
func (c *Credential) DisplayName() string {
	return c.displayName
}

// Active returns whether the credential is currently active.
func (c *Credential) Active() bool {
	return c.active
}

// CreatedAt returns when the credential was created.
func (c *Credential) CreatedAt() time.Time {
	return c.createdAt
}

// LastUsedAt returns when the credential was last used for authentication.
func (c *Credential) LastUsedAt() time.Time {
	return c.lastUsedAt
}

// Restore restores a Credential from database values without recording events.
func (c *Credential) Restore(id, agentID, provider, providerUserID, email, displayName string, active bool, createdAt, lastUsedAt time.Time) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	if agentID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}
	c.BaseEntity = ddd.NewBaseEntity(id)
	c.agentID = agentID
	c.provider = provider
	c.providerUserID = providerUserID
	c.email = email
	c.displayName = displayName
	c.active = active
	c.createdAt = createdAt
	c.lastUsedAt = lastUsedAt
	return nil
}

// MarkUsed records that this credential was used for authentication.
func (c *Credential) MarkUsed() error {
	c.lastUsedAt = time.Now()
	event := new(CredentialUsed).With()
	return c.BaseEntity.RecordEvent(event, event.EventType())
}

// Deactivate marks the credential as inactive.
func (c *Credential) Deactivate() error {
	if !c.active {
		return nil
	}
	c.active = false
	event := new(CredentialDeactivated).With()
	return c.BaseEntity.RecordEvent(event, event.EventType())
}

// Reactivate marks the credential as active.
func (c *Credential) Reactivate() error {
	if c.active {
		return nil
	}
	c.active = true
	event := new(CredentialReactivated).With()
	return c.BaseEntity.RecordEvent(event, event.EventType())
}

// ApplyEvent applies a domain event to reconstruct the aggregate state.
func (c *Credential) ApplyEvent(ctx context.Context, envelope domain.EventEnvelope[any]) error {
	if err := c.BaseEntity.ApplyEvent(ctx, envelope); err != nil {
		return fmt.Errorf("base entity apply event failed: %w", err)
	}

	switch payload := envelope.Payload.(type) {
	case CredentialCreated:
		c.agentID = payload.Subject
		c.provider = payload.Provider
		c.providerUserID = payload.ProviderUserID
		c.email = payload.Email
		c.displayName = payload.DisplayName
		c.active = true
		c.createdAt = payload.Timestamp
	case CredentialUsed:
		c.lastUsedAt = payload.Timestamp
	case CredentialDeactivated:
		c.active = false
	case CredentialReactivated:
		c.active = true
	default:
		return fmt.Errorf("unknown event type: %T", envelope.Payload)
	}
	return nil
}
