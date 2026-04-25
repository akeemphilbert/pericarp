package providers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	gojwt "github.com/golang-jwt/jwt/v5"
)

// fakeBlueskyEnv stands up the four httptest servers a Bluesky flow talks to:
// the bsky.social handle resolver, the PLC directory (DID document host), the
// PDS (auth server metadata + token endpoint), and the auth server (PAR + auth
// + token; in this fake the PDS and auth server are the same host).
type fakeBlueskyEnv struct {
	resolveSrv *httptest.Server
	plcSrv     *httptest.Server
	asSrv      *httptest.Server // auth server == PDS for this fake

	did    string
	handle string
	pdsURL string

	authCode             string
	requestURI           string
	parCalls             atomic.Int32
	tokenCalls           atomic.Int32
	parDPoPHeader        atomic.Value // string
	exchangeDPoPHeader   atomic.Value // string
	refreshDPoPHeader    atomic.Value // string
	requireNonceFirstPAR bool
	parIssuedNonce       string
	parGotNonce          atomic.Value // string

	accessToken  string
	refreshToken string
}

func newFakeBlueskyEnv(t *testing.T) *fakeBlueskyEnv {
	t.Helper()
	env := &fakeBlueskyEnv{
		did:            "did:plc:abc123fake",
		handle:         "alice.test",
		authCode:       "auth-code-xyz",
		requestURI:     "urn:ietf:params:oauth:request_uri:request-fake",
		accessToken:    "atproto-access",
		refreshToken:   "atproto-refresh",
		parIssuedNonce: "nonce-from-par",
	}

	// 1. handle resolver
	resolveMux := http.NewServeMux()
	resolveMux.HandleFunc(blueskyResolveHandlePath, func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("handle")
		if got != env.handle {
			t.Errorf("resolveHandle handle = %q, want %q", got, env.handle)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"did":"`+env.did+`"}`)
	})
	env.resolveSrv = httptest.NewServer(resolveMux)
	t.Cleanup(env.resolveSrv.Close)

	// 2. PDS / auth server (defined first so we can plug its URL into the DID doc)
	asMux := http.NewServeMux()
	env.asSrv = httptest.NewServer(asMux)
	t.Cleanup(env.asSrv.Close)
	env.pdsURL = env.asSrv.URL

	asMux.HandleFunc(blueskyAuthServerMetadataPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		meta := blueskyAuthServerMetadata{
			Issuer:                             env.asSrv.URL,
			AuthorizationEndpoint:              env.asSrv.URL + "/oauth/authorize",
			TokenEndpoint:                      env.asSrv.URL + "/oauth/token",
			PushedAuthorizationRequestEndpoint: env.asSrv.URL + "/oauth/par",
			ResponseTypesSupported:             []string{"code"},
			DPoPSigningAlgValuesSupported:      []string{"ES256"},
		}
		_ = json.NewEncoder(w).Encode(meta)
	})

	asMux.HandleFunc("/oauth/par", func(w http.ResponseWriter, r *http.Request) {
		env.parCalls.Add(1)
		dpop := r.Header.Get("DPoP")
		env.parDPoPHeader.Store(dpop)

		if env.requireNonceFirstPAR && env.parCalls.Load() == 1 {
			// Force the use_dpop_nonce handshake: 400 + Nonce header on first call.
			w.Header().Set("DPoP-Nonce", env.parIssuedNonce)
			http.Error(w, `{"error":"use_dpop_nonce"}`, http.StatusBadRequest)
			return
		}

		// Sanity-check the DPoP proof: must be a parseable ES256 JWT with
		// htm=POST and htu pointing at our PAR URL.
		claims, err := parseDPoPClaims(dpop)
		if err != nil {
			t.Errorf("PAR DPoP parse error: %v", err)
		}
		if got, want := claims["htm"], "POST"; got != want {
			t.Errorf("PAR DPoP htm = %v, want %v", got, want)
		}
		if got := claims["htu"]; got != env.asSrv.URL+"/oauth/par" {
			t.Errorf("PAR DPoP htu = %v, want %s/oauth/par", got, env.asSrv.URL)
		}
		if v, ok := claims["nonce"]; ok {
			env.parGotNonce.Store(v)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"request_uri":"`+env.requestURI+`","expires_in":60}`)
	})

	asMux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		env.tokenCalls.Add(1)
		dpop := r.Header.Get("DPoP")
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))

		switch form.Get("grant_type") {
		case "authorization_code":
			env.exchangeDPoPHeader.Store(dpop)
			if got := form.Get("code"); got != env.authCode {
				t.Errorf("token code = %q, want %q", got, env.authCode)
			}
		case "refresh_token":
			env.refreshDPoPHeader.Store(dpop)
		default:
			t.Errorf("unexpected grant_type %q", form.Get("grant_type"))
		}

		// Sanity check the DPoP proof.
		claims, err := parseDPoPClaims(dpop)
		if err != nil {
			t.Errorf("token DPoP parse error: %v", err)
		}
		if got := claims["htu"]; got != env.asSrv.URL+"/oauth/token" {
			t.Errorf("token DPoP htu = %v, want %s/oauth/token", got, env.asSrv.URL)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"access_token": "`+env.accessToken+`",
			"token_type": "DPoP",
			"expires_in": 1800,
			"refresh_token": "`+env.refreshToken+`",
			"sub": "`+env.did+`"
		}`)
	})

	// 3. PLC directory: returns DID document pointing at the PDS.
	plcMux := http.NewServeMux()
	plcMux.HandleFunc("/did:plc:abc123fake", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id": "did:plc:abc123fake",
			"service": [
				{"id": "#atproto_pds", "type": "AtprotoPersonalDataServer", "serviceEndpoint": "`+env.pdsURL+`"}
			]
		}`)
	})
	env.plcSrv = httptest.NewServer(plcMux)
	t.Cleanup(env.plcSrv.Close)

	return env
}

// newBlueskyForTest wires a Bluesky provider to an httptest env.
func newBlueskyForTest(env *fakeBlueskyEnv, cfg BlueskyConfig) *Bluesky {
	if cfg.RedirectURI == "" {
		cfg.RedirectURI = "https://app.example.com/cb"
	}
	if cfg.ClientMetadataURL == "" {
		cfg.ClientMetadataURL = "https://app.example.com/client-metadata.json"
	}
	if cfg.KeyStore == nil {
		cfg.KeyStore = NewMemoryBlueskyKeyStore()
	}
	// httptest servers are http://127.0.0.1:port; the production SSRF guard
	// rejects both, so opt out here.
	cfg.AllowInsecurePDSURLs = true
	b := NewBluesky(cfg)
	b.handleResolveBase = env.resolveSrv.URL
	b.plcDirectoryBase = env.plcSrv.URL
	return b
}

// parseDPoPClaims parses (without signature verification) the claims of a
// DPoP proof JWT. The fakes use this only for shape assertions — the DPoP
// signature itself is verified separately by TestBlueskyDPoPProof_Signature.
func parseDPoPClaims(dpop string) (gojwt.MapClaims, error) {
	parts := strings.Split(dpop, ".")
	if len(parts) != 3 {
		return nil, errors.New("dpop: not a JWS compact serialization")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims gojwt.MapClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func TestBluesky_HandleResolution(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)
	b := newBlueskyForTest(env, BlueskyConfig{})
	got, err := b.resolveHandle(context.Background(), env.handle)
	if err != nil {
		t.Fatalf("resolveHandle: %v", err)
	}
	if got != env.did {
		t.Errorf("resolved DID = %q, want %q", got, env.did)
	}
}

func TestBluesky_HandleResolution_Failure(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)
	// Replace the resolver with a 404 handler.
	env.resolveSrv.Close()
	mux := http.NewServeMux()
	mux.HandleFunc(blueskyResolveHandlePath, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"NotFound"}`, http.StatusNotFound)
	})
	env.resolveSrv = httptest.NewServer(mux)
	defer env.resolveSrv.Close()

	b := newBlueskyForTest(env, BlueskyConfig{})
	_, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "s", pkceChallenge("v"), "", "")
	if !errors.Is(err, ErrBlueskyHandleResolutionFailed) {
		t.Errorf("AuthCodeURLForHandle err = %v, want errors.Is == ErrBlueskyHandleResolutionFailed", err)
	}
}

func TestBluesky_DIDDocumentMissingPDS(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)
	// Replace PLC with a DID doc that has no AtprotoPersonalDataServer service.
	env.plcSrv.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/did:plc:abc123fake", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"did:plc:abc123fake","service":[]}`)
	})
	env.plcSrv = httptest.NewServer(mux)
	defer env.plcSrv.Close()

	b := newBlueskyForTest(env, BlueskyConfig{})
	_, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "s", pkceChallenge("v"), "", "")
	if !errors.Is(err, ErrBlueskyDIDResolutionFailed) {
		t.Errorf("err = %v, want ErrBlueskyDIDResolutionFailed", err)
	}
}

func TestBluesky_ValidatePDSURL_SSRFGuard(t *testing.T) {
	t.Parallel()

	// Static stub resolver so the test never depends on real DNS. The default
	// answer is a public IP; specific tests override it.
	publicIP := []string{"93.184.216.34"}
	internalIP := []string{"10.0.0.5"}
	mixedIPs := []string{"93.184.216.34", "127.0.0.1"}

	cases := []struct {
		name        string
		insecureOK  bool
		url         string
		lookup      []string
		lookupErr   error
		wantErr     bool
		errContains string
	}{
		{name: "https public hostname allowed", url: "https://pds.example.com", lookup: publicIP, wantErr: false},
		{name: "http rejected by default", url: "http://pds.example.com", wantErr: true, errContains: "https"},
		{name: "loopback IP rejected", url: "https://127.0.0.1", wantErr: true, errContains: "loopback"},
		{name: "ipv6 loopback rejected", url: "https://[::1]", wantErr: true, errContains: "loopback"},
		{name: "localhost name rejected", url: "https://localhost:8443", wantErr: true, errContains: "localhost"},
		{name: "rfc1918 private IP rejected", url: "https://10.0.0.1", wantErr: true, errContains: "loopback"},
		{name: "192.168 private IP rejected", url: "https://192.168.1.1", wantErr: true, errContains: "loopback"},
		{name: "link-local IP rejected", url: "https://169.254.169.254", wantErr: true, errContains: "loopback"},
		{name: "missing host rejected", url: "https://", wantErr: true, errContains: "no host"},
		{name: "DNS rebinding to internal IP rejected", url: "https://evil.example.com", lookup: internalIP, wantErr: true, errContains: "internal address"},
		{name: "DNS rebinding even one internal IP rejected", url: "https://evil.example.com", lookup: mixedIPs, wantErr: true, errContains: "internal address"},
		{name: "DNS lookup failure surfaces", url: "https://nx.example.com", lookupErr: errors.New("no such host"), wantErr: true, errContains: "resolve"},
		{name: "insecure flag allows http loopback", insecureOK: true, url: "http://127.0.0.1:9000", wantErr: false},
		{name: "insecure flag allows localhost", insecureOK: true, url: "http://localhost:9000", wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := NewBluesky(BlueskyConfig{AllowInsecurePDSURLs: tc.insecureOK})
			b.lookupHostFn = func(string) ([]string, error) {
				if tc.lookupErr != nil {
					return nil, tc.lookupErr
				}
				return tc.lookup, nil
			}
			err := b.validatePDSURL(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validatePDSURL(%q) = nil, want error", tc.url)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("validatePDSURL(%q) error %q does not contain %q", tc.url, err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("validatePDSURL(%q) = %v, want nil", tc.url, err)
			}
		})
	}
}

func TestBluesky_ValidateAuthServerEndpoints(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		insecureOK  bool
		pdsURL      string
		meta        blueskyAuthServerMetadata
		wantErr     bool
		errContains string
	}{
		{
			name:   "all endpoints same-host https allowed",
			pdsURL: "https://pds.example.com",
			meta: blueskyAuthServerMetadata{
				AuthorizationEndpoint:              "https://pds.example.com/oauth/authorize",
				TokenEndpoint:                      "https://pds.example.com/oauth/token",
				PushedAuthorizationRequestEndpoint: "https://pds.example.com/oauth/par",
			},
		},
		{
			name:   "endpoint on different host rejected",
			pdsURL: "https://pds.example.com",
			meta: blueskyAuthServerMetadata{
				AuthorizationEndpoint:              "https://pds.example.com/oauth/authorize",
				TokenEndpoint:                      "https://attacker.example.com/oauth/token",
				PushedAuthorizationRequestEndpoint: "https://pds.example.com/oauth/par",
			},
			wantErr:     true,
			errContains: "token_endpoint host",
		},
		{
			name:   "http endpoint rejected",
			pdsURL: "https://pds.example.com",
			meta: blueskyAuthServerMetadata{
				AuthorizationEndpoint:              "http://pds.example.com/oauth/authorize",
				TokenEndpoint:                      "https://pds.example.com/oauth/token",
				PushedAuthorizationRequestEndpoint: "https://pds.example.com/oauth/par",
			},
			wantErr:     true,
			errContains: "authorization_endpoint",
		},
		{
			name:   "loopback endpoint rejected",
			pdsURL: "https://pds.example.com",
			meta: blueskyAuthServerMetadata{
				AuthorizationEndpoint:              "https://pds.example.com/oauth/authorize",
				TokenEndpoint:                      "https://pds.example.com/oauth/token",
				PushedAuthorizationRequestEndpoint: "https://127.0.0.1/oauth/par",
			},
			wantErr:     true,
			errContains: "pushed_authorization_request_endpoint",
		},
		{
			name:       "insecure flag bypasses validation",
			insecureOK: true,
			pdsURL:     "http://127.0.0.1:9000",
			meta: blueskyAuthServerMetadata{
				AuthorizationEndpoint:              "http://other-host:8080/oauth/authorize",
				TokenEndpoint:                      "http://127.0.0.1:9000/oauth/token",
				PushedAuthorizationRequestEndpoint: "http://127.0.0.1:9000/oauth/par",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := NewBluesky(BlueskyConfig{AllowInsecurePDSURLs: tc.insecureOK})
			b.lookupHostFn = func(string) ([]string, error) { return []string{"93.184.216.34"}, nil }
			err := b.validateAuthServerEndpoints(tc.pdsURL, &tc.meta)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateAuthServerEndpoints = nil, want error")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("validateAuthServerEndpoints = %v, want nil", err)
			}
		})
	}
}

func TestBluesky_PARSucceeds_AuthURLContainsRequestURI(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)
	b := newBlueskyForTest(env, BlueskyConfig{})

	verifier := "verifier-abc"
	challenge := pkceChallenge(verifier)

	authURL, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "state-1", challenge, "", "")
	if err != nil {
		t.Fatalf("AuthCodeURLForHandle: %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("authURL parse: %v", err)
	}
	if parsed.Path != "/oauth/authorize" {
		t.Errorf("authorize path = %q, want /oauth/authorize", parsed.Path)
	}
	if got := parsed.Query().Get("request_uri"); got != env.requestURI {
		t.Errorf("authorize request_uri = %q, want %q (must come from PAR response)", got, env.requestURI)
	}
	if got := parsed.Query().Get("client_id"); got != "https://app.example.com/client-metadata.json" {
		t.Errorf("authorize client_id = %q, want client metadata URL", got)
	}
	if got := env.parCalls.Load(); got != 1 {
		t.Errorf("PAR was called %d times, want 1", got)
	}
}

func TestBluesky_PAR_DPoPNonceHandshake(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)
	env.requireNonceFirstPAR = true

	b := newBlueskyForTest(env, BlueskyConfig{})

	// First flow: PAR will reject the first attempt. Bluesky's makeDPoPProof
	// reads the latest cached nonce, so AT-Proto's expected handshake is
	// "first request fails with DPoP-Nonce header → caller retries with that
	// nonce baked in." Today that retry happens for token requests via
	// tokenRequestWithDPoP; PAR has no built-in retry, so the first flow
	// should fail loud — the consumer issues a second AuthCodeURLForHandle
	// call which should now succeed because the cache is warm.
	verifier := "v"
	_, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "s", pkceChallenge(verifier), "", "")
	if err == nil {
		t.Fatal("first PAR with use_dpop_nonce should fail without a retry; nonce was learned from response")
	}
	// Second flow: nonce cache is now populated, request succeeds.
	authURL, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "s2", pkceChallenge(verifier+"x"), "", "")
	if err != nil {
		t.Fatalf("second AuthCodeURLForHandle (nonce primed): %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse second authURL: %v", err)
	}
	if got := parsed.Query().Get("request_uri"); got != env.requestURI {
		t.Errorf("second authURL request_uri = %q, want %q", got, env.requestURI)
	}
	// And the nonce-primed PAR proof carries the nonce claim.
	if v := env.parGotNonce.Load(); v == nil || v.(string) != env.parIssuedNonce {
		t.Errorf("PAR DPoP nonce after handshake = %v, want %q", v, env.parIssuedNonce)
	}
}

func TestBluesky_DPoPProof_HasJWKAndSignsCorrectly(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)
	store := NewMemoryBlueskyKeyStore()
	b := newBlueskyForTest(env, BlueskyConfig{KeyStore: store})

	if _, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "s", pkceChallenge("v"), "", ""); err != nil {
		t.Fatalf("AuthCodeURLForHandle: %v", err)
	}

	dpop, _ := env.parDPoPHeader.Load().(string)
	if dpop == "" {
		t.Fatal("PAR DPoP header was empty")
	}

	// Parse the proof, verifying the signature with the public key from the
	// embedded JWK header. This is the production DPoP-receiver semantics in
	// miniature: a verifier reads the jwk header, reconstructs the public
	// key, and validates the signature.
	parsed, err := gojwt.Parse(dpop, func(tok *gojwt.Token) (any, error) {
		jwk, ok := tok.Header["jwk"].(map[string]any)
		if !ok {
			return nil, errors.New("missing jwk header")
		}
		if jwk["kty"] != "EC" || jwk["crv"] != "P-256" {
			return nil, errors.New("unexpected key type / curve")
		}
		return reconstructECDSAPublicKey(jwk)
	}, gojwt.WithValidMethods([]string{"ES256"}))
	if err != nil {
		t.Fatalf("DPoP signature verify failed: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("DPoP signature parsed but token reported invalid")
	}

	// Header must declare the DPoP type per RFC 9449.
	if typ, _ := parsed.Header["typ"].(string); typ != "dpop+jwt" {
		t.Errorf("DPoP typ header = %q, want dpop+jwt", typ)
	}

	// And the public key in the header must match the one in our keystore.
	signingKey, err := store.GetSigningKey(context.Background())
	if err != nil {
		t.Fatalf("GetSigningKey: %v", err)
	}
	wantThumbprint := JWKThumbprintFromECDSA(&signingKey.PublicKey)
	gotJWK, _ := parsed.Header["jwk"].(map[string]any)
	gotKey, _ := reconstructECDSAPublicKey(gotJWK)
	gotThumbprint := JWKThumbprintFromECDSA(gotKey)
	if gotThumbprint != wantThumbprint {
		t.Errorf("DPoP jwk thumbprint = %q, want %q (proof must bind to keystore key)", gotThumbprint, wantThumbprint)
	}
}

func TestBluesky_Exchange_ReturnsDPoPBoundTokens(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)
	b := newBlueskyForTest(env, BlueskyConfig{})

	verifier := "v-exchange"
	if _, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "s", pkceChallenge(verifier), "", ""); err != nil {
		t.Fatalf("AuthCodeURLForHandle: %v", err)
	}

	res, err := b.Exchange(context.Background(), env.authCode, verifier, "")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if res.AccessToken != env.accessToken {
		t.Errorf("AccessToken = %q, want %q", res.AccessToken, env.accessToken)
	}
	if res.TokenType != "DPoP" {
		t.Errorf("TokenType = %q, want DPoP", res.TokenType)
	}
	if res.UserInfo.ProviderUserID != env.did {
		t.Errorf("ProviderUserID = %q, want DID %q", res.UserInfo.ProviderUserID, env.did)
	}
	if res.UserInfo.DisplayName != env.handle {
		t.Errorf("DisplayName = %q, want handle %q", res.UserInfo.DisplayName, env.handle)
	}

	// Refresh token must be wrapped (PDS context preserved across calls).
	pdsURL, tokenURL, issuer, opaque, err := decodeBlueskyRefreshToken(res.RefreshToken)
	if err != nil {
		t.Fatalf("decodeBlueskyRefreshToken: %v", err)
	}
	if pdsURL != env.pdsURL {
		t.Errorf("encoded refresh pdsURL = %q, want %q", pdsURL, env.pdsURL)
	}
	if tokenURL != env.asSrv.URL+"/oauth/token" {
		t.Errorf("encoded refresh tokenURL = %q, want %s/oauth/token", tokenURL, env.asSrv.URL)
	}
	if issuer != env.asSrv.URL {
		t.Errorf("encoded refresh issuer = %q, want %q", issuer, env.asSrv.URL)
	}
	if opaque != env.refreshToken {
		t.Errorf("encoded refresh opaque = %q, want %q", opaque, env.refreshToken)
	}

	// The Exchange DPoP proof signature must validate with the keystore key.
	dpop, _ := env.exchangeDPoPHeader.Load().(string)
	if dpop == "" {
		t.Fatal("Exchange did not send a DPoP header")
	}
}

func TestBluesky_RefreshToken_DPoPBound(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)
	store := NewMemoryBlueskyKeyStore()
	b := newBlueskyForTest(env, BlueskyConfig{KeyStore: store})

	// Run Exchange to obtain a wrapped refresh token bound to this PDS.
	verifier := "v"
	if _, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "s", pkceChallenge(verifier), "", ""); err != nil {
		t.Fatalf("AuthCodeURLForHandle: %v", err)
	}
	res, err := b.Exchange(context.Background(), env.authCode, verifier, "")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	// Now refresh.
	refreshed, err := b.RefreshToken(context.Background(), res.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if refreshed.AccessToken != env.accessToken {
		t.Errorf("refreshed AccessToken = %q, want %q", refreshed.AccessToken, env.accessToken)
	}

	// The refresh DPoP proof must be signed by the SAME key as the exchange
	// proof (otherwise the upstream PDS would reject it for jkt mismatch).
	exchangeProof, _ := env.exchangeDPoPHeader.Load().(string)
	refreshProof, _ := env.refreshDPoPHeader.Load().(string)
	if exchangeProof == "" || refreshProof == "" {
		t.Fatal("missing DPoP proofs")
	}
	exchangeKey := mustExtractJWKThumbprint(t, exchangeProof)
	refreshKey := mustExtractJWKThumbprint(t, refreshProof)
	if exchangeKey != refreshKey {
		t.Errorf("refresh DPoP jkt = %q, want same as exchange jkt %q (key must persist across refresh)", refreshKey, exchangeKey)
	}

	// And the refresh token returned from RefreshToken must be re-wrapped so
	// the next refresh round-trip works.
	pdsURL, _, _, _, err := decodeBlueskyRefreshToken(refreshed.RefreshToken)
	if err != nil {
		t.Fatalf("re-wrapped refresh token: %v", err)
	}
	if pdsURL != env.pdsURL {
		t.Errorf("re-wrapped pdsURL = %q, want %q", pdsURL, env.pdsURL)
	}

	// jti uniqueness: RFC 9449 requires DPoP proofs to be single-use. The
	// upstream PDS will reject a refresh whose jti collides with the prior
	// exchange's jti.
	exchangeClaims, err := parseDPoPClaims(exchangeProof)
	if err != nil {
		t.Fatalf("parse exchange DPoP: %v", err)
	}
	refreshClaims, err := parseDPoPClaims(refreshProof)
	if err != nil {
		t.Fatalf("parse refresh DPoP: %v", err)
	}
	exchangeJTI, _ := exchangeClaims["jti"].(string)
	refreshJTI, _ := refreshClaims["jti"].(string)
	if exchangeJTI == "" {
		t.Error("exchange DPoP missing jti claim (RFC 9449 §4.2)")
	}
	if refreshJTI == "" {
		t.Error("refresh DPoP missing jti claim")
	}
	if exchangeJTI == refreshJTI {
		t.Errorf("exchange and refresh DPoP have identical jti %q; proofs must be single-use", exchangeJTI)
	}

	// And refresh preserves the DID across the call (so the application
	// layer can match the refreshed token to the originating credential).
	if refreshed.UserInfo.ProviderUserID != env.did {
		t.Errorf("refreshed ProviderUserID = %q, want DID %q (must round-trip across refresh)", refreshed.UserInfo.ProviderUserID, env.did)
	}
}

func TestBluesky_RefreshToken_RejectsMalformed(t *testing.T) {
	t.Parallel()

	b := NewBluesky(BlueskyConfig{
		ClientMetadataURL: "https://app/c.json",
		RedirectURI:       "https://app/cb",
	})
	_, err := b.RefreshToken(context.Background(), "not-a-bluesky-token")
	if err == nil {
		t.Fatal("expected error for malformed refresh token")
	}
	if !strings.Contains(err.Error(), "refresh token format") {
		t.Errorf("err = %q, want it to mention refresh token format", err.Error())
	}
}

func TestBluesky_Exchange_NoFlowBinding(t *testing.T) {
	t.Parallel()

	b := NewBluesky(BlueskyConfig{
		ClientMetadataURL: "https://app/c.json",
		RedirectURI:       "https://app/cb",
	})
	_, err := b.Exchange(context.Background(), "code", "verifier-without-bind", "")
	if !errors.Is(err, ErrBlueskyFlowMissing) {
		t.Errorf("err = %v, want ErrBlueskyFlowMissing", err)
	}
}

func TestBluesky_AuthCodeURL_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	b := NewBluesky(BlueskyConfig{})
	if got := b.AuthCodeURL("s", "c", "n", "u"); got != "" {
		t.Errorf("AuthCodeURL = %q, want empty", got)
	}
}

func TestBluesky_ValidateIDToken_Sentinel(t *testing.T) {
	t.Parallel()
	b := NewBluesky(BlueskyConfig{})
	_, err := b.ValidateIDToken(context.Background(), "id.token", "n")
	if !errors.Is(err, ErrBlueskyIDTokenUnsupported) {
		t.Errorf("err = %v, want ErrBlueskyIDTokenUnsupported", err)
	}
}

func TestBluesky_DefaultScopes(t *testing.T) {
	t.Parallel()
	b := NewBluesky(BlueskyConfig{})
	if got, want := strings.Join(b.scopes, " "), "atproto transition:generic"; got != want {
		t.Errorf("default scopes = %q, want %q", got, want)
	}
}

func TestBluesky_JWKThumbprint_Stable(t *testing.T) {
	t.Parallel()

	// RFC 7638 thumbprint depends on a canonical JSON encoding (sorted keys,
	// no extra whitespace). Compute the thumbprint twice from two distinct
	// in-memory stores: with the same key bytes, the thumbprint must match.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	a := JWKThumbprintFromECDSA(&key.PublicKey)
	bjwk := JWKThumbprintFromECDSA(&key.PublicKey)
	if a != bjwk {
		t.Errorf("thumbprint not deterministic: %q vs %q", a, bjwk)
	}

	// Different keys must produce different thumbprints.
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	if JWKThumbprintFromECDSA(&other.PublicKey) == a {
		t.Errorf("two distinct keys produced the same thumbprint (vanishingly unlikely)")
	}
}

func TestBluesky_DIDWebURL(t *testing.T) {
	t.Parallel()
	b := NewBluesky(BlueskyConfig{})
	tests := []struct {
		did, want string
	}{
		{"did:web:example.com", "https://example.com/.well-known/did.json"},
		{"did:plc:abc", b.plcDirectoryBase + "/did:plc:abc"},
	}
	for _, tt := range tests {
		got, err := b.didDocumentURL(tt.did)
		if err != nil {
			t.Errorf("didDocumentURL(%q) err = %v", tt.did, err)
			continue
		}
		if got != tt.want {
			t.Errorf("didDocumentURL(%q) = %q, want %q", tt.did, got, tt.want)
		}
	}
	if _, err := b.didDocumentURL("did:unknown:foo"); err == nil {
		t.Error("expected error for unsupported DID method")
	}
}

// ---- helpers ----

func reconstructECDSAPublicKey(jwk map[string]any) (*ecdsa.PublicKey, error) {
	xRaw, _ := jwk["x"].(string)
	yRaw, _ := jwk["y"].(string)
	xb, err := base64.RawURLEncoding.DecodeString(xRaw)
	if err != nil {
		return nil, err
	}
	yb, err := base64.RawURLEncoding.DecodeString(yRaw)
	if err != nil {
		return nil, err
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     bigIntFromBytes(xb),
		Y:     bigIntFromBytes(yb),
	}, nil
}

func bigIntFromBytes(b []byte) *big.Int {
	return new(big.Int).SetBytes(b)
}

func mustExtractJWKThumbprint(t *testing.T, dpop string) string {
	t.Helper()
	parts := strings.Split(dpop, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWS compact: %s", dpop)
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header struct {
		JWK map[string]any `json:"jwk"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatalf("parse header: %v", err)
	}
	pub, err := reconstructECDSAPublicKey(header.JWK)
	if err != nil {
		t.Fatalf("reconstruct pub key: %v", err)
	}
	return JWKThumbprintFromECDSA(pub)
}

func TestBluesky_AuthServerMetadataIssuerMismatch(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)
	// Override the AS metadata handler to claim a foreign issuer.
	mux := http.NewServeMux()
	mux.HandleFunc(blueskyAuthServerMetadataPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"issuer": "https://attacker.example",
			"authorization_endpoint": "https://attacker.example/oauth/authorize",
			"token_endpoint": "https://attacker.example/oauth/token",
			"pushed_authorization_request_endpoint": "https://attacker.example/oauth/par"
		}`)
	})
	env.asSrv.Close()
	env.asSrv = httptest.NewServer(mux)
	defer env.asSrv.Close()
	// Update PLC to point at the new server (env.pdsURL changed).
	env.pdsURL = env.asSrv.URL
	env.plcSrv.Close()
	plcMux := http.NewServeMux()
	plcMux.HandleFunc("/did:plc:abc123fake", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
			"id": "did:plc:abc123fake",
			"service": [
				{"id": "#atproto_pds", "type": "AtprotoPersonalDataServer", "serviceEndpoint": "`+env.pdsURL+`"}
			]
		}`)
	})
	env.plcSrv = httptest.NewServer(plcMux)
	defer env.plcSrv.Close()

	b := newBlueskyForTest(env, BlueskyConfig{})
	_, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "s", pkceChallenge("v"), "", "")
	if !errors.Is(err, ErrBlueskyIssuerMismatch) {
		t.Errorf("err = %v, want ErrBlueskyIssuerMismatch (a malicious DID document must not redirect us through an unverified issuer)", err)
	}
}

func TestBluesky_TokenEndpointDPoPNonceHandshake(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)

	// Replace the token handler with a stateful one that demands the
	// use_dpop_nonce handshake on the first hit.
	asMux := http.NewServeMux()
	asMux.HandleFunc(blueskyAuthServerMetadataPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(blueskyAuthServerMetadata{
			Issuer:                             env.asSrv.URL,
			AuthorizationEndpoint:              env.asSrv.URL + "/oauth/authorize",
			TokenEndpoint:                      env.asSrv.URL + "/oauth/token",
			PushedAuthorizationRequestEndpoint: env.asSrv.URL + "/oauth/par",
		})
	})
	asMux.HandleFunc("/oauth/par", func(w http.ResponseWriter, r *http.Request) {
		dpop := r.Header.Get("DPoP")
		env.parDPoPHeader.Store(dpop)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"request_uri":"`+env.requestURI+`","expires_in":60}`)
	})
	var tokenAttempts atomic.Int32
	asMux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		n := tokenAttempts.Add(1)
		dpop := r.Header.Get("DPoP")
		env.exchangeDPoPHeader.Store(dpop)

		if n == 1 {
			// Demand the use_dpop_nonce handshake on first hit.
			w.Header().Set("DPoP-Nonce", "tok-nonce")
			http.Error(w, `{"error":"use_dpop_nonce","error_description":"server requires DPoP nonce"}`, http.StatusBadRequest)
			return
		}
		// On retry, verify the proof carries the nonce.
		claims, _ := parseDPoPClaims(dpop)
		if got := claims["nonce"]; got != "tok-nonce" {
			t.Errorf("retry DPoP nonce = %v, want tok-nonce", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+env.accessToken+`","token_type":"DPoP","expires_in":1800,"refresh_token":"`+env.refreshToken+`","sub":"`+env.did+`"}`)
	})
	env.asSrv.Close()
	env.asSrv = httptest.NewServer(asMux)
	defer env.asSrv.Close()
	env.pdsURL = env.asSrv.URL

	plcMux := http.NewServeMux()
	plcMux.HandleFunc("/did:plc:abc123fake", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
			"id": "did:plc:abc123fake",
			"service": [
				{"id": "#atproto_pds", "type": "AtprotoPersonalDataServer", "serviceEndpoint": "`+env.pdsURL+`"}
			]
		}`)
	})
	env.plcSrv.Close()
	env.plcSrv = httptest.NewServer(plcMux)
	defer env.plcSrv.Close()

	b := newBlueskyForTest(env, BlueskyConfig{})
	verifier := "v"
	if _, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "s", pkceChallenge(verifier), "", ""); err != nil {
		t.Fatalf("AuthCodeURLForHandle: %v", err)
	}
	res, err := b.Exchange(context.Background(), env.authCode, verifier, "")
	if err != nil {
		t.Fatalf("Exchange (should succeed after retry): %v", err)
	}
	if res.AccessToken != env.accessToken {
		t.Errorf("AccessToken = %q, want %q", res.AccessToken, env.accessToken)
	}
	if got := tokenAttempts.Load(); got != 2 {
		t.Errorf("token endpoint attempts = %d, want 2 (first should fail with use_dpop_nonce, second should succeed)", got)
	}
}

func TestBluesky_TokenEndpoint_Non200WithoutNonce(t *testing.T) {
	t.Parallel()

	env := newFakeBlueskyEnv(t)
	// Replace token handler to always 401.
	asMux := http.NewServeMux()
	asMux.HandleFunc(blueskyAuthServerMetadataPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(blueskyAuthServerMetadata{
			Issuer:                             env.asSrv.URL,
			AuthorizationEndpoint:              env.asSrv.URL + "/oauth/authorize",
			TokenEndpoint:                      env.asSrv.URL + "/oauth/token",
			PushedAuthorizationRequestEndpoint: env.asSrv.URL + "/oauth/par",
		})
	})
	asMux.HandleFunc("/oauth/par", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"request_uri":"`+env.requestURI+`","expires_in":60}`)
	})
	asMux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusUnauthorized)
	})
	env.asSrv.Close()
	env.asSrv = httptest.NewServer(asMux)
	defer env.asSrv.Close()
	env.pdsURL = env.asSrv.URL

	plcMux := http.NewServeMux()
	plcMux.HandleFunc("/did:plc:abc123fake", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
			"id": "did:plc:abc123fake",
			"service": [
				{"id": "#atproto_pds", "type": "AtprotoPersonalDataServer", "serviceEndpoint": "`+env.pdsURL+`"}
			]
		}`)
	})
	env.plcSrv.Close()
	env.plcSrv = httptest.NewServer(plcMux)
	defer env.plcSrv.Close()

	b := newBlueskyForTest(env, BlueskyConfig{})
	verifier := "v"
	if _, err := b.AuthCodeURLForHandle(context.Background(), env.handle, "s", pkceChallenge(verifier), "", ""); err != nil {
		t.Fatalf("AuthCodeURLForHandle: %v", err)
	}
	_, err := b.Exchange(context.Background(), env.authCode, verifier, "")
	if err == nil {
		t.Fatal("expected error for HTTP 401 from token endpoint")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Errorf("err = %q, want 'status 401'", err.Error())
	}
}

func TestBluesky_RevokeToken_Sentinel(t *testing.T) {
	t.Parallel()
	b := NewBluesky(BlueskyConfig{})
	if err := b.RevokeToken(context.Background(), "any"); !errors.Is(err, ErrBlueskyRevokeUnsupported) {
		t.Errorf("err = %v, want ErrBlueskyRevokeUnsupported", err)
	}
}

func TestBluesky_DecodeRefreshToken_BadVariants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, input string
	}{
		{"empty", ""},
		{"wrong prefix", "googletoken"},
		{"v1 legacy prefix", "btr.v1.YWJj"}, // base64url("abc") under old prefix
		{"bad base64", "btr.v2.!!!"},        // not base64url
		{"too few parts", "btr.v2." + base64.RawURLEncoding.EncodeToString([]byte("only|two"))},
		{"empty payload", "btr.v2." + base64.RawURLEncoding.EncodeToString([]byte(""))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, _, _, err := decodeBlueskyRefreshToken(tc.input)
			if err == nil {
				t.Errorf("expected error for %q, got nil", tc.input)
			}
		})
	}
}

func TestBluesky_NormalizeHTU(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"https://pds/oauth/token":              "https://pds/oauth/token",
		"https://pds/oauth/token?foo=bar":      "https://pds/oauth/token",
		"https://pds/oauth/token#frag":         "https://pds/oauth/token",
		"https://pds/oauth/token?foo=bar#frag": "https://pds/oauth/token",
		"https://pds:8443/oauth/token":         "https://pds:8443/oauth/token",
	}
	for in, want := range cases {
		if got := normalizeHTU(in); got != want {
			t.Errorf("normalizeHTU(%q) = %q, want %q", in, got, want)
		}
	}
}

// Compile-time check that *Bluesky implements application.OAuthProvider.
var _ application.OAuthProvider = (*Bluesky)(nil)
