package application

import "context"

// TokenReissuer re-issues a JWT with a different active account without
// requiring entity lookups. Separate from JWTService to avoid breaking
// existing implementors.
type TokenReissuer interface {
	ReissueToken(ctx context.Context, claims *PericarpClaims, activeAccountID string) (string, error)
}
