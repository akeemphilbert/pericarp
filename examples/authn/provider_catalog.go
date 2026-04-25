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

// BuildProviderRegistry returns an OAuthProviderRegistry containing all
// seven Pericarp-shipped providers. Real consumers replace each *Config with
// values loaded from secrets management.
//
// The Mastodon and Bluesky entries use the in-memory app cache / keystore
// for demo purposes; multi-replica deployments must back both with shared
// stores so app credentials and DPoP signing keys survive restarts and span
// replicas.
func BuildProviderRegistry() application.OAuthProviderRegistry {
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

	return application.OAuthProviderRegistry{
		apple.Name():     apple,
		github.Name():    github,
		google.Name():    google,
		microsoft.Name(): microsoft,
		facebook.Name():  facebook,
		mastodon.Name():  mastodon,
		bluesky.Name():   bluesky,
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
func RunMastodonAgainstFake(ctx context.Context) error {
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
	})

	state := "state-demo"
	verifier := "verifier-demo-1234567890123456789012345"
	challenge := application.GenerateCodeChallenge(verifier)

	authURL, err := mastodon.AuthCodeURLForInstance(ctx, fakeHost, state, challenge, "", "https://app.example.com/cb")
	if err != nil {
		return fmt.Errorf("AuthCodeURLForInstance: %w", err)
	}
	fmt.Printf("[mastodon-demo] authorize URL: %s\n", authURL)

	result, err := mastodon.Exchange(ctx, "auth-code-demo", verifier, "https://app.example.com/cb")
	if err != nil {
		return fmt.Errorf("Exchange: %w", err)
	}
	fmt.Printf("[mastodon-demo] exchanged: provider_user_id=%s display_name=%s\n",
		result.UserInfo.ProviderUserID, result.UserInfo.DisplayName)

	if result.UserInfo.Provider != "mastodon" {
		return fmt.Errorf("expected provider mastodon, got %s", result.UserInfo.Provider)
	}
	return nil
}

// newFakeMastodonInstance serves the four endpoints Mastodon's OAuth flow
// touches: /api/v1/apps, /oauth/token, /api/v1/accounts/verify_credentials,
// /oauth/revoke. The /oauth/authorize redirect is not exercised by the demo
// — that's the user-facing browser step.
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
