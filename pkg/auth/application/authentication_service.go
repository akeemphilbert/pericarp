package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	"github.com/akeemphilbert/pericarp/pkg/ddd"
	esApplication "github.com/akeemphilbert/pericarp/pkg/eventsourcing/application"
	esDomain "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/segmentio/ksuid"
	"golang.org/x/crypto/bcrypt"
)

// Sentinel errors for the authentication domain.
var (
	ErrInvalidProvider              = errors.New("authentication: invalid provider")
	ErrInvalidState                 = errors.New("authentication: invalid state parameter")
	ErrCodeExchangeFailed           = errors.New("authentication: code exchange failed")
	ErrSessionNotFound              = errors.New("authentication: session not found")
	ErrSessionExpired               = errors.New("authentication: session expired")
	ErrSessionRevoked               = errors.New("authentication: session revoked")
	ErrTokenRefreshFailed           = errors.New("authentication: token refresh failed")
	ErrCredentialNotFound           = errors.New("authentication: credential not found")
	ErrEmailAlreadyTaken            = errors.New("authentication: email already registered with a password")
	ErrPasswordSupportNotConfigured = errors.New("authentication: password support not configured")
	ErrPasswordCredentialMissing    = errors.New("authentication: password credential not found for agent")
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
	// existing foreign keys must remain valid. Pass ImportWithSalt(salt)
	// for legacy systems that applied an extra application-layer salt
	// suffix on top of bcrypt.
	ImportPasswordCredential(ctx context.Context, email, displayName, bcryptHash, agentID, accountID string, opts ...ImportOption) error

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
	dispatcher          *esDomain.EventDispatcher
	tokens              TokenStore
	authorization       AuthorizationChecker
	logger              Logger
	jwtService          JWTService
	subscriptionService SubscriptionService
	bcryptCost          int
	dummyHashOnce       sync.Once
	dummyHashValue      string
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
		uow := esApplication.NewSimpleUnitOfWork(s.eventStore, s.dispatcher)
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

// IssueIdentityToken issues a signed JWT for the given agent, or ("", nil)
// if no JWTService is configured. When a SubscriptionService is wired the
// current claim is snapshotted into the token; lookup failures are logged
// but do not block issuance — billing-provider outages must not break
// login.
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
	var subscription *auth.SubscriptionClaim
	if s.subscriptionService != nil {
		sub, err := s.subscriptionService.GetSubscription(ctx, agent.GetID(), activeAccountID)
		if err != nil {
			s.logger.Warn(ctx, "auth: subscription lookup failed", "agent_id", agent.GetID(), "active_account_id", activeAccountID, "error", err)
		} else if sub != nil {
			if sub.Status.Valid() {
				subscription = sub
			} else {
				s.logger.Warn(ctx, "auth: subscription lookup returned invalid status", "agent_id", agent.GetID(), "active_account_id", activeAccountID, "status", sub.Status)
			}
		}
	}
	return s.jwtService.IssueToken(ctx, agent, accounts, activeAccountID, subscription)
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
		uow := esApplication.NewSimpleUnitOfWork(s.eventStore, s.dispatcher)
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
		// Race against a concurrent register: another caller wrote a
		// credential row for the same (provider, lower(email)) between
		// our pre-check and this Save. Translate to the same sentinel
		// the pre-check would have returned. Note: the agent + account
		// rows we wrote above are now orphaned; cleaning them up
		// requires wrapping projection writes in a DB transaction,
		// which is tracked as a follow-up (the same race exists in
		// FindOrCreateAgent for OAuth flows today).
		if errors.Is(err, repositories.ErrDuplicateCredential) {
			return nil, nil, nil, ErrEmailAlreadyTaken
		}
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
		s.runTimingShield(ctx, plaintext)
		s.logger.Warn(ctx, "auth: password verify failed (no active credential)", "email", normalizedEmail)
		return nil, nil, nil, ErrInvalidPassword
	}

	passwordCredential, err := s.passwordCredentials.FindByCredentialID(ctx, credential.GetID())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("authentication: lookup password credential: %w", err)
	}
	if passwordCredential == nil {
		s.runTimingShield(ctx, plaintext)
		s.logger.Warn(ctx, "auth: password verify failed (missing password row)", "credential_id", credential.GetID())
		return nil, nil, nil, ErrInvalidPassword
	}

	if err := verifyPassword(passwordCredential.Algorithm(), passwordCredential.Hash(), plaintext, passwordCredential.Salt()); err != nil {
		// Map any verify failure (mismatch OR corrupt hash / unsupported
		// algorithm) to ErrInvalidPassword so timing and error-type do not
		// distinguish the two cases. The internal log retains detail.
		if !errors.Is(err, ErrInvalidPassword) {
			s.logger.Error(ctx, "auth: password compare failed (non-mismatch error)", "credential_id", credential.GetID(), "error", err)
		} else {
			s.logger.Warn(ctx, "auth: password verify failed (mismatch)", "credential_id", credential.GetID())
		}
		return nil, nil, nil, ErrInvalidPassword
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

	// Housekeeping writes (last-used / last-verified) are best-effort: the
	// password was correct, so denying login because a metadata projection
	// failed to update would be a worse UX than a stale timestamp.
	if err := credential.MarkUsed(); err != nil {
		s.logger.Warn(ctx, "auth: mark credential used failed", "credential_id", credential.GetID(), "error", err)
	} else if err := s.credentials.Save(ctx, credential); err != nil {
		s.logger.Warn(ctx, "auth: save credential after verify failed", "credential_id", credential.GetID(), "error", err)
	}
	passwordCredential.MarkVerified()
	if err := s.passwordCredentials.Save(ctx, passwordCredential); err != nil {
		s.logger.Warn(ctx, "auth: save password credential after verify failed", "password_credential_id", passwordCredential.GetID(), "error", err)
	}

	return agent, credential, account, nil
}

// runTimingShield runs a real bcrypt compare against a parseable dummy
// hash so the unknown-email / orphan-row branches of VerifyPassword do
// not return measurably faster than a real failed login. The dummy hash
// matches the service's configured cost. A non-mismatch error from the
// compare means the shield is degraded; we log at Error level so it
// surfaces in operational telemetry instead of being silent.
func (s *DefaultAuthenticationService) runTimingShield(ctx context.Context, plaintext string) {
	if err := verifyPassword(entities.PasswordAlgorithmBcrypt, s.dummyBcryptHash(ctx), plaintext, ""); err != nil && !errors.Is(err, ErrInvalidPassword) {
		s.logger.Error(ctx, "auth: timing-shield bcrypt compare failed (shield degraded)", "error", err)
	}
}

// ImportPasswordCredential imports a pre-hashed bcrypt blob against existing
// agent/account IDs. Idempotent on (provider="password", lower(email)).
// Pass ImportWithSalt(salt) for legacy hashes that bcrypted plaintext+salt
// (an extra application-layer suffix on top of bcrypt's own per-hash salt).
func (s *DefaultAuthenticationService) ImportPasswordCredential(ctx context.Context, email, displayName, bcryptHash, agentID, accountID string, opts ...ImportOption) error {
	if s.passwordCredentials == nil {
		return ErrPasswordSupportNotConfigured
	}
	cfg := importConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}
	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" {
		return fmt.Errorf("authentication: email must not be empty")
	}
	if bcryptHash == "" {
		return fmt.Errorf("authentication: bcrypt hash must not be empty")
	}
	// Reject malformed hashes upfront so a corrupt migration row fails at
	// import time rather than silently turning into ErrInvalidPassword on
	// every future login attempt for that account.
	if _, costErr := bcrypt.Cost([]byte(bcryptHash)); costErr != nil {
		return fmt.Errorf("authentication: invalid bcrypt hash: %w", costErr)
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
	passwordCredential, err := new(entities.PasswordCredential).WithSalt(passwordCredentialID, credentialID, entities.PasswordAlgorithmBcrypt, bcryptHash, cfg.saltSuffix)
	if err != nil {
		return fmt.Errorf("authentication: create password credential: %w", err)
	}

	if s.eventStore != nil {
		uow := esApplication.NewSimpleUnitOfWork(s.eventStore, s.dispatcher)
		if err := uow.Track(credential, passwordCredential); err != nil {
			return fmt.Errorf("authentication: track entities: %w", err)
		}
		if err := uow.Commit(ctx); err != nil {
			return fmt.Errorf("authentication: commit unit of work: %w", err)
		}
	}

	if err := s.credentials.Save(ctx, credential); err != nil {
		// Race-induced dup-key after an idempotent pre-check returned
		// nil: another caller landed the same (provider, lower(email))
		// in the meantime. Treat the import as a no-op to preserve
		// idempotent semantics. The CredentialCreated and
		// PasswordCredentialCreated events we already committed to the
		// event store describe an aggregate without a projection row;
		// this is the same divergence the broader transaction
		// follow-up will close.
		if errors.Is(err, repositories.ErrDuplicateCredential) {
			return nil
		}
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

	if err := verifyPassword(pc.Algorithm(), pc.Hash(), oldPlaintext, pc.Salt()); err != nil {
		if errors.Is(err, ErrInvalidPassword) {
			return ErrInvalidPassword
		}
		return fmt.Errorf("authentication: verify old password: %w", err)
	}

	algorithm, hash, err := hashPassword(newPlaintext, s.bcryptCost)
	if err != nil {
		return fmt.Errorf("authentication: hash new password: %w", err)
	}

	// The aggregate was rehydrated from the projection via Restore(), which
	// resets sequenceNo to 0. If an EventStore is configured we must reseat
	// the BaseEntity to the stream's true current version before recording
	// a new event, otherwise UnitOfWork.Commit would call Append with
	// expectedVersion=0 and conflict with the existing stream — and skipping
	// the commit would silently drop the PasswordUpdated event, breaking
	// the audit trail.
	if s.eventStore != nil {
		currentVersion, verErr := s.eventStore.GetCurrentVersion(ctx, pc.GetID())
		if verErr != nil {
			return fmt.Errorf("authentication: load password credential version: %w", verErr)
		}
		pc.BaseEntity = ddd.RestoreBaseEntity(pc.GetID(), currentVersion)
	}

	if err := pc.Update(algorithm, hash); err != nil {
		return fmt.Errorf("authentication: update password credential: %w", err)
	}

	if s.eventStore != nil {
		uow := esApplication.NewSimpleUnitOfWork(s.eventStore, s.dispatcher)
		if err := uow.Track(pc); err != nil {
			return fmt.Errorf("authentication: track password credential: %w", err)
		}
		if err := uow.Commit(ctx); err != nil {
			return fmt.Errorf("authentication: commit password update: %w", err)
		}
	} else {
		// No event store wired: the recorded event has nowhere durable to
		// land, so drop it from the aggregate before saving the projection
		// to keep state consistent on subsequent operations.
		pc.ClearUncommittedEvents()
	}

	if err := s.passwordCredentials.Save(ctx, pc); err != nil {
		return fmt.Errorf("authentication: save password credential: %w", err)
	}
	return nil
}

// fallbackDummyBcryptHash is a real bcrypt(DefaultCost) hash used as a
// last-resort timing-shield target if dummy-hash generation ever fails at
// runtime. The corresponding plaintext is intentionally unguessable.
const fallbackDummyBcryptHash = "$2a$10$EEI56WhQ.0l6UnEoiuE3bOZM7ADLEEdvDaI6KNGpodfiLnQnL7kbO"

// dummyBcryptHash returns a parseable bcrypt hash whose cost matches the
// service's configured bcryptCost, generated once per service instance.
// VerifyPassword uses it as a timing-equalization target on the
// unknown-email branch. Matching the live cost keeps the unknown-email
// path indistinguishable from a real failed login by elapsed time, even
// when WithBcryptCost configures a non-default cost.
func (s *DefaultAuthenticationService) dummyBcryptHash(ctx context.Context) string {
	s.dummyHashOnce.Do(func() {
		cost := s.bcryptCost
		if cost <= 0 {
			cost = bcrypt.DefaultCost
		}
		out, err := bcrypt.GenerateFromPassword([]byte("__pericarp_dummy_unguessable_plaintext__"), cost)
		if err != nil {
			s.logger.Error(ctx, "auth: dummy hash generation failed; timing shield degraded to fallback cost", "error", err, "configured_cost", cost)
			s.dummyHashValue = fallbackDummyBcryptHash
			return
		}
		s.dummyHashValue = string(out)
	})
	return s.dummyHashValue
}
