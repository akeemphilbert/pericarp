package providers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

// newFacebookForTest constructs a Facebook provider whose endpoints are
// rerouted to the supplied httptest server. The auth dialog URL is also
// rewritten so AuthCodeURL output is deterministic for assertions.
func newFacebookForTest(cfg FacebookConfig, baseURL string) *Facebook {
	f := NewFacebook(cfg)
	f.authEndpoint = baseURL + "/dialog/oauth"
	f.tokenEndpoint = baseURL + "/oauth/access_token"
	f.userInfoEndpoint = baseURL + "/me"
	f.graphHost = baseURL
	return f
}

func TestFacebookAuthCodeURL(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NewServeMux())
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{
		ClientID:     "app-1",
		ClientSecret: "secret",
		Scopes:       []string{"email", "public_profile", "pages_show_list"},
	}, srv.URL)

	got := f.AuthCodeURL("state-xyz", "challenge-abc", "nonce-ignored", "https://example.com/cb")

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("AuthCodeURL returned unparseable URL: %v", err)
	}

	if parsed.Path != "/dialog/oauth" {
		t.Errorf("AuthCodeURL path = %q, want /dialog/oauth (a regression here would point users at the wrong endpoint)", parsed.Path)
	}

	q := parsed.Query()
	cases := map[string]string{
		"client_id":             "app-1",
		"redirect_uri":          "https://example.com/cb",
		"response_type":         "code",
		"scope":                 "email,public_profile,pages_show_list",
		"state":                 "state-xyz",
		"code_challenge":        "challenge-abc",
		"code_challenge_method": "S256",
	}
	for key, want := range cases {
		if got := q.Get(key); got != want {
			t.Errorf("AuthCodeURL %s = %q, want %q", key, got, want)
		}
	}
	if q.Has("nonce") {
		t.Error("AuthCodeURL should not emit a nonce parameter (Facebook Login is not OIDC)")
	}
}

func TestFacebookExchange_Success(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("token: method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("grant_type") != "authorization_code" {
			t.Errorf("token grant_type = %q, want authorization_code", form.Get("grant_type"))
		}
		if form.Get("code") != "the-code" {
			t.Errorf("token code = %q, want the-code", form.Get("code"))
		}
		if form.Get("code_verifier") != "the-verifier" {
			t.Errorf("token code_verifier = %q, want the-verifier", form.Get("code_verifier"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fb-access","token_type":"bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("userinfo: method = %s, want GET", r.Method)
		}
		// Token must come via the Authorization header, not the URL query —
		// otherwise it ends up in HTTP / proxy / referrer logs.
		if got, want := r.Header.Get("Authorization"), "Bearer fb-access"; got != want {
			t.Errorf("userinfo Authorization = %q, want %q", got, want)
		}
		if got := r.URL.Query().Get("access_token"); got != "" {
			t.Errorf("userinfo access_token leaked into URL query: %q", got)
		}
		if got := r.URL.Query().Get("fields"); got != "id,name,email,picture" {
			t.Errorf("userinfo fields = %q, want id,name,email,picture", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "1234567890",
			"name": "Ada Lovelace",
			"email": "ada@example.com",
			"picture": {"data": {"url": "https://cdn.example.com/ada.jpg"}}
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	result, err := f.Exchange(context.Background(), "the-code", "the-verifier", "https://example.com/cb")
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	if result.AccessToken != "fb-access" {
		t.Errorf("AccessToken = %q, want fb-access", result.AccessToken)
	}
	if result.RefreshToken != "" {
		t.Errorf("RefreshToken = %q, want empty (Facebook does not refresh)", result.RefreshToken)
	}
	if result.IDToken != "" {
		t.Errorf("IDToken = %q, want empty", result.IDToken)
	}
	if result.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn = %d, want 3600", result.ExpiresIn)
	}
	if result.UserInfo.ProviderUserID != "1234567890" {
		t.Errorf("ProviderUserID = %q, want 1234567890", result.UserInfo.ProviderUserID)
	}
	if result.UserInfo.Email != "ada@example.com" {
		t.Errorf("Email = %q, want ada@example.com", result.UserInfo.Email)
	}
	if result.UserInfo.DisplayName != "Ada Lovelace" {
		t.Errorf("DisplayName = %q, want Ada Lovelace", result.UserInfo.DisplayName)
	}
	if result.UserInfo.AvatarURL != "https://cdn.example.com/ada.jpg" {
		t.Errorf("AvatarURL = %q, want https://cdn.example.com/ada.jpg", result.UserInfo.AvatarURL)
	}
	if result.UserInfo.Provider != "facebook" {
		t.Errorf("Provider = %q, want facebook", result.UserInfo.Provider)
	}
}

func TestFacebookExchange_TokenError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"Invalid code","type":"OAuthException","code":100}}`, http.StatusBadRequest)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	_, err := f.Exchange(context.Background(), "bad", "verifier", "https://example.com/cb")
	if err == nil {
		t.Fatal("Exchange returned nil error for HTTP 400 token response")
	}
	if !strings.Contains(err.Error(), "facebook: token exchange failed") {
		t.Errorf("error message = %q, want it to wrap with 'facebook: token exchange failed'", err.Error())
	}
}

func TestFacebookExchange_UserInfoMissingID(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fb-access","token_type":"bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name": "no id here"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	_, err := f.Exchange(context.Background(), "code", "verifier", "https://example.com/cb")
	if err == nil {
		t.Fatal("Exchange returned nil error when /me response had no id")
	}
	if !strings.Contains(err.Error(), "userinfo response missing id") {
		t.Errorf("error message = %q, want it to mention missing id", err.Error())
	}
}

func TestFacebookRefreshToken_NotSupported(t *testing.T) {
	t.Parallel()

	f := NewFacebook(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"})

	result, err := f.RefreshToken(context.Background(), "anything")
	if result != nil {
		t.Errorf("RefreshToken result = %+v, want nil", result)
	}
	if !errors.Is(err, application.ErrTokenRefreshFailed) {
		t.Errorf("RefreshToken err = %v, want errors.Is == application.ErrTokenRefreshFailed", err)
	}
}

func TestFacebookValidateIDToken_NotSupported(t *testing.T) {
	t.Parallel()

	f := NewFacebook(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"})

	info, err := f.ValidateIDToken(context.Background(), "any.id.token", "nonce")
	if info != nil {
		t.Errorf("ValidateIDToken info = %+v, want nil", info)
	}
	if !errors.Is(err, ErrFacebookIDTokenUnsupported) {
		t.Errorf("ValidateIDToken err = %v, want errors.Is == ErrFacebookIDTokenUnsupported", err)
	}
}

func TestFacebookRevokeToken_Success(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/v18.0/me/permissions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("revoke method = %s, want DELETE", r.Method)
		}
		// Token must come via the Authorization header, not the URL query —
		// otherwise it ends up in HTTP / proxy / telemetry logs.
		if got, want := r.Header.Get("Authorization"), "Bearer fb-access"; got != want {
			t.Errorf("revoke Authorization = %q, want %q", got, want)
		}
		if got := r.URL.Query().Get("access_token"); got != "" {
			t.Errorf("revoke access_token leaked into URL query: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	if err := f.RevokeToken(context.Background(), "fb-access"); err != nil {
		t.Fatalf("RevokeToken returned error: %v", err)
	}
}

func TestFacebookRevokeToken_FailureResponse(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/v18.0/me/permissions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	err := f.RevokeToken(context.Background(), "fb-access")
	if err == nil {
		t.Fatal("RevokeToken returned nil error when API returned success=false")
	}
	if !strings.Contains(err.Error(), "success=false") {
		t.Errorf("error message = %q, want it to mention success=false", err.Error())
	}
}

func TestFacebookName(t *testing.T) {
	t.Parallel()

	f := NewFacebook(FacebookConfig{})
	if got := f.Name(); got != "facebook" {
		t.Errorf("Name() = %q, want facebook", got)
	}
}

func TestFacebookDefaultScopes(t *testing.T) {
	t.Parallel()

	f := NewFacebook(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"})
	if got, want := strings.Join(f.scopes, ","), "email,public_profile"; got != want {
		t.Errorf("default scopes = %q, want %q", got, want)
	}
}

func TestFacebookExchange_TokenResponseMissingAccessToken(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token_type":"bearer","expires_in":3600}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	_, err := f.Exchange(context.Background(), "code", "verifier", "https://example.com/cb")
	if err == nil {
		t.Fatal("Exchange returned nil error when token response had no access_token")
	}
	if !strings.Contains(err.Error(), "missing access_token") {
		t.Errorf("error = %q, want it to mention missing access_token", err.Error())
	}
}

func TestFacebookExchange_MalformedTokenJSON(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	_, err := f.Exchange(context.Background(), "code", "verifier", "https://example.com/cb")
	if err == nil {
		t.Fatal("Exchange returned nil error for malformed token JSON")
	}
	if !strings.Contains(err.Error(), "parse token response") {
		t.Errorf("error = %q, want parse token response", err.Error())
	}
}

func TestFacebookExchange_MalformedUserInfoJSON(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fb-access","token_type":"bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`also-not-json`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	_, err := f.Exchange(context.Background(), "code", "verifier", "https://example.com/cb")
	if err == nil {
		t.Fatal("Exchange returned nil error for malformed userinfo JSON")
	}
	if !strings.Contains(err.Error(), "parse userinfo response") {
		t.Errorf("error = %q, want parse userinfo response", err.Error())
	}
}

func TestFacebookRevokeToken_Non200Status(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/v18.0/me/permissions", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"Invalid OAuth access token","type":"OAuthException","code":190}}`, http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	err := f.RevokeToken(context.Background(), "fb-access")
	if err == nil {
		t.Fatal("RevokeToken returned nil error for HTTP 401")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Errorf("error = %q, want status 401", err.Error())
	}
}

func TestFacebookRevokeToken_UnparseableBody(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/v18.0/me/permissions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html>maintenance page</html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	err := f.RevokeToken(context.Background(), "fb-access")
	if err == nil {
		t.Fatal("RevokeToken returned nil error when 200 body was not JSON; this would be the silent-failure case")
	}
	if !strings.Contains(err.Error(), "unparseable body") {
		t.Errorf("error = %q, want it to mention unparseable body", err.Error())
	}
}

func TestFacebookRevokeToken_PassesTokenLiterally(t *testing.T) {
	t.Parallel()

	// A token containing characters that historically required URL encoding
	// when it lived in the query string. With the Authorization header it
	// must round-trip exactly; HTTP header values pass these chars through
	// without escaping.
	const tokenWithSpecialChars = "fb/access+token="

	mux := http.NewServeMux()
	mux.HandleFunc("/v18.0/me/permissions", func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer "+tokenWithSpecialChars; got != want {
			t.Errorf("revoke Authorization = %q, want %q", got, want)
		}
		// And the token must NOT have leaked back into the URL.
		if got := r.URL.Query().Get("access_token"); got != "" {
			t.Errorf("revoke access_token leaked into URL: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFacebookForTest(FacebookConfig{ClientID: "app-1", ClientSecret: "secret"}, srv.URL)

	if err := f.RevokeToken(context.Background(), tokenWithSpecialChars); err != nil {
		t.Fatalf("RevokeToken returned error: %v", err)
	}
}

// Compile-time check that *Facebook implements application.OAuthProvider.
var _ application.OAuthProvider = (*Facebook)(nil)
