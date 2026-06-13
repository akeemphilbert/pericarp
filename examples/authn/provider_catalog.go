// This file shows how a downstream service wires up the full Pericarp
// provider catalog. The constructors here are populated with placeholder
// configuration so the example compiles without real OAuth credentials —
// in production, populate every secret from your config/env loader.

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/providers"
)

// BuildProviderRegistry returns an OAuthProviderRegistry containing the
// full Pericarp-shipped provider catalog plus a mock provider used by the
// example authentication flow. Real consumers replace each *Config with
// values loaded from secrets management and drop the providers they don't
// need (along with the mock).
//
// The Mastodon and Bluesky entries use the in-memory app cache / keystore
// for demo purposes; multi-replica deployments must back both with shared
// stores so app credentials and DPoP signing keys survive restarts and span
// replicas.
func BuildProviderRegistry() application.OAuthProviderRegistry {
	mock := NewMockOAuthProvider("mock-idp")
	apple := providers.NewApple(providers.AppleConfig{
		ClientID:   "com.example.app.web",
		TeamID:     "TEAMID0000",
		KeyID:      "KEYID00000",
		PrivateKey: "-----BEGIN PRIVATE KEY-----\n<replace>\n-----END PRIVATE KEY-----",
	})
	github := providers.NewGitHub(providers.GitHubConfig{
		ClientID:     "Iv1.placeholder",
		ClientSecret: "placeholder",
	})
	google := providers.NewGoogle(providers.GoogleConfig{
		ClientID:     "1234.apps.googleusercontent.com",
		ClientSecret: "placeholder",
	})
	microsoft := providers.NewMicrosoft(providers.MicrosoftConfig{
		ClientID:     "placeholder-app-id",
		ClientSecret: "placeholder",
		TenantID:     "common",
	})
	facebook := providers.NewFacebook(providers.FacebookConfig{
		ClientID:     "1234567890",
		ClientSecret: "placeholder",
	})
	mastodon := providers.NewMastodon(providers.MastodonConfig{
		AppName:     "PericarpDemo",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    providers.NewMemoryMastodonAppCache(),
	})
	bluesky := providers.NewBluesky(providers.BlueskyConfig{
		ClientMetadataURL: "https://app.example.com/client-metadata.json",
		RedirectURI:       "https://app.example.com/cb",
		KeyStore:          providers.NewMemoryBlueskyKeyStore(),
	})
	netsuite := providers.NewNetSuite(providers.NetSuiteConfig{
		ClientID:     "your-netsuite-client-id",
		ClientSecret: "your-netsuite-client-secret",
		AccountID:    "1234567", // sandbox: "1234567_SB1" — auto-normalized to "1234567-sb1" in URLs
		// AuthEndpoint / TokenEndpoint / RevokeEndpoint / UserInfoEndpoint
		// can be set to point at a non-standard NetSuite host (e.g. a corporate
		// proxy or a future endpoint change). Each takes precedence over the
		// AccountID-derived URL when set.
	})

	return application.OAuthProviderRegistry{
		"mock-idp":       mock,
		apple.Name():     apple,
		github.Name():    github,
		google.Name():    google,
		microsoft.Name(): microsoft,
		facebook.Name():  facebook,
		mastodon.Name():  mastodon,
		bluesky.Name():   bluesky,
		netsuite.Name():  netsuite,
	}
}

// RunMastodonAgainstFake demonstrates an end-to-end Mastodon flow against a
// local httptest fake. It satisfies story #18's acceptance criterion that
// "at least one new provider has an end-to-end demo path that runs against
// a sandbox or fake without requiring real credentials."
//
// The fake instance auto-registers the app on first call (POST /api/v1/apps),
// accepts the authorization code at the token endpoint, and serves a
// verify_credentials response for the user. The full flow runs without any
// real Mastodon instance or network access.
//
// MastodonConfig.InstanceBase is the public seam used here — it routes
// every host lookup at the fake httptest server. Production deployments
// leave InstanceBase nil; staging mirrors set it to a fixed mirror URL.
//
// out is where the demo writes its narrative output (authorize URL, exchanged
// identity). main.go passes os.Stdout; tests pass io.Discard so parallel runs
// don't interleave with go test's progress lines. A nil out is also accepted
// as "discard all output."
func RunMastodonAgainstFake(ctx context.Context, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	fake := newFakeMastodonInstance()
	defer fake.Close()

	const fakeHost = "demo.mastodon.test"
	mastodon := providers.NewMastodon(providers.MastodonConfig{
		AppName:     "PericarpDemo",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    providers.NewMemoryMastodonAppCache(),
		InstanceBase: func(host string) string {
			// Route every host through the fake server.
			_ = host
			return fake.URL
		},
		// fakeHost ("demo.mastodon.test") doesn't resolve in DNS, so the
		// production SSRF guard would reject it. The demo legitimately
		// targets a local httptest server via InstanceBase, so opt out
		// of the guard the same way newMastodonForTest does.
		AllowInsecureInstanceHosts: true,
	})

	// Real services MUST generate state and the PKCE verifier with the
	// crypto/rand-backed helpers — hard-coded values would defeat both
	// CSRF protection and PKCE's purpose. This is the demo seam for that.
	state, err := application.GenerateState()
	if err != nil {
		return fmt.Errorf("GenerateState: %w", err)
	}
	verifier, err := application.GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("GenerateCodeVerifier: %w", err)
	}
	challenge := application.GenerateCodeChallenge(verifier)

	authURL, err := mastodon.AuthCodeURLForInstance(ctx, fakeHost, state, challenge, "", "https://app.example.com/cb")
	if err != nil {
		return fmt.Errorf("AuthCodeURLForInstance: %w", err)
	}
	_, _ = fmt.Fprintf(out, "[mastodon-demo] authorize URL: %s\n", authURL)

	result, err := mastodon.Exchange(ctx, "auth-code-demo", verifier, "https://app.example.com/cb")
	if err != nil {
		return fmt.Errorf("Exchange: %w", err)
	}
	_, _ = fmt.Fprintf(out, "[mastodon-demo] exchanged: provider_user_id=%s display_name=%s\n",
		result.UserInfo.ProviderUserID, result.UserInfo.DisplayName)

	if result.UserInfo.Provider != "mastodon" {
		return fmt.Errorf("expected provider mastodon, got %s", result.UserInfo.Provider)
	}
	return nil
}

// newFakeMastodonInstance serves the three endpoints the demo flow exercises:
// /api/v1/apps (auto-registration), /oauth/token (code exchange),
// /api/v1/accounts/verify_credentials (user info). /oauth/authorize is the
// user-facing browser redirect (not run by the demo) and /oauth/revoke is
// not exercised here — extending the demo to revocation would just add a
// 200 stub.
func newFakeMastodonInstance() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"client_id":"demo-cid","client_secret":"demo-sec","redirect_uri":"https://app.example.com/cb"}`)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"demo-access","token_type":"Bearer","scope":"read","created_at":1700000000}`)
	})
	mux.HandleFunc("/api/v1/accounts/verify_credentials", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"42","username":"alice","acct":"alice","display_name":"Alice (demo)","avatar":"https://demo.mastodon.test/a.png"}`)
	})
	return httptest.NewServer(mux)
}
