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

### 2026-04-25: SubscriptionService extensibility — interface + JWT wiring (Story 1 of #24)

- Added `auth.SubscriptionClaim` value type (`pkg/auth/subscription.go`) with `SubscriptionStatus` constants (`active`, `trialing`, `past_due`, `cancelled`, `inactive`) and an `IsActive()` helper that centralizes the "what counts as paying" rule (only `active` and `trialing` grant access)
- Type lives in `pkg/auth` (not `pkg/auth/application`) so both `Identity` (in `pkg/auth`) and `PericarpClaims` (in `pkg/auth/application`) can reference it without an inverse import — `application` already depends on the parent package, but `auth` does not depend on `application`
- Added `application.SubscriptionService` interface with `GetSubscription(ctx, agentID, accountID) (*auth.SubscriptionClaim, error)`
- `JWTService.IssueToken` gained a 5th `subscription *auth.SubscriptionClaim` parameter (breaking change to internal API; ~13 test callsites updated mechanically). `ReissueToken` preserves the existing claim verbatim — account-switch reissuance does not re-query the SubscriptionService, so the snapshot is stable for the lifetime of the original sign-in
- `AuthenticationService.IssueIdentityToken` orchestrates the lookup when a `SubscriptionService` is wired via `WithSubscriptionService(svc)`. Lookup failures are logged but do **not** block token issuance — billing-provider outages must not break login
- `RequireJWT` middleware now copies `claims.Subscription` onto `auth.Identity.Subscription`, so consumer services read it via `auth.AgentFromCtx(ctx).Subscription`. Session-based `RequireAuth` leaves it nil (sessions don't snapshot subscription state)
- **Why:** Apollo and other downstream services were hand-rolling subscription lookups on every protected request. Promoting subscription to a first-class JWT claim moves the lookup off the hot path (looked up once at issuance, lives for token TTL) and gives every consumer a single canonical type to read
- **Open follow-ups (Stories 2-4):** RevenueCat, Stripe, and GORM adapter implementations under `pkg/auth/infrastructure/subscription/`

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
