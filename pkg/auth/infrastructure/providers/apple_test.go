package providers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newAppleForTest constructs an Apple provider with a throwaway P-256 signing
// key, so the client-secret JWT can be minted, and its token endpoint rerouted
// to the supplied httptest server.
func newAppleForTest(t *testing.T, baseURL string) *Apple {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal signing key: %v", err)
	}
	a := NewApple(AppleConfig{
		ClientID:   "com.example.app.web",
		TeamID:     "TEAM123456",
		KeyID:      "KEY1234567",
		PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	a.tokenEndpoint = baseURL + "/auth/token"
	return a
}

// appleEmailVerifiedCases covers both shapes Apple sends email_verified in —
// a JSON boolean and a string — and its absence.
var appleEmailVerifiedCases = []struct {
	name    string
	claim   any
	present bool
	want    bool
}{
	{name: "boolean true", claim: true, present: true, want: true},
	{name: "boolean false", claim: false, present: true, want: false},
	{name: "string true", claim: "true", present: true, want: true},
	{name: "string false", claim: "false", present: true, want: false},
	{name: "absent", present: false, want: false},
}

func appleClaims(claim any, present bool) map[string]any {
	claims := map[string]any{
		"iss":             "https://appleid.apple.com",
		"aud":             "com.example.app.web",
		"exp":             time.Now().Add(time.Hour).Unix(),
		"nonce":           "nonce-1",
		"nonce_supported": true,
		"sub":             "apple-1",
		"email":           "user@example.com",
	}
	if present {
		claims["email_verified"] = claim
	}
	return claims
}

func TestAppleExchange_EmailVerified(t *testing.T) {
	t.Parallel()

	for _, tc := range appleEmailVerifiedCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idToken := unsignedIDToken(t, appleClaims(tc.claim, tc.present))
			mux := http.NewServeMux()
			mux.HandleFunc("/auth/token", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "a-access",
					"refresh_token": "a-refresh",
					"id_token":      idToken,
					"token_type":    "Bearer",
					"expires_in":    3600,
				})
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			result, err := newAppleForTest(t, srv.URL).Exchange(context.Background(), "the-code", "", "https://example.com/cb")
			if err != nil {
				t.Fatalf("Exchange: %v", err)
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

func TestAppleValidateIDToken_EmailVerified(t *testing.T) {
	t.Parallel()

	for _, tc := range appleEmailVerifiedCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := NewApple(AppleConfig{ClientID: "com.example.app.web"})
			info, err := a.ValidateIDToken(context.Background(), unsignedIDToken(t, appleClaims(tc.claim, tc.present)), "nonce-1")
			if err != nil {
				t.Fatalf("ValidateIDToken: %v", err)
			}
			if info.EmailVerified != tc.want {
				t.Errorf("UserInfo.EmailVerified = %v, want %v", info.EmailVerified, tc.want)
			}
		})
	}
}
