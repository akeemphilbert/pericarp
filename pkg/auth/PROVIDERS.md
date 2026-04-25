# Pericarp OAuth Provider Catalog

Each constructor returns a value that implements
`application.OAuthProvider` and can be registered in an
`application.OAuthProviderRegistry`. See `examples/authn/provider_catalog.go`
for a copy-pasteable wiring example.

---

## Apple

```go
providers.NewApple(providers.AppleConfig{
    ClientID:   "com.example.app.web",       // Services ID
    TeamID:     "ABCDE12345",                 // Apple Developer Team ID
    KeyID:      "ABC123DEFG",                 // Key ID for the .p8 private key
    PrivateKey: os.Getenv("APPLE_PRIVATE_KEY"), // PEM-encoded .p8 file contents
})
```

Default scopes: `["name", "email"]`. Apple does not support PKCE — the
`codeVerifier` argument to `Exchange` is ignored. Apple also does not return a
new ID token on refresh, so `RefreshToken` returns minimal user info.

## GitHub

```go
providers.NewGitHub(providers.GitHubConfig{
    ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
    ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
})
```

Default scopes: `["read:user", "user:email"]`. GitHub does not issue OIDC ID
tokens, so `ValidateIDToken` returns an error; user info is fetched via
`Exchange`. Refresh tokens are not supported.

## Google

```go
providers.NewGoogle(providers.GoogleConfig{
    ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
    ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
})
```

Default scopes: `["openid", "email", "profile"]`.

## Microsoft

```go
providers.NewMicrosoft(providers.MicrosoftConfig{
    ClientID:     os.Getenv("MS_CLIENT_ID"),
    ClientSecret: os.Getenv("MS_CLIENT_SECRET"),
    TenantID:     "common", // or a specific tenant GUID / domain
})
```

Default scopes: `["openid", "email", "profile", "offline_access"]`. The auth
and token URLs are templated per `TenantID`. Microsoft v2.0 does not support
token revocation, so `RevokeToken` returns an error.

## NetSuite

```go
providers.NewNetSuite(providers.NetSuiteConfig{
    ClientID:     os.Getenv("NETSUITE_CLIENT_ID"),
    ClientSecret: os.Getenv("NETSUITE_CLIENT_SECRET"),
    AccountID:    "1234567", // your NetSuite account ID
})
```

Default scopes: `["rest_webservices"]` — required to call NetSuite's
userinfo endpoint after token issuance.

### Per-account URL templating

Auth, token, revoke, and userinfo URLs are derived from `AccountID` using
NetSuite's documented host pattern:

| Endpoint  | Default URL |
| --------- | ----------- |
| Auth      | `https://<account>.app.netsuite.com/app/login/oauth2/authorize.nl` |
| Token     | `https://<account>.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/token` |
| Revoke    | `https://<account>.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/revoke` |
| User info | `https://<account>.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/userinfo` |

NetSuite requires sandbox account IDs to use a hyphen and lowercase in URL
hosts (per their docs). The provider performs that normalization for you, so
a sandbox account `"1234567_SB1"` resolves to
`https://1234567-sb1.app.netsuite.com/...` automatically:

```go
providers.NewNetSuite(providers.NetSuiteConfig{
    ClientID:     os.Getenv("NETSUITE_CLIENT_ID"),
    ClientSecret: os.Getenv("NETSUITE_CLIENT_SECRET"),
    AccountID:    "1234567_SB1", // -> "1234567-sb1" in URLs
})
```

### Endpoint overrides

Each endpoint can be overridden on the config. **An override always wins,
even when `AccountID` is set** — that is the safety valve for sandboxes whose
hosts deviate from the standard pattern, and for any future NetSuite endpoint
change that ships before a Pericarp release:

```go
providers.NewNetSuite(providers.NetSuiteConfig{
    ClientID:      os.Getenv("NETSUITE_CLIENT_ID"),
    ClientSecret:  os.Getenv("NETSUITE_CLIENT_SECRET"),
    AccountID:     "1234567_SB1",
    TokenEndpoint: "https://corporate-proxy.example.com/netsuite/token",
})
```

`ValidateIDToken` returns `ErrNetSuiteIDTokenNotSupported` because NetSuite's
OAuth 2.0 implementation does not reliably issue OIDC-conformant ID tokens;
user info is fetched via `Exchange`.
