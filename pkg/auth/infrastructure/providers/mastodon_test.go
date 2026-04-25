package providers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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
// routes every host to the corresponding fakeInstance.server.URL. Hosts not
// in the map panic instead of returning a synthetic URL: the previous
// "invalid.test.local" sentinel could still trigger real DNS lookups or
// proxied HTTP if a regression caused an unexpected host to slip through.
// Panicking turns any such regression into a loud test failure with no
// network fallback.
func newMastodonForTest(cfg MastodonConfig, instances map[string]*fakeInstance) *Mastodon {
	m := NewMastodon(cfg)
	m.instanceBase = func(host string) string {
		if fi, ok := instances[host]; ok {
			return fi.server.URL
		}
		panic("newMastodonForTest: unexpected host " + host + " (not in instances map)")
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

func TestMastodonAuthCodeURL_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	got := m.AuthCodeURL("state", "challenge", "nonce", "https://app/cb")
	// Returning empty is intentional: any non-empty URL would risk being
	// stuffed into a Location header by an unaware caller. Empty fails loud
	// at the first redirect attempt and forces callers onto the host-aware
	// AuthCodeURLForInstance path.
	if got != "" {
		t.Errorf("AuthCodeURL = %q, want empty string when no instance is bound", got)
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
		challenge := application.GenerateCodeChallenge(verifier)

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
		_, err := m.AuthCodeURLForInstance(ctx, "mastodon.social", "state", application.GenerateCodeChallenge(verifier), "", "https://app.example.com/cb")
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
	challenge := application.GenerateCodeChallenge(verifier)
	m.bindFlow(challenge, "mastodon.social")

	// Advance the clock past TTL.
	m.nowFn = func() time.Time { return now.Add(10 * time.Minute) }

	_, err := m.Exchange(context.Background(), "code", verifier, "https://app/cb")
	if !errors.Is(err, ErrMastodonFlowExpired) {
		t.Errorf("expired flow err = %v, want ErrMastodonFlowExpired (callers must distinguish expiry from never-bound)", err)
	}
}

func TestMastodonFlowBinding_SingleUse(t *testing.T) {
	t.Parallel()

	m := NewMastodon(MastodonConfig{AppCache: NewMemoryMastodonAppCache()})
	verifier := "v"
	challenge := application.GenerateCodeChallenge(verifier)
	m.bindFlow(challenge, "mastodon.social")

	if host, status := m.takeFlow(challenge); status != flowStatusOK || host != "mastodon.social" {
		t.Fatalf("first take = (%q, %v), want (mastodon.social, flowStatusOK)", host, status)
	}
	if _, status := m.takeFlow(challenge); status != flowStatusConsumed {
		t.Errorf("second take status = %v, want flowStatusConsumed (single-use must distinguish from missing)", status)
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

	_, err := m.AuthCodeURLForInstance(context.Background(), "mastodon.social", "state", application.GenerateCodeChallenge("v"), "", "https://attacker.test/cb")
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
	_, err := m.AuthCodeURLForInstance(context.Background(), "", "state", application.GenerateCodeChallenge("v"), "", "")
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
	challenge := application.GenerateCodeChallenge(verifier)
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

	_, err := m.AuthCodeURLForInstance(context.Background(), "mastodon.social", "state", application.GenerateCodeChallenge("v"), "", "https://app.example.com/cb")
	if err == nil {
		t.Fatal("expected error when /api/v1/apps returns 500")
	}
	if !strings.Contains(err.Error(), "ensure app") {
		t.Errorf("err = %q, want it to wrap with 'ensure app'", err.Error())
	}
}

func TestMastodonHostNormalization(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	cache := NewMemoryMastodonAppCache()
	m := newMastodonForTest(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    cache,
	}, map[string]*fakeInstance{"mastodon.social": social})

	ctx := context.Background()
	verifier := "v"
	challenge := application.GenerateCodeChallenge(verifier)

	// Different casing + whitespace must collapse to the same cache + binding.
	if _, err := m.AuthCodeURLForInstance(ctx, "  Mastodon.SOCIAL  ", "s", challenge, "", "https://app.example.com/cb"); err != nil {
		t.Fatalf("AuthCodeURLForInstance: %v", err)
	}
	if got := social.registerCalls.Load(); got != 1 {
		t.Errorf("first call register count = %d, want 1", got)
	}
	// Second call with canonical lowercase must hit the cache, not register again.
	if _, err := m.AuthCodeURLForInstance(ctx, "mastodon.social", "s2", application.GenerateCodeChallenge("v2"), "", "https://app.example.com/cb"); err != nil {
		t.Fatalf("AuthCodeURLForInstance second: %v", err)
	}
	if got := social.registerCalls.Load(); got != 1 {
		t.Errorf("after canonical-case call, register count = %d, want 1 (host normalization must collapse cache keys)", got)
	}

	// Exchange with the original challenge resolves user info, including the
	// normalized host in ProviderUserID.
	result, err := m.Exchange(ctx, social.authCode, verifier, "https://app.example.com/cb")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	want := social.accountID + "@mastodon.social"
	if result.UserInfo.ProviderUserID != want {
		t.Errorf("ProviderUserID = %q, want %q (normalized lowercase host)", result.UserInfo.ProviderUserID, want)
	}
}

func TestMastodonConcurrentRegistration_Singleflight(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	// Slow the registration handler so concurrent goroutines all collide
	// inside the singleflight critical section.
	slowMux := http.NewServeMux()
	slowMux.HandleFunc("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		social.registerCalls.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"client_id":"` + social.clientID + `","client_secret":"` + social.clientSecret + `"}`))
	})
	slowSrv := httptest.NewServer(slowMux)
	defer slowSrv.Close()

	cache := NewMemoryMastodonAppCache()
	m := NewMastodon(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    cache,
	})
	m.instanceBase = func(_ string) string { return slowSrv.URL }

	const N = 16
	var wg sync.WaitGroup
	wg.Add(N)
	errs := make(chan error, N)
	for i := range N {
		go func(i int) {
			defer wg.Done()
			verifier := "v" + string(rune('a'+i))
			_, err := m.AuthCodeURLForInstance(context.Background(), "mastodon.social", "s", application.GenerateCodeChallenge(verifier), "", "https://app.example.com/cb")
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("AuthCodeURLForInstance: %v", err)
		}
	}
	if got := social.registerCalls.Load(); got != 1 {
		t.Errorf("register calls = %d, want 1 (singleflight must dedupe concurrent first-flow registrations)", got)
	}
}

func TestMastodonExchange_TokenEndpointError(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	social.tokenStatus = http.StatusBadRequest
	m := newMastodonForTest(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    NewMemoryMastodonAppCache(),
	}, map[string]*fakeInstance{"mastodon.social": social})

	ctx := context.Background()
	verifier := "v"
	challenge := application.GenerateCodeChallenge(verifier)
	if _, err := m.AuthCodeURLForInstance(ctx, "mastodon.social", "s", challenge, "", "https://app.example.com/cb"); err != nil {
		t.Fatalf("AuthCodeURLForInstance: %v", err)
	}

	_, err := m.Exchange(ctx, social.authCode, verifier, "https://app.example.com/cb")
	if err == nil {
		t.Fatal("Exchange returned nil error when token endpoint returned 400")
	}
	if !strings.Contains(err.Error(), "token exchange") {
		t.Errorf("err = %q, want it to wrap with 'token exchange'", err.Error())
	}
}

func TestMastodonExchange_VerifyEndpointError(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	social.verifyStatus = http.StatusUnauthorized
	m := newMastodonForTest(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    NewMemoryMastodonAppCache(),
	}, map[string]*fakeInstance{"mastodon.social": social})

	ctx := context.Background()
	verifier := "v"
	challenge := application.GenerateCodeChallenge(verifier)
	if _, err := m.AuthCodeURLForInstance(ctx, "mastodon.social", "s", challenge, "", "https://app.example.com/cb"); err != nil {
		t.Fatalf("AuthCodeURLForInstance: %v", err)
	}

	_, err := m.Exchange(ctx, social.authCode, verifier, "https://app.example.com/cb")
	if err == nil {
		t.Fatal("Exchange returned nil error when verify endpoint returned 401")
	}
	if !strings.Contains(err.Error(), "fetch user info") {
		t.Errorf("err = %q, want it to wrap with 'fetch user info'", err.Error())
	}
}

func TestMastodonRegistrationMissingClientCredentials(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	social.registerBody = `{"client_id":"only-id-no-secret"}`
	m := newMastodonForTest(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    NewMemoryMastodonAppCache(),
	}, map[string]*fakeInstance{"mastodon.social": social})

	_, err := m.AuthCodeURLForInstance(context.Background(), "mastodon.social", "s", application.GenerateCodeChallenge("v"), "", "https://app.example.com/cb")
	if err == nil {
		t.Fatal("expected error when registration response was missing client_secret")
	}
	if !strings.Contains(err.Error(), "missing client credentials") {
		t.Errorf("err = %q, want it to mention missing client credentials", err.Error())
	}
}

func TestMastodonNilAppCache_FallsBackToMemory(t *testing.T) {
	t.Parallel()

	social := newFakeInstance(t, "mastodon.social")
	m := NewMastodon(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		// AppCache intentionally nil
	})
	m.instanceBase = func(_ string) string { return social.server.URL }

	if m.appCache == nil {
		t.Fatal("appCache is nil after construction with no AppCache; expected memory fallback")
	}
	if _, err := m.AuthCodeURLForInstance(context.Background(), "mastodon.social", "s", application.GenerateCodeChallenge("v"), "", "https://app.example.com/cb"); err != nil {
		t.Errorf("first flow with default cache: %v", err)
	}
}

func TestMastodonRegisterApp_SendsWebsite(t *testing.T) {
	t.Parallel()

	var capturedWebsite string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		capturedWebsite = form.Get("website")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"client_id":"cid","client_secret":"sec"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	m := NewMastodon(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    NewMemoryMastodonAppCache(),
		Website:     "https://app.example.com",
	})
	m.instanceBase = func(_ string) string { return srv.URL }

	if _, err := m.AuthCodeURLForInstance(context.Background(), "mastodon.social", "s", application.GenerateCodeChallenge("v"), "", "https://app.example.com/cb"); err != nil {
		t.Fatalf("AuthCodeURLForInstance: %v", err)
	}
	if capturedWebsite != "https://app.example.com" {
		t.Errorf("captured website = %q, want https://app.example.com", capturedWebsite)
	}
}

func TestMastodonRevokeTokenAtInstance_NoCachedApp(t *testing.T) {
	t.Parallel()

	m := NewMastodon(MastodonConfig{
		RedirectURI: "https://app.example.com/cb",
		AppCache:    NewMemoryMastodonAppCache(),
	})
	err := m.RevokeTokenAtInstance(context.Background(), "mastodon.social", "tok")
	if err == nil {
		t.Fatal("expected error when no app is cached for the host")
	}
	if !strings.Contains(err.Error(), "no cached app") {
		t.Errorf("err = %q, want it to mention missing cached app", err.Error())
	}
}

func TestMastodonRevokeTokenAtInstance_Non200(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/revoke", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid_token"}`, http.StatusForbidden)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewMemoryMastodonAppCache()
	_ = cache.SetApp(context.Background(), "mastodon.social", &MastodonApp{ClientID: "cid", ClientSecret: "sec"})
	m := NewMastodon(MastodonConfig{
		AppName:     "PericarpTest",
		RedirectURI: "https://app.example.com/cb",
		AppCache:    cache,
	})
	m.instanceBase = func(_ string) string { return srv.URL }

	err := m.RevokeTokenAtInstance(context.Background(), "mastodon.social", "tok")
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Errorf("err = %q, want status 403", err.Error())
	}
}

// Compile-time check that *Mastodon implements application.OAuthProvider.
var _ application.OAuthProvider = (*Mastodon)(nil)
