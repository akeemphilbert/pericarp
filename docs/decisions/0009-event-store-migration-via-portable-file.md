---
layout: default
title: "0009. Migration via portable file"
parent: Decisions
nav_order: 9
status: accepted
date: 2026-07-24
decision-makers: aphilbert
---

# 0009. Event-store migration is export → portable file → import

## Context and problem statement

Operators need to move a pericarp application's data to another instance, often on
a different backend — SQLite to Postgres, Postgres to DynamoDB. A database-level
dump cannot cross backends, and even between two Postgres instances it copies
global positions that the destination's own sequence would then collide with. The
question was what "migrate" means for an event-sourced store, and where the
driver-heavy tooling should live.

## Decision drivers

- Source and destination may not be reachable at the same time, and may not share
  a backend.
- The event store is the source of truth
  ([0001](0001-event-sourcing-append-contract.md)); projections are derived and
  belong to the application.
- Global positions are store-local
  ([0007](0007-global-ordered-event-feed.md)).
- The library packages must stay driver-free
  ([0002](0002-dependencies-point-inward-and-drivers-live-at-the-edge.md)).
- A migration that dies half-way must be resumable and re-runnable.

## Considered options

1. **Export the global feed to a portable newline-delimited JSON file; import
   appends in file order with `expectedVersion -1`; a `cmd/pericarp` binary
   holds every driver.**
2. **Store-to-store copy** in one process connected to both.
3. **Backend-native dump/restore**, documented per backend.

## Decision outcome

Chosen option: **option 1**. `pkg/eventsourcing/migration` depends on
`domain.EventStore` alone: `Export` streams `ReadAfter` in position order to one
envelope per line behind a version header; `Import` appends in file order. Global
order is preserved by *append order* — the destination reassigns `Position` — not
by copying positions. Payloads are copied as opaque JSON; there is no schema
upcasting. `ExportOptions.FromPosition` resumes an interrupted export;
`ImportOptions.SkipExisting` (via `GetEventByID`) makes re-runs idempotent.

The scope is deliberately **events only**. Destination projections are rebuilt by
the destination application's own subscribers, because projection handlers are
application-specific and not in core. Non-event-sourced tables are not carried.

`cmd/pericarp` — until then an empty placeholder — is where sqlite, postgres, and
dynamo drivers are all linked at once, behind `export`, `import`, and `serve`
subcommands. `serve` exposes the same operations as async HTTP jobs; because request
bodies carry database credentials it binds loopback by default and gates every
route except `/healthz` on `PERICARP_MIGRATE_TOKEN` with a constant-time check.

### Consequences

- Good: any backend to any backend, with the file as the contract between them.
- Good: the same file format is reused by compaction's archive
  ([0011](0011-compaction-archive-then-append-then-delete.md)), so an archive is
  importable.
- Good: no new module dependencies; the CLI reuses the existing driver set.
- Bad: an application must be able to rebuild its projections from the feed;
  one that cannot has no migration path.
- Bad: no upcasting — an event schema that changed between source and destination
  versions is the operator's problem.
- Neutral: `serve` is an operator tool, not a service surface; it is not
  hardened beyond token auth and loopback binding.

### Confirmation

Unit tests in `pkg/eventsourcing/migration/` against two `MemoryStore`s;
`cmd/pericarp/serve_test.go` and `store_test.go`, which run under `make test`
since 2026-09-03.

## Pros and cons of the options

### Option 1 — portable file

- Good, because source and destination are decoupled in time and backend.
- Good, because the file is inspectable and archivable.
- Bad, because it is two steps and a file to manage.

### Option 2 — direct copy

- Good, because it is one command.
- Bad, because both stores must be reachable at once, and the copying process
  must link every driver anyway.

### Option 3 — native dump

- Good, because DBAs already know it.
- Bad, because it cannot cross backends and carries positions and projections
  that should not travel.

## More information

`pkg/eventsourcing/migration/export.go`, `import.go`; `cmd/pericarp/`
(`main.go`, `serve.go`, `store.go`, `README.md`). Epic #60.

Reconstructed from the journal on 2026-09-03.
