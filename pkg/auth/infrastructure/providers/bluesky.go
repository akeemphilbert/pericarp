package providers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/segmentio/ksuid"
)

// AT Protocol OAuth (proposal 0004) endpoints relative to the PDS host. The
// PDS exposes its authorization server metadata via .well-known.
const (
	blueskyResolveHandlePath        = "/xrpc/com.atproto.identity.resolveHandle"
	blueskyAuthServerMetadataPath   = "/.well-known/oauth-authorization-server"
	blueskyDefaultHandleResolveBase = "https://bsky.social"
	blueskyDefaultPLCDirectoryBase  = "https://plc.directory"
	blueskyFlowTTL                  = 10 * time.Minute
	// blueskyDPoPNonceTTL bounds how long a cached DPoP-Nonce stays usable.
	// AT-Proto auth servers rotate nonces on the order of minutes; an hour
	// is a comfortable upper bound that also caps memory growth in
	// dpopNonceCacheM (entries are keyed by issuer, so a long-running
	// service that talks to many distinct PDSes would otherwise leak).
	blueskyDPoPNonceTTL = 1 * time.Hour
)

// Bluesky-specific sentinel errors. Each is PERMANENT for the failing flow;
// callers must not retry the same Exchange/RefreshToken call.
var (
	ErrBlueskyHandleResolutionFailed = errors.New("bluesky: handle resolution failed")
	ErrBlueskyDIDResolutionFailed    = errors.New("bluesky: DID document resolution failed")
	ErrBlueskyAuthServerDiscovery    = errors.New("bluesky: authorization server metadata discovery failed")
	ErrBlueskyPARFailed              = errors.New("bluesky: pushed authorization request failed")
	ErrBlueskyHandleRequired         = errors.New("bluesky: handle required; use AuthCodeURLForHandle")
	ErrBlueskyFlowMissing            = errors.New("bluesky: no flow binding for this code (forgot AuthCodeURLForHandle?)")
	ErrBlueskyFlowExpired            = errors.New("bluesky: flow binding expired before code was exchanged")
	ErrBlueskyFlowConsumed           = errors.New("bluesky: flow binding already consumed; start a fresh flow")
	ErrBlueskyIDTokenUnsupported     = errors.New("bluesky: AT Protocol OAuth does not issue ID tokens; use Exchange to resolve user info")
	ErrBlueskyRevokeUnsupported      = errors.New("bluesky: revocation through the OAuthProvider interface is not supported; revoke at the PDS directly")
	ErrBlueskyIssuerMismatch         = errors.New("bluesky: authorization server metadata issuer does not match the PDS")
)

// BlueskyKeyStore stores the ECDSA P-256 key used to sign DPoP proofs.
//
// Implementations must be safe for concurrent use. Single-replica deployments
// can use NewMemoryBlueskyKeyStore. Multi-replica deployments MUST back this
// with a shared secret store: every DPoP-bound token issued is cryptographically
// tied (via the JWK thumbprint in the cnf.jkt claim) to the public key that
// signed the proof, and a different replica without access to the same private
// key cannot prove possession during refresh.
type BlueskyKeyStore interface {
	GetSigningKey(ctx context.Context) (*ecdsa.PrivateKey, error)
	SetSigningKey(ctx context.Context, key *ecdsa.PrivateKey) error
}

// NewMemoryBlueskyKeyStore returns an in-memory BlueskyKeyStore. The key is
// generated lazily on first GetSigningKey if none is pre-seeded.
func NewMemoryBlueskyKeyStore() BlueskyKeyStore {
	return &memoryBlueskyKeyStore{}
}

type memoryBlueskyKeyStore struct {
	mu  sync.RWMutex
	key *ecdsa.PrivateKey
}

func (m *memoryBlueskyKeyStore) GetSigningKey(_ context.Context) (*ecdsa.PrivateKey, error) {
	m.mu.RLock()
	if m.key != nil {
		defer m.mu.RUnlock()
		return m.key, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.key != nil {
		return m.key, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ES256 key: %w", err)
	}
	m.key = key
	return key, nil
}

func (m *memoryBlueskyKeyStore) SetSigningKey(_ context.Context, key *ecdsa.PrivateKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.key = key
	return nil
}

// BlueskyConfig configures the Bluesky AT Protocol OAuth provider.
type BlueskyConfig struct {
	// ClientMetadataURL is the public URL where the consumer hosts the client
	// metadata JSON document. AT Protocol OAuth uses this URL as the
	// client_id value sent in OAuth requests.
	ClientMetadataURL string
	// RedirectURI must match a redirect_uris entry in the client metadata
	// document at ClientMetadataURL.
	RedirectURI string
	// Scopes default to ["atproto", "transition:generic"]; consumers can
	// pass narrower scopes for read-only or specific feature surfaces.
	Scopes []string
	// KeyStore stores the ECDSA P-256 signing key for DPoP proofs.
	// If nil, an in-memory ephemeral keystore is used.
	KeyStore BlueskyKeyStore
	// AllowInsecurePDSURLs disables most endpoint URL safety checks the
	// Bluesky OAuth flow uses end-to-end. With it set, the provider stops
	// enforcing https-only, internal-IP rejection, DNS-resolved internal-IP
	// rejection, and host-consistency checks across the DID document's
	// serviceEndpoint, the AS metadata's authorize/token/PAR endpoints,
	// and the URLs decoded from a wrapped refresh token. (The userinfo
	// rejection in validatePDSURL is unconditional and continues to fire,
	// since credential-in-URL has no legitimate use even in tests.)
	// Production must leave this false; only tests and local development
	// against trusted httptest fakes should set it to true.
	AllowInsecurePDSURLs bool
}

// blueskyFlow ties a codeChallenge to the PDS the auth flow targets, so
// Exchange can route the token request to the right authorization server.
type blueskyFlow struct {
	pdsURL    string
	authURL   string
	tokenURL  string
	parURL    string
	issuer    string // canonical AS issuer URL (cache key for DPoP nonces)
	did       string
	handle    string
	expiresAt time.Time
}

// canonicalIssuer normalises an AS issuer URL so it can be used as a cache
// key consistently across PAR and token endpoints. RFC 9449 §8 says nonces
// are scoped to the authorization server, not to a specific endpoint, so
// using the issuer as the canonical key lets a nonce learned during PAR be
// reused at the token endpoint and vice versa.
func canonicalIssuer(issuer string) string {
	return strings.TrimRight(issuer, "/")
}

// Bluesky implements application.OAuthProvider for the AT Protocol OAuth flow.
//
// Compared to traditional OAuth providers, Bluesky's flow is per-user
// federated: each user belongs to a Personal Data Server (PDS) that hosts
// their authorization server. The flow:
//
//  1. Caller invokes AuthCodeURLForHandle(handle) — this resolves the handle
//     through com.atproto.identity.resolveHandle to a DID, fetches the DID
//     document to find the user's PDS, fetches that PDS's
//     .well-known/oauth-authorization-server metadata, performs a Pushed
//     Authorization Request (PAR) at the metadata's pushed_authorization_request_endpoint
//     with a DPoP proof, and returns the authorize URL.
//  2. The user authenticates at the PDS and is redirected back with a code.
//  3. Caller invokes Exchange(code, codeVerifier, redirectURI) — this looks
//     up the flow by sha256(codeVerifier), POSTs to the token endpoint with
//     a DPoP proof, and stores the DPoP-bound access + refresh tokens.
//  4. Caller invokes RefreshToken(refreshToken) when the access token nears
//     expiry; refresh requires a fresh DPoP proof signed by the same key.
type Bluesky struct {
	clientMetadataURL string
	redirectURI       string
	scopes            []string
	keyStore          BlueskyKeyStore
	httpClient        *http.Client

	handleResolveBase string
	plcDirectoryBase  string

	flows                sync.Map // codeChallenge -> *blueskyFlow
	flowConsumed         sync.Map // codeChallenge -> *flowTombstone
	flowTTL              time.Duration
	tombstoneTTL         time.Duration
	bindCounter          atomic.Uint64
	nowFn                func() time.Time
	jtiFn                func() string // overridable for deterministic tests
	dpopNonceCacheM      sync.Map      // issuer -> *dpopNonceEntry
	dpopNonceCounter     atomic.Uint64 // drives probabilistic GC of dpopNonceCacheM
	allowInsecurePDSURLs bool
	// lookupHostFn is the resolver used by the SSRF guard; overridable in
	// tests so DNS doesn't have to leave the box. Context-aware so resolver
	// hangs respect the caller's deadline/cancellation.
	lookupHostFn func(ctx context.Context, host string) ([]string, error)
}

// NewBluesky constructs a Bluesky provider.
func NewBluesky(config BlueskyConfig) *Bluesky {
	scopes := config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"atproto", "transition:generic"}
	}
	store := config.KeyStore
	if store == nil {
		store = NewMemoryBlueskyKeyStore()
	}
	return &Bluesky{
		clientMetadataURL:    config.ClientMetadataURL,
		redirectURI:          config.RedirectURI,
		scopes:               scopes,
		keyStore:             store,
		httpClient:           &http.Client{Timeout: 30 * time.Second},
		handleResolveBase:    blueskyDefaultHandleResolveBase,
		plcDirectoryBase:     blueskyDefaultPLCDirectoryBase,
		flowTTL:              blueskyFlowTTL,
		tombstoneTTL:         blueskyFlowTTL,
		nowFn:                time.Now,
		jtiFn:                func() string { return ksuid.New().String() },
		allowInsecurePDSURLs: config.AllowInsecurePDSURLs,
		lookupHostFn:         net.DefaultResolver.LookupHost,
	}
}

// Name returns the provider identifier.
func (b *Bluesky) Name() string { return "bluesky" }

// AuthCodeURL satisfies application.OAuthProvider but cannot start a real
// Bluesky flow because the user's handle is not present at this signature.
// Returns empty; callers MUST use AuthCodeURLForHandle.
func (b *Bluesky) AuthCodeURL(_, _, _, _ string) string { return "" }

// AuthCodeURLForHandle resolves the handle, discovers the PDS authorization
// server, performs PAR, and returns the authorize URL.
func (b *Bluesky) AuthCodeURLForHandle(ctx context.Context, handle, state, codeChallenge, _, redirectURI string) (string, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return "", ErrBlueskyHandleRequired
	}
	// codeChallenge keys the flow binding; an empty key would collide every
	// concurrent flow into the same slot and let the first-finishing Exchange
	// route a code at the wrong PDS / issuer.
	if strings.TrimSpace(codeChallenge) == "" {
		return "", fmt.Errorf("bluesky: codeChallenge must not be empty")
	}
	if redirectURI == "" {
		redirectURI = b.redirectURI
	}
	if redirectURI != b.redirectURI {
		return "", fmt.Errorf("bluesky: redirectURI %q does not match configured RedirectURI %q", redirectURI, b.redirectURI)
	}

	did, err := b.resolveHandle(ctx, handle)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBlueskyHandleResolutionFailed, err)
	}

	pdsURL, err := b.resolveDIDToPDS(ctx, did)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBlueskyDIDResolutionFailed, err)
	}

	asMeta, err := b.fetchAuthServerMetadata(ctx, pdsURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBlueskyAuthServerDiscovery, err)
	}

	// RFC 8414 §3.3 requires the issuer in the metadata response to match the
	// authorization server identifier the client used to fetch the metadata.
	// Without this check, a malicious DID document could route us to a PDS
	// that pretends to be issued by a different host.
	if canonicalIssuer(asMeta.Issuer) != canonicalIssuer(pdsURL) {
		return "", fmt.Errorf("%w: metadata issuer %q != PDS %q", ErrBlueskyIssuerMismatch, asMeta.Issuer, pdsURL)
	}

	issuer := canonicalIssuer(asMeta.Issuer)
	requestURI, err := b.pushAuthorizationRequest(ctx, asMeta, issuer, state, codeChallenge, handle, redirectURI)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBlueskyPARFailed, err)
	}

	b.bindFlow(codeChallenge, &blueskyFlow{
		pdsURL:    pdsURL,
		authURL:   asMeta.AuthorizationEndpoint,
		tokenURL:  asMeta.TokenEndpoint,
		parURL:    asMeta.PushedAuthorizationRequestEndpoint,
		issuer:    issuer,
		did:       did,
		handle:    handle,
		expiresAt: b.nowFn().Add(b.flowTTL),
	})

	q := url.Values{
		"request_uri": {requestURI},
		"client_id":   {b.clientMetadataURL},
	}
	return asMeta.AuthorizationEndpoint + "?" + q.Encode(), nil
}

// Exchange swaps the authorization code for DPoP-bound tokens at the same
// PDS the auth URL was issued by.
func (b *Bluesky) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (*application.AuthResult, error) {
	challenge := application.GenerateCodeChallenge(codeVerifier)
	flow, status := b.takeFlow(challenge)
	switch status {
	case flowStatusOK:
	case flowStatusExpired:
		return nil, ErrBlueskyFlowExpired
	case flowStatusConsumed:
		return nil, ErrBlueskyFlowConsumed
	default:
		return nil, ErrBlueskyFlowMissing
	}
	if redirectURI == "" {
		redirectURI = b.redirectURI
	} else if redirectURI != b.redirectURI {
		// Mirror AuthCodeURLForHandle: a different redirect_uri here than the
		// one used during PAR will be rejected by the PDS, but checking
		// upfront keeps the error path local and unambiguous.
		return nil, fmt.Errorf("bluesky: redirectURI %q does not match configured RedirectURI %q", redirectURI, b.redirectURI)
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {b.clientMetadataURL},
		"code_verifier": {codeVerifier},
	}
	tokenResp, err := b.tokenRequestWithDPoP(ctx, flow.tokenURL, flow.issuer, form, "")
	if err != nil {
		return nil, fmt.Errorf("bluesky: token exchange: %w", err)
	}

	// Wrap the upstream refresh token with PDS context so RefreshToken can
	// route subsequent calls back to the same authorization server. Encode
	// issuer too so refresh proofs use the same nonce cache key.
	wrappedRefresh := ""
	if tokenResp.RefreshToken != "" {
		wrappedRefresh = encodeBlueskyRefreshToken(flow.pdsURL, flow.tokenURL, flow.issuer, tokenResp.RefreshToken)
	}

	return &application.AuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: wrappedRefresh,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		UserInfo: application.UserInfo{
			ProviderUserID: flow.did,
			DisplayName:    flow.handle,
			Provider:       "bluesky",
		},
	}, nil
}

// RefreshToken refreshes a DPoP-bound access token. AT Protocol access tokens
// are short-lived (~30 min); refresh is the normal path. The same signing
// key (and therefore the same JWK thumbprint, jkt) must be used as on the
// original Exchange — the upstream PDS rejects refreshes signed by a
// different key.
func (b *Bluesky) RefreshToken(ctx context.Context, refreshToken string) (*application.AuthResult, error) {
	if refreshToken == "" {
		return nil, application.ErrTokenRefreshFailed
	}
	// Refresh requires the original PDS's token endpoint and AS issuer.
	// OAuthProvider.RefreshToken has only a single refreshToken parameter, so
	// we require callers to pass the wrapped Bluesky refresh token format
	// emitted by Exchange via encodeBlueskyRefreshToken: a `btr.v2.`-prefixed
	// value that base64url-encodes pdsURL|tokenURL|issuer|opaqueRefresh.
	// decodeBlueskyRefreshToken below unwraps it; any other shape is rejected.
	pdsURL, tokenURL, issuer, opaque, err := decodeBlueskyRefreshToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("bluesky: refresh token format: %w (issued by Exchange)", err)
	}
	// The wrapped refresh token is application-layer state and may have been
	// tampered with in storage. Re-validate the decoded URLs against the
	// same SSRF rules `fetchAuthServerMetadata` enforces during Exchange:
	// scheme/host/non-internal checks plus a same-host check tying
	// tokenURL/issuer to pdsURL. Without this, a poisoned `btr.v2.` payload
	// could redirect refresh POSTs at internal services.
	if err := b.validateRefreshTokenURLs(ctx, pdsURL, tokenURL, issuer); err != nil {
		return nil, fmt.Errorf("bluesky: refresh token URLs: %w", err)
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {opaque},
		"client_id":     {b.clientMetadataURL},
	}
	tokenResp, err := b.tokenRequestWithDPoP(ctx, tokenURL, issuer, form, "")
	if err != nil {
		return nil, fmt.Errorf("bluesky: token refresh: %w", err)
	}

	// Re-wrap so the next refresh round-trips correctly. Some servers rotate
	// refresh tokens; others omit refresh_token on refresh and expect the
	// client to keep using the previously issued opaque value. Returning a
	// wrapped empty refresh would brick subsequent refreshes for the latter.
	nextOpaque := opaque
	if tokenResp.RefreshToken != "" {
		nextOpaque = tokenResp.RefreshToken
	}
	encoded := encodeBlueskyRefreshToken(pdsURL, tokenURL, issuer, nextOpaque)

	return &application.AuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: encoded,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		UserInfo: application.UserInfo{
			// Preserve identity fields across refresh — without this, the
			// application layer cannot match a refreshed token to its
			// originating credential.
			ProviderUserID: tokenResp.Sub,
			Provider:       "bluesky",
		},
	}, nil
}

// RevokeToken is not supported by the standard OAuth provider interface for
// Bluesky because the PDS-specific revocation endpoint is not threaded
// through. Returns ErrBlueskyRevokeUnsupported (errors.Is-friendly) so callers
// can route around it programmatically rather than string-matching.
func (b *Bluesky) RevokeToken(_ context.Context, _ string) error {
	return ErrBlueskyRevokeUnsupported
}

// ValidateIDToken returns ErrBlueskyIDTokenUnsupported because AT Protocol
// OAuth does not issue OIDC ID tokens.
func (b *Bluesky) ValidateIDToken(_ context.Context, _, _ string) (*application.UserInfo, error) {
	return nil, ErrBlueskyIDTokenUnsupported
}

// ----- handle / DID / auth-server discovery -----

// resolveHandle calls com.atproto.identity.resolveHandle (XRPC) on the well-
// known Bluesky resolver and returns the DID for the supplied handle.
func (b *Bluesky) resolveHandle(ctx context.Context, handle string) (string, error) {
	endpoint := b.handleResolveBase + blueskyResolveHandlePath + "?handle=" + url.QueryEscape(handle)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create resolveHandle request: %w", err)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send resolveHandle request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read resolveHandle body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		DID string `json:"did"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse resolveHandle response: %w", err)
	}
	if out.DID == "" {
		return "", fmt.Errorf("resolveHandle response missing did: %s", string(body))
	}
	return out.DID, nil
}

// didDocument models the subset of the AT Protocol DID document we consume.
type didDocument struct {
	Service []struct {
		ID              string `json:"id"`
		Type            string `json:"type"`
		ServiceEndpoint string `json:"serviceEndpoint"`
	} `json:"service"`
}

// resolveDIDToPDS fetches the DID document and returns the AtprotoPersonalDataServer URL.
func (b *Bluesky) resolveDIDToPDS(ctx context.Context, did string) (string, error) {
	endpoint, err := b.didDocumentURL(did)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create DID document request: %w", err)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch DID document: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read DID document body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var doc didDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("parse DID document: %w", err)
	}
	for _, svc := range doc.Service {
		if svc.Type == "AtprotoPersonalDataServer" {
			pdsURL := strings.TrimRight(svc.ServiceEndpoint, "/")
			if err := b.validatePDSURL(ctx, pdsURL); err != nil {
				return "", err
			}
			return pdsURL, nil
		}
	}
	return "", fmt.Errorf("DID document has no AtprotoPersonalDataServer service entry")
}

// validatePDSURL guards the serviceEndpoint we just fetched from a (possibly
// attacker-controlled) DID document — did:web in particular is hostable by
// anyone. Without this, a malicious DID document could route subsequent HTTP
// calls at internal hosts (SSRF) or downgrade the flow to plaintext http.
//
// Defaults: scheme must be https, host must be present, and (a) the literal
// host must not be a loopback / private / link-local / unspecified IP and
// (b) if the host is a DNS name, no resolved A/AAAA must fall in those
// ranges either — that defends against did:web documents that point at a
// public-looking name whose DNS happens to resolve to an internal IP. Tests
// and local development opt out via BlueskyConfig.AllowInsecurePDSURLs.
//
// This DNS check is best-effort: it does not defeat true DNS rebinding
// (where the resolver returns a different IP at dial time than at lookup
// time). Hardening that fully requires a custom Dialer that re-checks each
// resolved address; this guard catches the common static-misconfiguration
// case while we leave the rebinding-hardened dialer for a follow-up.
func (b *Bluesky) validatePDSURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("PDS URL %q: %w", raw, err)
	}
	if u.Host == "" {
		return fmt.Errorf("PDS URL %q has no host", raw)
	}
	// Reject userinfo regardless of insecure-mode: a serviceEndpoint shaped
	// like https://user:pass@host has no legitimate use, encourages
	// credential-in-URL anti-patterns, and enables URL-confusion tricks where
	// the apparent host differs from the actual authority.
	if u.User != nil {
		return fmt.Errorf("PDS URL %q must not contain userinfo", raw)
	}
	if b.allowInsecurePDSURLs {
		return nil
	}
	if u.Scheme != "https" {
		return fmt.Errorf("PDS URL %q must use https scheme", raw)
	}
	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("PDS URL %q has no hostname", raw)
	}
	if strings.EqualFold(hostname, "localhost") {
		return fmt.Errorf("PDS URL %q points at localhost (SSRF guard)", raw)
	}
	if ip := net.ParseIP(hostname); ip != nil {
		if isInternalIP(ip) {
			return fmt.Errorf("PDS URL %q points at loopback/private host (SSRF guard)", raw)
		}
		return nil
	}
	// DNS name: resolve and reject if any answer falls in an internal range.
	addrs, err := b.lookupHostFn(ctx, hostname)
	if err != nil {
		return fmt.Errorf("PDS URL %q: resolve %s: %w", raw, hostname, err)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if isInternalIP(ip) {
			return fmt.Errorf("PDS URL %q resolves to internal address %s (SSRF guard)", raw, addr)
		}
	}
	return nil
}

// isInternalIP reports whether ip is a loopback, private (RFC1918 / ULA),
// link-local, or unspecified address — i.e. one that an externally-issued
// DID document should never legitimately resolve to.
func isInternalIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// didDocumentURL maps a DID to a fetchable URL. did:plc:* uses the PLC
// directory; did:web:* uses the .well-known/did.json convention.
func (b *Bluesky) didDocumentURL(did string) (string, error) {
	switch {
	case strings.HasPrefix(did, "did:plc:"):
		return b.plcDirectoryBase + "/" + url.PathEscape(did), nil
	case strings.HasPrefix(did, "did:web:"):
		host := strings.TrimPrefix(did, "did:web:")
		host = strings.ReplaceAll(host, ":", "/")
		return "https://" + host + "/.well-known/did.json", nil
	default:
		return "", fmt.Errorf("unsupported DID method: %s", did)
	}
}

// blueskyAuthServerMetadata models the subset of OAuth 2.0 authorization
// server metadata (RFC 8414) we consume.
type blueskyAuthServerMetadata struct {
	Issuer                             string   `json:"issuer"`
	AuthorizationEndpoint              string   `json:"authorization_endpoint"`
	TokenEndpoint                      string   `json:"token_endpoint"`
	PushedAuthorizationRequestEndpoint string   `json:"pushed_authorization_request_endpoint"`
	ResponseTypesSupported             []string `json:"response_types_supported"`
	DPoPSigningAlgValuesSupported      []string `json:"dpop_signing_alg_values_supported"`
}

// fetchAuthServerMetadata fetches /.well-known/oauth-authorization-server from
// the PDS and returns the parsed metadata.
func (b *Bluesky) fetchAuthServerMetadata(ctx context.Context, pdsURL string) (*blueskyAuthServerMetadata, error) {
	endpoint := pdsURL + blueskyAuthServerMetadataPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create metadata request: %w", err)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var meta blueskyAuthServerMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" || meta.PushedAuthorizationRequestEndpoint == "" {
		return nil, fmt.Errorf("auth server metadata missing required endpoints: %s", string(body))
	}
	if err := b.validateAuthServerEndpoints(ctx, pdsURL, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// validateAuthServerEndpoints checks that the AS metadata's authorization /
// token / PAR endpoints are safe to call. The earlier validatePDSURL guard
// only protects the metadata-fetch URL itself; without this check a
// reachable-but-malicious PDS could return endpoints pointing at http:// or
// internal hosts and we would happily POST PAR/token requests there.
//
// Each endpoint is run through validatePDSURL, which parses the URL and
// rejects userinfo unconditionally (so the no-credential-in-URL invariant
// holds even with AllowInsecurePDSURLs=true). The remaining
// scheme/internal-IP and same-host-as-PDS checks only fire when
// AllowInsecurePDSURLs is false.
func (b *Bluesky) validateAuthServerEndpoints(ctx context.Context, pdsURL string, meta *blueskyAuthServerMetadata) error {
	var pdsHost string
	if !b.allowInsecurePDSURLs {
		pdsParsed, err := url.Parse(pdsURL)
		if err != nil {
			return fmt.Errorf("parse pdsURL %q: %w", pdsURL, err)
		}
		pdsHost = strings.ToLower(pdsParsed.Hostname())
	}
	for _, ep := range []struct {
		name string
		raw  string
	}{
		{"authorization_endpoint", meta.AuthorizationEndpoint},
		{"token_endpoint", meta.TokenEndpoint},
		{"pushed_authorization_request_endpoint", meta.PushedAuthorizationRequestEndpoint},
	} {
		if err := b.validatePDSURL(ctx, ep.raw); err != nil {
			return fmt.Errorf("auth server %s: %w", ep.name, err)
		}
		if b.allowInsecurePDSURLs {
			continue
		}
		u, err := url.Parse(ep.raw)
		if err != nil {
			return fmt.Errorf("auth server %s parse %q: %w", ep.name, ep.raw, err)
		}
		if strings.ToLower(u.Hostname()) != pdsHost {
			return fmt.Errorf("auth server %s host %q does not match PDS host %q", ep.name, u.Hostname(), pdsHost)
		}
	}
	return nil
}

// validateRefreshTokenURLs guards the URLs decoded from a wrapped Bluesky
// refresh token against tampering. Persisted refresh tokens are
// application-layer state, so a malicious or compromised store could craft
// a btr.v2 payload that points tokenURL at an internal host; without this
// check, RefreshToken would happily POST at it. The rules mirror
// validateAuthServerEndpoints: each URL is parsed and rejected if it
// contains userinfo (always), and in non-insecure mode each URL also goes
// through validatePDSURL's full SSRF guard plus a same-host check tying
// tokenURL/issuer to pdsURL.
func (b *Bluesky) validateRefreshTokenURLs(ctx context.Context, pdsURL, tokenURL, issuer string) error {
	if err := b.validatePDSURL(ctx, pdsURL); err != nil {
		return fmt.Errorf("pdsURL: %w", err)
	}
	var pdsHost string
	if !b.allowInsecurePDSURLs {
		pdsParsed, err := url.Parse(pdsURL)
		if err != nil {
			return fmt.Errorf("parse pdsURL %q: %w", pdsURL, err)
		}
		pdsHost = strings.ToLower(pdsParsed.Hostname())
	}
	for _, ep := range []struct {
		name string
		raw  string
	}{
		{"tokenURL", tokenURL},
		{"issuer", issuer},
	} {
		if err := b.validatePDSURL(ctx, ep.raw); err != nil {
			return fmt.Errorf("%s: %w", ep.name, err)
		}
		if b.allowInsecurePDSURLs {
			continue
		}
		u, err := url.Parse(ep.raw)
		if err != nil {
			return fmt.Errorf("parse %s %q: %w", ep.name, ep.raw, err)
		}
		if strings.ToLower(u.Hostname()) != pdsHost {
			return fmt.Errorf("%s host %q does not match pdsURL host %q", ep.name, u.Hostname(), pdsHost)
		}
	}
	return nil
}

// pushAuthorizationRequest performs PAR with a DPoP proof. The PAR endpoint
// returns an opaque request_uri that the user is redirected to.
//
// Mirrors the use_dpop_nonce handshake from tokenRequestWithDPoP: when the
// AS requires a server-issued nonce, the first attempt comes back as 400
// {"error":"use_dpop_nonce"} with a DPoP-Nonce header, and the second
// attempt regenerates the proof with that nonce. Without this, the very
// first AuthCodeURLForHandle call against any nonce-requiring AS would fail
// even though a single retry would succeed.
func (b *Bluesky) pushAuthorizationRequest(ctx context.Context, asMeta *blueskyAuthServerMetadata, issuer, state, codeChallenge, handle, redirectURI string) (string, error) {
	form := url.Values{
		"response_type":         {"code"},
		"client_id":             {b.clientMetadataURL},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(b.scopes, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"login_hint":            {handle},
	}

	for attempt := range 2 {
		dpop, err := b.makeDPoPProof(ctx, http.MethodPost, asMeta.PushedAuthorizationRequestEndpoint, "", b.dpopNonceFor(issuer))
		if err != nil {
			return "", fmt.Errorf("make DPoP proof: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, asMeta.PushedAuthorizationRequestEndpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return "", fmt.Errorf("create PAR request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("DPoP", dpop)

		resp, err := b.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("send PAR request: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read PAR response: %w", readErr)
		}

		if nonce := resp.Header.Get("DPoP-Nonce"); nonce != "" {
			b.storeDPoPNonce(issuer, nonce)
		}

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			var out struct {
				RequestURI string `json:"request_uri"`
				ExpiresIn  int    `json:"expires_in"`
			}
			if err := json.Unmarshal(body, &out); err != nil {
				return "", fmt.Errorf("parse PAR response: %w", err)
			}
			if out.RequestURI == "" {
				return "", fmt.Errorf("PAR response missing request_uri: %s", string(body))
			}
			return out.RequestURI, nil
		}

		// Same dispatch shape as tokenRequestWithDPoP: only retry on the
		// canonical `error` field, not on substrings of the body.
		if resp.StatusCode == http.StatusBadRequest && attempt == 0 {
			var oauthErr struct {
				Error string `json:"error"`
			}
			if json.Unmarshal(body, &oauthErr) == nil && oauthErr.Error == "use_dpop_nonce" {
				continue
			}
		}
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	// Unreachable: same reasoning as tokenRequestWithDPoP — every iteration
	// either continues (only on attempt 0) or returns. Go's flow analysis
	// can't prove `for range 2` returns inside, so a terminator is required.
	panic("unreachable: pushAuthorizationRequest must return inside the loop")
}

// blueskyTokenResponse models the subset of the token-endpoint response we use.
type blueskyTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Sub          string `json:"sub"`
}

// tokenRequestWithDPoP posts a token request carrying a DPoP proof, with one
// retry on the use_dpop_nonce error path (the standard AT Proto handshake).
//
// The nonce cache is keyed by the canonical issuer (not the tokenURL) so
// nonces learned during PAR are reused here, avoiding an unconditional
// extra round-trip on every Exchange.
func (b *Bluesky) tokenRequestWithDPoP(ctx context.Context, tokenURL, issuer string, form url.Values, accessTokenForAth string) (*blueskyTokenResponse, error) {
	for attempt := range 2 {
		dpop, err := b.makeDPoPProof(ctx, http.MethodPost, tokenURL, accessTokenForAth, b.dpopNonceFor(issuer))
		if err != nil {
			return nil, fmt.Errorf("make DPoP proof: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, fmt.Errorf("create token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("DPoP", dpop)
		req.Header.Set("Accept", "application/json")

		resp, err := b.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send token request: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read token response body: %w", readErr)
		}

		if nonce := resp.Header.Get("DPoP-Nonce"); nonce != "" {
			b.storeDPoPNonce(issuer, nonce)
		}

		// Parse the OAuth error JSON to dispatch on the canonical `error`
		// field rather than substring-matching the entire body. This rules
		// out an unrelated 400 whose error_description happens to mention
		// "use_dpop_nonce" from triggering a spurious retry.
		if resp.StatusCode == http.StatusBadRequest && attempt == 0 {
			var oauthErr struct {
				Error string `json:"error"`
			}
			if json.Unmarshal(body, &oauthErr) == nil && oauthErr.Error == "use_dpop_nonce" {
				continue
			}
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}

		var out blueskyTokenResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("parse token response: %w", err)
		}
		if out.AccessToken == "" {
			// Do not include `body` here — a 200 response that lacks
			// access_token may still contain refresh_token / id_token, and
			// dumping the body into wrapped errors leaks secrets to logs.
			return nil, fmt.Errorf("token response missing access_token")
		}
		return &out, nil
	}
	// Unreachable: every loop iteration either `continue`s (only possible on
	// attempt 0) or returns. Go's flow analysis can't prove that a `for range
	// 2` always returns inside, so this terminator is required to compile;
	// a panic flags any future drift in the loop body more loudly than a
	// misleading "exhausted retry" error would.
	panic("unreachable: tokenRequestWithDPoP must return inside the loop")
}

// dpopNonceEntry is the value stored in dpopNonceCacheM; it carries the
// latest DPoP-Nonce together with an expiry so the cache can't grow without
// bound for a long-running service that sees many distinct issuers.
type dpopNonceEntry struct {
	nonce     string
	expiresAt time.Time
}

// dpopNonceFor returns the most recently observed, still-valid DPoP-Nonce
// for server, or "" if none has been observed yet (or the cached one has
// expired). Expired entries are deleted lazily on read.
func (b *Bluesky) dpopNonceFor(server string) string {
	if v, ok := b.dpopNonceCacheM.Load(server); ok {
		entry := v.(*dpopNonceEntry)
		if b.nowFn().Before(entry.expiresAt) {
			return entry.nonce
		}
		b.dpopNonceCacheM.Delete(server)
	}
	return ""
}

// storeDPoPNonce caches a freshly-observed DPoP-Nonce for the given issuer
// with the package-wide TTL. Every gcSweepEvery writes it opportunistically
// sweeps expired entries so a service that churns through issuers eventually
// drops them — without that, the per-issuer entry only goes away on a
// subsequent read for that exact issuer.
func (b *Bluesky) storeDPoPNonce(issuer, nonce string) {
	now := b.nowFn()
	b.dpopNonceCacheM.Store(issuer, &dpopNonceEntry{
		nonce:     nonce,
		expiresAt: now.Add(blueskyDPoPNonceTTL),
	})
	if b.dpopNonceCounter.Add(1)%gcSweepEvery == 0 {
		b.sweepDPoPNonces(now)
	}
}

// sweepDPoPNonces drops entries whose TTL has passed. Called probabilistically
// from storeDPoPNonce so amortised cost is O(1) per write.
func (b *Bluesky) sweepDPoPNonces(now time.Time) {
	b.dpopNonceCacheM.Range(func(k, v any) bool {
		if now.After(v.(*dpopNonceEntry).expiresAt) {
			b.dpopNonceCacheM.Delete(k)
		}
		return true
	})
}

// ----- DPoP proof construction -----

// makeDPoPProof builds a DPoP proof JWT (RFC 9449) signed with the keystore's
// ECDSA P-256 key. The proof is single-use (random jti) and binds the HTTP
// method + URL + (optional) access-token-hash + (optional) server nonce.
//
// Header: {typ:"dpop+jwt", alg:"ES256", jwk:<public_jwk>}
// Claims: {jti, htm, htu, iat, optional nonce, optional ath}
//
// htu is normalized to drop query and fragment per RFC 9449 §4.2.
func (b *Bluesky) makeDPoPProof(ctx context.Context, method, urlStr, accessToken, nonce string) (string, error) {
	key, err := b.keyStore.GetSigningKey(ctx)
	if err != nil {
		return "", fmt.Errorf("get signing key: %w", err)
	}

	pubJWK := publicJWKFromECDSA(&key.PublicKey)
	claims := gojwt.MapClaims{
		"jti": b.jtiFn(),
		"htm": method,
		"htu": normalizeHTU(urlStr),
		"iat": b.nowFn().Unix(),
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	if accessToken != "" {
		// ath = base64url(sha256(access_token))
		h := sha256.Sum256([]byte(accessToken))
		claims["ath"] = base64.RawURLEncoding.EncodeToString(h[:])
	}

	tok := gojwt.NewWithClaims(gojwt.SigningMethodES256, claims)
	tok.Header["typ"] = "dpop+jwt"
	tok.Header["jwk"] = pubJWK

	signed, err := tok.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("sign DPoP proof: %w", err)
	}
	return signed, nil
}

// normalizeHTU strips query and fragment from a URL for the htu DPoP claim
// per RFC 9449 §4.2. If parsing fails it falls back to a manual strip so a
// malformed metadata URL still produces a usable (if imperfect) htu.
func normalizeHTU(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		u.RawQuery = ""
		u.Fragment = ""
		u.RawFragment = ""
		return u.String()
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		return raw[:i]
	}
	return raw
}

// publicJWKFromECDSA builds a JWK representation of an ECDSA P-256 public key.
// Field order in JWK thumbprint hashing is alphabetical per RFC 7638; consumers
// of this map should not rely on iteration order.
func publicJWKFromECDSA(pub *ecdsa.PublicKey) map[string]string {
	xBytes, yBytes := pub.X.Bytes(), pub.Y.Bytes()
	// P-256 coordinates are 32 bytes; left-pad if a leading zero was stripped.
	xPadded := make([]byte, 32)
	yPadded := make([]byte, 32)
	copy(xPadded[32-len(xBytes):], xBytes)
	copy(yPadded[32-len(yBytes):], yBytes)
	return map[string]string{
		"crv": "P-256",
		"kty": "EC",
		"x":   base64.RawURLEncoding.EncodeToString(xPadded),
		"y":   base64.RawURLEncoding.EncodeToString(yPadded),
	}
}

// JWKThumbprintFromECDSA computes the RFC 7638 SHA-256 thumbprint (jkt) of the
// public key. Exposed so callers can verify DPoP-bound tokens by checking the
// access token's cnf.jkt claim against this thumbprint.
func JWKThumbprintFromECDSA(pub *ecdsa.PublicKey) string {
	jwk := publicJWKFromECDSA(pub)
	canonical := fmt.Sprintf(`{"crv":"%s","kty":"%s","x":"%s","y":"%s"}`, jwk["crv"], jwk["kty"], jwk["x"], jwk["y"])
	h := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// ----- Refresh token wrapping -----
//
// AT Protocol refresh tokens are PDS-bound: the caller must remember which
// PDS issued them, which token endpoint to call, and which AS issuer keys
// the DPoP nonce cache. The OAuthProvider.RefreshToken signature gives us no
// per-credential context, so we wrap the upstream refresh token with all
// three URLs, base64url-encoded. Format version "btr.v2" is incompatible
// with v1 — the v1 wrapping omitted issuer, which made cross-replica nonce
// reuse impossible.

func encodeBlueskyRefreshToken(pdsURL, tokenURL, issuer, opaque string) string {
	raw := pdsURL + "|" + tokenURL + "|" + issuer + "|" + opaque
	return "btr.v2." + base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeBlueskyRefreshToken(token string) (pdsURL, tokenURL, issuer, opaque string, err error) {
	const prefix = "btr.v2."
	if !strings.HasPrefix(token, prefix) {
		return "", "", "", "", fmt.Errorf("unrecognized refresh token format")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, prefix))
	if err != nil {
		return "", "", "", "", fmt.Errorf("decode refresh token: %w", err)
	}
	parts := strings.SplitN(string(decoded), "|", 4)
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("malformed refresh token payload")
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

// ----- Flow binding -----

func (b *Bluesky) bindFlow(codeChallenge string, flow *blueskyFlow) {
	b.flows.Store(codeChallenge, flow)
	if b.bindCounter.Add(1)%gcSweepEvery == 0 {
		b.sweepExpired(b.nowFn())
	}
}

func (b *Bluesky) takeFlow(codeChallenge string) (*blueskyFlow, flowStatus) {
	v, ok := b.flows.LoadAndDelete(codeChallenge)
	if !ok {
		if t, ok := b.flowConsumed.Load(codeChallenge); ok {
			tomb := t.(*flowTombstone)
			if b.nowFn().Before(tomb.expiresAt) {
				return nil, tomb.status
			}
			b.flowConsumed.Delete(codeChallenge)
		}
		return nil, flowStatusMissing
	}
	flow := v.(*blueskyFlow)
	now := b.nowFn()
	if now.After(flow.expiresAt) {
		// Tombstone the expired flow so a duplicate callback keeps returning
		// ErrBlueskyFlowExpired instead of degrading to ErrBlueskyFlowMissing
		// once the binding has been removed.
		b.flowConsumed.Store(codeChallenge, &flowTombstone{
			status:    flowStatusExpired,
			expiresAt: now.Add(b.tombstoneTTL),
		})
		return nil, flowStatusExpired
	}
	b.flowConsumed.Store(codeChallenge, &flowTombstone{
		status:    flowStatusConsumed,
		expiresAt: now.Add(b.tombstoneTTL),
	})
	return flow, flowStatusOK
}

func (b *Bluesky) sweepExpired(now time.Time) {
	b.flows.Range(func(k, v any) bool {
		if now.After(v.(*blueskyFlow).expiresAt) {
			b.flows.Delete(k)
		}
		return true
	})
	b.flowConsumed.Range(func(k, v any) bool {
		if now.After(v.(*flowTombstone).expiresAt) {
			b.flowConsumed.Delete(k)
		}
		return true
	})
}
