---
layout: default
title: "0010. Session account scoping"
parent: Decisions
nav_order: 10
status: accepted
date: 2026-08-09
decision-makers: aphilbert
---

# 0010. A session's account is fixed at sign-in, and the compiler forces every caller to say which

## Context and problem statement

`AuthSession` rows were never scoped to an account, so `Identity.ActiveAccountID`
was empty on every request served through `RequireAuth`, and
`ResourceOwnershipFromCtx` — which refuses an identity with no active account —
made it impossible to tag a resource with the caller's tenant under session auth.
`AuthSession.With` took no account, and `ScopeToAccount`, the only writer of the
field, had no production caller. Local runs looked healthy because weos's
`SoftAuth` populated the field by a different path. The question was where the
account should come from, and how to make sure no call site keeps dropping it.

## Decision drivers

- Multi-tenant identity is the point of `pkg/auth`; a session without a tenant is
  not a usable session.
- `AcceptInvite` adds an agent to the *inviter's* account and creates no personal
  one, so any "resolve the personal account" shortcut fixes normal sign-ups and
  leaves every invited agent broken — a partial fix that passes every test written
  with a normal account.
- The account already existed at the sign-in call site and was simply dropped
  two statements later.
- The repository was at `v1.0.0-beta.3`; breaking the exported surface was
  permitted, but had to be deliberate (Constitution Article X).

## Considered options

1. **The account flows from sign-in as a positional parameter**: `CreateSession`
   and `AuthSession.With` gain `accountID`; `AuthenticationService` gains
   `ScopeSessionToAccount`; every caller breaks until it says which account.
2. **A variadic option** `WithAccount(id)` — existing callers keep compiling.
3. **Resolve the account inside `CreateSession`** via `FindPersonalByMember`.

## Decision outcome

Chosen option: **option 1**. Option 3 is the partial fix described above. Option 2
is worse than it looks: it lets every existing call site keep compiling *and keep
silently dropping the account*, which is the defect itself. The compiler is the
forcing function — a breaking signature is the only change that guarantees every
caller was looked at.

Two policies were set alongside:

- **Sign-in is permissive, requests are strict.** An unresolvable account yields
  an unscoped session rather than a failed callback; `RequireAuth` then refuses it
  with 401 and a machine-readable `code: "unscoped_session"`. Without the code a
  client cannot tell "sign in again" (expired) from "this will never work"
  (unscoped), and loops.
- **The event carries the account.** `SessionCreated` gained the field and
  `ApplyEvent` restores it, because a field-only assignment would have produced a
  correct row and a wrong event stream.

No backfill migration was written: every pre-existing session row is unscoped, so
deploying logs everyone out once, and a mixed rolling deploy produces a login loop
until the fleet converges.

### Consequences

- Good: `Identity.ActiveAccountID` is populated on session auth, so resource
  ownership works for invited and normal agents alike.
- Good: the first Gherkin suite in the repository (`pkg/auth/features/`, 23
  scenarios) binds the *real* `DefaultAuthenticationService` over SQLite-backed
  repositories — the stubbed test that let this defect through could not have
  caught it, and an acceptance suite built the same way would have reproduced the
  blind spot.
- Bad: a breaking change for every consumer that creates sessions.
- Bad: a one-time forced sign-out on deploy.
- Neutral, and worth settling: `AuthSession` events are never appended to an
  event store — nothing commits them through a unit of work, nothing replays them.
  The account now depends on `ApplyEvent` correctness that production does not
  exercise.

### Confirmation

`pkg/auth/acceptance_test.go` running `pkg/auth/features/*.feature` under
`make test`; `RequireAuth` middleware tests for `unscoped_session`.

## Pros and cons of the options

### Option 1 — positional parameter

- Good, because every caller is forced to decide.
- Bad, because it breaks the exported surface.

### Option 2 — variadic option

- Good, because nothing breaks.
- Bad, because nothing breaks: the defect survives at every unchanged call site.

### Option 3 — resolve internally

- Good, because callers change nothing.
- Bad, because it is wrong for invited agents, and invisibly so.

## More information

`pkg/auth/application/authentication_service.go` (`CreateSession`,
`ScopeSessionToAccount`), `pkg/auth/domain/entities/` (`AuthSession.With`,
`SessionCreated`), `pkg/auth/infrastructure/http/middleware.go`
(`unscoped_session`), `switch_account.go`. Issue #68.

Reconstructed from the journal on 2026-09-03.
