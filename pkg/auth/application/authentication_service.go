package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	esApplication "github.com/akeemphilbert/pericarp/pkg/eventsourcing/application"
	esDomain "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/segmentio/ksuid"
)

// Sentinel errors for the authentication domain.
var (
	ErrInvalidProvider               = errors.New("authentication: invalid provider")
	ErrInvalidState                  = errors.New("authentication: invalid state parameter")
	ErrCodeExchangeFailed            = errors.New("authentication: code exchange failed")
	ErrSessionNotFound               = errors.New("authentication: session not found")
	ErrSessionExpired                = errors.New("authentication: session expired")
	ErrSessionRevoked                = errors.New("authentication: session revoked")
	ErrTokenRefreshFailed            = errors.New("authentication: token refresh failed")
	ErrCredentialNotFound            = errors.New("authentication: credential not found")
	ErrEmailAlreadyTaken             = errors.New("authentication: email already registered with a password")
	ErrPasswordSupportNotConfigured  = errors.New("authentication: password support not configured")
	ErrPasswordCredentialMissing     = errors.New("authentication: password credential not found for agent")
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

// TokenStore defines the interface for server-side token storage.
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
	// For new users, a personal Account is also created with the agent as owner.
	FindOrCreateAgent(ctx context.Context, userInfo UserInfo) (*entities.Agent, *entities.Credential, *entities.Account, error)

	// RegisterPassword creates a new Agent + personal Account + Credential
	// (provider="password") + PasswordCredential. Returns ErrEmailAlreadyTaken
	// when a password credential for the email already exists.
	RegisterPassword(ctx context.Context, email, displayName, plaintext string) (*entities.Agent, *entities.Credential, *entities.Account, error)

	// VerifyPassword authenticates an email + plaintext pair against a stored
	// PasswordCredential and returns the associated Agent, Credential and
	// (optional) personal Account on success. To prevent account enumeration,
	// both wrong-password and unknown-email cases return ErrInvalidPassword.
	VerifyPassword(ctx context.Context, email, plaintext string) (*entities.Agent, *entities.Credential, *entities.Account, error)

	// ImportPasswordCredential imports an already-hashed legacy bcrypt blob
	// against a caller-supplied agentID/accountID. Idempotent on
	// (provider="password", lower(email)). Used for bulk migration where
	// existing foreign keys must remain valid.
	ImportPasswordCredential(ctx context.Context, email, displayName, bcryptHash, agentID, accountID string) error

	// UpdatePassword rotates the stored password for the given agent.
	// Verifies oldPlaintext before applying the change.
	UpdatePassword(ctx context.Context, agentID, oldPlaintext, newPlaintext string) error

	// IssueIdentityToken issues a signed JWT for the given agent.
	// Returns ("", nil) if no JWTService is configured.
	IssueIdentityToken(ctx context.Context, agent *entities.Agent, activeAccountID string) (string, error)

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
// and domain aggregates.
type DefaultAuthenticationService struct {
	providers           OAuthProviderRegistry
	agents              repositories.AgentRepository
	credentials         repositories.CredentialRepository
	sessions            repositories.AuthSessionRepository
	accounts            repositories.AccountRepository
	passwordCredentials repositories.PasswordCredentialRepository
	eventStore          esDomain.EventStore
	tokens              TokenStore
	authorization       AuthorizationChecker
	logger              Logger
	jwtService          JWTService
	bcryptCost          int
}

// NewDefaultAuthenticationService creates a new DefaultAuthenticationService.
// Required dependencies are the provider registry and repositories. Optional
// dependencies (TokenStore, AuthorizationChecker, Logger, EventStore) can be
// configured via functional options; safe no-op defaults are used when not provided.
func NewDefaultAuthenticationService(
	providers OAuthProviderRegistry,
	agents repositories.AgentRepository,
	credentials repositories.CredentialRepository,
	sessions repositories.AuthSessionRepository,
	accounts repositories.AccountRepository,
	opts ...AuthServiceOption,
) *DefaultAuthenticationService {
	s := &DefaultAuthenticationService{
		providers:   providers,
		agents:      agents,
		credentials: credentials,
		sessions:    sessions,
		accounts:    accounts,
		tokens:      noOpTokenStore{},
		logger:      NoOpLogger{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Deprecated: NewDefaultAuthenticationServiceLegacy creates a DefaultAuthenticationService
// with a positional-parameter signature. Use NewDefaultAuthenticationService
// with functional options instead.
func NewDefaultAuthenticationServiceLegacy(
	providers OAuthProviderRegistry,
	agents repositories.AgentRepository,
	credentials repositories.CredentialRepository,
	sessions repositories.AuthSessionRepository,
	accounts repositories.AccountRepository,
	tokens TokenStore,
	authorization AuthorizationChecker,
) *DefaultAuthenticationService {
	return NewDefaultAuthenticationService(
		providers, agents, credentials, sessions, accounts,
		WithTokenStore(tokens),
		WithAuthorizationChecker(authorization),
	)
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
// For new users, a personal Account is also created with the agent as owner.
// For existing users, the personal Account is returned if one exists (may be nil).
func (s *DefaultAuthenticationService) FindOrCreateAgent(ctx context.Context, userInfo UserInfo) (*entities.Agent, *entities.Credential, *entities.Account, error) {
	// Look up existing credential by provider
	credential, err := s.credentials.FindByProvider(ctx, userInfo.Provider, userInfo.ProviderUserID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to look up credential for provider %s: %w", userInfo.Provider, err)
	}
	if credential != nil {
		// Credential exists, fetch the agent
		agent, agentErr := s.agents.FindByID(ctx, credential.AgentID())
		if agentErr != nil {
			return nil, nil, nil, fmt.Errorf("failed to find agent for credential: %w", agentErr)
		}
		if agent == nil {
			return nil, nil, nil, fmt.Errorf("failed to find agent for credential: agent %s not found", credential.AgentID())
		}

		// Look up personal account
		var account *entities.Account
		if s.accounts != nil {
			account, err = s.accounts.FindPersonalByMember(ctx, agent.GetID())
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to find personal account: %w", err)
			}
		}

		// Mark credential as used
		if markErr := credential.MarkUsed(); markErr != nil {
			return nil, nil, nil, fmt.Errorf("failed to mark credential as used: %w", markErr)
		}
		if saveErr := s.credentials.Save(ctx, credential); saveErr != nil {
			return nil, nil, nil, fmt.Errorf("failed to save credential: %w", saveErr)
		}

		return agent, credential, account, nil
	}

	// No existing credential found -- create new agent, personal account, and credential

	agentID := ksuid.New().String()
	agent := new(entities.Agent)
	agent, err = agent.With(agentID, userInfo.DisplayName, entities.AgentTypePerson)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create agent: %w", err)
	}

	accountID := ksuid.New().String()
	account := new(entities.Account)
	account, err = account.With(accountID, userInfo.DisplayName+"'s Account", entities.AccountTypePersonal)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create account: %w", err)
	}
	if err = account.AddMember(agentID, entities.RoleOwner); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to add member to account: %w", err)
	}

	credentialID := ksuid.New().String()
	credential = new(entities.Credential)
	credential, err = credential.With(credentialID, agentID, userInfo.Provider, userInfo.ProviderUserID, userInfo.Email, userInfo.DisplayName)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create credential: %w", err)
	}

	// Commit events atomically to event store via UnitOfWork
	if s.eventStore != nil {
		uow := esApplication.NewSimpleUnitOfWork(s.eventStore, nil)
		if err = uow.Track(agent, account, credential); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to track entities: %w", err)
		}
		if err = uow.Commit(ctx); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to commit unit of work: %w", err)
		}
	}

	// Save projections to read-model repos
	if err = s.agents.Save(ctx, agent); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to save agent: %w", err)
	}
	if s.accounts != nil {
		if err = s.accounts.Save(ctx, account); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to save account: %w", err)
		}
		if err = s.accounts.SaveMember(ctx, accountID, agentID, entities.RoleOwner); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to save account member: %w", err)
		}
	}
	if err = s.credentials.Save(ctx, credential); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to save credential: %w", err)
	}

	return agent, credential, account, nil
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
	if session == nil {
		return nil, ErrSessionNotFound
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
	if credential == nil {
		return nil, ErrCredentialNotFound
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
	if session == nil {
		return ErrSessionNotFound
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

// IssueIdentityToken issues a signed JWT for the given agent.
// Returns ("", nil) if no JWTService is configured.
func (s *DefaultAuthenticationService) IssueIdentityToken(ctx context.Context, agent *entities.Agent, activeAccountID string) (string, error) {
	if s.jwtService == nil {
		return "", nil
	}
	if agent == nil {
		return "", fmt.Errorf("authentication: agent must not be nil for token issuance")
	}
	var accounts []*entities.Account
	if s.accounts != nil {
		var err error
		accounts, err = s.accounts.FindByMember(ctx, agent.GetID())
		if err != nil {
			return "", fmt.Errorf("authentication: failed to fetch accounts for token: %w", err)
		}
	}
	return s.jwtService.IssueToken(ctx, agent, accounts, activeAccountID)
}

// normalizeEmail returns the canonical form of an email used as the
// password credential's provider_user_id.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// RegisterPassword creates a new Agent + personal Account + Credential
// (provider="password") + PasswordCredential.
func (s *DefaultAuthenticationService) RegisterPassword(ctx context.Context, email, displayName, plaintext string) (*entities.Agent, *entities.Credential, *entities.Account, error) {
	if s.passwordCredentials == nil {
		return nil, nil, nil, ErrPasswordSupportNotConfigured
	}
	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" {
		return nil, nil, nil, fmt.Errorf("authentication: email must not be empty")
	}
	if plaintext == "" {
		return nil, nil, nil, fmt.Errorf("authentication: password must not be empty")
	}

	existing, err := s.credentials.FindByProvider(ctx, entities.ProviderPassword, normalizedEmail)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: lookup existing credential: %w", err)
	}
	if existing != nil {
		return nil, nil, nil, ErrEmailAlreadyTaken
	}

	algorithm, hash, err := hashPassword(plaintext, s.bcryptCost)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: hash password: %w", err)
	}

	agentID := ksuid.New().String()
	agent, err := new(entities.Agent).With(agentID, displayName, entities.AgentTypePerson)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: create agent: %w", err)
	}

	accountID := ksuid.New().String()
	account, err := new(entities.Account).With(accountID, displayName+"'s Account", entities.AccountTypePersonal)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: create account: %w", err)
	}
	if err := account.AddMember(agentID, entities.RoleOwner); err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: add account member: %w", err)
	}

	credentialID := ksuid.New().String()
	credential, err := new(entities.Credential).With(credentialID, agentID, entities.ProviderPassword, normalizedEmail, normalizedEmail, displayName)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: create credential: %w", err)
	}

	passwordCredentialID := ksuid.New().String()
	passwordCredential, err := new(entities.PasswordCredential).With(passwordCredentialID, credentialID, algorithm, hash)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: create password credential: %w", err)
	}

	if s.eventStore != nil {
		uow := esApplication.NewSimpleUnitOfWork(s.eventStore, nil)
		if err := uow.Track(agent, account, credential, passwordCredential); err != nil {
			return nil, nil, nil, fmt.Errorf("authentication: track entities: %w", err)
		}
		if err := uow.Commit(ctx); err != nil {
			return nil, nil, nil, fmt.Errorf("authentication: commit unit of work: %w", err)
		}
	}

	if err := s.agents.Save(ctx, agent); err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: save agent: %w", err)
	}
	if s.accounts != nil {
		if err := s.accounts.Save(ctx, account); err != nil {
			return nil, nil, nil, fmt.Errorf("authentication: save account: %w", err)
		}
		if err := s.accounts.SaveMember(ctx, accountID, agentID, entities.RoleOwner); err != nil {
			return nil, nil, nil, fmt.Errorf("authentication: save account member: %w", err)
		}
	}
	if err := s.credentials.Save(ctx, credential); err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: save credential: %w", err)
	}
	if err := s.passwordCredentials.Save(ctx, passwordCredential); err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: save password credential: %w", err)
	}

	return agent, credential, account, nil
}

// VerifyPassword authenticates an email + plaintext pair. Returns
// ErrInvalidPassword for both wrong-password and unknown-email so callers
// cannot enumerate registered emails.
func (s *DefaultAuthenticationService) VerifyPassword(ctx context.Context, email, plaintext string) (*entities.Agent, *entities.Credential, *entities.Account, error) {
	if s.passwordCredentials == nil {
		return nil, nil, nil, ErrPasswordSupportNotConfigured
	}
	normalizedEmail := normalizeEmail(email)

	credential, err := s.credentials.FindByProvider(ctx, entities.ProviderPassword, normalizedEmail)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: lookup credential: %w", err)
	}
	if credential == nil || !credential.Active() {
		// Constant-time-ish: still hash the plaintext against a junk hash so
		// failed lookups don't return measurably faster than failed compares.
		// Errors from the dummy compare are discarded.
		_ = verifyPassword(entities.PasswordAlgorithmBcrypt, dummyBcryptHash, plaintext)
		s.logger.Info(ctx, "password verify: no active credential", "email", normalizedEmail)
		return nil, nil, nil, ErrInvalidPassword
	}

	passwordCredential, err := s.passwordCredentials.FindByCredentialID(ctx, credential.GetID())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: lookup password credential: %w", err)
	}
	if passwordCredential == nil {
		_ = verifyPassword(entities.PasswordAlgorithmBcrypt, dummyBcryptHash, plaintext)
		s.logger.Info(ctx, "password verify: missing password row", "credential_id", credential.GetID())
		return nil, nil, nil, ErrInvalidPassword
	}

	if err := verifyPassword(passwordCredential.Algorithm(), passwordCredential.Hash(), plaintext); err != nil {
		if errors.Is(err, ErrInvalidPassword) {
			return nil, nil, nil, ErrInvalidPassword
		}
		return nil, nil, nil, fmt.Errorf("authentication: verify password: %w", err)
	}

	agent, err := s.agents.FindByID(ctx, credential.AgentID())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: load agent: %w", err)
	}
	if agent == nil {
		return nil, nil, nil, fmt.Errorf("authentication: agent %s not found", credential.AgentID())
	}

	var account *entities.Account
	if s.accounts != nil {
		account, err = s.accounts.FindPersonalByMember(ctx, agent.GetID())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("authentication: load personal account: %w", err)
		}
	}

	if err := credential.MarkUsed(); err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: mark credential used: %w", err)
	}
	if err := s.credentials.Save(ctx, credential); err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: save credential: %w", err)
	}
	passwordCredential.MarkVerified()
	if err := s.passwordCredentials.Save(ctx, passwordCredential); err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: save password credential: %w", err)
	}

	return agent, credential, account, nil
}

// ImportPasswordCredential imports a pre-hashed bcrypt blob against existing
// agent/account IDs. Idempotent on (provider="password", lower(email)).
func (s *DefaultAuthenticationService) ImportPasswordCredential(ctx context.Context, email, displayName, bcryptHash, agentID, accountID string) error {
	if s.passwordCredentials == nil {
		return ErrPasswordSupportNotConfigured
	}
	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" {
		return fmt.Errorf("authentication: email must not be empty")
	}
	if bcryptHash == "" {
		return fmt.Errorf("authentication: bcrypt hash must not be empty")
	}
	if agentID == "" {
		return fmt.Errorf("authentication: agent ID must not be empty")
	}

	existing, err := s.credentials.FindByProvider(ctx, entities.ProviderPassword, normalizedEmail)
	if err != nil {
		return fmt.Errorf("authentication: lookup credential: %w", err)
	}
	if existing != nil {
		return nil
	}

	agent, err := s.agents.FindByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("authentication: load agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("authentication: agent %s not found", agentID)
	}
	if accountID != "" && s.accounts != nil {
		account, err := s.accounts.FindByID(ctx, accountID)
		if err != nil {
			return fmt.Errorf("authentication: load account: %w", err)
		}
		if account == nil {
			return fmt.Errorf("authentication: account %s not found", accountID)
		}
	}

	credentialID := ksuid.New().String()
	credential, err := new(entities.Credential).With(credentialID, agentID, entities.ProviderPassword, normalizedEmail, normalizedEmail, displayName)
	if err != nil {
		return fmt.Errorf("authentication: create credential: %w", err)
	}

	passwordCredentialID := ksuid.New().String()
	passwordCredential, err := new(entities.PasswordCredential).With(passwordCredentialID, credentialID, entities.PasswordAlgorithmBcrypt, bcryptHash)
	if err != nil {
		return fmt.Errorf("authentication: create password credential: %w", err)
	}

	if s.eventStore != nil {
		uow := esApplication.NewSimpleUnitOfWork(s.eventStore, nil)
		if err := uow.Track(credential, passwordCredential); err != nil {
			return fmt.Errorf("authentication: track entities: %w", err)
		}
		if err := uow.Commit(ctx); err != nil {
			return fmt.Errorf("authentication: commit unit of work: %w", err)
		}
	}

	if err := s.credentials.Save(ctx, credential); err != nil {
		return fmt.Errorf("authentication: save credential: %w", err)
	}
	if err := s.passwordCredentials.Save(ctx, passwordCredential); err != nil {
		return fmt.Errorf("authentication: save password credential: %w", err)
	}
	return nil
}

// UpdatePassword rotates the stored password for the given agent.
func (s *DefaultAuthenticationService) UpdatePassword(ctx context.Context, agentID, oldPlaintext, newPlaintext string) error {
	if s.passwordCredentials == nil {
		return ErrPasswordSupportNotConfigured
	}
	if agentID == "" {
		return fmt.Errorf("authentication: agent ID must not be empty")
	}
	if newPlaintext == "" {
		return fmt.Errorf("authentication: new password must not be empty")
	}

	creds, err := s.credentials.FindByAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("authentication: lookup credentials: %w", err)
	}
	var passwordCredentialEntity *entities.Credential
	for _, c := range creds {
		if c.Provider() == entities.ProviderPassword && c.Active() {
			passwordCredentialEntity = c
			break
		}
	}
	if passwordCredentialEntity == nil {
		return ErrPasswordCredentialMissing
	}

	pc, err := s.passwordCredentials.FindByCredentialID(ctx, passwordCredentialEntity.GetID())
	if err != nil {
		return fmt.Errorf("authentication: lookup password credential: %w", err)
	}
	if pc == nil {
		return ErrPasswordCredentialMissing
	}

	if err := verifyPassword(pc.Algorithm(), pc.Hash(), oldPlaintext); err != nil {
		if errors.Is(err, ErrInvalidPassword) {
			return ErrInvalidPassword
		}
		return fmt.Errorf("authentication: verify old password: %w", err)
	}

	algorithm, hash, err := hashPassword(newPlaintext, s.bcryptCost)
	if err != nil {
		return fmt.Errorf("authentication: hash new password: %w", err)
	}
	if err := pc.Update(algorithm, hash); err != nil {
		return fmt.Errorf("authentication: update password credential: %w", err)
	}

	// Note: we deliberately do not commit a UnitOfWork here. The aggregate
	// was rehydrated from the read-model via Restore(), which sets
	// sequenceNo=0; appending to the event store with expectedVersion=0
	// would conflict with the live history. This matches the existing
	// pattern in FindOrCreateAgent's MarkUsed path (read-model-only
	// mutations on rehydrated aggregates skip the event store).
	if err := s.passwordCredentials.Save(ctx, pc); err != nil {
		return fmt.Errorf("authentication: save password credential: %w", err)
	}
	return nil
}

// dummyBcryptHash is a real bcrypt(DefaultCost) hash used to keep
// VerifyPassword's timing roughly constant when no credential row exists
// for the given email. Generated once via bcrypt.GenerateFromPassword over
// an unguessable plaintext so it parses correctly and forces a real
// CompareHashAndPassword cycle. A unit test asserts that comparing it
// returns ErrMismatchedHashAndPassword (not a parse error) so the timing
// shield does not regress to a no-op.
const dummyBcryptHash = "$2a$10$EEI56WhQ.0l6UnEoiuE3bOZM7ADLEEdvDaI6KNGpodfiLnQnL7kbO"
