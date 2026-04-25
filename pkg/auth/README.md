# pkg/auth

Authentication and authorization primitives for Pericarp. Ships an aggregate-rooted credential model, a provider-agnostic OAuth abstraction, password support, and a catalogue of pre-built OAuth providers.

The application-layer entry point is `application.DefaultAuthenticationService` (constructed via `NewDefaultAuthenticationService`). It takes an `OAuthProviderRegistry` plus repositories for agents, credentials, sessions, and accounts.

## Provider catalogue

All providers under `pkg/auth/infrastructure/providers/` implement `application.OAuthProvider`. Construct each via its `*Config` struct. Configuration values come from your secrets store — never check secrets into source.

| Provider | Constructor | Notes |
| -------- | ----------- | ----- |
| Apple    | `NewApple(AppleConfig{ClientID, TeamID, KeyID, PrivateKey, Scopes})` | Uses ES256 client_secret JWT (signed at request time). `response_mode=form_post`. **Does not support PKCE** — `codeVerifier` is ignored by `Exchange`; rely on `state` for CSRF defence. ID token validation checks `iss`/`aud`/`exp`/`nonce` but does **not** verify the JWT signature (production deployments should add JWKS verification against `https://appleid.apple.com/auth/keys`). |
| GitHub   | `NewGitHub(GitHubConfig{ClientID, ClientSecret, Scopes})` | No refresh tokens, no ID tokens. Email resolved via two-step `/user` + `/user/emails` API call. |
| Google   | `NewGoogle(GoogleConfig{ClientID, ClientSecret, Scopes})` | Standard OIDC. Default scopes: `openid email profile`. Refresh requires `access_type=offline` (set by default). |
| Microsoft | `NewMicrosoft(MicrosoftConfig{ClientID, ClientSecret, TenantID, Scopes})` | Identity platform v2.0 (Entra ID / Azure AD). `TenantID` defaults to `common`. No revocation endpoint. |
| Facebook | `NewFacebook(FacebookConfig{ClientID, ClientSecret, Scopes})` | Graph API v18.0. Refresh tokens **not** supported (returns `application.ErrTokenRefreshFailed`). ID tokens **not** supported (returns `ErrFacebookIDTokenUnsupported`). User identity resolved via `/me?fields=id,name,email,picture`. Long-lived tokens require a separate server-side `fb_exchange_token` call outside this interface. |
| Mastodon | `NewMastodon(MastodonConfig{AppName, RedirectURI, Scopes, AppCache, Website, InstanceBase})` | Federated. Use `AuthCodeURLForInstance(ctx, host, …)` to start a flow against a specific Mastodon server (`mastodon.social`, `hachyderm.io`, etc.). Apps are auto-registered per host via `POST /api/v1/apps` and cached by `MastodonAppCache`. **Pick a persistent shared `MastodonAppCache` for multi-replica deploys** — Mastodon does not deduplicate registrations server-side, so two replicas without a shared cache leak abandoned apps forever. The default `AppCache` is `NewMemoryMastodonAppCache()` (single-replica only). Email is **not** exposed by Mastodon's public API; `UserInfo.Email` is empty. `UserInfo.ProviderUserID` is namespaced as `<id>@<host>`. ID tokens **not** supported. Refresh tokens **not** supported. |
| Bluesky  | `NewBluesky(BlueskyConfig{ClientMetadataURL, RedirectURI, Scopes, KeyStore})` | AT Protocol OAuth (proposal 0004). Use `AuthCodeURLForHandle(ctx, handle, …)` to start a flow for a Bluesky user; the provider resolves handle → DID → PDS, fetches the PDS's `/.well-known/oauth-authorization-server`, performs PAR, and returns the authorize URL. Tokens are DPoP-bound: `BlueskyKeyStore` stores the ECDSA P-256 signing key whose JWK thumbprint binds every token. **Pick a persistent shared `BlueskyKeyStore` for multi-replica deploys** — refreshing a DPoP-bound token requires the same key that minted it. The default `KeyStore` is `NewMemoryBlueskyKeyStore()` (single-replica only). The consumer must serve a client metadata JSON document at `ClientMetadataURL`; that URL is used as the OAuth `client_id`. ID tokens **not** supported. Standard `RevokeToken` returns `ErrBlueskyRevokeUnsupported` — revoke at the PDS directly. |
| NetSuite | `NewNetSuite(NetSuiteConfig{ClientID, ClientSecret, AccountID, Scopes, AuthEndpoint, TokenEndpoint, RevokeEndpoint, UserInfoEndpoint})` | Per-account hosts derived from `AccountID` (sandbox suffixes like `_SB1` are normalized to `-sb1` in URLs). `AuthEndpoint` / `TokenEndpoint` / `RevokeEndpoint` / `UserInfoEndpoint` each take precedence over the derived URL when set — the safety valve for non-standard hosts and future endpoint changes. ID tokens **not** supported (returns `ErrNetSuiteIDTokenNotSupported`); use `Exchange` to fetch user info from the SuiteTalk REST userinfo endpoint. |

## Sensible default scopes

| Provider | Default | Adjust when |
| -------- | ------- | ---------- |
| Apple | `["name", "email"]` | Always sufficient for sign-in. |
| GitHub | `["read:user", "user:email"]` | Add `repo` only if your service operates on repositories. |
| Google | `["openid", "email", "profile"]` | Pericarp sends `access_type=offline` so refresh tokens work with the default scopes; widen only if you need additional Google API surfaces. |
| Microsoft | `["openid", "email", "profile", "offline_access"]` | Add Graph scopes only if your service calls Microsoft Graph for things beyond identity. |
| Facebook | `["email", "public_profile"]` | Add `pages_show_list` etc. only if your app is approved for those scopes. |
| Mastodon | `["read"]` | Add `write` or specific `read:*` scopes only if your service posts on the user's behalf (out of scope for sign-in). |
| Bluesky | `["atproto", "transition:generic"]` | Narrow to read-only or specific feature surfaces if the consumer doesn't need general-purpose access. |
| NetSuite | `["rest_webservices"]` | The SuiteTalk REST scope authorizes the userinfo call `Exchange` makes after token issuance; widen only if the integration calls additional SuiteCloud surfaces. |

## Wiring up the registry

```go
google := providers.NewGoogle(providers.GoogleConfig{...})
github := providers.NewGitHub(providers.GitHubConfig{...})
facebook := providers.NewFacebook(providers.FacebookConfig{...})
mastodon := providers.NewMastodon(providers.MastodonConfig{
    AppName:     "MyApp",
    RedirectURI: "https://app.example.com/cb",
    AppCache:    providers.NewMemoryMastodonAppCache(),
})
bluesky := providers.NewBluesky(providers.BlueskyConfig{
    ClientMetadataURL: "https://app.example.com/client-metadata.json",
    RedirectURI:       "https://app.example.com/cb",
    KeyStore:          providers.NewMemoryBlueskyKeyStore(),
})

// Use provider.Name() as the registry key so renames flow through one place.
registry := application.OAuthProviderRegistry{
    google.Name():    google,
    github.Name():    github,
    facebook.Name():  facebook,
    mastodon.Name():  mastodon,
    bluesky.Name():   bluesky,
}

svc := application.NewDefaultAuthenticationService(
    registry,
    agentRepo, credentialRepo, sessionRepo, accountRepo,
    application.WithEventStore(eventStore),
    application.WithJWTService(jwtSvc),
    application.WithTokenStore(tokenStore),
)
```

## Federated providers (Mastodon, Bluesky)

Federated providers cannot satisfy `OAuthProvider.AuthCodeURL` because that interface signature has no place to thread the per-user instance/handle. The standard `AuthCodeURL` returns the empty string for both — callers must use the host-aware methods:

- Mastodon: `mastodon.AuthCodeURLForInstance(ctx, host, state, codeChallenge, nonce, redirectURI)`
- Bluesky: `bluesky.AuthCodeURLForHandle(ctx, handle, state, codeChallenge, nonce, redirectURI)`

Both bind the per-flow context (host / PDS) internally, keyed by the codeChallenge (which Exchange recomputes from the codeVerifier). Bindings are TTL'd (10 min default) and single-use.

Distinguishable PERMANENT sentinels — callers MUST `errors.Is`-route on these and not retry:

- `providers.ErrMastodonInstanceRequired` — caller forgot to bind via `AuthCodeURLForInstance`.
- `providers.ErrMastodonFlowExpired` — binding TTL'd before `Exchange` ran. Start a fresh flow.
- `providers.ErrMastodonFlowAlreadyConsumed` — `Exchange` already consumed this binding (e.g. duplicate callback). Start a fresh flow.
- `providers.ErrMastodonIDTokenUnsupported` — Mastodon does not issue ID tokens; resolve identity via `Exchange`.
- `providers.ErrBlueskyFlowMissing` / `ErrBlueskyFlowExpired` / `ErrBlueskyFlowConsumed` — same shape for Bluesky.
- `providers.ErrBlueskyHandleResolutionFailed` / `ErrBlueskyDIDResolutionFailed` / `ErrBlueskyAuthServerDiscovery` / `ErrBlueskyPARFailed` / `ErrBlueskyIssuerMismatch` — discovery/PAR-stage failures during `AuthCodeURLForHandle`.
- `providers.ErrBlueskyRevokeUnsupported` / `ErrBlueskyIDTokenUnsupported` — capability mismatches; route around them, do not treat as auth failure.

## Worked example

`examples/authn/` ships a complete demo that:

1. Wires `DefaultAuthenticationService` against in-memory repos
2. Registers all seven providers via `BuildProviderRegistry()`
3. Runs an end-to-end Mastodon flow against a local httptest fake (`RunMastodonAgainstFake`)
4. Walks through the full identity lifecycle: initiate flow, exchange code, find/create agent, create session, validate session, issue JWT, derive resource ownership, revoke session

Run it:

```
go run ./examples/authn/
```
