package application

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// InviteClaims contains the JWT claims for an invite token.
type InviteClaims struct {
	jwt.RegisteredClaims
	InviteID string `json:"invite_id"`
}

// InviteTokenService defines the interface for issuing and validating invite tokens.
// This is separate from JWTService to avoid breaking existing implementors.
type InviteTokenService interface {
	// IssueInviteToken creates a signed JWT for the given invite.
	IssueInviteToken(ctx context.Context, inviteID string, expiry time.Duration) (string, error)

	// ValidateInviteToken parses and validates an invite token string, returning the claims.
	ValidateInviteToken(ctx context.Context, tokenString string) (*InviteClaims, error)
}
