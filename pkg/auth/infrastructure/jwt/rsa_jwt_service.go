package jwt

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/segmentio/ksuid"
)

var _ application.TokenReissuer = (*RSAJWTService)(nil)

// RSAJWTService implements application.JWTService using RS256 signing.
// It can be constructed without keys; calls to IssueToken/ValidateToken will
// return application.ErrNoSigningKey until a key is provided via options.
type RSAJWTService struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	tokenTTL   time.Duration
	issuer     string
}

// NewRSAJWTService creates a new RSAJWTService with the given options.
func NewRSAJWTService(opts ...RSAJWTServiceOption) *RSAJWTService {
	s := &RSAJWTService{
		tokenTTL: defaultTokenTTL,
		issuer:   defaultIssuer,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// IssueToken implements [application.JWTService.IssueToken]. See the
// interface doc for the extras / reserved-name contract.
func (s *RSAJWTService) IssueToken(ctx context.Context, agent *entities.Agent, accounts []*entities.Account, activeAccountID string, subscription *auth.SubscriptionClaim, extras map[string]any) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.privateKey == nil {
		return "", application.ErrNoSigningKey
	}
	if agent == nil {
		return "", fmt.Errorf("authentication: agent must not be nil")
	}
	// Snapshot extras before validation + signing so a caller mutating
	// the map from another goroutine cannot panic our iteration or slip
	// a reserved key in between ValidateExtras and the marshal pass.
	extras = application.CloneExtras(extras)
	if err := application.ValidateExtras(extras); err != nil {
		return "", err
	}

	accountIDs := make([]string, len(accounts))
	for i, a := range accounts {
		accountIDs[i] = a.GetID()
	}

	now := time.Now()
	claims := application.PericarpClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   agent.GetID(),
			IssuedAt:  gojwt.NewNumericDate(now),
			ExpiresAt: gojwt.NewNumericDate(now.Add(s.tokenTTL)),
			ID:        ksuid.New().String(),
		},
		AgentID:         agent.GetID(),
		AccountIDs:      accountIDs,
		ActiveAccountID: activeAccountID,
		Subscription:    subscription,
		Extras:          extras,
	}

	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", fmt.Errorf("%w: %v", application.ErrSigningFailed, err)
	}

	return tokenString, nil
}

// ValidateToken parses and validates a JWT string, returning the claims.
func (s *RSAJWTService) ValidateToken(ctx context.Context, tokenString string) (*application.PericarpClaims, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.publicKey == nil {
		return nil, application.ErrNoSigningKey
	}

	claims := &application.PericarpClaims{}
	token, err := gojwt.ParseWithClaims(tokenString, claims, func(token *gojwt.Token) (any, error) {
		if _, ok := token.Method.(*gojwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.publicKey, nil
	})
	if err != nil {
		if errors.Is(err, gojwt.ErrTokenExpired) {
			return nil, application.ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", application.ErrTokenInvalid, err)
	}
	if !token.Valid {
		return nil, application.ErrTokenInvalid
	}

	return claims, nil
}

// IssueInviteToken creates a signed JWT for an invite.
func (s *RSAJWTService) IssueInviteToken(ctx context.Context, inviteID string, expiry time.Duration) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.privateKey == nil {
		return "", application.ErrNoSigningKey
	}
	if inviteID == "" {
		return "", fmt.Errorf("authentication: invite ID must not be empty")
	}

	now := time.Now()
	claims := application.InviteClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   inviteID,
			IssuedAt:  gojwt.NewNumericDate(now),
			ExpiresAt: gojwt.NewNumericDate(now.Add(expiry)),
			ID:        ksuid.New().String(),
		},
		InviteID: inviteID,
	}

	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", fmt.Errorf("%w: %v", application.ErrSigningFailed, err)
	}

	return tokenString, nil
}

// ValidateInviteToken parses and validates an invite token string, returning the claims.
func (s *RSAJWTService) ValidateInviteToken(ctx context.Context, tokenString string) (*application.InviteClaims, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.publicKey == nil {
		return nil, application.ErrNoSigningKey
	}

	claims := &application.InviteClaims{}
	token, err := gojwt.ParseWithClaims(tokenString, claims, func(token *gojwt.Token) (any, error) {
		if _, ok := token.Method.(*gojwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.publicKey, nil
	})
	if err != nil {
		if errors.Is(err, gojwt.ErrTokenExpired) {
			return nil, application.ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", application.ErrTokenInvalid, err)
	}
	if !token.Valid {
		return nil, application.ErrTokenInvalid
	}

	return claims, nil
}

// ReissueToken creates a new JWT with a different ActiveAccountID, copying
// AgentID, Subject, AccountIDs, Subscription, and Extras from the existing
// claims. Subscription and Extras are copied verbatim — account-switch
// reissuance prefers a stale-but-stable snapshot over a per-switch billing
// or enricher call. Fresh snapshots are only taken on the next
// IssueIdentityToken (e.g. when the JWT expires and the user re-authenticates).
//
// Extras are re-validated even though the source token was already
// validated when first parsed: the reserved-name set may have grown
// since the token was issued, an alternate JWTService implementation
// may not have enforced ValidateExtras, or the claims pointer may have
// been mutated in memory between ValidateToken and ReissueToken.
func (s *RSAJWTService) ReissueToken(ctx context.Context, claims *application.PericarpClaims, activeAccountID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.privateKey == nil {
		return "", application.ErrNoSigningKey
	}
	if claims == nil {
		return "", fmt.Errorf("authentication: claims must not be nil")
	}
	// Snapshot Extras up front so the validated map is the same map
	// that gets signed: a concurrent mutation cannot slip a reserved
	// key past ValidateExtras and into the re-signed token, and our
	// iteration cannot panic mid-flight if the caller mutates from
	// another goroutine.
	extras := application.CloneExtras(claims.Extras)
	if err := application.ValidateExtras(extras); err != nil {
		return "", err
	}

	now := time.Now()
	newClaims := application.PericarpClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   claims.Subject,
			IssuedAt:  gojwt.NewNumericDate(now),
			ExpiresAt: gojwt.NewNumericDate(now.Add(s.tokenTTL)),
			ID:        ksuid.New().String(),
		},
		AgentID:         claims.AgentID,
		AccountIDs:      claims.AccountIDs,
		ActiveAccountID: activeAccountID,
		Subscription:    claims.Subscription,
		Extras:          extras,
	}

	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, newClaims)
	tokenString, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", fmt.Errorf("%w: %v", application.ErrSigningFailed, err)
	}

	return tokenString, nil
}
