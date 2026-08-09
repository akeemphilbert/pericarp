package authhttp_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	authhttp "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/http"
	authjwt "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/jwt"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"
	"github.com/gorilla/sessions"
)

func TestRequireAuth_NoSession_Returns401(t *testing.T) {
	t.Parallel()

	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!!"))
	sm := session.NewGorillaSessionManager("test-session", store, session.DefaultSessionOptions())
	svc := &mockAuthService{}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware := authhttp.RequireAuth(sm, svc)
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if nextCalled {
		t.Error("next handler should NOT have been called")
	}
}

func TestRequireAuth_InvalidSession_Returns401(t *testing.T) {
	t.Parallel()

	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!!"))
	sm := session.NewGorillaSessionManager("test-session", store, session.DefaultSessionOptions())

	svc := &mockAuthService{
		validateSessFunc: func(_ context.Context, _ string) (*application.SessionInfo, error) {
			return nil, application.ErrSessionExpired
		},
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware := authhttp.RequireAuth(sm, svc)
	handler := middleware(next)

	// Create a session cookie
	r := httptest.NewRequest("GET", "/protected", nil)
	r.Host = "example.com"
	sessionW := httptest.NewRecorder()
	_ = sm.CreateHTTPSession(sessionW, r, session.SessionData{
		SessionID: "expired-sess",
		AgentID:   "agent-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})

	r2 := httptest.NewRequest("GET", "/protected", nil)
	for _, cookie := range sessionW.Result().Cookies() {
		r2.AddCookie(cookie)
	}
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r2)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if nextCalled {
		t.Error("next handler should NOT have been called")
	}
}

func TestRequireAuth_ValidSession_InjectsSessionInfo(t *testing.T) {
	t.Parallel()

	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!!"))
	sm := session.NewGorillaSessionManager("test-session", store, session.DefaultSessionOptions())

	expectedInfo := &application.SessionInfo{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		// A session must be account-scoped to authorize a request.
		AccountID: "acct-1",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	svc := &mockAuthService{
		validateSessFunc: func(_ context.Context, _ string) (*application.SessionInfo, error) {
			return expectedInfo, nil
		},
	}

	var capturedInfo *application.SessionInfo
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedInfo = authhttp.GetSessionInfo(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := authhttp.RequireAuth(sm, svc)
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	r.Host = "example.com"
	sessionW := httptest.NewRecorder()
	_ = sm.CreateHTTPSession(sessionW, r, session.SessionData{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	r2 := httptest.NewRequest("GET", "/protected", nil)
	for _, cookie := range sessionW.Result().Cookies() {
		r2.AddCookie(cookie)
	}
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r2)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedInfo == nil {
		t.Fatal("expected SessionInfo in context, got nil")
	}
	if capturedInfo.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", capturedInfo.AgentID, "agent-1")
	}
}

func TestGetSessionInfo_NoContext_ReturnsNil(t *testing.T) {
	t.Parallel()

	info := authhttp.GetSessionInfo(context.Background())
	if info != nil {
		t.Errorf("expected nil SessionInfo from empty context, got %v", info)
	}
}

// --- RequireJWT middleware tests ---

func newTestJWTService(t *testing.T) (*authjwt.RSAJWTService, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	return svc, key
}

func issueTestToken(t *testing.T, svc *authjwt.RSAJWTService) string {
	t.Helper()
	agent, err := new(entities.Agent).With("agent-1", "Test User", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	account, err := new(entities.Account).With("acc-1", "Account One", entities.AccountTypePersonal)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}
	tokenString, err := svc.IssueToken(context.Background(), agent, []*entities.Account{account}, "acc-1", nil, nil)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}
	return tokenString
}

func TestRequireJWT_NoToken_Returns401(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if nextCalled {
		t.Error("next handler should NOT have been called")
	}
}

func TestRequireJWT_InvalidToken_Returns401(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	r.Header.Set("Authorization", "Bearer garbage-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if nextCalled {
		t.Error("next handler should NOT have been called")
	}
}

func TestRequireJWT_ValidBearerToken_InjectsClaimsAndCallsNext(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	tokenString := issueTestToken(t, svc)

	var capturedClaims *application.PericarpClaims
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedClaims = authhttp.GetJWTClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	r.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedClaims == nil {
		t.Fatal("expected PericarpClaims in context, got nil")
	}
	if capturedClaims.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", capturedClaims.AgentID, "agent-1")
	}
	if capturedClaims.ActiveAccountID != "acc-1" {
		t.Errorf("ActiveAccountID = %q, want %q", capturedClaims.ActiveAccountID, "acc-1")
	}
}

func TestRequireJWT_PopulatesIdentitySubscription(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)

	agent, err := new(entities.Agent).With("agent-1", "Test User", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	subscription := &auth.SubscriptionClaim{
		Status:   auth.SubscriptionStatusActive,
		Plan:     "pro",
		Provider: "stripe",
	}
	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", subscription, nil)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	var capturedIdentity *auth.Identity
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIdentity = auth.AgentFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	r.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedIdentity == nil {
		t.Fatal("expected Identity in context, got nil")
	}
	if capturedIdentity.Subscription == nil {
		t.Fatal("expected Subscription on Identity, got nil")
	}
	if capturedIdentity.Subscription.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want %q", capturedIdentity.Subscription.Status, auth.SubscriptionStatusActive)
	}
	if !capturedIdentity.Subscription.IsActive() {
		t.Error("IsActive() = false, want true")
	}
}

func TestRequireJWT_ValidCookieToken_InjectsClaimsAndCallsNext(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	tokenString := issueTestToken(t, svc)

	var capturedClaims *application.PericarpClaims
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedClaims = authhttp.GetJWTClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := authhttp.RequireJWT(svc, "pericarp_token")
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	r.AddCookie(&http.Cookie{Name: "pericarp_token", Value: tokenString})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedClaims == nil {
		t.Fatal("expected PericarpClaims in context, got nil")
	}
	if capturedClaims.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", capturedClaims.AgentID, "agent-1")
	}
}

func TestRequireJWT_LowercaseBearer_Accepted(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	tokenString := issueTestToken(t, svc)

	var capturedClaims *application.PericarpClaims
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedClaims = authhttp.GetJWTClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	r.Header.Set("Authorization", "bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedClaims == nil {
		t.Fatal("expected PericarpClaims in context, got nil")
	}
	if capturedClaims.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", capturedClaims.AgentID, "agent-1")
	}
}

func TestGetJWTClaims_NoContext_ReturnsNil(t *testing.T) {
	t.Parallel()

	claims := authhttp.GetJWTClaims(context.Background())
	if claims != nil {
		t.Errorf("expected nil PericarpClaims from empty context, got %v", claims)
	}
}

// --- Identity injection tests ---

func TestRequireJWT_ValidToken_InjectsIdentity(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	tokenString := issueTestToken(t, svc)

	var capturedID *auth.Identity
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = auth.AgentFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	r.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedID == nil {
		t.Fatal("expected Identity in context, got nil")
	}
	if capturedID.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", capturedID.AgentID, "agent-1")
	}
	if capturedID.ActiveAccountID != "acc-1" {
		t.Errorf("ActiveAccountID = %q, want %q", capturedID.ActiveAccountID, "acc-1")
	}
	if len(capturedID.AccountIDs) != 1 || capturedID.AccountIDs[0] != "acc-1" {
		t.Errorf("AccountIDs = %v, want [acc-1]", capturedID.AccountIDs)
	}
}

func TestRequireAuth_ValidSession_InjectsIdentity(t *testing.T) {
	t.Parallel()

	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!!"))
	sm := session.NewGorillaSessionManager("test-session", store, session.DefaultSessionOptions())

	expectedInfo := &application.SessionInfo{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		AccountID: "acc-1",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	svc := &mockAuthService{
		validateSessFunc: func(_ context.Context, _ string) (*application.SessionInfo, error) {
			return expectedInfo, nil
		},
	}

	var capturedID *auth.Identity
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = auth.AgentFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := authhttp.RequireAuth(sm, svc)
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	r.Host = "example.com"
	sessionW := httptest.NewRecorder()
	_ = sm.CreateHTTPSession(sessionW, r, session.SessionData{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	r2 := httptest.NewRequest("GET", "/protected", nil)
	for _, cookie := range sessionW.Result().Cookies() {
		r2.AddCookie(cookie)
	}
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r2)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedID == nil {
		t.Fatal("expected Identity in context, got nil")
	}
	if capturedID.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", capturedID.AgentID, "agent-1")
	}
	if capturedID.ActiveAccountID != "acc-1" {
		t.Errorf("ActiveAccountID = %q, want %q", capturedID.ActiveAccountID, "acc-1")
	}
	if len(capturedID.AccountIDs) != 1 || capturedID.AccountIDs[0] != "acc-1" {
		t.Errorf("AccountIDs = %v, want [acc-1]", capturedID.AccountIDs)
	}
	if capturedID.Subscription != nil {
		t.Errorf("Identity.Subscription = %+v, want nil — sessions don't carry subscription state", capturedID.Subscription)
	}
}

func TestRequireJWT_NoToken_NoIdentity(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)

	var capturedID *auth.Identity
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = auth.AgentFromCtx(r.Context())
	})

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(next)

	r := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	// Handler should not have been called (401), so capturedID stays nil.
	if capturedID != nil {
		t.Errorf("expected nil Identity for unauthenticated request, got %v", capturedID)
	}
}

// --- Account scoping (#68) ---

// requestWithSessionCookie mints a session cookie for sessionID and returns a
// request carrying it.
func requestWithSessionCookie(t *testing.T, sm session.SessionManager, sessionID, agentID string) *http.Request {
	t.Helper()
	seed := httptest.NewRequest("GET", "/protected", nil)
	seed.Host = "example.com"
	w := httptest.NewRecorder()
	if err := sm.CreateHTTPSession(w, seed, session.SessionData{
		SessionID: sessionID,
		AgentID:   agentID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateHTTPSession: %v", err)
	}
	r := httptest.NewRequest("GET", "/protected", nil)
	for _, cookie := range w.Result().Cookies() {
		r.AddCookie(cookie)
	}
	return r
}

// Sessions stored before sessions carried an account are unscoped, and there
// is no backfill. They are refused so their holders sign in again.
func TestRequireAuth_UnscopedSession_Rejected(t *testing.T) {
	t.Parallel()

	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!!"))
	sm := session.NewGorillaSessionManager("test-session", store, session.DefaultSessionOptions())

	svc := &mockAuthService{
		validateSessFunc: func(_ context.Context, _ string) (*application.SessionInfo, error) {
			return &application.SessionInfo{
				SessionID: "sess-1",
				AgentID:   "agent-1",
				AccountID: "",
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
	}

	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	authhttp.RequireAuth(sm, svc)(next).ServeHTTP(w, requestWithSessionCookie(t, sm, "sess-1", "agent-1"))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if reached {
		t.Error("handler ran for an unscoped session")
	}
}

func TestRequireAuth_PopulatesEveryMembership(t *testing.T) {
	t.Parallel()

	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!!"))
	sm := session.NewGorillaSessionManager("test-session", store, session.DefaultSessionOptions())

	svc := &mockAuthService{
		validateSessFunc: func(_ context.Context, _ string) (*application.SessionInfo, error) {
			return &application.SessionInfo{
				SessionID:  "sess-1",
				AgentID:    "agent-1",
				AccountID:  "acct-1",
				AccountIDs: []string{"acct-1", "acct-2"},
				ExpiresAt:  time.Now().Add(24 * time.Hour),
			}, nil
		},
	}

	var got *auth.Identity
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.AgentFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	authhttp.RequireAuth(sm, svc)(next).ServeHTTP(w, requestWithSessionCookie(t, sm, "sess-1", "agent-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got == nil {
		t.Fatal("expected an identity on the request")
	}
	if got.ActiveAccountID != "acct-1" {
		t.Errorf("ActiveAccountID = %q, want %q", got.ActiveAccountID, "acct-1")
	}
	if len(got.AccountIDs) != 2 {
		t.Fatalf("AccountIDs = %v, want two entries", got.AccountIDs)
	}
	for _, id := range got.AccountIDs {
		if id == "" {
			t.Error("AccountIDs contains an empty value")
		}
	}
}

// A handler must be able to derive resource ownership — the symptom reported
// in #68 was ResourceOwnershipFromCtx failing with "empty ActiveAccountID".
func TestRequireAuth_ResourceOwnershipIsDerivable(t *testing.T) {
	t.Parallel()

	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!!"))
	sm := session.NewGorillaSessionManager("test-session", store, session.DefaultSessionOptions())

	svc := &mockAuthService{
		validateSessFunc: func(_ context.Context, _ string) (*application.SessionInfo, error) {
			return &application.SessionInfo{
				SessionID:  "sess-1",
				AgentID:    "agent-1",
				AccountID:  "acct-1",
				AccountIDs: []string{"acct-1"},
				ExpiresAt:  time.Now().Add(24 * time.Hour),
			}, nil
		},
	}

	var ownership auth.ResourceOwnership
	var ownershipErr error
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ownership, ownershipErr = auth.ResourceOwnershipFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	authhttp.RequireAuth(sm, svc)(next).ServeHTTP(w, requestWithSessionCookie(t, sm, "sess-1", "agent-1"))

	if ownershipErr != nil {
		t.Fatalf("ResourceOwnershipFromCtx() error: %v", ownershipErr)
	}
	if ownership.AccountID != "acct-1" || ownership.CreatedByAgentID != "agent-1" {
		t.Errorf("ownership = %+v, want account acct-1 created by agent-1", ownership)
	}
}

// The remedy for an unscoped session differs from an expired one: signing in
// again fixes an expiry and cannot fix a missing account. A client that retries
// login on a bare 401 loops forever, so this case carries its own code.
func TestRequireAuth_UnscopedSession_IsDistinguishable(t *testing.T) {
	t.Parallel()

	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!!"))
	sm := session.NewGorillaSessionManager("test-session", store, session.DefaultSessionOptions())

	unscoped := &mockAuthService{
		validateSessFunc: func(_ context.Context, _ string) (*application.SessionInfo, error) {
			return &application.SessionInfo{SessionID: "sess-1", AgentID: "agent-1", ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
	}
	expired := &mockAuthService{
		validateSessFunc: func(_ context.Context, _ string) (*application.SessionInfo, error) {
			return nil, application.ErrSessionExpired
		},
	}
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	decode := func(svc *mockAuthService) map[string]string {
		w := httptest.NewRecorder()
		authhttp.RequireAuth(sm, svc)(noop).ServeHTTP(w, requestWithSessionCookie(t, sm, "sess-1", "agent-1"))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		return body
	}

	if code := decode(unscoped)["code"]; code != "unscoped_session" {
		t.Errorf("unscoped session code = %q, want %q", code, "unscoped_session")
	}
	if code := decode(expired)["code"]; code == "unscoped_session" {
		t.Error("an expired session must not report itself as unscoped")
	}
}

// Signing in again DOES resolve a revoked membership — the next session is
// scoped to an account the agent still belongs to — so this must not be
// confused with unscoped_session, where signing in again never helps.
func TestRequireAuth_RevokedMembership_IsCodedSeparately(t *testing.T) {
	t.Parallel()

	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!!"))
	sm := session.NewGorillaSessionManager("test-session", store, session.DefaultSessionOptions())

	svc := &mockAuthService{
		validateSessFunc: func(_ context.Context, _ string) (*application.SessionInfo, error) {
			return nil, application.ErrSessionAccountRevoked
		},
	}

	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	authhttp.RequireAuth(sm, svc)(next).ServeHTTP(w, requestWithSessionCookie(t, sm, "sess-1", "agent-1"))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if reached {
		t.Error("handler ran for a session whose membership was revoked")
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["code"] != "account_access_revoked" {
		t.Errorf("code = %q, want %q", body["code"], "account_access_revoked")
	}
}
