---
layout: default
title: "0004. Token claim seams"
parent: Decisions
nav_order: 4
status: accepted
date: 2026-05-09
decision-makers: aphilbert
---

# 0004. Token claims are extended through typed seams, not by reimplementing the JWT service

## Context and problem statement

Downstream services (Apollo first) needed two things in the identity token that
`pkg/auth` did not carry: a subscription tier, and app-specific authorization
claims such as a role. Their options were to reimplement `JWTService` end to end —
losing the RSA validate/reissue/invite implementations — or to skip pericarp's
token issuance entirely. Both meant every consumer re-solving the same problem and
drifting from the core token format. The question was how `pkg/auth` should let a
consumer put its own data in the token without letting it forge the core claims.

## Decision drivers

- Subscription lookups were being hand-rolled on every protected request; moving
  them to token issuance takes them off the hot path.
- A consumer's claim must never overwrite `sub`, `agent_id`, `account_ids`,
  `active_account_id`, or `subscription`.
- Billing-provider outages must not break login; an enricher bug must not issue a
  wrong token. Those are different failure policies and both must be explicit.
- Account-switch reissuance must not re-query external providers.

## Considered options

1. **Two typed seams on `DefaultAuthenticationService`**: a `SubscriptionService`
   interface (`WithSubscriptionService`) that yields a first-class
   `auth.SubscriptionClaim`, and a `ClaimsEnricher` callback
   (`WithClaimsEnricher`) whose map is flattened into top-level claims, with
   reserved names rejected.
2. **An opaque `map[string]any` hook only** — one seam for everything, the
   consumer builds whatever it likes.
3. **Make `JWTService` pluggable and let consumers implement it** — pericarp ships
   the interface and a reference implementation.

## Decision outcome

Chosen option: **option 1**. Subscription is common enough across consumers to
deserve a typed claim with an `IsActive()` rule that centralizes "what counts as
paying" (only `active` and `trialing`); everything else goes through the enricher.
`ValidateExtras` rejects reserved names at `IssueToken` and `MarshalJSON` re-runs
the check as defense in depth; `ReissueToken` re-validates because the reserved set
can grow.

The two seams fail differently on purpose. A `SubscriptionService` error is logged
and the token is issued without the claim — **fail-open**, because a billing outage
must not lock out every user. A `ClaimsEnricher` error is returned and no token is
issued — **fail-closed**, because a partial or wrong authorization claim is worse
than no token. Both seams snapshot at issuance: `ReissueToken` copies `Subscription`
and `Extras` verbatim on account switch and does not re-invoke either.

### Consequences

- Good: consumers read `auth.AgentFromCtx(ctx).Subscription` and their own claims
  from one canonical place; no per-request lookups.
- Good: the core claims cannot be forged through the extension surface.
- Bad: `JWTService.IssueToken` changed signature twice (a `subscription`
  parameter, then an `extras` parameter) — breaking for anyone implementing the
  interface.
- Bad: a claim is only as fresh as the token; a subscription cancelled mid-TTL
  stays active until the token expires or `ExpiresAt` passes.
- Neutral: `RequireAuth` (session) leaves `Subscription` nil — sessions do not
  snapshot subscription state; only `RequireJWT` populates it.

### Confirmation

`pkg/auth/application/` tests for reserved-name rejection, enricher fail-closed,
subscription fail-open, and reissue snapshotting; `examples/authn/main_test.go`
asserts a `role` claim round-trips through `ValidateToken`.

## Pros and cons of the options

### Option 1 — typed subscription seam plus enricher

- Good, because the common case is typed and the long tail is open.
- Good, because the failure policy of each seam is stated where it is wired.
- Bad, because two seams are more surface than one.

### Option 2 — one opaque map

- Good, because it is minimal.
- Bad, because every consumer re-derives `IsActive`, and nothing stops a map key
  from being `sub`.

### Option 3 — pluggable `JWTService`

- Good, because a consumer has total control.
- Bad, because total control is the problem: it discards the validate/reissue/
  invite implementations and every consumer drifts.

## More information

`pkg/auth/subscription.go` (`SubscriptionClaim`, `IsActive`),
`pkg/auth/application/jwt_service.go` (`PericarpClaims.Extras`,
`ValidateExtras`, `ErrReservedClaim`), `pkg/auth/application/options.go`
(`WithSubscriptionService`, `WithClaimsEnricher`),
`pkg/auth/infrastructure/subscription/` (RevenueCat, Stripe, GORM adapters).
Epics #24 and #35; PR #25 review added the `ExpiresAt` check.

Reconstructed on 2026-09-03 from the journal entries of 2026-04-25 (subscription
seam and adapters) and 2026-05-09 (claims enricher).
