# Pericarp Journal

Append-only log of major changes to Pericarp. Each entry records what changed,
why, and any key design decisions. Claude Code reads this at the start of major
tasks to maintain context across sessions. Entries are never edited or removed.

---

### 2026-03-17: Multi-tenant auth foundations

- Added `pkg/auth` package with `Identity`, context helpers (`AgentFromCtx`, `ContextWithAgent`)
- Added JWT-based authentication service with RSA key support
- Added invite system (`InviteService`, `InviteTokenService`) for account onboarding
- Added `ResourceOwnership` helpers for tenant-scoped resource creation and access verification
- Added account switching support via session management
- **Why:** Building toward a multi-tenant IAM layer where agents belong to accounts, resources are tenant-scoped, and authentication flows support invite-based onboarding

---

### 2026-03-21: BigQuery EventStore implementation

- Added `BigQueryEventStore` in `pkg/eventsourcing/infrastructure/bigquery_store.go` — 5th EventStore implementation
- Uses BigQuery DML (INSERT via `client.Query()`) for strong read-after-write consistency
- Optimistic concurrency via BigQuery scripting blocks (DECLARE + IF/ELSE) — DML table-level locks serialize concurrent writes
- Payload/metadata stored as JSON STRING columns (schemaless, like GORM's JSONB approach)
- `GetEventByID` performs full table scan since `id` is not in the clustering key — documented trade-off
- Integration tests use `ghcr.io/goccy/bigquery-emulator` via testcontainers; skip gracefully without Docker
- Added BigQuery variants to shared table-driven tests in `eventstore_test.go` and `eventstore_range_test.go`
- **Why:** BigQuery is append-optimized and enables powerful analytics over event streams — natural fit for event sourcing at scale

---

### 2026-04-04: Transaction ID for unit of work correlation

- Added `TransactionID` field to `EventEnvelope[T]` — a KSUID string that correlates all events committed in the same unit of work
- `SimpleUnitOfWork.Commit()` generates a single transaction ID and stamps it on every event before persisting
- Updated `ToAnyEnvelope` to copy the `TransactionID` field
- Field is `omitempty` in JSON so existing events without a transaction ID remain backward-compatible
- Added tests: same-commit events share a transaction ID, different commits get different IDs, dispatched events carry the transaction ID
- **Why:** Enables correlating events across multiple aggregates that were committed together, useful for auditing, debugging, and cross-aggregate consistency tracking

---

### 2026-04-25: Password credential support in pkg/auth

- Added `PasswordCredential` aggregate (`pkg/auth/domain/entities/password_credential.go`) — its own KSUID, linked 1:1 to a `Credential` row of `provider="password"` by `CredentialID`. Bcrypt hash stored only on this row, never on `Credential` and never in any event payload
- New events `PasswordCredentialCreated` / `PasswordUpdated` carry only metadata (algorithm, timestamps) — no hash, no plaintext
- Added `PasswordCredentialRepository` (domain interface + GORM impl + table in AutoMigrate)
- Extended `AuthenticationService` with `RegisterPassword`, `VerifyPassword`, `ImportPasswordCredential`, `UpdatePassword`. Anti-enumeration: `VerifyPassword` returns `ErrInvalidPassword` for both wrong-password and unknown-email, and runs a dummy bcrypt compare to keep timing roughly constant
- `provider_user_id` for password credentials is the lowercased email; existing OAuth invariant (`provider_user_id = IdP subject`) is untouched
- Bcrypt via `golang.org/x/crypto/bcrypt`, default cost configurable via `WithBcryptCost`. Algorithm stored on the row to allow future migration (e.g. argon2id)
- Password support is opt-in via `WithPasswordCredentialRepository`; password methods return `ErrPasswordSupportNotConfigured` until wired
- **Why:** Apollo and other downstream services need email/password sign-in without bolting their own auth lane on top. OAuth lane untouched; bcrypt blobs from legacy systems can be imported as-is via `ImportPasswordCredential`

---

### 2026-04-25: OAuth provider catalog expansion (Facebook, Mastodon, Bluesky)

- Added three providers under `pkg/auth/infrastructure/providers/`:
  - **Facebook** (`facebook.go`): Graph API v18.0. Refresh and ID tokens not supported by the standard flow — `RefreshToken` returns `application.ErrTokenRefreshFailed`; `ValidateIDToken` returns the new `ErrFacebookIDTokenUnsupported`. Identity resolved via `/me?fields=id,name,email,picture`.
  - **Mastodon** (`mastodon.go`): federated. Per-flow instance host is a runtime input on the new `AuthCodeURLForInstance(ctx, host, …)` method, not on `MastodonConfig`. Apps are auto-registered per host via `POST /api/v1/apps` and cached behind the pluggable `MastodonAppCache` interface (default in-memory; consumers wire a shared store for multi-replica). The flow-to-instance binding is keyed by `codeChallenge` so `Exchange` (which gets `codeVerifier`) can recover the host without expanding the `OAuthProvider` interface. Single-flight registration prevents N concurrent first-flow logins from leaking N upstream apps. Distinguishable flow-state sentinels (`ErrMastodonInstanceRequired` / `ErrMastodonFlowExpired` / `ErrMastodonFlowAlreadyConsumed`) replace the original single sentinel — single-error overload invited infinite retry loops.
  - **Bluesky** (`bluesky.go`): AT Protocol OAuth (proposal 0004). Use `AuthCodeURLForHandle(ctx, handle, …)` to start a flow; the provider resolves handle → DID (via `com.atproto.identity.resolveHandle`) → PDS (from the DID document's AtprotoPersonalDataServer service entry) → AS metadata (`/.well-known/oauth-authorization-server`), then performs PAR with a DPoP proof and returns the authorize URL. DPoP proofs are signed with an ECDSA P-256 key from the pluggable `BlueskyKeyStore`. RFC 9449 + RFC 7638 implementation hand-rolled around `golang-jwt/jwt/v5` (the only JWT lib already in `go.mod`). Refresh tokens are wrapped (`btr.v2.<base64url(pdsURL|tokenURL|issuer|opaque)>`) to thread PDS context across the host-agnostic `OAuthProvider.RefreshToken` signature. AS metadata `issuer` is verified against the PDS URL to defeat malicious did:web documents pointing at attacker-controlled auth servers.
- Established a convention for non-OIDC providers in this package: `ValidateIDToken` returns a dedicated sentinel that callers must distinguish via `errors.Is`; standard `AuthCodeURL` for federated providers returns the empty string (any non-empty default would risk being stuffed into a `Location` header by an unaware caller).
- `examples/authn/` updated: `BuildProviderRegistry()` registers all seven providers; `RunMastodonAgainstFake()` runs an end-to-end Mastodon flow against an httptest fake to satisfy story #18's "no real credentials required" demo path. `MastodonConfig.InstanceBase` is the public seam used.
- `pkg/auth/README.md` documents the full provider catalogue with one-paragraph setup notes per provider, sensible default scopes, and the federated-provider entry-point conventions.
- **Why:** Pericarp consumers can now register Facebook, Mastodon, or Bluesky OAuth in the same registry-based way they register Google today, without writing per-provider adapters. Federation (Mastodon) and DPoP-bound tokens (Bluesky) are absorbed inside provider implementations rather than expanding the shared interface (per the epic's anti-pattern callout).
