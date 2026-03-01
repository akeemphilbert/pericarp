package session_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/sessions"

	sess "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"
)

func newTestManager() *sess.GorillaSessionManager {
	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!"))
	return sess.NewGorillaSessionManager("test-session", store, sess.DefaultSessionOptions())
}

func TestDefaultSessionOptions(t *testing.T) {
	t.Parallel()

	opts := sess.DefaultSessionOptions()
	if opts.MaxAge != 86400 {
		t.Errorf("MaxAge = %d, want 86400", opts.MaxAge)
	}
	if opts.Path != "/" {
		t.Errorf("Path = %q, want %q", opts.Path, "/")
	}
	if !opts.HttpOnly {
		t.Error("expected HttpOnly to be true")
	}
	if !opts.Secure {
		t.Error("expected Secure to be true")
	}
	if opts.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %d, want %d", opts.SameSite, http.SameSiteLaxMode)
	}
}

func TestGorillaSessionManager_CreateAndGetHTTPSession(t *testing.T) {
	t.Parallel()

	mgr := newTestManager()
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)

	sessionData := sess.SessionData{
		SessionID: "sess-123",
		AgentID:   "agent-456",
		AccountID: "account-789",
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	// Create session
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := mgr.CreateHTTPSession(w, r, sessionData); err != nil {
		t.Fatalf("CreateHTTPSession() error: %v", err)
	}

	// Extract cookie from response
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie to be set")
	}

	// Read session back
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		r2.AddCookie(c)
	}

	data, err := mgr.GetHTTPSession(r2)
	if err != nil {
		t.Fatalf("GetHTTPSession() error: %v", err)
	}

	if data.SessionID != "sess-123" {
		t.Errorf("SessionID = %q, want %q", data.SessionID, "sess-123")
	}
	if data.AgentID != "agent-456" {
		t.Errorf("AgentID = %q, want %q", data.AgentID, "agent-456")
	}
	if data.AccountID != "account-789" {
		t.Errorf("AccountID = %q, want %q", data.AccountID, "account-789")
	}
	if data.CreatedAt.Unix() != now.Unix() {
		t.Errorf("CreatedAt = %v, want %v", data.CreatedAt.Unix(), now.Unix())
	}
	if data.ExpiresAt.Unix() != expiresAt.Unix() {
		t.Errorf("ExpiresAt = %v, want %v", data.ExpiresAt.Unix(), expiresAt.Unix())
	}
}

func TestGorillaSessionManager_GetHTTPSession_NotFound(t *testing.T) {
	t.Parallel()

	mgr := newTestManager()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := mgr.GetHTTPSession(r)
	if err == nil {
		t.Fatal("expected error when no session exists")
	}
}

func TestGorillaSessionManager_DestroyHTTPSession(t *testing.T) {
	t.Parallel()

	mgr := newTestManager()
	now := time.Now()

	sessionData := sess.SessionData{
		SessionID: "sess-123",
		AgentID:   "agent-456",
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
	}

	// Create session
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := mgr.CreateHTTPSession(w, r, sessionData); err != nil {
		t.Fatalf("CreateHTTPSession() error: %v", err)
	}

	cookies := w.Result().Cookies()

	// Destroy session
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		r2.AddCookie(c)
	}

	if err := mgr.DestroyHTTPSession(w2, r2); err != nil {
		t.Fatalf("DestroyHTTPSession() error: %v", err)
	}

	// Verify cookie is expired
	destroyCookies := w2.Result().Cookies()
	found := false
	for _, c := range destroyCookies {
		if c.Name == "test-session" {
			found = true
			if c.MaxAge >= 0 {
				t.Errorf("expected MaxAge < 0 for destroyed session, got %d", c.MaxAge)
			}
		}
	}
	if !found {
		t.Error("expected to find test-session cookie in destroy response")
	}
}

func TestGorillaSessionManager_SetAndGetFlowData(t *testing.T) {
	t.Parallel()

	mgr := newTestManager()
	now := time.Now()

	flowData := sess.FlowData{
		State:        "state-abc",
		CodeVerifier: "verifier-xyz",
		Nonce:        "nonce-123",
		Provider:     "google",
		RedirectURI:  "https://example.com/callback",
		CreatedAt:    now,
	}

	// Set flow data
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := mgr.SetFlowData(w, r, flowData); err != nil {
		t.Fatalf("SetFlowData() error: %v", err)
	}

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected flow session cookie to be set")
	}

	// Get flow data
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		r2.AddCookie(c)
	}

	data, err := mgr.GetFlowData(w2, r2)
	if err != nil {
		t.Fatalf("GetFlowData() error: %v", err)
	}

	if data.State != "state-abc" {
		t.Errorf("State = %q, want %q", data.State, "state-abc")
	}
	if data.CodeVerifier != "verifier-xyz" {
		t.Errorf("CodeVerifier = %q, want %q", data.CodeVerifier, "verifier-xyz")
	}
	if data.Nonce != "nonce-123" {
		t.Errorf("Nonce = %q, want %q", data.Nonce, "nonce-123")
	}
	if data.Provider != "google" {
		t.Errorf("Provider = %q, want %q", data.Provider, "google")
	}
	if data.RedirectURI != "https://example.com/callback" {
		t.Errorf("RedirectURI = %q, want %q", data.RedirectURI, "https://example.com/callback")
	}
	if data.CreatedAt.Unix() != now.Unix() {
		t.Errorf("CreatedAt = %v, want %v", data.CreatedAt.Unix(), now.Unix())
	}
}

func TestGorillaSessionManager_GetFlowData_NotFound(t *testing.T) {
	t.Parallel()

	mgr := newTestManager()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := mgr.GetFlowData(w, r)
	if err == nil {
		t.Fatal("expected error when no flow data exists")
	}
}

func TestGorillaSessionManager_GetFlowData_ClearsAfterRetrieval(t *testing.T) {
	t.Parallel()

	mgr := newTestManager()
	now := time.Now()

	flowData := sess.FlowData{
		State:        "state-abc",
		CodeVerifier: "verifier-xyz",
		Nonce:        "nonce-123",
		Provider:     "google",
		RedirectURI:  "https://example.com/callback",
		CreatedAt:    now,
	}

	// Set flow data
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := mgr.SetFlowData(w, r, flowData); err != nil {
		t.Fatalf("SetFlowData() error: %v", err)
	}

	cookies := w.Result().Cookies()

	// First retrieval should succeed
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		r2.AddCookie(c)
	}

	_, err := mgr.GetFlowData(w2, r2)
	if err != nil {
		t.Fatalf("GetFlowData() first call error: %v", err)
	}

	// Second retrieval with the destroy cookies should fail
	destroyCookies := w2.Result().Cookies()
	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range destroyCookies {
		r3.AddCookie(c)
	}

	_, err = mgr.GetFlowData(w3, r3)
	if err == nil {
		t.Error("expected error on second GetFlowData (should be cleared)")
	}
}

func TestGorillaSessionManager_CreateHTTPSession_EmptyAccountID(t *testing.T) {
	t.Parallel()

	mgr := newTestManager()
	now := time.Now()

	sessionData := sess.SessionData{
		SessionID: "sess-123",
		AgentID:   "agent-456",
		AccountID: "",
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := mgr.CreateHTTPSession(w, r, sessionData); err != nil {
		t.Fatalf("CreateHTTPSession() error: %v", err)
	}

	cookies := w.Result().Cookies()
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		r2.AddCookie(c)
	}

	data, err := mgr.GetHTTPSession(r2)
	if err != nil {
		t.Fatalf("GetHTTPSession() error: %v", err)
	}

	if data.AccountID != "" {
		t.Errorf("AccountID = %q, want empty string", data.AccountID)
	}
}
