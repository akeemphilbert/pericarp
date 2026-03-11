package application

import (
	"context"
	"errors"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/golang-jwt/jwt/v5"
)

// Sentinel errors for JWT operations.
var (
	ErrTokenInvalid  = errors.New("authentication: token is invalid")
	ErrTokenExpired  = errors.New("authentication: token has expired")
	ErrSigningFailed = errors.New("authentication: failed to sign token")
	ErrNoSigningKey  = errors.New("authentication: no signing key configured")
)

// PericarpClaims contains the JWT claims issued by the auth system.
// AgentID mirrors RegisteredClaims.Subject for convenient access without
// parsing the standard "sub" field.
type PericarpClaims struct {
	jwt.RegisteredClaims
	AgentID         string   `json:"agent_id"`
	AccountIDs      []string `json:"account_ids"`
	ActiveAccountID string   `json:"active_account_id,omitempty"`
}

// JWTService defines the interface for issuing and validating JWTs.
type JWTService interface {
	// IssueToken creates a signed JWT for the given agent and accounts.
	IssueToken(ctx context.Context, agent *entities.Agent, accounts []*entities.Account, activeAccountID string) (string, error)

	// ValidateToken parses and validates a JWT string, returning the claims.
	ValidateToken(ctx context.Context, tokenString string) (*PericarpClaims, error)
}
