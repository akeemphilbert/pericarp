---
layout: default
title: "0002. Dependencies point inward"
parent: Decisions
nav_order: 2
status: accepted
date: 2026-09-03
decision-makers: aphilbert
---

# 0002. Dependencies point inward, and drivers live at the edge

## Context and problem statement

Pericarp is imported by every vine-os service. Whatever it imports, they import:
its transitive tree, its CVEs, its upgrade schedule. The event-sourcing core
(`pkg/eventsourcing/domain`, `pkg/ddd`) must stay usable by a service that wants
the contract and nothing else, while the repository also ships GORM, pgx, and
DynamoDB stores, a Postgres listener, and a migration CLI. Where does each
dependency belong, and what stops it from creeping inward?

## Decision drivers

- A domain package that imports a driver cannot be tested without that driver.
- A consumer that wants only `MemoryStore` and the interfaces should not pull the
  AWS SDK.
- Go's import graph is the only layering that survives contact with a deadline;
  a convention that is not linted is a convention that erodes.
- The CLI needs every driver at once, which is the opposite requirement.

## Considered options

1. **Inner ring imports stdlib plus two core deps; drivers in `infrastructure/`
   and `subscriptions/`; every driver at once only in `cmd/pericarp`** — enforced
   by `depguard`.
2. **Split the module** — `pericarp` (domain) and `pericarp-stores` as separate Go
   modules.
3. **Convention only** — document the layering and rely on review.

## Decision outcome

Chosen option: **option 1**. `pkg/eventsourcing/domain` and `pkg/ddd` import the
standard library, each other, `segmentio/ksuid`, and `golang.org/x/sync` — nothing
else. `application/` depends on `domain/` only. Persistence drivers live in
`infrastructure/` (GORM, DynamoDB) and `subscriptions/` (pgx for LISTEN/NOTIFY).
`cmd/pericarp` is the one place that links every driver, so the library packages
stay driver-free while the operator tool has everything.

Option 2 was deferred rather than rejected: a module split is the right answer if
the store dependencies ever become a problem for consumers, but it costs a second
release train today for a benefit Go's lazy module loading mostly delivers already.
Option 3 was the state of the tree until 2026-09-03, and it held only because
nobody had yet been tempted.

### Consequences

- Good: the domain is testable with no database and no container.
- Good: a consumer that imports only `domain/` and `application/` links no driver.
- Bad: a capability the domain needs from a driver must be expressed as an
  interface in `domain/` and implemented outward, which is more ceremony than a
  direct call (see [0006](0006-optional-store-capabilities-are-declared-and-refused.md)).
- Neutral: `pkg/auth` follows the same shape (`domain/`, `application/`,
  `infrastructure/`) but is not yet under the `depguard` rule.

### Confirmation

The `depguard` rule `article-ii-domain-points-inward` in `.golangci.yml`, run by
`make lint` and the CI linter step. An outward import in the inner ring fails with
a message naming Constitution Article II. Test files are exempt, because
`package domain_test` is an external consumer of the domain by Go's own rules.

## Pros and cons of the options

### Option 1 — linted layering in one module

- Good, because the gate is mechanical and names the rule it enforces.
- Good, because one module means one version for consumers to track.
- Bad, because `go.mod` still lists every driver; consumers see them even if they
  do not link them.

### Option 2 — split modules

- Good, because the dependency boundary becomes a module boundary.
- Bad, because two modules need coordinated releases and cross-module test setup.

### Option 3 — convention

- Good, because it costs nothing.
- Bad, because it is not enforced, and the first violation becomes precedent.

## More information

`.golangci.yml`; `pkg/eventsourcing/domain/`, `pkg/ddd/`,
`pkg/eventsourcing/infrastructure/`, `pkg/eventsourcing/subscriptions/`,
`cmd/pericarp/`. Constitution Articles II and XI.

Recorded retroactively on 2026-09-03. The layering predates the journal; the
"every driver only in `cmd/pericarp`" half was decided with the migration tool on
2026-07-24 ([0009](0009-event-store-migration-via-portable-file.md)); the linter
gate landed 2026-09-03.
