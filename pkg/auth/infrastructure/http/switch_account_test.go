package authhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	authhttp "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/http"
	authjwt "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/jwt"
)

// mockAccountRepo implements repositories.AccountRepository for testing.
type mockAccountRepo struct {
	findMemberRoleFunc func(ctx context.Context, accountID, agentID string) (string, error)
}

func (m *mockAccountRepo) Save(_ context.Context, _ *entities.Account) error { return nil }
func (m *mockAccountRepo) FindByID(_ context.Context, _ string) (*entities.Account, error) {
	return nil, nil
}
func (m *mockAccountRepo) FindByMember(_ context.Context, _ string) ([]*entities.Account, error) {
	return nil, nil
}
func (m *mockAccountRepo) FindPersonalByMember(_ context.Context, _ string) (*entities.Account, error) {
	return nil, nil
}
func (m *mockAccountRepo) SaveMember(_ context.Context, _, _, _ string) error { return nil }
func (m *mockAccountRepo) FindAll(_ context.Context, _ string, _ int) (*repositories.PaginatedResponse[*entities.Account], error) {
	return nil, nil
}
func (m *mockAccountRepo) FindMemberRole(ctx context.Context, accountID, agentID string) (string, error) {
	if m.findMemberRoleFunc != nil {
		return m.findMemberRoleFunc(ctx, accountID, agentID)
	}
	return "", nil
}

// issueMultiAccountToken creates a JWT with multiple account memberships.
func issueMultiAccountToken(t *testing.T, svc *authjwt.RSAJWTService, activeAccountID string) string {
	t.Helper()
	agent, err := new(entities.Agent).With("agent-1", "Test User", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	acc1, err := new(entities.Account).With("acc-1", "Account One", entities.AccountTypePersonal)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}
	acc2, err := new(entities.Account).With("acc-2", "Account Two", entities.AccountTypeOrganization)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}
	tokenString, err := svc.IssueToken(context.Background(), agent, []*entities.Account{acc1, acc2}, activeAccountID, nil)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}
	return tokenString
}

// serveSwitchRequest is a helper that sends a POST to the switch-account handler
// with an optional Bearer token and JSON body.
func serveSwitchRequest(t *testing.T, handler http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/switch-account", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestSwitchActiveAccount_NoClaims_Returns401(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	handler := authhttp.SwitchActiveAccountHandler(svc, nil)

	// No middleware wrapping — no claims in context.
	r := httptest.NewRequest("POST", "/switch-account",
		strings.NewReader(`{"account_id":"acc-2"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSwitchActiveAccount_EmptyBody_Returns400(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	token := issueMultiAccountToken(t, svc, "acc-1")

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(authhttp.SwitchActiveAccountHandler(svc, nil))

	w := serveSwitchRequest(t, handler, token, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSwitchActiveAccount_MissingAccountID_Returns400(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	token := issueMultiAccountToken(t, svc, "acc-1")

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(authhttp.SwitchActiveAccountHandler(svc, nil))

	w := serveSwitchRequest(t, handler, token, `{"account_id":""}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSwitchActiveAccount_NotMember_Returns403(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	token := issueMultiAccountToken(t, svc, "acc-1")

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(authhttp.SwitchActiveAccountHandler(svc, nil))

	w := serveSwitchRequest(t, handler, token, `{"account_id":"acc-999"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestSwitchActiveAccount_HappyPath_NilRepo(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	token := issueMultiAccountToken(t, svc, "acc-1")

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(authhttp.SwitchActiveAccountHandler(svc, nil))

	w := serveSwitchRequest(t, handler, token, `{"account_id":"acc-2"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["active_account_id"] != "acc-2" {
		t.Errorf("active_account_id = %q, want %q", resp["active_account_id"], "acc-2")
	}
}

func TestSwitchActiveAccount_HappyPath_RepoConfirms(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	token := issueMultiAccountToken(t, svc, "acc-1")

	repo := &mockAccountRepo{
		findMemberRoleFunc: func(_ context.Context, _, _ string) (string, error) {
			return "member", nil
		},
	}

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(authhttp.SwitchActiveAccountHandler(svc, repo))

	w := serveSwitchRequest(t, handler, token, `{"account_id":"acc-2"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSwitchActiveAccount_RepoReturnsEmpty_Returns403(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	token := issueMultiAccountToken(t, svc, "acc-1")

	repo := &mockAccountRepo{
		findMemberRoleFunc: func(_ context.Context, _, _ string) (string, error) {
			return "", nil // membership revoked
		},
	}

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(authhttp.SwitchActiveAccountHandler(svc, repo))

	w := serveSwitchRequest(t, handler, token, `{"account_id":"acc-2"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestSwitchActiveAccount_RepoError_Returns500(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	token := issueMultiAccountToken(t, svc, "acc-1")

	repo := &mockAccountRepo{
		findMemberRoleFunc: func(_ context.Context, _, _ string) (string, error) {
			return "", errors.New("db connection failed")
		},
	}

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(authhttp.SwitchActiveAccountHandler(svc, repo))

	w := serveSwitchRequest(t, handler, token, `{"account_id":"acc-2"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestSwitchActiveAccount_SetsCookie(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	token := issueMultiAccountToken(t, svc, "acc-1")

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(authhttp.SwitchActiveAccountHandler(svc, nil))

	w := serveSwitchRequest(t, handler, token, `{"account_id":"acc-2"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	cookies := w.Result().Cookies()
	var jwtCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "pericarp_token" {
			jwtCookie = c
			break
		}
	}
	if jwtCookie == nil {
		t.Fatal("expected pericarp_token cookie to be set")
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
}

func TestSwitchActiveAccount_ReissuedTokenValid(t *testing.T) {
	t.Parallel()

	svc, _ := newTestJWTService(t)
	token := issueMultiAccountToken(t, svc, "acc-1")

	middleware := authhttp.RequireJWT(svc, "")
	handler := middleware(authhttp.SwitchActiveAccountHandler(svc, nil))

	w := serveSwitchRequest(t, handler, token, `{"account_id":"acc-2"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Extract the reissued token from the cookie.
	var jwtCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "pericarp_token" {
			jwtCookie = c
			break
		}
	}
	if jwtCookie == nil {
		t.Fatal("expected pericarp_token cookie")
	}

	// Validate the reissued token.
	claims, err := svc.ValidateToken(context.Background(), jwtCookie.Value)
	if err != nil {
		t.Fatalf("ValidateToken on reissued token failed: %v", err)
	}
	if claims.ActiveAccountID != "acc-2" {
		t.Errorf("ActiveAccountID = %q, want %q", claims.ActiveAccountID, "acc-2")
	}
	if claims.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", claims.AgentID, "agent-1")
	}
	if len(claims.AccountIDs) != 2 {
		t.Fatalf("AccountIDs length = %d, want 2", len(claims.AccountIDs))
	}
	if claims.AccountIDs[0] != "acc-1" || claims.AccountIDs[1] != "acc-2" {
		t.Errorf("AccountIDs = %v, want [acc-1, acc-2]", claims.AccountIDs)
	}
}
