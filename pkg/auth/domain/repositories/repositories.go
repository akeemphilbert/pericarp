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
	// Save persists the Agent aggregate state.
	Save(ctx context.Context, agent *entities.Agent) error

	// FindByID retrieves an Agent aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.Agent, error)

	// FindAll retrieves Agent aggregates with cursor-based pagination.
	FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Agent], error)
}

// PolicyRepository defines the interface for Policy aggregate persistence.
type PolicyRepository interface {
	// Save persists the Policy aggregate state.
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
	// Save persists the Role aggregate state.
	Save(ctx context.Context, role *entities.Role) error

	// FindByID retrieves a Role aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.Role, error)

	// FindAll retrieves Role aggregates with cursor-based pagination.
	FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Role], error)
}

// AccountRepository defines the interface for Account aggregate persistence.
type AccountRepository interface {
	// Save persists the Account aggregate state.
	Save(ctx context.Context, account *entities.Account) error

	// FindByID retrieves an Account aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.Account, error)

	// FindByMember retrieves all accounts that the given agent is a member of.
	FindByMember(ctx context.Context, agentID string) ([]*entities.Account, error)

	// FindPersonalByMember retrieves the personal account for the given agent.
	// Returns (nil, nil) if the agent has no personal account.
	FindPersonalByMember(ctx context.Context, agentID string) (*entities.Account, error)

	// FindMemberRole retrieves the role of a member in an account.
	// Returns ("", nil) if the agent is not a member of the account.
	FindMemberRole(ctx context.Context, accountID, agentID string) (string, error)

	// SaveMember persists a membership between an account and an agent with a role.
	SaveMember(ctx context.Context, accountID, agentID, roleID string) error

	// FindAll retrieves Account aggregates with cursor-based pagination.
	FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Account], error)
}

// CredentialRepository defines the interface for Credential aggregate persistence.
type CredentialRepository interface {
	// Save persists the Credential aggregate state.
	Save(ctx context.Context, credential *entities.Credential) error

	// FindByID retrieves a Credential aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.Credential, error)

	// FindByProvider retrieves a Credential by provider and provider user ID.
	FindByProvider(ctx context.Context, provider, providerUserID string) (*entities.Credential, error)

	// FindByEmail retrieves all credentials associated with the given email address.
	FindByEmail(ctx context.Context, email string) ([]*entities.Credential, error)

	// FindByAgent retrieves all credentials for the given agent.
	FindByAgent(ctx context.Context, agentID string) ([]*entities.Credential, error)

	// FindAll retrieves Credential aggregates with cursor-based pagination.
	FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Credential], error)
}

// PasswordCredentialRepository defines the interface for PasswordCredential
// aggregate persistence. PasswordCredentials are linked 1:1 to a Credential
// of provider="password" by CredentialID.
type PasswordCredentialRepository interface {
	// Save persists the PasswordCredential aggregate state.
	Save(ctx context.Context, credential *entities.PasswordCredential) error

	// FindByID retrieves a PasswordCredential by its ID.
	// Returns (nil, nil) if not found.
	FindByID(ctx context.Context, id string) (*entities.PasswordCredential, error)

	// FindByCredentialID retrieves the PasswordCredential linked to the
	// given Credential. Returns (nil, nil) if not found.
	FindByCredentialID(ctx context.Context, credentialID string) (*entities.PasswordCredential, error)

	// Delete removes the PasswordCredential row linked to the given
	// Credential. A no-op (returns nil) when no row exists.
	Delete(ctx context.Context, credentialID string) error
}

// InviteRepository defines the interface for Invite aggregate persistence.
type InviteRepository interface {
	// Save persists the Invite aggregate state.
	Save(ctx context.Context, invite *entities.Invite) error

	// FindByID retrieves an Invite aggregate by its ID.
	FindByID(ctx context.Context, id string) (*entities.Invite, error)

	// FindByEmail retrieves all invites for the given email address.
	FindByEmail(ctx context.Context, email string) ([]*entities.Invite, error)

	// FindByAccount retrieves all invites for the given account.
	FindByAccount(ctx context.Context, accountID string) ([]*entities.Invite, error)

	// FindPendingByEmail retrieves all pending invites for the given email address.
	FindPendingByEmail(ctx context.Context, email string) ([]*entities.Invite, error)
}

// AuthSessionRepository defines the interface for AuthSession aggregate persistence.
type AuthSessionRepository interface {
	// Save persists the AuthSession aggregate state.
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
