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

### 2026-04-25: SubscriptionService adapters — RevenueCat / Stripe / GORM (Stories 2-4 of #24)

- `pkg/auth/infrastructure/subscription/` ships three adapters implementing `application.SubscriptionService`. Each returns `(nil, nil)` for the canonical "no record" answer and an error only for transport / decode / config failures
- **RevenueCat** (`revenuecat.go`): `GET /v1/subscribers/{agent_id}`, latest-expiring entitlement wins, lifetime entitlements (null `expires_date`) supersede time-bounded. Status switch consults the matched subscription row first then falls back to scanning all rows so multi-SKU offerings / upgrades / promo grants don't silently default to Active. Honors `unsubscribe_detected_at` so refunded lifetimes flip to Inactive
- **Stripe** (`stripe.go`): `customers/search?query=metadata['agent_id']:'...'&expand[]=data.subscriptions`. Selection ranks active/trialing > past_due > canceled. The non-obvious case: Stripe transitions to `canceled` immediately when the merchant cancels even with `current_period_end` in the future and the customer still entitled — adapter keeps that as Active with `cancel_at_period_end=true` in Metadata until the period actually lapses, mirroring the existing treatment of cancel_at_period_end on still-active subscriptions. Stamps `customer_id` and `customer_match_count` to make split-brain billing setups (multiple customers per agent) detectable
- **GORM** (`gorm.go`): default schema `SubscriptionRecord`. **Strict account scoping** is the load-bearing decision: when a non-empty accountID is requested, no agent-only fallback runs — the original implementation had a fallback that would silently return a paid personal-account subscription to a B2B/team token (caught by /pr-review's silent-failure-hunter). Returned claim's Status is validated via `SubscriptionStatus.Valid()` so a typo'd row or buggy resolver lands as an error, not a JWT claim
- All three adapters take a configurable HTTP/DB seam plus relevant nil-safe option wrappers; tests use `httptest.Server` (REST) or in-memory SQLite via `glebarez/sqlite` (GORM)
- **Why:** Closes the implementation half of issue #24 — the SubscriptionService interface from Story 1 needs concrete backends to be useful. Apollo's existing `infrastructure.Subscription` projection is a drop-in target for the GORM adapter; finexity and future MCP tiers can pick whichever fits their billing stack

---

### 2026-04-25: SubscriptionClaim.IsActive honors ExpiresAt (PR #25 review)

- `IsActive()` now returns false when `ExpiresAt` is non-zero and in the past, regardless of `Status`. Previously the field was carried through the JWT and into `ReissueToken` snapshots but never consulted, so a stale claim held across an account-switch could grant paid access past the provider-attested expiry. Zero `ExpiresAt` is still treated as "no expiry expressed" so providers without a fixed expiry (lifetime entitlements) keep working
- Added `SubscriptionStatus.Valid()` so adapter-side tests can assert the provider's status string normalization didn't drift (e.g., catching `"ACTIVE"` vs `"active"` before it silently downgrades a paying customer)
- Tightened doc-strings flagged by review: `SubscriptionService` resolved the contradictory "should return non-nil Inactive" / "nil is also valid" prose into a single canonical contract; `ReissueToken` no longer overstates its claim-stability invariant; `RequireAuth` documents the session-vs-JWT divergence at the wiring point so a service mounting both middlewares isn't surprised by inconsistent `IsActive()` results
- **Why:** Surfaced by silent-failure-hunter PR review on #25 — the `ExpiresAt` field existed end-to-end but no code path read it, which is exactly the kind of trap the review process is meant to catch

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
