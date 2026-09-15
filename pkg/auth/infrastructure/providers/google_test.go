package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

// A userinfo response with no email_verified claim at all reads as unverified,
// which refuses the sign-in. One Debug line separates that case from a claim
// that said false, and it names the subject, never the email.
func TestGoogleUserInfo_AbsentEmailVerifiedClaim_LogsDebug(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		claim   string
		wantLog bool
	}{
		{name: "claim absent", claim: ``, wantLog: true},
		{name: "claim true", claim: `,"email_verified":true`, wantLog: false},
		{name: "claim false", claim: `,"email_verified":false`, wantLog: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			srv := newGoogleServer(t, `{"sub":"g-1","email":"user@example.com","name":"Test User"`+tc.claim+`}`)
			g := NewGoogle(GoogleConfig{ClientID: "client-1", ClientSecret: "secret", Logger: logger})
			g.tokenEndpoint = srv.URL + "/token"
			g.userInfoEndpoint = srv.URL + "/userinfo"

			if _, err := g.Exchange(context.Background(), "the-code", "the-verifier", "https://example.com/cb"); err != nil {
				t.Fatalf("Exchange: %v", err)
			}

			var records []map[string]any
			for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
				if len(line) == 0 {
					continue
				}
				record := map[string]any{}
				if err := json.Unmarshal(line, &record); err != nil {
					t.Fatalf("decode log line %q: %v", line, err)
				}
				records = append(records, record)
			}

			if !tc.wantLog {
				if len(records) != 0 {
					t.Errorf("logged %d lines, want none: %s", len(records), buf.String())
				}
				return
			}
			if len(records) != 1 {
				t.Fatalf("logged %d lines, want 1: %s", len(records), buf.String())
			}
			if got := records[0]["level"]; got != "DEBUG" {
				t.Errorf("level = %v, want DEBUG", got)
			}
			if got := records[0]["provider_user_id"]; got != "g-1" {
				t.Errorf("provider_user_id = %v, want g-1", got)
			}
			if strings.Contains(buf.String(), "user@example.com") {
				t.Errorf("debug line carries the email: %s", buf.String())
			}
		})
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
