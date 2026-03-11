package jwt

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/segmentio/ksuid"
)

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

// IssueToken creates a signed JWT for the given agent and accounts.
func (s *RSAJWTService) IssueToken(ctx context.Context, agent *entities.Agent, accounts []*entities.Account, activeAccountID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.privateKey == nil {
		return "", application.ErrNoSigningKey
	}
	if agent == nil {
		return "", fmt.Errorf("authentication: agent must not be nil")
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
