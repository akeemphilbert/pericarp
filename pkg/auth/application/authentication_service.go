package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	"github.com/segmentio/ksuid"
)

// Sentinel errors for the authentication domain.
var (
	ErrInvalidProvider    = errors.New("authentication: invalid provider")
	ErrInvalidState       = errors.New("authentication: invalid state parameter")
	ErrCodeExchangeFailed = errors.New("authentication: code exchange failed")
	ErrSessionNotFound    = errors.New("authentication: session not found")
	ErrSessionExpired     = errors.New("authentication: session expired")
	ErrSessionRevoked     = errors.New("authentication: session revoked")
	ErrTokenRefreshFailed = errors.New("authentication: token refresh failed")
	ErrCredentialNotFound = errors.New("authentication: credential not found")
)

// AuthRequest represents the result of initiating an OAuth authorization flow.
type AuthRequest struct {
	AuthURL      string
	State        string
	CodeVerifier string
	Nonce        string
	Provider     string
}

// AuthResult represents the result of a successful token exchange.
type AuthResult struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	TokenType    string
	ExpiresIn    int
	UserInfo     UserInfo
}

// UserInfo represents normalized user information from any identity provider.
type UserInfo struct {
	ProviderUserID string
	Email          string
	DisplayName    string
	AvatarURL      string
	Provider       string
}

// SessionInfo represents validated session information returned to consumers.
type SessionInfo struct {
	SessionID   string
	AgentID     string
	AccountID   string
	Permissions []Permission
	ExpiresAt   time.Time
}

// OAuthProvider defines a provider-agnostic interface for OAuth 2.0 / OpenID Connect operations.
type OAuthProvider interface {
	// Name returns the provider identifier (e.g., "google", "github").
	Name() string

	// AuthCodeURL generates the authorization URL with PKCE parameters.
	AuthCodeURL(state string, codeChallenge string, nonce string, redirectURI string) string

	// Exchange exchanges an authorization code for tokens.
	Exchange(ctx context.Context, code string, codeVerifier string, redirectURI string) (*AuthResult, error)

	// RefreshToken refreshes an access token using a refresh token.
	RefreshToken(ctx context.Context, refreshToken string) (*AuthResult, error)

	// RevokeToken revokes a token at the provider.
	RevokeToken(ctx context.Context, token string) error

	// ValidateIDToken validates the ID token and extracts user claims.
	ValidateIDToken(ctx context.Context, idToken string, nonce string) (*UserInfo, error)
}

// TokenStore defines the interface for encrypted server-side token storage.
type TokenStore interface {
	// StoreTokens stores OAuth tokens for a credential.
	StoreTokens(ctx context.Context, credentialID string, accessToken, refreshToken, idToken string, expiresAt time.Time) error

	// GetTokens retrieves stored OAuth tokens for a credential.
	GetTokens(ctx context.Context, credentialID string) (accessToken, refreshToken string, expiresAt time.Time, err error)

	// DeleteTokens removes all stored tokens for a credential.
	DeleteTokens(ctx context.Context, credentialID string) error

	// NeedsRefresh checks if the stored access token needs refreshing.
	NeedsRefresh(ctx context.Context, credentialID string) (bool, error)
}

// AuthenticationService defines the interface for authentication operations.
type AuthenticationService interface {
	// InitiateAuthFlow generates PKCE parameters and returns the authorization URL.
	InitiateAuthFlow(ctx context.Context, provider string, redirectURI string) (*AuthRequest, error)

	// ExchangeCode exchanges an authorization code for tokens (server-to-server).
	ExchangeCode(ctx context.Context, code string, codeVerifier string, provider string, redirectURI string) (*AuthResult, error)

	// ValidateState verifies the OAuth state parameter matches the stored state.
	ValidateState(ctx context.Context, receivedState string, storedState string) error

	// FindOrCreateAgent looks up an agent by provider credentials, creates if not found.
	FindOrCreateAgent(ctx context.Context, userInfo UserInfo) (*entities.Agent, *entities.Credential, error)

	// CreateSession creates an authenticated session for an agent.
	CreateSession(ctx context.Context, agentID string, credentialID string, ipAddress string, userAgent string, duration time.Duration) (*entities.AuthSession, error)

	// ValidateSession validates and returns session info.
	ValidateSession(ctx context.Context, sessionID string) (*SessionInfo, error)

	// RefreshTokens refreshes OAuth tokens for a credential.
	RefreshTokens(ctx context.Context, credentialID string) (*AuthResult, error)

	// RevokeSession revokes an active session.
	RevokeSession(ctx context.Context, sessionID string) error

	// RevokeAllSessions revokes all sessions for an agent.
	RevokeAllSessions(ctx context.Context, agentID string) error
}

// OAuthProviderRegistry maps provider names to their OAuthProvider implementations.
type OAuthProviderRegistry map[string]OAuthProvider

// DefaultAuthenticationService implements AuthenticationService using OAuth providers
// and the domain's event-sourced aggregates.
type DefaultAuthenticationService struct {
	providers     OAuthProviderRegistry
	agents        repositories.AgentRepository
	credentials   repositories.CredentialRepository
	sessions      repositories.AuthSessionRepository
	tokens        TokenStore
	authorization AuthorizationChecker
}

// NewDefaultAuthenticationService creates a new DefaultAuthenticationService.
func NewDefaultAuthenticationService(
	providers OAuthProviderRegistry,
	agents repositories.AgentRepository,
	credentials repositories.CredentialRepository,
	sessions repositories.AuthSessionRepository,
	tokens TokenStore,
	authorization AuthorizationChecker,
) *DefaultAuthenticationService {
	return &DefaultAuthenticationService{
		providers:     providers,
		agents:        agents,
		credentials:   credentials,
		sessions:      sessions,
		tokens:        tokens,
		authorization: authorization,
	}
}

// InitiateAuthFlow generates PKCE parameters and returns the authorization URL.
func (s *DefaultAuthenticationService) InitiateAuthFlow(ctx context.Context, provider string, redirectURI string) (*AuthRequest, error) {
	p, ok := s.providers[provider]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, provider)
	}

	codeVerifier, err := GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}

	state, err := GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	nonce, err := GenerateNonce()
	if err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	codeChallenge := GenerateCodeChallenge(codeVerifier)
	authURL := p.AuthCodeURL(state, codeChallenge, nonce, redirectURI)

	return &AuthRequest{
		AuthURL:      authURL,
		State:        state,
		CodeVerifier: codeVerifier,
		Nonce:        nonce,
		Provider:     provider,
	}, nil
}

// ExchangeCode exchanges an authorization code for tokens.
func (s *DefaultAuthenticationService) ExchangeCode(ctx context.Context, code string, codeVerifier string, provider string, redirectURI string) (*AuthResult, error) {
	p, ok := s.providers[provider]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, provider)
	}

	result, err := p.Exchange(ctx, code, codeVerifier, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCodeExchangeFailed, err)
	}

	return result, nil
}

// ValidateState verifies the OAuth state parameter matches the stored state.
// Uses constant-time comparison to prevent timing attacks.
func (s *DefaultAuthenticationService) ValidateState(_ context.Context, receivedState string, storedState string) error {
	if subtle.ConstantTimeCompare([]byte(receivedState), []byte(storedState)) != 1 {
		return ErrInvalidState
	}
	return nil
}

// FindOrCreateAgent looks up an agent by provider credentials, creates if not found.
func (s *DefaultAuthenticationService) FindOrCreateAgent(ctx context.Context, userInfo UserInfo) (*entities.Agent, *entities.Credential, error) {
	// Look up existing credential by provider
	credential, err := s.credentials.FindByProvider(ctx, userInfo.Provider, userInfo.ProviderUserID)
	if err == nil && credential != nil {
		// Credential exists, fetch the agent
		agent, agentErr := s.agents.FindByID(ctx, credential.AgentID())
		if agentErr != nil {
			return nil, nil, fmt.Errorf("failed to find agent for credential: %w", agentErr)
		}

		// Mark credential as used
		if markErr := credential.MarkUsed(); markErr != nil {
			return nil, nil, fmt.Errorf("failed to mark credential as used: %w", markErr)
		}
		if saveErr := s.credentials.Save(ctx, credential); saveErr != nil {
			return nil, nil, fmt.Errorf("failed to save credential: %w", saveErr)
		}

		return agent, credential, nil
	}

	// Create new agent
	agentID := ksuid.New().String()
	agent := new(entities.Agent)
	agent, err = agent.With(agentID, userInfo.DisplayName, entities.AgentTypePerson)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create agent: %w", err)
	}
	if err = s.agents.Save(ctx, agent); err != nil {
		return nil, nil, fmt.Errorf("failed to save agent: %w", err)
	}

	// Create credential linked to the agent
	credentialID := ksuid.New().String()
	credential = new(entities.Credential)
	credential, err = credential.With(credentialID, agentID, userInfo.Provider, userInfo.ProviderUserID, userInfo.Email, userInfo.DisplayName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create credential: %w", err)
	}
	if err = s.credentials.Save(ctx, credential); err != nil {
		return nil, nil, fmt.Errorf("failed to save credential: %w", err)
	}

	return agent, credential, nil
}

// CreateSession creates an authenticated session for an agent.
func (s *DefaultAuthenticationService) CreateSession(ctx context.Context, agentID string, credentialID string, ipAddress string, userAgent string, duration time.Duration) (*entities.AuthSession, error) {
	sessionID := ksuid.New().String()
	expiresAt := time.Now().Add(duration)

	session := new(entities.AuthSession)
	session, err := session.With(sessionID, agentID, credentialID, ipAddress, userAgent, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	if err = s.sessions.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	return session, nil
}

// ValidateSession validates and returns session info.
func (s *DefaultAuthenticationService) ValidateSession(ctx context.Context, sessionID string) (*SessionInfo, error) {
	session, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSessionNotFound, err)
	}

	if !session.Active() {
		return nil, ErrSessionRevoked
	}

	if session.IsExpired() {
		return nil, ErrSessionExpired
	}

	// Touch the session to update last accessed time
	if touchErr := session.Touch(); touchErr != nil {
		return nil, fmt.Errorf("failed to touch session: %w", touchErr)
	}
	if saveErr := s.sessions.Save(ctx, session); saveErr != nil {
		return nil, fmt.Errorf("failed to save session: %w", saveErr)
	}

	// Resolve permissions via authorization checker
	var permissions []Permission
	if s.authorization != nil {
		permissions, err = s.authorization.GetPermissions(ctx, session.AgentID())
		if err != nil {
			return nil, fmt.Errorf("failed to get permissions: %w", err)
		}
	}

	return &SessionInfo{
		SessionID:   session.GetID(),
		AgentID:     session.AgentID(),
		AccountID:   session.AccountID(),
		Permissions: permissions,
		ExpiresAt:   session.ExpiresAt(),
	}, nil
}

// RefreshTokens refreshes OAuth tokens for a credential.
func (s *DefaultAuthenticationService) RefreshTokens(ctx context.Context, credentialID string) (*AuthResult, error) {
	credential, err := s.credentials.FindByID(ctx, credentialID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCredentialNotFound, err)
	}

	provider, ok := s.providers[credential.Provider()]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, credential.Provider())
	}

	_, refreshToken, _, err := s.tokens.GetTokens(ctx, credentialID)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored tokens: %w", err)
	}

	result, err := provider.RefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenRefreshFailed, err)
	}

	// Store the new tokens
	expiresAt := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	if storeErr := s.tokens.StoreTokens(ctx, credentialID, result.AccessToken, result.RefreshToken, result.IDToken, expiresAt); storeErr != nil {
		return nil, fmt.Errorf("failed to store refreshed tokens: %w", storeErr)
	}

	return result, nil
}

// RevokeSession revokes an active session.
func (s *DefaultAuthenticationService) RevokeSession(ctx context.Context, sessionID string) error {
	session, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSessionNotFound, err)
	}

	if revokeErr := session.Revoke(); revokeErr != nil {
		return fmt.Errorf("failed to revoke session: %w", revokeErr)
	}

	return s.sessions.Save(ctx, session)
}

// RevokeAllSessions revokes all sessions for an agent.
func (s *DefaultAuthenticationService) RevokeAllSessions(ctx context.Context, agentID string) error {
	return s.sessions.RevokeAllForAgent(ctx, agentID)
}
