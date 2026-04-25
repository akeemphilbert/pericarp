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
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
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
var (
	// ErrMastodonInstanceRequired is returned by AuthCodeURL/Exchange when
	// no instance host has been bound to the flow. Callers must use
	// AuthCodeURLForInstance to start a Mastodon flow.
	ErrMastodonInstanceRequired = errors.New("mastodon: instance host required; use AuthCodeURLForInstance to start a flow")

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
	// AppCache stores per-instance app credentials. Required.
	AppCache MastodonAppCache
	// Website is optional; populated in the app registration request.
	Website string
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
	return &Mastodon{
		appName:      config.AppName,
		redirectURI:  config.RedirectURI,
		scopes:       scopes,
		website:      config.Website,
		appCache:     cache,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		flowTTL:      mastodonFlowTTL,
		instanceBase: func(host string) string { return "https://" + host },
		nowFn:        time.Now,
	}
}

// Name returns the provider identifier.
func (m *Mastodon) Name() string {
	return "mastodon"
}

// AuthCodeURL satisfies application.OAuthProvider but cannot start a real
// Mastodon flow because the instance host is not known here. It returns a
// sentinel-bearing pseudo-URL that can be safely surfaced to callers; the
// production code path is AuthCodeURLForInstance. Returning empty would risk
// silent redirects to a bogus relative URL, so the URL form makes failure
// diagnosable in browser logs.
func (m *Mastodon) AuthCodeURL(_, _, _, _ string) string {
	return "about:blank#" + url.QueryEscape(ErrMastodonInstanceRequired.Error())
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
	if host == "" {
		return "", fmt.Errorf("mastodon: host must not be empty")
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
func (m *Mastodon) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (*application.AuthResult, error) {
	challenge := pkceChallenge(codeVerifier)
	host, ok := m.takeFlow(challenge)
	if !ok {
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

	body, _ := io.ReadAll(resp.Body)
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
func (m *Mastodon) ensureApp(ctx context.Context, host string) (*MastodonApp, error) {
	app, err := m.appCache.GetApp(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("get cached app: %w", err)
	}
	if app != nil {
		return app, nil
	}

	registered, err := m.registerApp(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("register app: %w", err)
	}
	if err := m.appCache.SetApp(ctx, host, registered); err != nil {
		// Cache write failure is not fatal — the app is registered upstream
		// already; we just won't reuse credentials. Log via the wrapped error.
		return registered, fmt.Errorf("cache app: %w", err)
	}
	return registered, nil
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
		return nil, fmt.Errorf("registration response missing client credentials: %s", string(body))
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
		return nil, fmt.Errorf("token response missing access_token: %s", string(body))
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

// bindFlow stashes a codeChallenge -> host binding with TTL for later retrieval
// by Exchange.
func (m *Mastodon) bindFlow(codeChallenge, host string) {
	m.flowBindings.Store(codeChallenge, &flowBinding{
		host:      host,
		expiresAt: m.nowFn().Add(m.flowTTL),
	})
}

// takeFlow returns the host bound to the given codeChallenge and removes it,
// or ("", false) if absent or expired. Single-use to prevent the same code
// being replayed against multiple instances.
func (m *Mastodon) takeFlow(codeChallenge string) (string, bool) {
	v, ok := m.flowBindings.LoadAndDelete(codeChallenge)
	if !ok {
		return "", false
	}
	binding := v.(*flowBinding)
	if m.nowFn().After(binding.expiresAt) {
		return "", false
	}
	return binding.host, true
}

// pkceChallenge computes the S256 PKCE challenge for a verifier (matches
// application.GenerateCodeChallenge).
func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
