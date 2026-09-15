package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

// newGoogleForTest constructs a Google provider whose token and userinfo
// endpoints are rerouted to the supplied httptest server.
func newGoogleForTest(baseURL string) *Google {
	g := NewGoogle(GoogleConfig{ClientID: "client-1", ClientSecret: "secret"})
	g.tokenEndpoint = baseURL + "/token"
	g.userInfoEndpoint = baseURL + "/userinfo"
	return g
}

// newGoogleServer answers the token endpoint with a fixed token response and
// the userinfo endpoint with userInfoBody.
func newGoogleServer(t *testing.T, userInfoBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"g-access","refresh_token":"g-refresh","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(userInfoBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGoogleUserInfo_EmailVerified(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		claim string // the email_verified member appended to the userinfo body
		want  bool
	}{
		{name: "claim true", claim: `,"email_verified":true`, want: true},
		{name: "claim false", claim: `,"email_verified":false`, want: false},
		{name: "claim absent", claim: ``, want: false},
	}
	calls := []struct {
		name string
		call func(context.Context, *Google) (*application.AuthResult, error)
	}{
		{name: "Exchange", call: func(ctx context.Context, g *Google) (*application.AuthResult, error) {
			return g.Exchange(ctx, "the-code", "the-verifier", "https://example.com/cb")
		}},
		{name: "RefreshToken", call: func(ctx context.Context, g *Google) (*application.AuthResult, error) {
			return g.RefreshToken(ctx, "g-refresh")
		}},
	}
	for _, c := range calls {
		for _, tc := range cases {
			t.Run(c.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				srv := newGoogleServer(t, `{"sub":"g-1","email":"user@example.com","name":"Test User"`+tc.claim+`}`)
				result, err := c.call(context.Background(), newGoogleForTest(srv.URL))
				if err != nil {
					t.Fatalf("%s: %v", c.name, err)
				}
				if got := result.UserInfo.EmailVerified; got != tc.want {
					t.Errorf("UserInfo.EmailVerified = %v, want %v", got, tc.want)
				}
				if got := result.UserInfo.Email; got != "user@example.com" {
					t.Errorf("UserInfo.Email = %q, want user@example.com", got)
				}
			})
		}
	}
}

func TestGoogleValidateIDToken_EmailVerified(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		claim   any
		present bool
		want    bool
	}{
		{name: "claim true", claim: true, present: true, want: true},
		{name: "claim false", claim: false, present: true, want: false},
		{name: "claim absent", present: false, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			claims := map[string]any{
				"iss":   "https://accounts.google.com",
				"aud":   "client-1",
				"exp":   time.Now().Add(time.Hour).Unix(),
				"nonce": "nonce-1",
				"sub":   "g-1",
				"email": "user@example.com",
			}
			if tc.present {
				claims["email_verified"] = tc.claim
			}
			g := NewGoogle(GoogleConfig{ClientID: "client-1", ClientSecret: "secret"})

			info, err := g.ValidateIDToken(context.Background(), unsignedIDToken(t, claims), "nonce-1")
			if err != nil {
				t.Fatalf("ValidateIDToken: %v", err)
			}
			if info.EmailVerified != tc.want {
				t.Errorf("UserInfo.EmailVerified = %v, want %v", info.EmailVerified, tc.want)
			}
		})
	}
}
