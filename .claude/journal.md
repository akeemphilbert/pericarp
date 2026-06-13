# Pericarp Journal

Append-only log of major changes to Pericarp. Each entry records what changed,
why, and any key design decisions. Claude Code reads this at the start of major
tasks to maintain context across sessions. Entries are never edited or removed.

---

### 2026-05-30: Removed the BigQuery EventStore

- Deleted `BigQueryEventStore` (`pkg/eventsourcing/infrastructure/bigquery_store.go`) and all its tests, including the `bigquery:` cases and `setupBigQueryStore*` helpers woven through the shared `eventstore_test.go` / `eventstore_range_test.go` table-driven suites. Reverses the 2026-03-21 addition below.
- **Why:** the integration tests (added 2026-03-21, "skip gracefully without Docker") spun up the `ghcr.io/goccy/bigquery-emulator` testcontainer unconditionally when Docker *was* present, and the emulator flaked under CI with TCP `i/o timeout`s — each subtest stalling 30–48s and reds the whole `Test` job. The store had no in-repo consumers and was not used by downstream services, so it was removed rather than gated.
- `go mod tidy` dropped the BigQuery-only dependency tree: `cloud.google.com/go/bigquery`, `google.golang.org/api`, and ~20 transitive deps (arrow, flatbuffers, gax-go, genproto, …). `testcontainers-go` stays — still used by other integration tests. Surviving EventStore implementations: Memory, File, GORM, Postgres.
- Done on the #43 auth branch at the maintainer's request rather than a separate PR.

---

### 2026-05-30: Bluesky auth-server discovery follows the RFC 9728 protected-resource hop (#43)

- Fixed `Bluesky.AuthCodeURLForHandle`, which queried the **PDS** for `/.well-known/oauth-authorization-server` and got an Express 404 (a Bluesky PDS does not serve that document — only the auth server, e.g. `bsky.social`, does). Discovery is two hops: resolve handle→DID→PDS, then `GET PDS/.well-known/oauth-protected-resource` → `authorization_servers[0]` → fetch AS metadata from *that* host.
- New `discoverAuthServer(ctx, pdsURL)` performs the protected-resource hop. **Falls back** to treating the PDS as its own AS when the document is missing/unreachable/empty (backward-compat for self-hosted PDS==AS deployments, chosen over a hard fail). A document that *names* an AS which fails the SSRF guard is a hard error, not a fallback. The RFC 9728 `resource` field is intentionally not validated.
- Re-anchored host checks on the **AS host**, not the PDS — since PDS and AS are routinely different hosts: the issuer-match now compares `asMeta.Issuer` to the discovered AS URL; `validateAuthServerEndpoints` ties authorize/token/PAR endpoints to the AS host; `validateRefreshTokenURLs` ties `tokenURL` to the **issuer** host (pdsURL is still SSRF-validated but no longer required to share the token host). `ErrBlueskyIssuerMismatch` text updated accordingly.
- The shared test fake now serves the protected-resource document so existing tests exercise the real discovery path; `TestBluesky_TwoHopDiscovery_SeparateAuthServer` is the regression test (separate PDS/AS hosts, PDS 404s on oauth-authorization-server).
- Follow-up still open (not this issue): provider hardcodes `clientMetadataURL` as `client_id`, blocking the AT Protocol localhost dev-client form.

---

### 2026-05-12: Opt-in agent-only fallback for GORM SubscriptionService (#41)

- New `WithGORMAgentFallback() GORMOption` on the GORM subscription adapter. When enabled, an account-scoped lookup that returns `gorm.ErrRecordNotFound` retries against the existing agent-only branch (`account_id = '' OR NULL`). The account-scoped row still wins on a hit (the `err == nil` early-return is unconditional on the flag), and a real DB error on the first query short-circuits before the flag is consulted — the fallback does not mask errors
- Default behavior is unchanged: the strict `(agent, account)` match documented in `defaultLookup`'s doc comment remains the load-bearing security invariant against the personal→B2B paid-tier leak. Existing `TestGORM_NonEmptyAccountWithNoMatch_ReturnsNil` still pins the strict default by exercising the new `if !g.agentFallback { return nil, nil }` short-circuit
- Implementation factored the agent-only query into a separate `agentOnlyLookup` helper so the fall-through path and the original `accountID == ""` path share one query body. The `accountID == ""` path is byte-identical to before (same predicate, same NULL handling, same ordering, same error-wrap message)
- **Why:** Some consumers' subscriptions belong to the human (agent), not to a specific tenant — the agent can be a member of multiple tenant accounts they don't own and the entitlement should follow them across all of them. Before this option, the only escape hatch was `WithGORMResolver`, which forced reimplementing the entire query path (status validation, ordering, NULL handling) just to add one fallback `WHERE`. Strict-by-default is preserved; opt-in shape is the right contract here
- The same /pr-review hunter that originally caught the cross-tenant leak (see 2026-04-25 entry) signed off on this PR — the fallback can only reach the agent-only row when both the option is on and the account-scoped query returned `ErrRecordNotFound`

---

### 2026-05-09: Custom JWT claims via ClaimsEnricher (#35)

- `PericarpClaims` (`pkg/auth/application/jwt_service.go`) gained `Extras map[string]any` with custom `MarshalJSON`/`UnmarshalJSON` that flatten extras to top-level JWT claims and re-collect them on parse. Reserved claim names — standard JWT registered (`iss sub aud exp nbf iat jti`) plus pericarp core (`agent_id account_ids active_account_id subscription`) — cannot be smuggled in: `ValidateExtras` rejects them at `IssueToken` time with a wrapped `ErrReservedClaim` listing every offender sorted, and `MarshalJSON` re-runs the same check as a defense-in-depth backstop. `UnmarshalJSON` silently excludes reserved siblings from `Extras` to keep validation tolerant of forged tokens that try to land spoofed core values in the map
- `JWTService.IssueToken` interface signature gained a trailing `extras map[string]any` parameter (breaking — every caller in this repo updated; existing tests pass `nil`). `RSAJWTService.ReissueToken` re-validates extras before re-signing because the reserved set may grow over time, an alt-implementation may not enforce `ValidateExtras`, or the claims pointer may have been mutated in memory between Validate and Reissue
- `claimsAlias` is the `type claimsAlias PericarpClaims` trick used inside `MarshalJSON`/`UnmarshalJSON` to avoid recursing into our own custom marshalers. Aliasing instead of duplicating the field list eliminates drift when new core claims are added; the alias type MUST stay private and MUST NOT regain a custom marshaler
- New `application.ClaimsEnricher` callback type and `application.WithClaimsEnricher(...)` option on `DefaultAuthenticationService`. `IssueIdentityToken` invokes the enricher with `(ctx, agent, accounts, activeAccountID)` and passes its returned map verbatim as extras. Enricher errors are **fail-closed** — wrapped error returned, no token issued — explicitly contrasting `SubscriptionService` (fail-open for third-party billing outages). The contract docs spell out the distinction in three places (type, option, method) so the rationale is reachable from any direction
- `TokenReissuer.ReissueToken` snapshots `Extras` verbatim onto the new token (mirroring the `Subscription` snapshot policy) — the enricher is not re-invoked on account switch; a fresh snapshot only happens on the next `IssueIdentityToken`
- `pkg/auth/README.md` gained a "Custom JWT claims" section with a runnable snippet plus the boundary list (reserved names, fail-closed, snapshot-on-reissue, encoding/json numeric-type defaults). `examples/authn/main.go` step `[10]` now wires a sample enricher and `examples/authn/main_test.go` asserts the `role` claim round-trips through `ValidateToken`
- **Why:** Closes epic #35. Pericarp consumers had two bad options for app-specific authorization claims: reimplement `JWTService` end-to-end (losing the RSA validate/reissue/invite implementations) or skip pericarp's token issuance entirely. The enricher closes that gap with a single typed callback while the reserved-name protection (developer-facing gate at `IssueToken`, defense-in-depth at `MarshalJSON`, and re-validation at `ReissueToken`) ensures the new surface cannot be used to forge core claims like `sub` or `agent_id`

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

### 2026-04-25: OAuth provider catalog expansion (Facebook, Mastodon, Bluesky)

- Added three providers under `pkg/auth/infrastructure/providers/`:
  - **Facebook** (`facebook.go`): Graph API v18.0. Refresh and ID tokens not supported by the standard flow — `RefreshToken` returns `application.ErrTokenRefreshFailed`; `ValidateIDToken` returns the new `ErrFacebookIDTokenUnsupported`. Identity resolved via `/me?fields=id,name,email,picture`.
  - **Mastodon** (`mastodon.go`): federated. Per-flow instance host is a runtime input on the new `AuthCodeURLForInstance(ctx, host, …)` method, not on `MastodonConfig`. Apps are auto-registered per host via `POST /api/v1/apps` and cached behind the pluggable `MastodonAppCache` interface (default in-memory; consumers wire a shared store for multi-replica). The flow-to-instance binding is keyed by `codeChallenge` so `Exchange` (which gets `codeVerifier`) can recover the host without expanding the `OAuthProvider` interface. Single-flight registration prevents N concurrent first-flow logins from leaking N upstream apps. Distinguishable flow-state sentinels (`ErrMastodonInstanceRequired` / `ErrMastodonFlowExpired` / `ErrMastodonFlowAlreadyConsumed`) replace the original single sentinel — single-error overload invited infinite retry loops.
  - **Bluesky** (`bluesky.go`): AT Protocol OAuth (proposal 0004). Use `AuthCodeURLForHandle(ctx, handle, …)` to start a flow; the provider resolves handle → DID (via `com.atproto.identity.resolveHandle`) → PDS (from the DID document's AtprotoPersonalDataServer service entry) → AS metadata (`/.well-known/oauth-authorization-server`), then performs PAR with a DPoP proof and returns the authorize URL. DPoP proofs are signed with an ECDSA P-256 key from the pluggable `BlueskyKeyStore`. RFC 9449 + RFC 7638 implementation hand-rolled around `golang-jwt/jwt/v5` (the only JWT lib already in `go.mod`). Refresh tokens are wrapped (`btr.v2.<base64url(pdsURL|tokenURL|issuer|opaque)>`) to thread PDS context across the host-agnostic `OAuthProvider.RefreshToken` signature. AS metadata `issuer` is verified against the PDS URL to defeat malicious did:web documents pointing at attacker-controlled auth servers.
- Established a convention for non-OIDC providers in this package: `ValidateIDToken` returns a dedicated sentinel that callers must distinguish via `errors.Is`; standard `AuthCodeURL` for federated providers returns the empty string (any non-empty default would risk being stuffed into a `Location` header by an unaware caller).
- `examples/authn/` updated: `BuildProviderRegistry()` registers all seven providers; `RunMastodonAgainstFake()` runs an end-to-end Mastodon flow against an httptest fake to satisfy story #18's "no real credentials required" demo path. `MastodonConfig.InstanceBase` is the public seam used.
- `pkg/auth/README.md` documents the full provider catalogue with one-paragraph setup notes per provider, sensible default scopes, and the federated-provider entry-point conventions.
- **Why:** Pericarp consumers can now register Facebook, Mastodon, or Bluesky OAuth in the same registry-based way they register Google today, without writing per-provider adapters. Federation (Mastodon) and DPoP-bound tokens (Bluesky) are absorbed inside provider implementations rather than expanding the shared interface (per the epic's anti-pattern callout).

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

---

### 2026-04-25: PasswordCredential salt-suffix support for legacy hash imports (#30)

- `PasswordCredential` aggregate gained an optional `salt string` field (private; `Salt()` accessor) and a new `WithSalt(id, credentialID, algorithm, hash, salt string)` constructor. `With(...)` now delegates to `WithSalt(..., "")` so existing pericarp callers see no behaviour change
- `Restore()` signature gained a `salt string` parameter (5th positional) for projection hydration. `Update()` clears `salt` as part of password rotation — once a user picks a new password through pericarp, the credential permanently exits the legacy salt-suffix scheme
- `ApplyEvent()` does NOT restore salt — the event payloads carry only metadata (same convention as hash). The projection is the canonical hydration source for any path that ends in `verifyPassword`; event replay alone yields an aggregate with `salt=""` and is documented as audit/dispatch-only
- New `application.ImportOption` functional option type with `application.ImportWithSalt(salt)`. `AuthenticationService.ImportPasswordCredential` interface signature gained `opts ...ImportOption` — call-site backward-compatible (existing callers pass no opts), but breaks anyone implementing the interface (their method signatures need a trailing variadic). Test mocks in `infrastructure/http/handlers_test.go` updated
- `verifyPassword(algorithm, hash, plaintext, saltSuffix string)` — the saltSuffix is concatenated to plaintext before `bcrypt.CompareHashAndPassword`. `VerifyPassword` and `UpdatePassword` pass `pc.Salt()`; `runTimingShield` passes `""` (the unknown-email branch has no real salt to mirror; bcrypt cost dominates timing and absorbs the input-length difference for typical inputs)
- Persistence: `PasswordCredentialModel` gained `Salt string \`gorm:"size:64"\`` — additive, AutoMigrate handles fresh schemas. Existing deployments with pre-existing tables need a manual `ALTER TABLE` to add the column (GORM AutoMigrate adds new columns reliably across drivers)
- Domain validates salt against the projection cap (`entities.MaxSaltLength = 64`) at construction so a salt that fits in the aggregate is guaranteed to fit in the row — closes the partial-failure window where the event store would commit but `Save` would truncate or error. Also rejects `salt != "" && algorithm != bcrypt` because `verifyPassword` only consumes salt for bcrypt; a future Argon2 import with a salt would otherwise produce a record that always fails to verify
- `String()` / `GoString()` redact salt presence (`Salt:[REDACTED]` when set, `Salt:[EMPTY]` when not). Pairing the salt with the hash in the same log line is exactly what an offline attacker would need; redaction follows the existing hash-redaction precedent
- **Why:** Apollo's bulk migration imports legacy IAM bcrypt hashes that were computed over `plaintext + 5char_salt` (see `weos-iam-service/domain/user.go:158`), so without per-credential salt support every imported user fails `VerifyPassword` and the cutover cannot satisfy the "users authenticate without re-registering" definition of done

---

### 2026-06-12: Crash-safe background subscribers over a global ordered event feed (epic #51)

- **Global ordered feed (#52):** `EventStore` gained `ReadAfter(ctx, afterPosition, limit)` and `HeadPosition(ctx)`; `EventEnvelope` gained a store-assigned `Position int64`. GORM schema adds a unique `position` column (Postgres: assigned by `events_position_seq` default; SQLite: `MAX+1` inside the write tx) and, on Postgres, `xact_id xid8 DEFAULT pg_current_xact_id()`. The Postgres read path filters `xact_id < pg_snapshot_xmin(pg_current_snapshot())` so an earlier-position transaction that commits last is never skipped — the cost is liveness (a long-running write tx anywhere in the database delays the feed), never correctness. Migration is idempotent (advisory-locked, backfills by KSUID id order, never rewinds a live sequence); dialects other than postgres/sqlite are rejected because the `MAX+1` path would silently corrupt the feed on multi-writer engines. Dynamo returns `ErrGlobalOrderingNotSupported`. Postgres coverage runs via testcontainers (`POSTGRES_TEST_DSN` bypasses; tests are deliberately non-parallel since the visibility guard is cluster-wide).
- **Subscription runtime (#53):** new opt-in `pkg/eventsourcing/subscriptions` package — `Subscriber` loop (acquire checkpoint → `ReadAfter` batch → handlers → advance checkpoint), `CheckpointStore`/`Batch` interfaces with GORM and memory implementations. Each GORM batch is one DB transaction begun on a **non-cancellable** context (database/sql auto-rollback on ctx cancel silently broke drain-on-shutdown — caught by review, pinned by a GORM-backed drain test) and exposed to handlers via `TxFromContext`, so same-DB projection writes commit atomically with the checkpoint (exactly-once). `processBatch` defers a rollback guard so handler panics can't leak the checkpoint row lock. Checkpoint advance is conditional on the position not having moved — a concurrent `ResetCheckpoint` aborts the in-flight batch instead of being clobbered. `EventDispatcher.Dispatch` satisfies the `Handler` signature directly.
- **Poison events (#54):** with `WithParkingLot`, per-event retries with doubling backoff (default 5 after the initial attempt), each attempt bracketed in a tx savepoint so failed partial writes are discarded; after exhaustion the event lands in `parked_events` (same tx as the checkpoint advance) with an error-level log, and the feed keeps flowing. `ListParked`/`ReplayParked` re-run the handler and clear the row in one transaction; replay races are guarded (row lock + RowsAffected check; in-memory in-flight marker). `Park` only joins a batch tx whose connection pool matches its own DB — a foreign tx could hide a skipped event in the wrong database.
- **Replicas + notifications (#55):** checkpoint row taken `FOR UPDATE SKIP LOCKED` on Postgres → N same-name processes are active/passive with no leader election (two-replica exactly-once test). The Postgres store fires `NOTIFY pericarp_events` inside the append tx (failure is logged, never aborts the append); `PostgresListener` (pgx, dedicated conn, reconnect loop) and `InProcessNotifier`/`NotifyingEventStore` (SQLite/single-process) fan wake signals out per-subscriber via `Subscribe()` — point-to-point channels, one per subscriber. `WithWakeSignal` selects alongside the poll timer; polling is the floor, so notifications are never load-bearing; a wake that finds nothing (NOTIFY beat the visibility guard) re-checks after 200ms; a closed wake channel degrades to pure polling instead of a hot loop.
- **Why:** consumers (first: wepala/weos, wepala/weos#365) need projections/process managers that survive crashes, park poison events instead of halting, and coordinate across replicas — without a broker, an outbox, or CDC. The event store itself is the durable ordered queue; pericarp exposes the API, consumers own CLI/projection concerns. Synchronous in-commit dispatch is untouched; the runtime is opt-in. New direct deps: `gorm.io/driver/postgres`, `github.com/jackc/pgx/v5`.
