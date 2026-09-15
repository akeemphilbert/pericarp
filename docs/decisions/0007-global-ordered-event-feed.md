---
layout: default
title: "0007. Global ordered feed"
parent: Decisions
nav_order: 7
status: accepted
date: 2026-06-12
decision-makers: aphilbert
---

# 0007. The event store exposes a global ordered feed with store-assigned positions

## Context and problem statement

Projections, process managers, and the migration tool all need to read *every*
event across *all* aggregates in one stable order, and to resume from where they
stopped. The store had per-aggregate order only. The question was whether to add a
global order to the store itself, or to introduce a broker, an outbox table, or
change-data-capture beside it — and, if in the store, how to make the order safe
under concurrent writers on Postgres.

## Decision drivers

- Consumers (wepala/weos first) wanted crash-safe subscribers without running a
  broker or CDC pipeline.
- A resumable feed must never deliver an event with a lower position after one
  with a higher position; a subscriber that checkpoints past a gap loses the
  event forever.
- Postgres assigns sequence values at insert, but a transaction that got a lower
  value can commit *after* one that got a higher value. A naive reader skips it.
- SQLite has one writer; DynamoDB has no global order at all.

## Considered options

1. **A store-assigned `Position` on the envelope, plus `ReadAfter`/`HeadPosition`
   on `EventStore`; on Postgres, a commit-visibility guard on `xid8`.**
2. **An outbox table** written in the same transaction as the append, drained by
   a relay.
3. **A message broker** (Watermill/NATS/Kafka) fed by the unit of work after
   commit.
4. **Change-data-capture** on the events table.

## Decision outcome

Chosen option: **option 1**. The event store is already the durable, ordered
record; a second copy of it (outbox, broker topic, CDC stream) is a second thing
to keep consistent and a second thing to operate. Global order lives where the
events live.

The mechanism per backend:

- **Postgres** — `position` from `events_position_seq`, and `xact_id xid8 DEFAULT
  pg_current_xact_id()`. `ReadAfter` filters
  `xact_id < pg_snapshot_xmin(pg_current_snapshot())`, so an event whose
  transaction is still in flight is withheld even though its position is visible.
  An earlier-position transaction that commits last is therefore never skipped.
  The price is liveness, never correctness: a long-running write transaction
  anywhere in the database delays the feed.
- **SQLite** — `MAX(position)+1` inside the write transaction; the single writer
  makes it safe.
- **Other GORM dialects** — refused by the migration, because `MAX+1` on a
  multi-writer engine silently corrupts the feed.
- **DynamoDB** — `ErrGlobalOrderingNotSupported`
  ([0006](0006-optional-store-capabilities-are-declared-and-refused.md)).

The schema migration is idempotent (advisory-locked), backfills positions by KSUID
order, and never rewinds a live sequence.

### Consequences

- Good: subscribers, migration, and compaction all consume one primitive.
- Good: no broker, outbox, or CDC to deploy; exactly-once is achievable in the
  consumer's own database ([0008](0008-crash-safe-subscriber-runtime.md)).
- Bad: an empty `ReadAfter` result is ambiguous — "caught up" or "withheld behind
  an in-flight writer" — so subscribers poll and wake signals are advisory.
- Bad: Postgres 13+ is required for `xid8`.
- Bad: two new direct dependencies, `gorm.io/driver/postgres` and
  `github.com/jackc/pgx/v5`.
- Neutral: positions are store-local. Export/import reassigns them
  ([0009](0009-event-store-migration-via-portable-file.md)); compaction retires
  them permanently and never reuses one
  ([0011](0011-compaction-archive-then-append-then-delete.md)).

### Confirmation

`eventstore_readafter_test.go` in the shared suite; Postgres coverage via
testcontainers (`POSTGRES_TEST_DSN` bypasses the container), deliberately
non-parallel because the visibility guard is cluster-wide.

## Pros and cons of the options

### Option 1 — order in the store

- Good, because there is one source of truth and one thing to back up.
- Bad, because every store must either implement a safe order or refuse.

### Option 2 — outbox

- Good, because it is a known pattern.
- Bad, because it duplicates every event and needs a relay process.

### Option 3 — broker

- Good, because fan-out and retention are the broker's problem.
- Bad, because a post-commit publish can be lost, and ordering across
  partitions is the broker's rules, not ours.

### Option 4 — CDC

- Good, because it is transparent to the application.
- Bad, because it ties the library to one database's replication stack.

## More information

`pkg/eventsourcing/domain/eventstore.go`, `pkg/eventsourcing/infrastructure/
gorm_store.go`, `gorm_model.go`, `gorm_migration.go`, `gorm_repository.go`
(the `pg_snapshot_xmin` guard). Epic #51, story #52.

Reconstructed from the journal on 2026-09-03.
