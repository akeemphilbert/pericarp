package repositories

import (
	"context"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

// PaginatedResponse represents a paginated response with cursor-based pagination.
type PaginatedResponse[T any] struct {
	// Data contains the paginated items.
	Data []T

	// Cursor is the cursor for the next page. Empty string indicates no more pages.
	Cursor string

	// Limit is the number of items per page.
	Limit int

	// HasMore indicates whether there are more items available.
	HasMore bool
}

// AgentRepository defines the interface for Agent aggregate persistence.
type AgentRepository interface {
	// Save persists the Agent aggregate and its uncommitted events.
	Save(ctx context.Context, agent *entities.Agent) error

	// FindByID retrieves an Agent aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.Agent, error)

	// FindAll retrieves Agent aggregates with cursor-based pagination.
	FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Agent], error)
}

// PolicyRepository defines the interface for Policy aggregate persistence.
type PolicyRepository interface {
	// Save persists the Policy aggregate and its uncommitted events.
	Save(ctx context.Context, policy *entities.Policy) error

	// FindByID retrieves a Policy aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.Policy, error)

	// FindByAssignee retrieves all policies assigned to a given agent or role.
	FindByAssignee(ctx context.Context, assigneeID string) ([]*entities.Policy, error)

	// FindAll retrieves Policy aggregates with cursor-based pagination.
	FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Policy], error)
}

// RoleRepository defines the interface for Role aggregate persistence.
type RoleRepository interface {
	// Save persists the Role aggregate and its uncommitted events.
	Save(ctx context.Context, role *entities.Role) error

	// FindByID retrieves a Role aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.Role, error)

	// FindAll retrieves Role aggregates with cursor-based pagination.
	FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Role], error)
}

// AccountRepository defines the interface for Account aggregate persistence.
type AccountRepository interface {
	// Save persists the Account aggregate and its uncommitted events.
	Save(ctx context.Context, account *entities.Account) error

	// FindByID retrieves an Account aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.Account, error)

	// FindByMember retrieves all accounts that the given agent is a member of.
	FindByMember(ctx context.Context, agentID string) ([]*entities.Account, error)

	// FindAll retrieves Account aggregates with cursor-based pagination.
	FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Account], error)
}

// CredentialRepository defines the interface for Credential aggregate persistence.
type CredentialRepository interface {
	// Save persists the Credential aggregate and its uncommitted events.
	Save(ctx context.Context, credential *entities.Credential) error

	// FindByID retrieves a Credential aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.Credential, error)

	// FindByProvider retrieves a Credential by provider and provider user ID.
	FindByProvider(ctx context.Context, provider, providerUserID string) (*entities.Credential, error)

	// FindByAgent retrieves all credentials for the given agent.
	FindByAgent(ctx context.Context, agentID string) ([]*entities.Credential, error)

	// FindAll retrieves Credential aggregates with cursor-based pagination.
	FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Credential], error)
}

// AuthSessionRepository defines the interface for AuthSession aggregate persistence.
type AuthSessionRepository interface {
	// Save persists the AuthSession aggregate and its uncommitted events.
	Save(ctx context.Context, session *entities.AuthSession) error

	// FindByID retrieves an AuthSession aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.AuthSession, error)

	// FindByAgent retrieves all sessions for the given agent.
	FindByAgent(ctx context.Context, agentID string) ([]*entities.AuthSession, error)

	// FindActive retrieves all active (non-revoked, non-expired) sessions for the given agent.
	FindActive(ctx context.Context, agentID string) ([]*entities.AuthSession, error)

	// FindAll retrieves AuthSession aggregates with cursor-based pagination.
	FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.AuthSession], error)

	// RevokeAllForAgent revokes all active sessions for the given agent.
	RevokeAllForAgent(ctx context.Context, agentID string) error
}
