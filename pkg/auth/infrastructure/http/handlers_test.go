package authhttp_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	authhttp "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/http"
	authjwt "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/jwt"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"
	"github.com/gorilla/sessions"
)

// --- Mock AuthenticationService ---

type mockAuthService struct {
	initiateFunc           func(ctx context.Context, provider, redirectURI string) (*application.AuthRequest, error)
	exchangeFunc           func(ctx context.Context, code, codeVerifier, provider, redirectURI string) (*application.AuthResult, error)
	validateStateFunc      func(ctx context.Context, received, stored string) error
	findOrCreateFunc       func(ctx context.Context, userInfo application.UserInfo) (*entities.Agent, *entities.Credential, *entities.Account, error)
	createSessionFunc      func(ctx context.Context, agentID, credentialID, ipAddress, userAgent string, duration time.Duration) (*entities.AuthSession, error)
	validateSessFunc       func(ctx context.Context, sessionID string) (*application.SessionInfo, error)
	revokeFunc             func(ctx context.Context, sessionID string) error
	revokeAllFunc          func(ctx context.Context, agentID string) error
	refreshFunc            func(ctx context.Context, credentialID string) (*application.AuthResult, error)
	issueIdentityTokenFunc func(ctx context.Context, agent *entities.Agent, activeAccountID string) (string, error)
}

func (m *mockAuthService) InitiateAuthFlow(ctx context.Context, provider, redirectURI string) (*application.AuthRequest, error) {
	if m.initiateFunc != nil {
		return m.initiateFunc(ctx, provider, redirectURI)
	}
	return &application.AuthRequest{
		AuthURL:      "https://provider.example.com/auth?state=test-state",
		State:        "test-state",
		CodeVerifier: "test-verifier",
		Nonce:        "test-nonce",
		Provider:     provider,
	}, nil
}

func (m *mockAuthService) ExchangeCode(ctx context.Context, code, codeVerifier, provider, redirectURI string) (*application.AuthResult, error) {
	if m.exchangeFunc != nil {
		return m.exchangeFunc(ctx, code, codeVerifier, provider, redirectURI)
	}
	return &application.AuthResult{
		AccessToken: "access-token",
		UserInfo: application.UserInfo{
			ProviderUserID: "google-123",
			Email:          "user@example.com",
			DisplayName:    "Test User",
			Provider:       provider,
		},
	}, nil
}

func (m *mockAuthService) ValidateState(ctx context.Context, received, stored string) error {
	if m.validateStateFunc != nil {
		return m.validateStateFunc(ctx, received, stored)
	}
	if received != stored {
		return application.ErrInvalidState
	}
	return nil
}

func (m *mockAuthService) FindOrCreateAgent(ctx context.Context, userInfo application.UserInfo) (*entities.Agent, *entities.Credential, *entities.Account, error) {
	if m.findOrCreateFunc != nil {
		return m.findOrCreateFunc(ctx, userInfo)
	}
	agent, _ := new(entities.Agent).With("agent-1", userInfo.DisplayName, entities.AgentTypePerson)
	cred, _ := new(entities.Credential).With("cred-1", "agent-1", userInfo.Provider, userInfo.ProviderUserID, userInfo.Email, userInfo.DisplayName)
	account, _ := new(entities.Account).With("account-1", userInfo.DisplayName+"'s Account", entities.AccountTypePersonal)
	return agent, cred, account, nil
}

func (m *mockAuthService) CreateSession(ctx context.Context, agentID, credentialID, ipAddress, userAgent string, duration time.Duration) (*entities.AuthSession, error) {
	if m.createSessionFunc != nil {
		return m.createSessionFunc(ctx, agentID, credentialID, ipAddress, userAgent, duration)
	}
	sess, _ := new(entities.AuthSession).With("sess-1", agentID, credentialID, ipAddress, userAgent, time.Now().Add(duration))
	return sess, nil
}

func (m *mockAuthService) ValidateSession(ctx context.Context, sessionID string) (*application.SessionInfo, error) {
	if m.validateSessFunc != nil {
		return m.validateSessFunc(ctx, sessionID)
	}
	return &application.SessionInfo{
		SessionID: sessionID,
		AgentID:   "agent-1",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}, nil
}

func (m *mockAuthService) RevokeSession(ctx context.Context, sessionID string) error {
	if m.revokeFunc != nil {
		return m.revokeFunc(ctx, sessionID)
	}
	return nil
}

func (m *mockAuthService) RevokeAllSessions(ctx context.Context, agentID string) error {
	if m.revokeAllFunc != nil {
		return m.revokeAllFunc(ctx, agentID)
	}
	return nil
}

func (m *mockAuthService) RefreshTokens(ctx context.Context, credentialID string) (*application.AuthResult, error) {
	if m.refreshFunc != nil {
		return m.refreshFunc(ctx, credentialID)
	}
	return &application.AuthResult{AccessToken: "new-token"}, nil
}

func (m *mockAuthService) IssueIdentityToken(ctx context.Context, agent *entities.Agent, activeAccountID string) (string, error) {
	if m.issueIdentityTokenFunc != nil {
		return m.issueIdentityTokenFunc(ctx, agent, activeAccountID)
	}
	return "", nil
}

// --- Mock CredentialRepository ---

type mockCredRepo struct {
	findByAgentFunc func(ctx context.Context, agentID string) ([]*entities.Credential, error)
}

func (m *mockCredRepo) Save(_ context.Context, _ *entities.Credential) error { return nil }
func (m *mockCredRepo) FindByID(_ context.Context, _ string) (*entities.Credential, error) {
	return nil, nil
}
func (m *mockCredRepo) FindByProvider(_ context.Context, _, _ string) (*entities.Credential, error) {
	return nil, nil
}
func (m *mockCredRepo) FindByEmail(_ context.Context, _ string) ([]*entities.Credential, error) {
	return nil, nil
}
func (m *mockCredRepo) FindByAgent(ctx context.Context, agentID string) ([]*entities.Credential, error) {
	if m.findByAgentFunc != nil {
		return m.findByAgentFunc(ctx, agentID)
	}
	return nil, nil
}
func (m *mockCredRepo) FindAll(_ context.Context, _ string, _ int) (*repositories.PaginatedResponse[*entities.Credential], error) {
	return nil, nil
}

// --- Test helpers ---

func newTestHandlers(authSvc *mockAuthService, credRepo *mockCredRepo) (*authhttp.AuthHandlers, *session.GorillaSessionManager) {
	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!!"))
	sm := session.NewGorillaSessionManager("test-session", store, session.DefaultSessionOptions())

	if credRepo == nil {
		credRepo = &mockCredRepo{}
	}

	cfg := authhttp.HandlerConfig{
		AuthService:    authSvc,
		SessionManager: sm,
		Credentials:    credRepo,
		RedirectURI:    authhttp.RedirectURIConfig{CallbackPath: "/api/auth/callback"},
	}
	return authhttp.NewAuthHandlers(cfg), sm
}

func parseJSONResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	return resp
}

// --- Login handler tests ---

func TestLogin_Success_Redirects(t *testing.T) {
	t.Parallel()

	handlers, _ := newTestHandlers(&mockAuthService{}, nil)

	r := httptest.NewRequest("GET", "/auth/login", nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()

	handlers.Login(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc == "" {
		t.Fatal("expected Location header")
	}
}

func TestLogin_WithRedirectParam_StoresInFlowMetadata(t *testing.T) {
	t.Parallel()

	handlers, sm := newTestHandlers(&mockAuthService{}, nil)

	r := httptest.NewRequest("GET", "/auth/login?redirect=/dashboard", nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()

	handlers.Login(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}

	// Verify flow data was stored with metadata by reading it back
	// We need to use the response cookies on a new request
	resp := w.Result()
	callbackReq := httptest.NewRequest("GET", "/api/auth/callback?state=test-state&code=auth-code", nil)
	for _, cookie := range resp.Cookies() {
		callbackReq.AddCookie(cookie)
	}

	callbackW := httptest.NewRecorder()
	flowData, err := sm.GetFlowData(callbackW, callbackReq)
	if err != nil {
		t.Fatalf("failed to get flow data: %v", err)
	}
	if flowData.Metadata == nil {
		t.Fatal("expected non-nil metadata")
	}
	if flowData.Metadata["post_login_redirect"] != "/dashboard" {
		t.Errorf("metadata[post_login_redirect] = %q, want %q", flowData.Metadata["post_login_redirect"], "/dashboard")
	}
}

func TestLogin_InvalidRedirectParam_IgnoresIt(t *testing.T) {
	t.Parallel()

	handlers, sm := newTestHandlers(&mockAuthService{}, nil)

	r := httptest.NewRequest("GET", "/auth/login?redirect=//evil.com", nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()

	handlers.Login(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}

	resp := w.Result()
	callbackReq := httptest.NewRequest("GET", "/api/auth/callback", nil)
	for _, cookie := range resp.Cookies() {
		callbackReq.AddCookie(cookie)
	}

	callbackW := httptest.NewRecorder()
	flowData, err := sm.GetFlowData(callbackW, callbackReq)
	if err != nil {
		t.Fatalf("failed to get flow data: %v", err)
	}
	if flowData.Metadata != nil {
		t.Errorf("expected nil metadata for invalid redirect, got %v", flowData.Metadata)
	}
}

func TestLogin_CustomProvider(t *testing.T) {
	t.Parallel()

	var capturedProvider string
	svc := &mockAuthService{
		initiateFunc: func(_ context.Context, provider, _ string) (*application.AuthRequest, error) {
			capturedProvider = provider
			return &application.AuthRequest{
				AuthURL:      "https://github.com/auth",
				State:        "state",
				CodeVerifier: "verifier",
				Nonce:        "nonce",
				Provider:     provider,
			}, nil
		},
	}

	handlers, _ := newTestHandlers(svc, nil)
	r := httptest.NewRequest("GET", "/auth/login?provider=github", nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()

	handlers.Login(w, r)

	if capturedProvider != "github" {
		t.Errorf("provider = %q, want %q", capturedProvider, "github")
	}
}

func TestLogin_InitiateFlowFails_Returns500(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{
		initiateFunc: func(_ context.Context, _, _ string) (*application.AuthRequest, error) {
			return nil, errors.New("provider not found")
		},
	}

	handlers, _ := newTestHandlers(svc, nil)
	r := httptest.NewRequest("GET", "/auth/login", nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()

	handlers.Login(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}

// --- Callback handler tests ---

func TestCallback_FullFlow_Success(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{}
	handlers, sm := newTestHandlers(svc, nil)

	// Step 1: Simulate Login to set flow data
	loginReq := httptest.NewRequest("GET", "/auth/login", nil)
	loginReq.Host = "example.com"
	loginW := httptest.NewRecorder()
	handlers.Login(loginW, loginReq)

	// Set up flow data directly via session manager
	callbackReq := httptest.NewRequest("GET", "/api/auth/callback?state=test-state&code=auth-code", nil)
	callbackReq.Host = "example.com"
	flowW := httptest.NewRecorder()
	flowData := session.FlowData{
		State:        "test-state",
		CodeVerifier: "test-verifier",
		Nonce:        "test-nonce",
		Provider:     "google",
		RedirectURI:  "http://example.com/api/auth/callback",
		CreatedAt:    time.Now(),
	}
	if err := sm.SetFlowData(flowW, callbackReq, flowData); err != nil {
		t.Fatalf("failed to set flow data: %v", err)
	}

	// Build callback request with flow cookies
	callbackReq2 := httptest.NewRequest("GET", "/api/auth/callback?state=test-state&code=auth-code", nil)
	callbackReq2.Host = "example.com"
	for _, cookie := range flowW.Result().Cookies() {
		callbackReq2.AddCookie(cookie)
	}

	callbackW := httptest.NewRecorder()
	handlers.Callback(callbackW, callbackReq2)

	if callbackW.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d; body: %s", callbackW.Code, callbackW.Body.String())
	}

	loc := callbackW.Header().Get("Location")
	if loc != "/" {
		t.Errorf("Location = %q, want %q", loc, "/")
	}
}

func TestCallback_MissingFlowData_Returns400(t *testing.T) {
	t.Parallel()

	handlers, _ := newTestHandlers(&mockAuthService{}, nil)

	r := httptest.NewRequest("GET", "/api/auth/callback?state=test&code=auth", nil)
	w := httptest.NewRecorder()

	handlers.Callback(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
	resp := parseJSONResponse(t, w)
	if resp["error"] != "missing or expired flow data" {
		t.Errorf("error = %q, want %q", resp["error"], "missing or expired flow data")
	}
}

func TestCallback_InvalidState_Returns400(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{
		validateStateFunc: func(_ context.Context, _, _ string) error {
			return application.ErrInvalidState
		},
	}

	handlers, sm := newTestHandlers(svc, nil)

	// Set up flow data
	r := httptest.NewRequest("GET", "/api/auth/callback?state=wrong-state&code=auth", nil)
	r.Host = "example.com"
	flowW := httptest.NewRecorder()
	if err := sm.SetFlowData(flowW, r, session.FlowData{
		State:     "correct-state",
		Provider:  "google",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("failed to set flow data: %v", err)
	}

	r2 := httptest.NewRequest("GET", "/api/auth/callback?state=wrong-state&code=auth", nil)
	for _, cookie := range flowW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Callback(w, r2)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestCallback_ExchangeFails_Returns500(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{
		exchangeFunc: func(_ context.Context, _, _, _, _ string) (*application.AuthResult, error) {
			return nil, errors.New("exchange failed")
		},
	}

	handlers, sm := newTestHandlers(svc, nil)

	r := httptest.NewRequest("GET", "/api/auth/callback?state=s&code=c", nil)
	r.Host = "example.com"
	flowW := httptest.NewRecorder()
	_ = sm.SetFlowData(flowW, r, session.FlowData{
		State: "s", Provider: "google", CreatedAt: time.Now(),
	})

	r2 := httptest.NewRequest("GET", "/api/auth/callback?state=s&code=c", nil)
	for _, cookie := range flowW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Callback(w, r2)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCallback_WithPostLoginRedirect(t *testing.T) {
	t.Parallel()

	handlers, sm := newTestHandlers(&mockAuthService{}, nil)

	r := httptest.NewRequest("GET", "/api/auth/callback?state=s&code=c", nil)
	r.Host = "example.com"
	flowW := httptest.NewRecorder()
	_ = sm.SetFlowData(flowW, r, session.FlowData{
		State:     "s",
		Provider:  "google",
		CreatedAt: time.Now(),
		Metadata:  map[string]string{"post_login_redirect": "/settings"},
	})

	r2 := httptest.NewRequest("GET", "/api/auth/callback?state=s&code=c", nil)
	for _, cookie := range flowW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Callback(w, r2)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d; body: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/settings" {
		t.Errorf("Location = %q, want %q", loc, "/settings")
	}
}

// --- Me handler tests ---

func TestMe_Authenticated_ReturnsProfile(t *testing.T) {
	t.Parallel()

	cred, _ := new(entities.Credential).With("cred-1", "agent-1", "google", "g-123", "user@example.com", "Test User")
	credRepo := &mockCredRepo{
		findByAgentFunc: func(_ context.Context, _ string) ([]*entities.Credential, error) {
			return []*entities.Credential{cred}, nil
		},
	}

	handlers, sm := newTestHandlers(&mockAuthService{}, credRepo)

	// Create a session first
	r := httptest.NewRequest("GET", "/api/auth/me", nil)
	r.Host = "example.com"
	sessionW := httptest.NewRecorder()
	_ = sm.CreateHTTPSession(sessionW, r, session.SessionData{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	r2 := httptest.NewRequest("GET", "/api/auth/me", nil)
	for _, cookie := range sessionW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Me(w, r2)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	resp := parseJSONResponse(t, w)
	if resp["id"] != "agent-1" {
		t.Errorf("id = %q, want %q", resp["id"], "agent-1")
	}
	if resp["name"] != "Test User" {
		t.Errorf("name = %q, want %q", resp["name"], "Test User")
	}
	if resp["email"] != "user@example.com" {
		t.Errorf("email = %q, want %q", resp["email"], "user@example.com")
	}
}

func TestMe_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	handlers, _ := newTestHandlers(&mockAuthService{}, nil)

	r := httptest.NewRequest("GET", "/api/auth/me", nil)
	w := httptest.NewRecorder()

	handlers.Me(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestMe_InvalidSession_Returns401(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{
		validateSessFunc: func(_ context.Context, _ string) (*application.SessionInfo, error) {
			return nil, application.ErrSessionExpired
		},
	}

	handlers, sm := newTestHandlers(svc, nil)

	r := httptest.NewRequest("GET", "/api/auth/me", nil)
	r.Host = "example.com"
	sessionW := httptest.NewRecorder()
	_ = sm.CreateHTTPSession(sessionW, r, session.SessionData{
		SessionID: "expired-sess",
		AgentID:   "agent-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})

	r2 := httptest.NewRequest("GET", "/api/auth/me", nil)
	for _, cookie := range sessionW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Me(w, r2)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestMe_NoCredentials_ReturnsEmptyProfile(t *testing.T) {
	t.Parallel()

	credRepo := &mockCredRepo{
		findByAgentFunc: func(_ context.Context, _ string) ([]*entities.Credential, error) {
			return nil, nil
		},
	}

	handlers, sm := newTestHandlers(&mockAuthService{}, credRepo)

	r := httptest.NewRequest("GET", "/api/auth/me", nil)
	r.Host = "example.com"
	sessionW := httptest.NewRecorder()
	_ = sm.CreateHTTPSession(sessionW, r, session.SessionData{
		SessionID: "sess-1", AgentID: "agent-1",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	r2 := httptest.NewRequest("GET", "/api/auth/me", nil)
	for _, cookie := range sessionW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Me(w, r2)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	resp := parseJSONResponse(t, w)
	if resp["name"] != "" {
		t.Errorf("name = %q, want empty", resp["name"])
	}
}

func TestMe_CredentialRepoError_Returns500(t *testing.T) {
	t.Parallel()

	credRepo := &mockCredRepo{
		findByAgentFunc: func(_ context.Context, _ string) ([]*entities.Credential, error) {
			return nil, errors.New("database down")
		},
	}

	handlers, sm := newTestHandlers(&mockAuthService{}, credRepo)

	r := httptest.NewRequest("GET", "/api/auth/me", nil)
	r.Host = "example.com"
	sessionW := httptest.NewRecorder()
	_ = sm.CreateHTTPSession(sessionW, r, session.SessionData{
		SessionID: "sess-1", AgentID: "agent-1",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	r2 := httptest.NewRequest("GET", "/api/auth/me", nil)
	for _, cookie := range sessionW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Me(w, r2)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// --- Logout handler tests ---

func TestLogout_Success(t *testing.T) {
	t.Parallel()

	var revokedSessionID string
	svc := &mockAuthService{
		revokeFunc: func(_ context.Context, sessionID string) error {
			revokedSessionID = sessionID
			return nil
		},
	}

	handlers, sm := newTestHandlers(svc, nil)

	r := httptest.NewRequest("POST", "/api/auth/logout", nil)
	r.Host = "example.com"
	sessionW := httptest.NewRecorder()
	_ = sm.CreateHTTPSession(sessionW, r, session.SessionData{
		SessionID: "sess-1", AgentID: "agent-1",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	r2 := httptest.NewRequest("POST", "/api/auth/logout", nil)
	for _, cookie := range sessionW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Logout(w, r2)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	resp := parseJSONResponse(t, w)
	if resp["status"] != "logged out" {
		t.Errorf("status = %q, want %q", resp["status"], "logged out")
	}

	if revokedSessionID != "sess-1" {
		t.Errorf("revoked session ID = %q, want %q", revokedSessionID, "sess-1")
	}
}

func TestLogout_RevokeError_StillDestroysSession(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{
		revokeFunc: func(_ context.Context, _ string) error {
			return errors.New("database down")
		},
	}

	handlers, sm := newTestHandlers(svc, nil)

	r := httptest.NewRequest("POST", "/api/auth/logout", nil)
	r.Host = "example.com"
	sessionW := httptest.NewRecorder()
	_ = sm.CreateHTTPSession(sessionW, r, session.SessionData{
		SessionID: "sess-1", AgentID: "agent-1",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	r2 := httptest.NewRequest("POST", "/api/auth/logout", nil)
	for _, cookie := range sessionW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Logout(w, r2)

	// Logout should still succeed (best-effort revocation)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestLogout_NoSession_StillSucceeds(t *testing.T) {
	t.Parallel()

	handlers, _ := newTestHandlers(&mockAuthService{}, nil)

	r := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()

	handlers.Logout(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestLogout_ClearsJWTCookie(t *testing.T) {
	t.Parallel()

	handlers, sm := newTestHandlers(&mockAuthService{}, nil)

	r := httptest.NewRequest("POST", "/api/auth/logout", nil)
	r.Host = "example.com"
	sessionW := httptest.NewRecorder()
	_ = sm.CreateHTTPSession(sessionW, r, session.SessionData{
		SessionID: "sess-1", AgentID: "agent-1",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	r2 := httptest.NewRequest("POST", "/api/auth/logout", nil)
	for _, cookie := range sessionW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Logout(w, r2)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify JWT cookie is cleared with MaxAge -1.
	var jwtCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "pericarp_token" {
			jwtCookie = c
			break
		}
	}
	if jwtCookie == nil {
		t.Fatal("expected pericarp_token cookie to be set (for deletion)")
	}
	if jwtCookie.MaxAge != -1 {
		t.Errorf("JWT cookie MaxAge = %d, want -1", jwtCookie.MaxAge)
	}
}

// --- JWT handler tests ---

func TestCallback_WithJWT_SetsCookie(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	jwtSvc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	svc := &mockAuthService{
		issueIdentityTokenFunc: func(ctx context.Context, agent *entities.Agent, activeAccountID string) (string, error) {
			account, _ := new(entities.Account).With("account-1", "Test Account", entities.AccountTypePersonal)
			return jwtSvc.IssueToken(ctx, agent, []*entities.Account{account}, activeAccountID)
		},
	}

	handlers, sm := newTestHandlers(svc, nil)

	// Set up flow data
	r := httptest.NewRequest("GET", "/api/auth/callback?state=s&code=c", nil)
	r.Host = "example.com"
	flowW := httptest.NewRecorder()
	_ = sm.SetFlowData(flowW, r, session.FlowData{
		State: "s", Provider: "google", CreatedAt: time.Now(),
	})

	r2 := httptest.NewRequest("GET", "/api/auth/callback?state=s&code=c", nil)
	r2.Host = "example.com"
	for _, cookie := range flowW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Callback(w, r2)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify JWT cookie is set
	var jwtCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "pericarp_token" {
			jwtCookie = c
			break
		}
	}
	if jwtCookie == nil {
		t.Fatal("expected pericarp_token cookie, not found")
	}
	if jwtCookie.Value == "" {
		t.Error("JWT cookie value should not be empty")
	}
	if !jwtCookie.HttpOnly {
		t.Error("JWT cookie should be HttpOnly")
	}
	if !jwtCookie.Secure {
		t.Error("JWT cookie should be Secure")
	}
	if jwtCookie.MaxAge != 900 {
		t.Errorf("JWT cookie MaxAge = %d, want 900", jwtCookie.MaxAge)
	}

	// Validate the token content
	claims, validateErr := jwtSvc.ValidateToken(context.Background(), jwtCookie.Value)
	if validateErr != nil {
		t.Fatalf("JWT cookie contains invalid token: %v", validateErr)
	}
	if claims.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", claims.AgentID, "agent-1")
	}
}

func TestCallback_JWTIssueFails_StillRedirects(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{
		issueIdentityTokenFunc: func(_ context.Context, _ *entities.Agent, _ string) (string, error) {
			return "", errors.New("signing failure")
		},
	}

	handlers, sm := newTestHandlers(svc, nil)

	r := httptest.NewRequest("GET", "/api/auth/callback?state=s&code=c", nil)
	r.Host = "example.com"
	flowW := httptest.NewRecorder()
	_ = sm.SetFlowData(flowW, r, session.FlowData{
		State: "s", Provider: "google", CreatedAt: time.Now(),
	})

	r2 := httptest.NewRequest("GET", "/api/auth/callback?state=s&code=c", nil)
	r2.Host = "example.com"
	for _, cookie := range flowW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Callback(w, r2)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 (graceful degradation), got %d; body: %s", w.Code, w.Body.String())
	}

	// No JWT cookie should be set
	for _, c := range w.Result().Cookies() {
		if c.Name == "pericarp_token" {
			t.Error("expected no pericarp_token cookie when JWT issuance fails")
		}
	}
}

func TestCallback_NoJWTService_NoCookie(t *testing.T) {
	t.Parallel()

	handlers, sm := newTestHandlers(&mockAuthService{}, nil)

	r := httptest.NewRequest("GET", "/api/auth/callback?state=s&code=c", nil)
	r.Host = "example.com"
	flowW := httptest.NewRecorder()
	_ = sm.SetFlowData(flowW, r, session.FlowData{
		State: "s", Provider: "google", CreatedAt: time.Now(),
	})

	r2 := httptest.NewRequest("GET", "/api/auth/callback?state=s&code=c", nil)
	r2.Host = "example.com"
	for _, cookie := range flowW.Result().Cookies() {
		r2.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	handlers.Callback(w, r2)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d; body: %s", w.Code, w.Body.String())
	}

	for _, c := range w.Result().Cookies() {
		if c.Name == "pericarp_token" {
			t.Error("expected no pericarp_token cookie when JWTService is nil")
		}
	}
}
