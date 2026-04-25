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

### 2026-04-25: NetSuite OAuth 2.0 provider

- Added `providers.NewNetSuite(NetSuiteConfig{...})` in `pkg/auth/infrastructure/providers/netsuite.go` — fifth shipping OAuth provider alongside Apple/GitHub/Google/Microsoft
- Per-account hosts (auth → `<account>.app.netsuite.com`, token/revoke/userinfo → `<account>.suitetalk.api.netsuite.com`) are derived from `AccountID` with NetSuite's documented normalization (lowercase + `_` → `-`), mirroring how Microsoft templates `tenantID`. Sandbox `1234567_SB1` resolves to `1234567-sb1.app.netsuite.com` automatically
- Each endpoint accepts an explicit override that **wins over the derived URL even when `AccountID` is set** — safety valve for sandboxes with non-standard hosts and any future NetSuite endpoint change. Override-only-when-AccountID-is-empty was explicitly called out as the trap to avoid
- `ValidateIDToken` returns sentinel `ErrNetSuiteIDTokenNotSupported` because NetSuite's OAuth 2.0 doesn't reliably issue OIDC-conformant ID tokens; user info is fetched via `Exchange` (NetSuite's `userinfo` endpoint)
- Default scope is `["rest_webservices"]` — required for the SuiteTalk REST userinfo endpoint, so `Exchange` works against a real tenant out of the box
- Token endpoint surfaces OAuth 2.0 error bodies returned with HTTP 200 and rejects empty `access_token`; userinfo rejects empty `sub` — guards against silent partial-response failure modes that otherwise produce blank-identity sessions
- Added `pkg/auth/PROVIDERS.md` provider catalog (first time the auth package has shipped a catalog doc) and `examples/authn/provider_catalog.go` with `BuildProviderRegistry()` as the copy-pasteable wiring template for downstream services
- **Why:** Closes epic #26 — downstream services can register NetSuite the same way they register Google/Microsoft, picking up sandbox support and endpoint overrides without writing their own NetSuite adapter
