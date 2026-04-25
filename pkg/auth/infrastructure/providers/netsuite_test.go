package providers_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/providers"
)

func TestNetSuite_Name(t *testing.T) {
	t.Parallel()

	n := providers.NewNetSuite(providers.NetSuiteConfig{AccountID: "1234567"})
	if got := n.Name(); got != "netsuite" {
		t.Errorf("Name() = %q, want %q", got, "netsuite")
	}
}

func TestNetSuite_DefaultScopes(t *testing.T) {
	t.Parallel()

	n := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:  "client-id",
		AccountID: "1234567",
	})

	authURL := n.AuthCodeURL("state", "challenge", "nonce", "https://app.example.com/callback")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}

	if got := parsed.Query().Get("scope"); got != "restlets" {
		t.Errorf("default scope = %q, want %q", got, "restlets")
	}
}

func TestNetSuite_AuthCodeURL_DerivedHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		accountID string
		wantHost  string
	}{
		{name: "production account", accountID: "1234567", wantHost: "1234567.app.netsuite.com"},
		{name: "sandbox account normalized", accountID: "1234567_SB1", wantHost: "1234567-sb1.app.netsuite.com"},
		{name: "uppercase normalized", accountID: "ABC123", wantHost: "abc123.app.netsuite.com"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n := providers.NewNetSuite(providers.NetSuiteConfig{
				ClientID:  "client-id",
				AccountID: tt.accountID,
			})

			authURL := n.AuthCodeURL("state", "challenge", "nonce", "https://app.example.com/callback")
			parsed, err := url.Parse(authURL)
			if err != nil {
				t.Fatalf("parse auth URL: %v", err)
			}

			if parsed.Host != tt.wantHost {
				t.Errorf("auth host = %q, want %q", parsed.Host, tt.wantHost)
			}
			if parsed.Path != "/app/login/oauth2/authorize.nl" {
				t.Errorf("auth path = %q, want %q", parsed.Path, "/app/login/oauth2/authorize.nl")
			}
		})
	}
}

func TestNetSuite_AuthCodeURL_OverrideWinsOverDerived(t *testing.T) {
	t.Parallel()

	// AccountID is set AND override is set; override must win. This is the
	// trap the issue calls out: "don't fall through to derived only when
	// AccountID is empty — explicit overrides win".
	override := "https://sandbox.example.com/auth"
	n := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:     "client-id",
		AccountID:    "1234567",
		AuthEndpoint: override,
	})

	authURL := n.AuthCodeURL("state", "challenge", "nonce", "https://app.example.com/callback")
	if !strings.HasPrefix(authURL, override+"?") {
		t.Errorf("auth URL = %q, want prefix %q", authURL, override+"?")
	}
}

// TestNetSuite_TokenURL_OverrideWinsOverDerived verifies the override behavior
// for the token endpoint via the request actually sent to httptest.
func TestNetSuite_TokenURL_OverrideWinsOverDerived(t *testing.T) {
	t.Parallel()

	var hit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path
		writeTokenResponse(t, w, netSuiteTokenStub{AccessToken: "tok", TokenType: "Bearer", ExpiresIn: 3600})
	}))
	defer srv.Close()

	userSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeUserInfoResponse(t, w, netSuiteUserInfoStub{Sub: "u1", Email: "u@example.com"})
	}))
	defer userSrv.Close()

	// AccountID set AND TokenEndpoint override set; override must win.
	n := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:         "id",
		ClientSecret:     "secret",
		AccountID:        "1234567",
		TokenEndpoint:    srv.URL + "/token",
		UserInfoEndpoint: userSrv.URL,
	})

	if _, err := n.Exchange(context.Background(), "code", "verifier", "https://app.example.com/cb"); err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if hit != "/token" {
		t.Errorf("token endpoint hit = %q, want %q (override should win over derived URL)", hit, "/token")
	}
}

func TestNetSuite_Exchange_HappyPath(t *testing.T) {
	t.Parallel()

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("token method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("token Content-Type = %q, want %q", got, "application/x-www-form-urlencoded")
		}
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Errorf("token request missing Basic auth")
		}
		if user != "client-id" || pass != "client-secret" {
			t.Errorf("token Basic auth = (%q,%q), want (client-id,client-secret)", user, pass)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read token body: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if got := form.Get("grant_type"); got != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", got)
		}
		if got := form.Get("code"); got != "auth-code-xyz" {
			t.Errorf("code = %q, want auth-code-xyz", got)
		}
		if got := form.Get("code_verifier"); got != "verifier-abc" {
			t.Errorf("code_verifier = %q, want verifier-abc", got)
		}
		if got := form.Get("redirect_uri"); got != "https://app.example.com/cb" {
			t.Errorf("redirect_uri = %q, want https://app.example.com/cb", got)
		}

		writeTokenResponse(t, w, netSuiteTokenStub{
			AccessToken:  "access-123",
			RefreshToken: "refresh-456",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		})
	}))
	defer tokenSrv.Close()

	userSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("userinfo method = %q, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-123" {
			t.Errorf("userinfo Authorization = %q, want %q", got, "Bearer access-123")
		}
		writeUserInfoResponse(t, w, netSuiteUserInfoStub{
			Sub:   "user-internal-123",
			Email: "alice@example.com",
			Name:  "Alice Doe",
		})
	}))
	defer userSrv.Close()

	n := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:         "client-id",
		ClientSecret:     "client-secret",
		AccountID:        "1234567",
		TokenEndpoint:    tokenSrv.URL,
		UserInfoEndpoint: userSrv.URL,
	})

	res, err := n.Exchange(context.Background(), "auth-code-xyz", "verifier-abc", "https://app.example.com/cb")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if res.AccessToken != "access-123" {
		t.Errorf("AccessToken = %q, want access-123", res.AccessToken)
	}
	if res.RefreshToken != "refresh-456" {
		t.Errorf("RefreshToken = %q, want refresh-456", res.RefreshToken)
	}
	if res.UserInfo.ProviderUserID != "user-internal-123" {
		t.Errorf("ProviderUserID = %q, want user-internal-123", res.UserInfo.ProviderUserID)
	}
	if res.UserInfo.Email != "alice@example.com" {
		t.Errorf("Email = %q, want alice@example.com", res.UserInfo.Email)
	}
	if res.UserInfo.DisplayName != "Alice Doe" {
		t.Errorf("DisplayName = %q, want Alice Doe", res.UserInfo.DisplayName)
	}
	if res.UserInfo.Provider != "netsuite" {
		t.Errorf("Provider = %q, want netsuite", res.UserInfo.Provider)
	}
}

func TestNetSuite_Exchange_TokenEndpointError(t *testing.T) {
	t.Parallel()

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer tokenSrv.Close()

	n := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:      "id",
		ClientSecret:  "secret",
		AccountID:     "1234567",
		TokenEndpoint: tokenSrv.URL,
	})

	_, err := n.Exchange(context.Background(), "code", "verifier", "https://app.example.com/cb")
	if err == nil {
		t.Fatal("expected error from Exchange when token endpoint returns 400, got nil")
	}
	if !strings.Contains(err.Error(), "token exchange failed") {
		t.Errorf("error = %v, want it to mention token exchange failure", err)
	}
}

func TestNetSuite_RefreshToken_HappyPath(t *testing.T) {
	t.Parallel()

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if got := form.Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}
		if got := form.Get("refresh_token"); got != "old-refresh" {
			t.Errorf("refresh_token = %q, want old-refresh", got)
		}
		writeTokenResponse(t, w, netSuiteTokenStub{
			AccessToken:  "access-new",
			RefreshToken: "refresh-new",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		})
	}))
	defer tokenSrv.Close()

	userSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeUserInfoResponse(t, w, netSuiteUserInfoStub{Sub: "u1", Email: "u@example.com"})
	}))
	defer userSrv.Close()

	n := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:         "id",
		ClientSecret:     "secret",
		AccountID:        "1234567",
		TokenEndpoint:    tokenSrv.URL,
		UserInfoEndpoint: userSrv.URL,
	})

	res, err := n.RefreshToken(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if res.AccessToken != "access-new" {
		t.Errorf("AccessToken = %q, want access-new", res.AccessToken)
	}
	if res.RefreshToken != "refresh-new" {
		t.Errorf("RefreshToken = %q, want refresh-new", res.RefreshToken)
	}
}

func TestNetSuite_RefreshToken_PreservesOriginalWhenAbsent(t *testing.T) {
	t.Parallel()

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No refresh_token in the response body — caller should keep the old one.
		writeTokenResponse(t, w, netSuiteTokenStub{
			AccessToken: "access-rolled",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer tokenSrv.Close()

	userSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeUserInfoResponse(t, w, netSuiteUserInfoStub{Sub: "u1", Email: "u@example.com"})
	}))
	defer userSrv.Close()

	n := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:         "id",
		ClientSecret:     "secret",
		AccountID:        "1234567",
		TokenEndpoint:    tokenSrv.URL,
		UserInfoEndpoint: userSrv.URL,
	})

	res, err := n.RefreshToken(context.Background(), "original-refresh")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if res.RefreshToken != "original-refresh" {
		t.Errorf("RefreshToken = %q, want original-refresh (preserved)", res.RefreshToken)
	}
}

func TestNetSuite_RevokeToken_HappyPath(t *testing.T) {
	t.Parallel()

	revokeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("revoke method = %q, want POST", r.Method)
		}
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Errorf("revoke missing Basic auth")
		}
		if user != "id" || pass != "secret" {
			t.Errorf("revoke Basic auth = (%q,%q), want (id,secret)", user, pass)
		}
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if got := form.Get("token"); got != "to-revoke" {
			t.Errorf("token = %q, want to-revoke", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer revokeSrv.Close()

	n := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:       "id",
		ClientSecret:   "secret",
		AccountID:      "1234567",
		RevokeEndpoint: revokeSrv.URL,
	})

	if err := n.RevokeToken(context.Background(), "to-revoke"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
}

func TestNetSuite_RevokeToken_NonOKReturnsError(t *testing.T) {
	t.Parallel()

	revokeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer revokeSrv.Close()

	n := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:       "id",
		ClientSecret:   "secret",
		AccountID:      "1234567",
		RevokeEndpoint: revokeSrv.URL,
	})

	err := n.RevokeToken(context.Background(), "tok")
	if err == nil {
		t.Fatal("expected error from RevokeToken when revoke endpoint returns 401, got nil")
	}
	if !strings.Contains(err.Error(), "revoke failed with status 401") {
		t.Errorf("error = %v, want it to mention status 401", err)
	}
}

func TestNetSuite_ValidateIDToken_ReturnsSentinel(t *testing.T) {
	t.Parallel()

	n := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:  "id",
		AccountID: "1234567",
	})

	_, err := n.ValidateIDToken(context.Background(), "any.id.token", "nonce")
	if !errors.Is(err, providers.ErrNetSuiteIDTokenNotSupported) {
		t.Errorf("ValidateIDToken err = %v, want ErrNetSuiteIDTokenNotSupported", err)
	}
}

// --- helpers ---

type netSuiteTokenStub struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
}

type netSuiteUserInfoStub struct {
	Sub               string `json:"sub,omitempty"`
	Email             string `json:"email,omitempty"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
}

func writeTokenResponse(t *testing.T, w http.ResponseWriter, body netSuiteTokenStub) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encode token response: %v", err)
	}
}

func writeUserInfoResponse(t *testing.T, w http.ResponseWriter, body netSuiteUserInfoStub) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encode userinfo response: %v", err)
	}
}
