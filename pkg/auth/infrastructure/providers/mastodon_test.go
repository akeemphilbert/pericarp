package providers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

// fakeInstance simulates a single Mastodon instance with the OAuth and account
// endpoints the provider talks to. It tracks how many times /api/v1/apps was
// hit so the cache-per-host behaviour can be verified.
type fakeInstance struct {
	server         *httptest.Server
	host           string
	clientID       string
	clientSecret   string
	registerCalls  atomic.Int32
	tokenCalls     atomic.Int32
	verifyCalls    atomic.Int32
	revokeCalls    atomic.Int32
	authCode       string
	accessToken    string
	accountID      string
	accountName    string
	displayName    string
	avatarURL      string
	tokenStatus    int    // override; 0 means 200
	verifyStatus   int    // override; 0 means 200
	registerStatus int    // override; 0 means 200
	registerBody   string // override; empty means default JSON
}

func newFakeInstance(t *testing.T, host string) *fakeInstance {
	t.Helper()
	fi := &fakeInstance{
		host:         host,
		clientID:     "cid-" + host,
		clientSecret: "sec-" + host,
		authCode:     "code-" + host,
		accessToken:  "tok-" + host,
		accountID:    "acct-" + host,
		accountName:  "user_" + host,
		displayName:  "User at " + host,
		avatarURL:    "https://" + host + "/avatar.png",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		fi.registerCalls.Add(1)
		if fi.registerStatus != 0 {
			w.WriteHeader(fi.registerStatus)
			_, _ = io.WriteString(w, `{"error":"forced"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := fi.registerBody
		if body == "" {
			body = `{"client_id":"` + fi.clientID + `","client_secret":"` + fi.clientSecret + `","redirect_uri":"https://app.example.com/cb"}`
		}
		_, _ = io.WriteString(w, body)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		fi.tokenCalls.Add(1)
		if fi.tokenStatus != 0 {
			http.Error(w, `{"error":"invalid_grant"}`, fi.tokenStatus)
			return
		}
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("client_id") != fi.clientID {
			t.Errorf("token client_id at %q = %q, want %q", fi.host, form.Get("client_id"), fi.clientID)
		}
		if form.Get("code") != fi.authCode {
			t.Errorf("token code at %q = %q, want %q", fi.host, form.Get("code"), fi.authCode)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+fi.accessToken+`","token_type":"Bearer","scope":"read","created_at":1700000000}`)
	})
	mux.HandleFunc("/api/v1/accounts/verify_credentials", func(w http.ResponseWriter, r *http.Request) {
		fi.verifyCalls.Add(1)
		if fi.verifyStatus != 0 {
			http.Error(w, `{"error":"forced"}`, fi.verifyStatus)
			return
		}
		if got, want := r.Header.Get("Authorization"), "Bearer "+fi.accessToken; got != want {
			t.Errorf("verify auth header at %q = %q, want %q", fi.host, got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id": "`+fi.accountID+`",
			"username": "`+fi.accountName+`",
			"acct": "`+fi.accountName+`",
			"display_name": "`+fi.displayName+`",
			"avatar": "`+fi.avatarURL+`"
		}`)
	})
	mux.HandleFunc("/oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		fi.revokeCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	fi.server = httptest.NewServer(mux)
	t.Cleanup(fi.server.Close)
	return fi
}

// newMastodonForTest builds a Mastodon provider whose instanceBase resolver
// routes every host to the corresponding fakeInstance.server.URL. Hosts not in
// the map fail loud rather than escaping to the real internet.
func newMastodonForTest(cfg MastodonConfig, instances map[string]*fakeInstance) *Mastodon {
	m := NewMastodon(cfg)
	m.instanceBase = func(host string) string {
		if fi, ok := instances[host]; ok {
			return fi.server.URL
		}
		return "http://invalid.test.local/" + host
	}
	return m
}

func TestMastodonName(t *testing.T) {
	t.Parallel()
	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	if got := m.Name(); got != "mastodon" {
		t.Errorf("Name() = %q, want mastodon", got)
	}
}

func TestMastodonDefaultScopes(t *testing.T) {
	t.Parallel()
	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	if got, want := strings.Join(m.scopes, " "), "read"; got != want {
		t.Errorf("default scopes = %q, want %q", got, want)
	}
}

func TestMastodonAuthCodeURL_RequiresInstance(t *testing.T) {
	t.Parallel()
	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	got := m.AuthCodeURL("state", "challenge", "nonce", "https://app/cb")
	// AuthCodeURL must not silently return a usable URL — diagnostics should
	// land in the URL itself when the host-aware path is skipped.
	if !strings.HasPrefix(got, "about:blank#") {
		t.Errorf("AuthCodeURL = %q, want it to start with about:blank# when no instance is bound", got)
	}
	// url.QueryEscape encodes spaces as '+', not %20, so check for that form.
	if !strings.Contains(got, "instance+host+required") {
		t.Errorf("AuthCodeURL = %q, want it to surface the instance-required sentinel", got)
	}
}

// TestMastodonFederation_TwoInstances exercises the core acceptance criterion:
// a single Mastodon provider successfully completes the auth flow against two
// distinct instances, with each instance's app credentials cached separately.
func TestMastodonFederation_TwoInstances(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	hachy := newFakeInstance(t, "hachyderm.io")

	m := newMastodonForTest(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		Scopes:      []string{"read"},
		AppCache:    NewMemoryMastodonAppCache(),
	}, map[string]*fakeInstance{
		"mastodon.social": social,
		"hachyderm.io":    hachy,
	})

	for _, fi := range []*fakeInstance{social, hachy} {
		ctx := context.Background()
		verifier := "verifier-for-" + fi.host
		challenge := pkceChallenge(verifier)

		authURL, err := m.AuthCodeURLForInstance(ctx, fi.host, "state-"+fi.host, challenge, "", "https://app.example.com/cb")
		if err != nil {
			t.Fatalf("AuthCodeURLForInstance(%s): %v", fi.host, err)
		}
		parsed, err := url.Parse(authURL)
		if err != nil {
			t.Fatalf("AuthCodeURLForInstance returned unparseable URL for %s: %v", fi.host, err)
		}
		if parsed.Path != mastodonAuthorizePath {
			t.Errorf("authURL path for %s = %q, want %s", fi.host, parsed.Path, mastodonAuthorizePath)
		}
		if got := parsed.Query().Get("client_id"); got != fi.clientID {
			t.Errorf("authURL client_id for %s = %q, want %q", fi.host, got, fi.clientID)
		}

		result, err := m.Exchange(ctx, fi.authCode, verifier, "https://app.example.com/cb")
		if err != nil {
			t.Fatalf("Exchange(%s): %v", fi.host, err)
		}
		wantProviderID := fi.accountID + "@" + fi.host
		if result.UserInfo.ProviderUserID != wantProviderID {
			t.Errorf("ProviderUserID for %s = %q, want %q (federation must namespace IDs by host)", fi.host, result.UserInfo.ProviderUserID, wantProviderID)
		}
		if result.UserInfo.DisplayName != fi.displayName {
			t.Errorf("DisplayName for %s = %q, want %q", fi.host, result.UserInfo.DisplayName, fi.displayName)
		}
		if result.UserInfo.AvatarURL != fi.avatarURL {
			t.Errorf("AvatarURL for %s = %q, want %q", fi.host, result.UserInfo.AvatarURL, fi.avatarURL)
		}
		if result.UserInfo.Provider != "mastodon" {
			t.Errorf("Provider for %s = %q, want mastodon", fi.host, result.UserInfo.Provider)
		}
	}

	// Each instance was registered exactly once, despite this test running
	// two flows against each conceptually (one full + the other host alone).
	if got := social.registerCalls.Load(); got != 1 {
		t.Errorf("mastodon.social register calls = %d, want 1", got)
	}
	if got := hachy.registerCalls.Load(); got != 1 {
		t.Errorf("hachyderm.io register calls = %d, want 1", got)
	}
}

func TestMastodonAppCache_RegisteredOncePerHost(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	cache := NewMemoryMastodonAppCache()
	m := newMastodonForTest(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    cache,
	}, map[string]*fakeInstance{"mastodon.social": social})

	ctx := context.Background()
	for i := range 3 {
		verifier := "v" + string(rune('a'+i))
		_, err := m.AuthCodeURLForInstance(ctx, "mastodon.social", "state", pkceChallenge(verifier), "", "https://app.example.com/cb")
		if err != nil {
			t.Fatalf("AuthCodeURLForInstance call %d: %v", i, err)
		}
	}
	if got := social.registerCalls.Load(); got != 1 {
		t.Errorf("register calls = %d, want 1 (cache must reuse credentials across flows)", got)
	}
	cached, _ := cache.GetApp(ctx, "mastodon.social")
	if cached == nil || cached.ClientID != social.clientID {
		t.Errorf("cached app = %+v, want non-nil with client_id %q", cached, social.clientID)
	}
}

func TestMastodonExchange_NoFlowBinding(t *testing.T) {
	t.Parallel()

	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	_, err := m.Exchange(context.Background(), "code", "verifier-without-bind", "https://app/cb")
	if !errors.Is(err, ErrMastodonInstanceRequired) {
		t.Errorf("Exchange err = %v, want errors.Is == ErrMastodonInstanceRequired", err)
	}
}

func TestMastodonFlowBinding_ExpiredIsIgnored(t *testing.T) {
	t.Parallel()

	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	now := time.Now()
	m.nowFn = func() time.Time { return now }
	m.flowTTL = 5 * time.Minute

	verifier := "v"
	challenge := pkceChallenge(verifier)
	m.bindFlow(challenge, "mastodon.social")

	// Advance the clock past TTL.
	m.nowFn = func() time.Time { return now.Add(10 * time.Minute) }

	_, err := m.Exchange(context.Background(), "code", verifier, "https://app/cb")
	if !errors.Is(err, ErrMastodonInstanceRequired) {
		t.Errorf("expired flow err = %v, want ErrMastodonInstanceRequired", err)
	}
}

func TestMastodonFlowBinding_SingleUse(t *testing.T) {
	t.Parallel()

	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	verifier := "v"
	challenge := pkceChallenge(verifier)
	m.bindFlow(challenge, "mastodon.social")

	if host, ok := m.takeFlow(challenge); !ok || host != "mastodon.social" {
		t.Fatalf("first take = (%q, %v), want (mastodon.social, true)", host, ok)
	}
	if _, ok := m.takeFlow(challenge); ok {
		t.Error("second take returned true; flow bindings must be single-use to prevent code replay across instances")
	}
}

func TestMastodonRevokeToken_RequiresInstance(t *testing.T) {
	t.Parallel()
	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	if err := m.RevokeToken(context.Background(), "tok"); !errors.Is(err, ErrMastodonInstanceRequired) {
		t.Errorf("RevokeToken err = %v, want ErrMastodonInstanceRequired", err)
	}
}

func TestMastodonRevokeTokenAtInstance_Success(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	cache := NewMemoryMastodonAppCache()
	_ = cache.SetApp(context.Background(), "mastodon.social", &MastodonApp{ClientID: social.clientID, ClientSecret: social.clientSecret})

	m := newMastodonForTest(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    cache,
	}, map[string]*fakeInstance{"mastodon.social": social})

	if err := m.RevokeTokenAtInstance(context.Background(), "mastodon.social", social.accessToken); err != nil {
		t.Fatalf("RevokeTokenAtInstance: %v", err)
	}
	if got := social.revokeCalls.Load(); got != 1 {
		t.Errorf("revoke calls = %d, want 1", got)
	}
}

func TestMastodonValidateIDToken_NotSupported(t *testing.T) {
	t.Parallel()
	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	_, err := m.ValidateIDToken(context.Background(), "id.token", "nonce")
	if !errors.Is(err, ErrMastodonIDTokenUnsupported) {
		t.Errorf("err = %v, want ErrMastodonIDTokenUnsupported", err)
	}
}

func TestMastodonRefreshToken_NotSupported(t *testing.T) {
	t.Parallel()
	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	_, err := m.RefreshToken(context.Background(), "rt")
	if !errors.Is(err, application.ErrTokenRefreshFailed) {
		t.Errorf("err = %v, want ErrTokenRefreshFailed", err)
	}
}

func TestMastodonAuthCodeURLForInstance_RejectsMismatchedRedirect(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	m := newMastodonForTest(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    NewMemoryMastodonAppCache(),
	}, map[string]*fakeInstance{"mastodon.social": social})

	_, err := m.AuthCodeURLForInstance(context.Background(), "mastodon.social", "state", pkceChallenge("v"), "", "https://attacker.test/cb")
	if err == nil {
		t.Fatal("expected error when redirect URI does not match config")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Errorf("err = %q, want it to mention the mismatch", err.Error())
	}
}

func TestMastodonAuthCodeURLForInstance_EmptyHost(t *testing.T) {
	t.Parallel()

	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache(), RedirectURI: "https://app/cb"})
	_, err := m.AuthCodeURLForInstance(context.Background(), "", "state", pkceChallenge("v"), "", "")
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

func TestMastodonExchange_RegistrationGoneFromCache(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	// AppCache that drops everything: GetApp always returns nil. Simulates a
	// shared cache where a different replica's flow binding survived but the
	// app credential was evicted.
	cache := &droppingMastodonAppCache{inner: NewMemoryMastodonAppCache()}

	m := newMastodonForTest(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    cache,
	}, map[string]*fakeInstance{"mastodon.social": social})

	verifier := "v"
	challenge := pkceChallenge(verifier)
	// Manually bind without going through AuthCodeURLForInstance to avoid
	// populating the cache.
	m.bindFlow(challenge, "mastodon.social")

	_, err := m.Exchange(context.Background(), social.authCode, verifier, "https://app.example.com/cb")
	if err == nil {
		t.Fatal("expected error when app credentials are missing from cache")
	}
	if !strings.Contains(err.Error(), "no cached app") {
		t.Errorf("err = %q, want it to mention missing cached app", err.Error())
	}
}

// droppingMastodonAppCache wraps a real cache but always returns nil from GetApp.
type droppingMastodonAppCache struct{ inner MastodonAppCache }

func (d *droppingMastodonAppCache) GetApp(_ context.Context, _ string) (*MastodonApp, error) {
	return nil, nil
}
func (d *droppingMastodonAppCache) SetApp(ctx context.Context, host string, app *MastodonApp) error {
	return d.inner.SetApp(ctx, host, app)
}

func TestMastodonRegisterApp_ServerError(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	social.registerStatus = http.StatusInternalServerError
	m := newMastodonForTest(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    NewMemoryMastodonAppCache(),
	}, map[string]*fakeInstance{"mastodon.social": social})

	_, err := m.AuthCodeURLForInstance(context.Background(), "mastodon.social", "state", pkceChallenge("v"), "", "https://app.example.com/cb")
	if err == nil {
		t.Fatal("expected error when /api/v1/apps returns 500")
	}
	if !strings.Contains(err.Error(), "ensure app") {
		t.Errorf("err = %q, want it to wrap with 'ensure app'", err.Error())
	}
}

// Compile-time check that *Mastodon implements application.OAuthProvider.
var _ application.OAuthProvider = (*Mastodon)(nil)
