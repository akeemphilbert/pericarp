package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
)

// --- Mock implementations ---

type mockOAuthProvider struct {
	name            string
	authCodeURLFunc func(state, codeChallenge, nonce, redirectURI string) string
	exchangeFunc    func(ctx context.Context, code, codeVerifier, redirectURI string) (*application.AuthResult, error)
	refreshFunc     func(ctx context.Context, refreshToken string) (*application.AuthResult, error)
	revokeFunc      func(ctx context.Context, token string) error
	validateFunc    func(ctx context.Context, idToken, nonce string) (*application.UserInfo, error)
}

func (m *mockOAuthProvider) Name() string { return m.name }
func (m *mockOAuthProvider) AuthCodeURL(state, codeChallenge, nonce, redirectURI string) string {
	if m.authCodeURLFunc != nil {
		return m.authCodeURLFunc(state, codeChallenge, nonce, redirectURI)
	}
	return "https://provider.example.com/auth?state=" + state
}
func (m *mockOAuthProvider) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (*application.AuthResult, error) {
	if m.exchangeFunc != nil {
		return m.exchangeFunc(ctx, code, codeVerifier, redirectURI)
	}
	return &application.AuthResult{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		IDToken:      "id-token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	}, nil
}
func (m *mockOAuthProvider) RefreshToken(ctx context.Context, refreshToken string) (*application.AuthResult, error) {
	if m.refreshFunc != nil {
		return m.refreshFunc(ctx, refreshToken)
	}
	return &application.AuthResult{
		AccessToken:  "new-access-token",
		RefreshToken: "new-refresh-token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	}, nil
}
func (m *mockOAuthProvider) RevokeToken(ctx context.Context, token string) error {
	if m.revokeFunc != nil {
		return m.revokeFunc(ctx, token)
	}
	return nil
}
func (m *mockOAuthProvider) ValidateIDToken(ctx context.Context, idToken, nonce string) (*application.UserInfo, error) {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, idToken, nonce)
	}
	return &application.UserInfo{
		ProviderUserID: "provider-user-123",
		Email:          "user@example.com",
		DisplayName:    "Test User",
		Provider:       m.name,
	}, nil
}

type mockAgentRepo struct {
	agents   map[string]*entities.Agent
	saveFunc func(ctx context.Context, agent *entities.Agent) error
}

func newMockAgentRepo() *mockAgentRepo {
	return &mockAgentRepo{agents: make(map[string]*entities.Agent)}
}

func (m *mockAgentRepo) Save(ctx context.Context, agent *entities.Agent) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, agent)
	}
	m.agents[agent.GetID()] = agent
	return nil
}

func (m *mockAgentRepo) FindByID(_ context.Context, id string) (*entities.Agent, error) {
	agent, ok := m.agents[id]
	if !ok {
		return nil, nil // matches GORM repo: not-found returns (nil, nil)
	}
	return agent, nil
}

func (m *mockAgentRepo) FindAll(_ context.Context, _ string, _ int) (*repositories.PaginatedResponse[*entities.Agent], error) {
	return nil, nil
}

type mockCredentialRepo struct {
	credentials map[string]*entities.Credential
	byProvider  map[string]*entities.Credential // key: provider:providerUserID
}

func newMockCredentialRepo() *mockCredentialRepo {
	return &mockCredentialRepo{
		credentials: make(map[string]*entities.Credential),
		byProvider:  make(map[string]*entities.Credential),
	}
}

func (m *mockCredentialRepo) Save(_ context.Context, credential *entities.Credential) error {
	m.credentials[credential.GetID()] = credential
	key := credential.Provider() + ":" + credential.ProviderUserID()
	m.byProvider[key] = credential
	return nil
}

func (m *mockCredentialRepo) FindByID(_ context.Context, id string) (*entities.Credential, error) {
	cred, ok := m.credentials[id]
	if !ok {
		return nil, nil // matches GORM repo: not-found returns (nil, nil)
	}
	return cred, nil
}

func (m *mockCredentialRepo) FindByProvider(_ context.Context, provider, providerUserID string) (*entities.Credential, error) {
	key := provider + ":" + providerUserID
	cred, ok := m.byProvider[key]
	if !ok {
		return nil, nil // matches GORM repo: not-found returns (nil, nil)
	}
	return cred, nil
}

func (m *mockCredentialRepo) FindByEmail(_ context.Context, email string) ([]*entities.Credential, error) {
	var result []*entities.Credential
	for _, cred := range m.credentials {
		if cred.Email() == email {
			result = append(result, cred)
		}
	}
	return result, nil
}

func (m *mockCredentialRepo) FindByAgent(_ context.Context, _ string) ([]*entities.Credential, error) {
	return nil, nil
}

func (m *mockCredentialRepo) FindAll(_ context.Context, _ string, _ int) (*repositories.PaginatedResponse[*entities.Credential], error) {
	return nil, nil
}

type mockSessionRepo struct {
	sessions map[string]*entities.AuthSession
}

func newMockSessionRepo() *mockSessionRepo {
	return &mockSessionRepo{sessions: make(map[string]*entities.AuthSession)}
}

func (m *mockSessionRepo) Save(_ context.Context, session *entities.AuthSession) error {
	m.sessions[session.GetID()] = session
	return nil
}

func (m *mockSessionRepo) FindByID(_ context.Context, id string) (*entities.AuthSession, error) {
	session, ok := m.sessions[id]
	if !ok {
		return nil, nil // matches GORM repo: not-found returns (nil, nil)
	}
	return session, nil
}

func (m *mockSessionRepo) FindByAgent(_ context.Context, _ string) ([]*entities.AuthSession, error) {
	return nil, nil
}

func (m *mockSessionRepo) FindActive(_ context.Context, _ string) ([]*entities.AuthSession, error) {
	return nil, nil
}

func (m *mockSessionRepo) FindAll(_ context.Context, _ string, _ int) (*repositories.PaginatedResponse[*entities.AuthSession], error) {
	return nil, nil
}

func (m *mockSessionRepo) RevokeAllForAgent(_ context.Context, agentID string) error {
	for _, sess := range m.sessions {
		if sess.AgentID() == agentID && sess.Active() {
			_ = sess.Revoke()
		}
	}
	return nil
}

type mockTokenStore struct {
	tokens map[string]tokenEntry
}

type tokenEntry struct {
	accessToken  string
	refreshToken string
	expiresAt    time.Time
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{tokens: make(map[string]tokenEntry)}
}

func (m *mockTokenStore) StoreTokens(_ context.Context, credentialID string, accessToken, refreshToken, _ string, expiresAt time.Time) error {
	m.tokens[credentialID] = tokenEntry{
		accessToken:  accessToken,
		refreshToken: refreshToken,
		expiresAt:    expiresAt,
	}
	return nil
}

func (m *mockTokenStore) GetTokens(_ context.Context, credentialID string) (string, string, time.Time, error) {
	entry, ok := m.tokens[credentialID]
	if !ok {
		return "", "", time.Time{}, errors.New("tokens not found")
	}
	return entry.accessToken, entry.refreshToken, entry.expiresAt, nil
}

func (m *mockTokenStore) DeleteTokens(_ context.Context, credentialID string) error {
	delete(m.tokens, credentialID)
	return nil
}

func (m *mockTokenStore) NeedsRefresh(_ context.Context, credentialID string) (bool, error) {
	entry, ok := m.tokens[credentialID]
	if !ok {
		return false, errors.New("tokens not found")
	}
	return time.Now().After(entry.expiresAt), nil
}

type mockAccountRepo struct {
	accounts map[string]*entities.Account
	byMember map[string]*entities.Account // key: agentID -> personal account
}

func newMockAccountRepo() *mockAccountRepo {
	return &mockAccountRepo{
		accounts: make(map[string]*entities.Account),
		byMember: make(map[string]*entities.Account),
	}
}

func (m *mockAccountRepo) Save(_ context.Context, account *entities.Account) error {
	m.accounts[account.GetID()] = account
	return nil
}

func (m *mockAccountRepo) SaveMember(_ context.Context, _, agentID string, _ string) error {
	// Link agent to the most recently saved account for lookup
	for _, account := range m.accounts {
		if account.AccountType() == entities.AccountTypePersonal {
			m.byMember[agentID] = account
			break
		}
	}
	return nil
}

func (m *mockAccountRepo) FindByID(_ context.Context, id string) (*entities.Account, error) {
	account, ok := m.accounts[id]
	if !ok {
		return nil, nil
	}
	return account, nil
}

func (m *mockAccountRepo) FindByMember(_ context.Context, agentID string) ([]*entities.Account, error) {
	var result []*entities.Account
	if account, ok := m.byMember[agentID]; ok {
		result = append(result, account)
	}
	return result, nil
}

func (m *mockAccountRepo) FindPersonalByMember(_ context.Context, agentID string) (*entities.Account, error) {
	account, ok := m.byMember[agentID]
	if !ok {
		return nil, nil
	}
	return account, nil
}

func (m *mockAccountRepo) FindAll(_ context.Context, _ string, _ int) (*repositories.PaginatedResponse[*entities.Account], error) {
	return nil, nil
}

type mockAuthorizationChecker struct {
	permissions []application.Permission
}

func (m *mockAuthorizationChecker) IsAuthorized(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

func (m *mockAuthorizationChecker) IsAuthorizedInAccount(_ context.Context, _, _, _, _ string) (bool, error) {
	return true, nil
}

func (m *mockAuthorizationChecker) GetPermissions(_ context.Context, _ string) ([]application.Permission, error) {
	return m.permissions, nil
}

func (m *mockAuthorizationChecker) GetProhibitions(_ context.Context, _ string) ([]application.Permission, error) {
	return nil, nil
}

// --- Helper to create service with mocks ---

type testDeps struct {
	providers   application.OAuthProviderRegistry
	agents      *mockAgentRepo
	credentials *mockCredentialRepo
	sessions    *mockSessionRepo
	accounts    *mockAccountRepo
	tokens      *mockTokenStore
	authz       *mockAuthorizationChecker
}

func newTestService() (*application.DefaultAuthenticationService, *testDeps) {
	deps := &testDeps{
		providers: application.OAuthProviderRegistry{
			"google": &mockOAuthProvider{name: "google"},
		},
		agents:      newMockAgentRepo(),
		credentials: newMockCredentialRepo(),
		sessions:    newMockSessionRepo(),
		accounts:    newMockAccountRepo(),
		tokens:      newMockTokenStore(),
		authz:       &mockAuthorizationChecker{},
	}

	svc := application.NewDefaultAuthenticationService(
		deps.providers,
		deps.agents,
		deps.credentials,
		deps.sessions,
		deps.accounts,
		application.WithTokenStore(deps.tokens),
		application.WithAuthorizationChecker(deps.authz),
	)

	return svc, deps
}

// --- Tests ---

func TestDefaultAuthenticationService_InitiateAuthFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _ := newTestService()

	req, err := svc.InitiateAuthFlow(ctx, "google", "https://example.com/callback")
	if err != nil {
		t.Fatalf("InitiateAuthFlow() error: %v", err)
	}

	if req.AuthURL == "" {
		t.Error("expected non-empty AuthURL")
	}
	if req.State == "" {
		t.Error("expected non-empty State")
	}
	if req.CodeVerifier == "" {
		t.Error("expected non-empty CodeVerifier")
	}
	if req.Nonce == "" {
		t.Error("expected non-empty Nonce")
	}
	if req.Provider != "google" {
		t.Errorf("Provider = %q, want %q", req.Provider, "google")
	}
}

func TestDefaultAuthenticationService_InitiateAuthFlow_InvalidProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _ := newTestService()

	_, err := svc.InitiateAuthFlow(ctx, "unknown-provider", "https://example.com/callback")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !errors.Is(err, application.ErrInvalidProvider) {
		t.Errorf("error = %v, want ErrInvalidProvider", err)
	}
}

func TestDefaultAuthenticationService_ExchangeCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _ := newTestService()

	result, err := svc.ExchangeCode(ctx, "auth-code", "code-verifier", "google", "https://example.com/callback")
	if err != nil {
		t.Fatalf("ExchangeCode() error: %v", err)
	}

	if result.AccessToken == "" {
		t.Error("expected non-empty AccessToken")
	}
	if result.TokenType == "" {
		t.Error("expected non-empty TokenType")
	}
}

func TestDefaultAuthenticationService_ExchangeCode_InvalidProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _ := newTestService()

	_, err := svc.ExchangeCode(ctx, "auth-code", "code-verifier", "unknown", "https://example.com/callback")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !errors.Is(err, application.ErrInvalidProvider) {
		t.Errorf("error = %v, want ErrInvalidProvider", err)
	}
}

func TestDefaultAuthenticationService_ExchangeCode_ExchangeFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	providers := application.OAuthProviderRegistry{
		"google": &mockOAuthProvider{
			name: "google",
			exchangeFunc: func(_ context.Context, _, _, _ string) (*application.AuthResult, error) {
				return nil, errors.New("exchange failed")
			},
		},
	}
	svc := application.NewDefaultAuthenticationService(providers, newMockAgentRepo(), newMockCredentialRepo(), newMockSessionRepo(), newMockAccountRepo(), application.WithTokenStore(newMockTokenStore()))

	_, err := svc.ExchangeCode(ctx, "auth-code", "code-verifier", "google", "https://example.com/callback")
	if err == nil {
		t.Fatal("expected error when exchange fails")
	}
	if !errors.Is(err, application.ErrCodeExchangeFailed) {
		t.Errorf("error = %v, want ErrCodeExchangeFailed", err)
	}
}

func TestDefaultAuthenticationService_ValidateState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _ := newTestService()

	tests := []struct {
		name     string
		received string
		stored   string
		wantErr  bool
	}{
		{
			name:     "matching states",
			received: "abc123",
			stored:   "abc123",
			wantErr:  false,
		},
		{
			name:     "mismatched states",
			received: "abc123",
			stored:   "xyz789",
			wantErr:  true,
		},
		{
			name:     "empty received state",
			received: "",
			stored:   "abc123",
			wantErr:  true,
		},
		{
			name:     "both empty",
			received: "",
			stored:   "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := svc.ValidateState(ctx, tt.received, tt.stored)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, application.ErrInvalidState) {
					t.Errorf("error = %v, want ErrInvalidState", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDefaultAuthenticationService_FindOrCreateAgent_New(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newTestService()

	userInfo := application.UserInfo{
		ProviderUserID: "google-user-123",
		Email:          "alice@example.com",
		DisplayName:    "Alice",
		Provider:       "google",
	}

	agent, credential, account, err := svc.FindOrCreateAgent(ctx, userInfo)
	if err != nil {
		t.Fatalf("FindOrCreateAgent() error: %v", err)
	}

	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if credential == nil {
		t.Fatal("expected non-nil credential")
	}
	if account == nil {
		t.Fatal("expected non-nil account")
	}

	if agent.Name() != "Alice" {
		t.Errorf("agent Name() = %q, want %q", agent.Name(), "Alice")
	}
	if credential.Provider() != "google" {
		t.Errorf("credential Provider() = %q, want %q", credential.Provider(), "google")
	}
	if credential.ProviderUserID() != "google-user-123" {
		t.Errorf("credential ProviderUserID() = %q, want %q", credential.ProviderUserID(), "google-user-123")
	}
	if credential.Email() != "alice@example.com" {
		t.Errorf("credential Email() = %q, want %q", credential.Email(), "alice@example.com")
	}

	// Verify account is personal type
	if account.AccountType() != entities.AccountTypePersonal {
		t.Errorf("account AccountType() = %q, want %q", account.AccountType(), entities.AccountTypePersonal)
	}

	// Verify agent was saved
	if len(deps.agents.agents) != 1 {
		t.Errorf("expected 1 saved agent, got %d", len(deps.agents.agents))
	}

	// Verify credential was saved
	if len(deps.credentials.credentials) != 1 {
		t.Errorf("expected 1 saved credential, got %d", len(deps.credentials.credentials))
	}

	// Verify account was saved
	if len(deps.accounts.accounts) != 1 {
		t.Errorf("expected 1 saved account, got %d", len(deps.accounts.accounts))
	}
}

func TestDefaultAuthenticationService_FindOrCreateAgent_Existing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newTestService()

	// Pre-create agent, credential, and personal account
	agent, _ := new(entities.Agent).With("agent-existing", "Alice", entities.AgentTypePerson)
	deps.agents.agents["agent-existing"] = agent

	cred, _ := new(entities.Credential).With("cred-existing", "agent-existing", "google", "google-user-123", "alice@example.com", "Alice")
	deps.credentials.credentials["cred-existing"] = cred
	deps.credentials.byProvider["google:google-user-123"] = cred

	personalAccount, _ := new(entities.Account).With("account-existing", "Alice's Account", entities.AccountTypePersonal)
	deps.accounts.accounts["account-existing"] = personalAccount
	deps.accounts.byMember["agent-existing"] = personalAccount

	userInfo := application.UserInfo{
		ProviderUserID: "google-user-123",
		Email:          "alice@example.com",
		DisplayName:    "Alice",
		Provider:       "google",
	}

	foundAgent, foundCredential, foundAccount, err := svc.FindOrCreateAgent(ctx, userInfo)
	if err != nil {
		t.Fatalf("FindOrCreateAgent() error: %v", err)
	}

	if foundAgent.GetID() != "agent-existing" {
		t.Errorf("agent ID = %q, want %q", foundAgent.GetID(), "agent-existing")
	}
	if foundCredential.GetID() != "cred-existing" {
		t.Errorf("credential ID = %q, want %q", foundCredential.GetID(), "cred-existing")
	}
	if foundAccount == nil {
		t.Fatal("expected non-nil account")
	}
	if foundAccount.GetID() != "account-existing" {
		t.Errorf("account ID = %q, want %q", foundAccount.GetID(), "account-existing")
	}

	// Should not have created new agents (still just the 1 we pre-created)
	if len(deps.agents.agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(deps.agents.agents))
	}
}

func TestDefaultAuthenticationService_CreateSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newTestService()

	session, err := svc.CreateSession(ctx, "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	if session == nil {
		t.Fatal("expected non-nil session")
	}
	if session.AgentID() != "agent-1" {
		t.Errorf("AgentID() = %q, want %q", session.AgentID(), "agent-1")
	}
	if session.CredentialID() != "cred-1" {
		t.Errorf("CredentialID() = %q, want %q", session.CredentialID(), "cred-1")
	}
	if !session.Active() {
		t.Error("expected session to be active")
	}
	if session.GetID() == "" {
		t.Error("expected non-empty session ID")
	}

	// Verify session was saved
	if len(deps.sessions.sessions) != 1 {
		t.Errorf("expected 1 saved session, got %d", len(deps.sessions.sessions))
	}
}

func TestDefaultAuthenticationService_ValidateSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newTestService()

	// Create a session first
	session, err := svc.CreateSession(ctx, "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	// Clear uncommitted events from the session so Touch() works properly
	session.ClearUncommittedEvents()
	deps.sessions.sessions[session.GetID()] = session

	info, err := svc.ValidateSession(ctx, session.GetID())
	if err != nil {
		t.Fatalf("ValidateSession() error: %v", err)
	}

	if info.SessionID != session.GetID() {
		t.Errorf("SessionID = %q, want %q", info.SessionID, session.GetID())
	}
	if info.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", info.AgentID, "agent-1")
	}
}

func TestDefaultAuthenticationService_ValidateSession_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _ := newTestService()

	_, err := svc.ValidateSession(ctx, "nonexistent-session")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if !errors.Is(err, application.ErrSessionNotFound) {
		t.Errorf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestDefaultAuthenticationService_ValidateSession_Revoked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newTestService()

	// Create and revoke session
	session, _ := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	_ = session.Revoke()
	deps.sessions.sessions["sess-1"] = session

	_, err := svc.ValidateSession(ctx, "sess-1")
	if err == nil {
		t.Fatal("expected error for revoked session")
	}
	if !errors.Is(err, application.ErrSessionRevoked) {
		t.Errorf("error = %v, want ErrSessionRevoked", err)
	}
}

func TestDefaultAuthenticationService_ValidateSession_Expired(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newTestService()

	// Create expired session
	session, _ := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(-1*time.Hour))
	deps.sessions.sessions["sess-1"] = session

	_, err := svc.ValidateSession(ctx, "sess-1")
	if err == nil {
		t.Fatal("expected error for expired session")
	}
	if !errors.Is(err, application.ErrSessionExpired) {
		t.Errorf("error = %v, want ErrSessionExpired", err)
	}
}

func TestDefaultAuthenticationService_RevokeSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newTestService()

	session, _ := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	deps.sessions.sessions["sess-1"] = session

	if err := svc.RevokeSession(ctx, "sess-1"); err != nil {
		t.Fatalf("RevokeSession() error: %v", err)
	}

	// Verify session is now revoked
	savedSession := deps.sessions.sessions["sess-1"]
	if savedSession.Active() {
		t.Error("expected session to be inactive after revocation")
	}
}

func TestDefaultAuthenticationService_RevokeSession_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _ := newTestService()

	err := svc.RevokeSession(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if !errors.Is(err, application.ErrSessionNotFound) {
		t.Errorf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestDefaultAuthenticationService_RevokeAllSessions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newTestService()

	// Create multiple sessions for the same agent
	sess1, _ := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	sess2, _ := new(entities.AuthSession).With("sess-2", "agent-1", "cred-1", "192.168.1.2", "Chrome", time.Now().Add(24*time.Hour))
	sess3, _ := new(entities.AuthSession).With("sess-3", "agent-2", "cred-2", "10.0.0.1", "Safari", time.Now().Add(24*time.Hour))

	deps.sessions.sessions["sess-1"] = sess1
	deps.sessions.sessions["sess-2"] = sess2
	deps.sessions.sessions["sess-3"] = sess3

	if err := svc.RevokeAllSessions(ctx, "agent-1"); err != nil {
		t.Fatalf("RevokeAllSessions() error: %v", err)
	}

	// Agent-1's sessions should be revoked
	if deps.sessions.sessions["sess-1"].Active() {
		t.Error("expected sess-1 to be revoked")
	}
	if deps.sessions.sessions["sess-2"].Active() {
		t.Error("expected sess-2 to be revoked")
	}

	// Agent-2's session should still be active
	if !deps.sessions.sessions["sess-3"].Active() {
		t.Error("expected sess-3 to still be active")
	}
}

func TestDefaultAuthenticationService_RefreshTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newTestService()

	// Pre-create credential and stored tokens
	cred, _ := new(entities.Credential).With("cred-1", "agent-1", "google", "google-user-123", "alice@example.com", "Alice")
	deps.credentials.credentials["cred-1"] = cred
	deps.credentials.byProvider["google:google-user-123"] = cred
	deps.tokens.tokens["cred-1"] = tokenEntry{
		accessToken:  "old-access-token",
		refreshToken: "old-refresh-token",
		expiresAt:    time.Now().Add(-1 * time.Hour),
	}

	result, err := svc.RefreshTokens(ctx, "cred-1")
	if err != nil {
		t.Fatalf("RefreshTokens() error: %v", err)
	}

	if result.AccessToken == "" {
		t.Error("expected non-empty AccessToken")
	}

	// Verify new tokens were stored
	stored := deps.tokens.tokens["cred-1"]
	if stored.accessToken == "old-access-token" {
		t.Error("expected stored access token to be updated")
	}
}

func TestDefaultAuthenticationService_RefreshTokens_CredentialNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _ := newTestService()

	_, err := svc.RefreshTokens(ctx, "nonexistent-cred")
	if err == nil {
		t.Fatal("expected error for nonexistent credential")
	}
	if !errors.Is(err, application.ErrCredentialNotFound) {
		t.Errorf("error = %v, want ErrCredentialNotFound", err)
	}
}

func TestDefaultAuthenticationService_RefreshTokens_ProviderFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	providers := application.OAuthProviderRegistry{
		"google": &mockOAuthProvider{
			name: "google",
			refreshFunc: func(_ context.Context, _ string) (*application.AuthResult, error) {
				return nil, errors.New("refresh failed")
			},
		},
	}
	credentials := newMockCredentialRepo()
	tokens := newMockTokenStore()

	cred, _ := new(entities.Credential).With("cred-1", "agent-1", "google", "google-user-123", "alice@example.com", "Alice")
	credentials.credentials["cred-1"] = cred
	tokens.tokens["cred-1"] = tokenEntry{
		accessToken:  "old-access-token",
		refreshToken: "old-refresh-token",
		expiresAt:    time.Now().Add(-1 * time.Hour),
	}

	svc := application.NewDefaultAuthenticationService(providers, newMockAgentRepo(), credentials, newMockSessionRepo(), newMockAccountRepo(), application.WithTokenStore(tokens))

	_, err := svc.RefreshTokens(ctx, "cred-1")
	if err == nil {
		t.Fatal("expected error when provider refresh fails")
	}
	if !errors.Is(err, application.ErrTokenRefreshFailed) {
		t.Errorf("error = %v, want ErrTokenRefreshFailed", err)
	}
}

func TestNewDefaultAuthenticationService_NilAuthorization(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sessions := newMockSessionRepo()
	session, _ := new(entities.AuthSession).With("sess-1", "agent-1", "cred-1", "192.168.1.1", "Mozilla/5.0", time.Now().Add(24*time.Hour))
	session.ClearUncommittedEvents()
	sessions.sessions["sess-1"] = session

	svc := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		newMockAgentRepo(),
		newMockCredentialRepo(),
		sessions,
		newMockAccountRepo(),
		application.WithTokenStore(newMockTokenStore()),
		// no authorization checker — should default to nil
	)

	info, err := svc.ValidateSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("ValidateSession() error: %v", err)
	}

	// With nil authorization checker, permissions should be nil/empty
	if len(info.Permissions) != 0 {
		t.Errorf("expected 0 permissions with nil authorization checker, got %d", len(info.Permissions))
	}
}
