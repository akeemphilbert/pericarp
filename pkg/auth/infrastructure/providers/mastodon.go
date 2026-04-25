package providers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"golang.org/x/sync/singleflight"
)

// Mastodon endpoint paths (the host is provided per-flow by the caller).
const (
	mastodonAppRegistrationPath = "/api/v1/apps"
	mastodonAuthorizePath       = "/oauth/authorize"
	mastodonTokenPath           = "/oauth/token"
	mastodonRevokePath          = "/oauth/revoke"
	mastodonVerifyPath          = "/api/v1/accounts/verify_credentials"

	// mastodonFlowTTL bounds how long a (codeChallenge -> host) binding is
	// kept while the user is at the instance authorising. 10 min is well
	// above any normal auth dialog.
	mastodonFlowTTL = 10 * time.Minute
)

// Mastodon-specific sentinel errors. Callers MUST distinguish these via
// errors.Is — generic "any non-nil error from ValidateIDToken means failed
// login" handlers would incorrectly reject every Mastodon login.
//
// All four flow-state sentinels below are PERMANENT for the failing flow:
// callers must NOT retry the same Exchange call. Start a fresh flow via
// AuthCodeURLForInstance instead.
var (
	// ErrMastodonInstanceRequired is returned by AuthCodeURL/Exchange when
	// no instance host has ever been bound to the flow. Caller misuse
	// (forgot to call AuthCodeURLForInstance) — permanent.
	ErrMastodonInstanceRequired = errors.New("mastodon: instance host required; use AuthCodeURLForInstance to start a flow")

	// ErrMastodonFlowExpired is returned by Exchange when the flow binding
	// existed but TTL'd before the user completed the auth dialog. Permanent.
	ErrMastodonFlowExpired = errors.New("mastodon: flow binding expired before code was exchanged; start a fresh flow")

	// ErrMastodonFlowAlreadyConsumed is returned by Exchange when the flow
	// binding has already been consumed by an earlier Exchange call (e.g. a
	// duplicate callback). Permanent — single-use is intentional to prevent
	// authorization-code replay across instances.
	ErrMastodonFlowAlreadyConsumed = errors.New("mastodon: flow binding already consumed; start a fresh flow")

	// ErrMastodonIDTokenUnsupported is returned by ValidateIDToken because
	// Mastodon does not issue OIDC ID tokens. Resolve identity via Exchange.
	ErrMastodonIDTokenUnsupported = errors.New("mastodon: ID tokens are not issued by Mastodon; use Exchange to resolve user info")
)

// MastodonApp holds an instance's per-app credentials. One app is registered
// per (host, AppName, RedirectURI) tuple and reused across all flows for that
// host.
type MastodonApp struct {
	ClientID     string
	ClientSecret string
}

// MastodonAppCache stores Mastodon app registration credentials keyed by
// instance host. Implementations must be safe for concurrent use.
//
// Single-replica deployments can use NewMemoryMastodonAppCache. Multi-replica
// deployments should back this with a shared store (Redis, DynamoDB, etc.) so
// every replica reuses the registration that one replica created — Mastodon's
// /api/v1/apps endpoint creates a new app on every call, and abandoned apps
// accumulate forever.
type MastodonAppCache interface {
	GetApp(ctx context.Context, host string) (*MastodonApp, error)
	SetApp(ctx context.Context, host string, app *MastodonApp) error
}

// NewMemoryMastodonAppCache returns an in-memory MastodonAppCache suitable for
// single-replica deployments and tests.
func NewMemoryMastodonAppCache() MastodonAppCache {
	return &memoryMastodonAppCache{}
}

type memoryMastodonAppCache struct {
	apps sync.Map // host -> *MastodonApp
}

func (m *memoryMastodonAppCache) GetApp(_ context.Context, host string) (*MastodonApp, error) {
	v, ok := m.apps.Load(host)
	if !ok {
		return nil, nil
	}
	return v.(*MastodonApp), nil
}

func (m *memoryMastodonAppCache) SetApp(_ context.Context, host string, app *MastodonApp) error {
	m.apps.Store(host, app)
	return nil
}

// MastodonConfig configures the Mastodon OAuth provider. AppCache is required.
type MastodonConfig struct {
	// AppName is the client_name registered at each instance.
	AppName string
	// RedirectURI is the OAuth redirect URI registered at each instance. It
	// must match the redirectURI passed to AuthCodeURLForInstance and Exchange.
	RedirectURI string
	// Scopes default to ["read"]. Mastodon does not expose user email via
	// its public API regardless of scope, so UserInfo.Email is left empty.
	Scopes []string
	// AppCache stores per-instance app credentials. If nil, an in-memory
	// cache is used — this is only safe for single-replica deployments and
	// tests because Mastodon's /api/v1/apps endpoint creates a new app on
	// every call. Multi-replica deployments MUST supply a shared backing
	// store (Redis, DynamoDB, etc.) so every replica reuses one registration
	// per instance host instead of leaving abandoned upstream apps behind.
	AppCache MastodonAppCache
	// Website is optional; populated in the app registration request.
	Website string
	// InstanceBase resolves a host (e.g. "mastodon.social") to a base URL
	// (e.g. "https://mastodon.social"). Defaults to "https://" + host.
	// Override to point flows at a staging mirror, a corporate proxy, or an
	// httptest server for end-to-end fakes. Production deployments should
	// leave this nil unless they have a specific routing need.
	InstanceBase func(host string) string
}

// flowBinding ties a codeChallenge to the instance host the user was redirected
// at. Exchange recomputes the challenge from codeVerifier to look this up.
type flowBinding struct {
	host      string
	expiresAt time.Time
}

// Mastodon implements application.OAuthProvider with per-instance federation.
type Mastodon struct {
	appName     string
	redirectURI string
	scopes      []string
	website     string
	appCache    MastodonAppCache
	httpClient  *http.Client

	flowBindings sync.Map // codeChallenge -> *flowBinding
	flowTTL      time.Duration

	// flowConsumed records challenges that have already been taken so a
	// duplicate Exchange call can return ErrMastodonFlowAlreadyConsumed
	// instead of conflating with "never bound." TTL'd via tombstoneTTL.
	flowConsumed sync.Map // codeChallenge -> consumedAt
	tombstoneTTL time.Duration
	registerSF   singleflight.Group // dedupes concurrent /api/v1/apps registrations per host
	bindCounter  atomic.Uint64      // drives probabilistic GC sweeps in bindFlow

	// instanceBase resolves a host (e.g. "mastodon.social") to a base URL
	// (e.g. "https://mastodon.social"). Tests override it to point at an
	// httptest server.
	instanceBase func(host string) string

	// nowFn is overridable for deterministic flow-binding TTL tests.
	nowFn func() time.Time
}

// NewMastodon creates a Mastodon provider. AppCache is required.
func NewMastodon(config MastodonConfig) *Mastodon {
	scopes := config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"read"}
	}
	cache := config.AppCache
	if cache == nil {
		cache = NewMemoryMastodonAppCache()
	}
	instanceBase := config.InstanceBase
	if instanceBase == nil {
		instanceBase = func(host string) string { return "https://" + strings.ToLower(host) }
	}
	return &Mastodon{
		appName:      config.AppName,
		redirectURI:  config.RedirectURI,
		scopes:       scopes,
		website:      config.Website,
		appCache:     cache,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		flowTTL:      mastodonFlowTTL,
		tombstoneTTL: mastodonFlowTTL, // tombstone lives at least as long as a binding could
		instanceBase: instanceBase,
		nowFn:        time.Now,
	}
}

// normalizeHost lowercases and trims the instance host so `Mastodon.Social`
// and `mastodon.social ` collapse to the same key. DNS hostnames are
// case-insensitive — without this, the same user signing in via two
// differently-cased URLs would be issued two distinct credentials, and the
// MastodonAppCache would register two upstream apps. This is enforced at
// every entry point that accepts a host: AuthCodeURLForInstance, Exchange,
// RevokeTokenAtInstance.
func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

// validateMastodonHost rejects values that aren't a bare DNS hostname.
// AuthCodeURLForInstance feeds host straight into instanceBase(host), which by
// default builds "https://" + host; without this check, a caller passing
// "evil.com/path", "internal:8080", "user@host", or "http://internal" can turn
// a Mastodon flow into an SSRF vector or produce malformed authorize URLs.
func validateMastodonHost(host string) error {
	if host == "" {
		return fmt.Errorf("mastodon: host must not be empty")
	}
	// '/' (paths), ':' (ports/scheme delimiter), '@' (userinfo), '?'/'#'
	// (query/fragment), and whitespace are all illegal in a bare hostname.
	if strings.ContainsAny(host, "/:@?# \t\r\n") {
		return fmt.Errorf("mastodon: host %q must be a bare DNS hostname (no scheme, port, path, or userinfo)", host)
	}
	return nil
}

// Name returns the provider identifier.
func (m *Mastodon) Name() string {
	return "mastodon"
}

// AuthCodeURL satisfies application.OAuthProvider, but cannot start a real
// Mastodon flow because the instance host is not known at this signature. It
// returns the empty string. Callers MUST detect empty-string and route to
// AuthCodeURLForInstance (a non-empty URL going through this method would be
// a Mastodon misuse and dangerous if any handler put it in a Location
// header — see Exchange, which returns ErrMastodonInstanceRequired so the
// downstream error path can be programmatically detected).
func (m *Mastodon) AuthCodeURL(_, _, _, _ string) string {
	return ""
}

// AuthCodeURLForInstance registers (or fetches a cached) app at the given
// Mastodon instance, binds the flow to that host, and returns the authorize URL.
//
// host is the bare instance domain (e.g. "mastodon.social", "hachyderm.io").
// It must match the host the consumer will route Exchange against. The
// codeChallenge is used as the flow's primary key — Exchange recomputes it
// from codeVerifier and looks the host back up.
//
// The redirectURI must match MastodonConfig.RedirectURI; mismatched URIs would
// cause Mastodon to reject the callback and would also bind the wrong app.
func (m *Mastodon) AuthCodeURLForInstance(ctx context.Context, host, state, codeChallenge, _, redirectURI string) (string, error) {
	host = normalizeHost(host)
	if err := validateMastodonHost(host); err != nil {
		return "", err
	}
	// codeChallenge keys the flow binding; an empty key would collide every
	// concurrent flow into the same slot and let the first-finishing Exchange
	// route a code at the wrong instance.
	if codeChallenge == "" {
		return "", fmt.Errorf("mastodon: codeChallenge must not be empty")
	}
	if redirectURI != "" && redirectURI != m.redirectURI {
		return "", fmt.Errorf("mastodon: redirectURI %q does not match configured RedirectURI %q", redirectURI, m.redirectURI)
	}
	if redirectURI == "" {
		redirectURI = m.redirectURI
	}

	app, err := m.ensureApp(ctx, host)
	if err != nil {
		return "", fmt.Errorf("mastodon: ensure app at %q: %w", host, err)
	}

	m.bindFlow(codeChallenge, host)

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {app.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(m.scopes, " ")},
		"state":                 {state},
		"force_login":           {"false"},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}

	return m.instanceBase(host) + mastodonAuthorizePath + "?" + params.Encode(), nil
}

// Exchange swaps the authorization code for an access token at the same
// instance the auth URL pointed at, then resolves user info via
// /api/v1/accounts/verify_credentials. The instance is recovered from the
// flow binding keyed by sha256(codeVerifier) (which equals codeChallenge).
//
// Returns one of three distinguishable PERMANENT sentinels on flow-state
// failures: ErrMastodonInstanceRequired (no binding ever existed),
// ErrMastodonFlowExpired (binding TTL'd), or ErrMastodonFlowAlreadyConsumed
// (duplicate Exchange). None of these should be retried — callers must start
// a fresh flow via AuthCodeURLForInstance.
func (m *Mastodon) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (*application.AuthResult, error) {
	challenge := pkceChallenge(codeVerifier)
	host, status := m.takeFlow(challenge)
	switch status {
	case flowStatusOK:
		// proceed
	case flowStatusExpired:
		return nil, ErrMastodonFlowExpired
	case flowStatusConsumed:
		return nil, ErrMastodonFlowAlreadyConsumed
	default:
		return nil, ErrMastodonInstanceRequired
	}
	if redirectURI == "" {
		redirectURI = m.redirectURI
	}

	app, err := m.appCache.GetApp(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("mastodon: load app for %q: %w", host, err)
	}
	if app == nil {
		return nil, fmt.Errorf("mastodon: no cached app for instance %q (was AuthCodeURLForInstance called?)", host)
	}

	tokenResp, err := m.requestToken(ctx, host, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {app.ClientID},
		"client_secret": {app.ClientSecret},
		"redirect_uri":  {redirectURI},
		"code":          {code},
		"code_verifier": {codeVerifier},
		"scope":         {strings.Join(m.scopes, " ")},
	})
	if err != nil {
		return nil, fmt.Errorf("mastodon: token exchange at %q: %w", host, err)
	}

	userInfo, err := m.fetchUserInfo(ctx, host, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("mastodon: fetch user info at %q: %w", host, err)
	}

	return &application.AuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		UserInfo:     *userInfo,
	}, nil
}

// RefreshToken returns application.ErrTokenRefreshFailed because Mastodon's
// standard OAuth flow does not issue refresh tokens. Like Facebook, this is a
// permanent capability mismatch — callers SHOULD NOT retry.
func (m *Mastodon) RefreshToken(_ context.Context, _ string) (*application.AuthResult, error) {
	return nil, application.ErrTokenRefreshFailed
}

// RevokeToken cannot be supported through the OAuthProvider.RevokeToken
// signature because the instance host is not present. Use RevokeTokenAtInstance.
func (m *Mastodon) RevokeToken(_ context.Context, _ string) error {
	return ErrMastodonInstanceRequired
}

// RevokeTokenAtInstance revokes a token at the given Mastodon instance.
func (m *Mastodon) RevokeTokenAtInstance(ctx context.Context, host, token string) error {
	host = normalizeHost(host)
	app, err := m.appCache.GetApp(ctx, host)
	if err != nil {
		return fmt.Errorf("mastodon: load app for %q: %w", host, err)
	}
	if app == nil {
		return fmt.Errorf("mastodon: no cached app for instance %q", host)
	}

	data := url.Values{
		"client_id":     {app.ClientID},
		"client_secret": {app.ClientSecret},
		"token":         {token},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.instanceBase(host)+mastodonRevokePath, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("mastodon: failed to create revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mastodon: revoke request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("mastodon: read revoke response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mastodon: revoke at %q failed with status %d: %s", host, resp.StatusCode, string(body))
	}
	return nil
}

// ValidateIDToken returns ErrMastodonIDTokenUnsupported because Mastodon does
// not issue ID tokens. Resolve identity via Exchange.
func (m *Mastodon) ValidateIDToken(_ context.Context, _, _ string) (*application.UserInfo, error) {
	return nil, ErrMastodonIDTokenUnsupported
}

// ensureApp returns the cached MastodonApp for host, registering one via
// /api/v1/apps if no entry exists.
//
// Concurrent first-flow registrations for the same host are deduplicated via
// singleflight: without this, N simultaneous logins to a fresh instance would
// each hit POST /api/v1/apps and leave N-1 abandoned upstream apps forever
// (Mastodon does not deduplicate registrations server-side).
//
// A successful registration whose cache write fails is treated as
// best-effort: the registered credential is returned and the flow continues.
// Returning an error here would make the caller discard the just-issued
// credential, then on retry register yet another app — exactly the pollution
// pattern the singleflight prevents.
func (m *Mastodon) ensureApp(ctx context.Context, host string) (*MastodonApp, error) {
	if app, err := m.appCache.GetApp(ctx, host); err != nil {
		return nil, fmt.Errorf("get cached app: %w", err)
	} else if app != nil {
		return app, nil
	}

	v, err, _ := m.registerSF.Do(host, func() (any, error) {
		// Re-check the cache inside the singleflight critical section: the
		// first goroutine to win the flight may have populated it; subsequent
		// goroutines that joined the flight should see the cached value.
		if app, getErr := m.appCache.GetApp(ctx, host); getErr == nil && app != nil {
			return app, nil
		}
		registered, regErr := m.registerApp(ctx, host)
		if regErr != nil {
			return nil, fmt.Errorf("register app: %w", regErr)
		}
		// Best-effort cache write: a failure here is logged via a no-op
		// (callers wire their own logger at the application layer) but does
		// not poison the in-flight registration.
		_ = m.appCache.SetApp(ctx, host, registered)
		return registered, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*MastodonApp), nil
}

// registerApp posts to /api/v1/apps at the given instance to obtain client
// credentials.
func (m *Mastodon) registerApp(ctx context.Context, host string) (*MastodonApp, error) {
	data := url.Values{
		"client_name":   {m.appName},
		"redirect_uris": {m.redirectURI},
		"scopes":        {strings.Join(m.scopes, " ")},
	}
	if m.website != "" {
		data.Set("website", m.website)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.instanceBase(host)+mastodonAppRegistrationPath, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if out.ClientID == "" || out.ClientSecret == "" {
		// Body is omitted: a successful /api/v1/apps response carries
		// client_secret, and dumping it into wrapped errors would leak
		// secrets to logs/metrics for any caller that prints the error chain.
		return nil, fmt.Errorf("registration response missing client credentials")
	}
	return &MastodonApp{ClientID: out.ClientID, ClientSecret: out.ClientSecret}, nil
}

type mastodonTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	CreatedAt    int64  `json:"created_at"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func (m *Mastodon) requestToken(ctx context.Context, host string, data url.Values) (*mastodonTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.instanceBase(host)+mastodonTokenPath, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var out mastodonTokenResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if out.AccessToken == "" {
		// Body is omitted: a 200 response that lacks access_token may still
		// carry refresh_token / id_token / scope, and dumping the raw body
		// into wrapped errors would leak those secrets to logs.
		return nil, errors.New("token response missing access_token")
	}
	return &out, nil
}

// mastodonAccount mirrors the subset of /api/v1/accounts/verify_credentials we
// consume. Mastodon does not return email via the public API.
type mastodonAccount struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Acct        string `json:"acct"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
}

func (m *Mastodon) fetchUserInfo(ctx context.Context, host, accessToken string) (*application.UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.instanceBase(host)+mastodonVerifyPath, nil)
	if err != nil {
		return nil, fmt.Errorf("create verify request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send verify request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read verify response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var account mastodonAccount
	if err := json.Unmarshal(body, &account); err != nil {
		return nil, fmt.Errorf("parse verify response: %w", err)
	}
	if account.ID == "" {
		return nil, fmt.Errorf("verify response missing id: %s", string(body))
	}

	displayName := account.DisplayName
	if displayName == "" {
		displayName = account.Username
	}

	// ProviderUserID is namespaced by host so the same numeric account id at
	// two different instances is treated as two distinct credentials. Without
	// this, "alice" on mastodon.social and "alice" on hachyderm.io would
	// collide on credential lookup.
	return &application.UserInfo{
		ProviderUserID: account.ID + "@" + host,
		DisplayName:    displayName,
		AvatarURL:      account.Avatar,
		Provider:       "mastodon",
	}, nil
}

// flowStatus categorises the result of takeFlow so Exchange can return a
// distinguishable sentinel per case.
type flowStatus int

const (
	flowStatusMissing  flowStatus = iota // never bound
	flowStatusOK                         // bound, in-TTL, single-use claim succeeded
	flowStatusExpired                    // bound, but TTL'd
	flowStatusConsumed                   // bound previously, already taken
)

// flowTombstone records what happened to a previously-bound flow so a
// duplicate Exchange returns the same sentinel until TTL elapses, instead of
// degrading to flowStatusMissing once the bind entry has been removed.
type flowTombstone struct {
	status    flowStatus // flowStatusConsumed or flowStatusExpired
	expiresAt time.Time
}

// gcSweepEvery sets how often (per bindFlow call) the provider opportunistically
// scans for and removes expired flow bindings, bounding sync.Map retention for
// abandoned flows. Probabilistic so amortised cost is O(1) per bind.
const gcSweepEvery = 64

// bindFlow stashes a codeChallenge -> host binding with TTL for later retrieval
// by Exchange. Every gcSweepEvery calls it sweeps expired bindings to bound
// memory growth from abandoned flows.
func (m *Mastodon) bindFlow(codeChallenge, host string) {
	now := m.nowFn()
	m.flowBindings.Store(codeChallenge, &flowBinding{
		host:      host,
		expiresAt: now.Add(m.flowTTL),
	})
	if m.bindCounter.Add(1)%gcSweepEvery == 0 {
		m.sweepExpired(now)
	}
}

// takeFlow returns (host, flowStatusOK) if a fresh binding is consumed, or one
// of the other flowStatus values to let the caller distinguish missing /
// expired / already-consumed. Single-use to prevent code replay across
// instances.
func (m *Mastodon) takeFlow(codeChallenge string) (string, flowStatus) {
	v, ok := m.flowBindings.LoadAndDelete(codeChallenge)
	if !ok {
		// Distinguish "never bound" from "already consumed/expired" via the
		// tombstone map so retried callbacks return the same sentinel until
		// the tombstone TTL elapses.
		if t, ok := m.flowConsumed.Load(codeChallenge); ok {
			tomb := t.(*flowTombstone)
			if m.nowFn().Before(tomb.expiresAt) {
				return "", tomb.status
			}
			m.flowConsumed.Delete(codeChallenge)
		}
		return "", flowStatusMissing
	}
	binding := v.(*flowBinding)
	now := m.nowFn()
	if now.After(binding.expiresAt) {
		// Tombstone the expired flow so a duplicate callback keeps returning
		// ErrMastodonFlowExpired instead of degrading to ErrMastodonInstanceRequired
		// once the binding has been removed.
		m.flowConsumed.Store(codeChallenge, &flowTombstone{
			status:    flowStatusExpired,
			expiresAt: now.Add(m.tombstoneTTL),
		})
		return "", flowStatusExpired
	}
	// Mark as consumed so a duplicate Exchange returns ErrMastodonFlowAlreadyConsumed
	// instead of looking like "never bound."
	m.flowConsumed.Store(codeChallenge, &flowTombstone{
		status:    flowStatusConsumed,
		expiresAt: now.Add(m.tombstoneTTL),
	})
	return binding.host, flowStatusOK
}

// sweepExpired removes bindings whose expiresAt is in the past and tombstones
// whose retention window has passed. Called probabilistically from bindFlow
// to keep amortised cost low.
func (m *Mastodon) sweepExpired(now time.Time) {
	m.flowBindings.Range(func(k, v any) bool {
		if now.After(v.(*flowBinding).expiresAt) {
			m.flowBindings.Delete(k)
		}
		return true
	})
	m.flowConsumed.Range(func(k, v any) bool {
		if now.After(v.(*flowTombstone).expiresAt) {
			m.flowConsumed.Delete(k)
		}
		return true
	})
}

// pkceChallenge computes the S256 PKCE challenge for a verifier (matches
// application.GenerateCodeChallenge).
func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
