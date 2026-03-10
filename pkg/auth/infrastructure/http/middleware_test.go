package authhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	authhttp "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/http"
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
