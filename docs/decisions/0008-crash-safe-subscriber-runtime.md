---
layout: default
title: "0008. Subscriber runtime"
parent: Decisions
nav_order: 8
status: accepted
date: 2026-06-12
decision-makers: aphilbert
---

# 0008. Background subscribers checkpoint in the handler's transaction, park poison events, and coordinate without a leader

## Context and problem statement

With a global feed in place ([0007](0007-global-ordered-event-feed.md)),
consumers needed a runtime that reads it in the background, survives crashes
without replaying or skipping, does not halt on a bad event, and can run as
several replicas. The existing `EventDispatcher` runs handlers synchronously inside
the commit path and was to stay untouched. The question was what guarantees the
runtime makes and where the state that backs them lives.

## Decision drivers

- Exactly-once projection into the consumer's own database, with no broker.
- A poison event must not stop the feed, and must not be lost.
- N replicas of one subscriber must not double-process, and must not need a
  leader election.
- Wake-up should be prompt, but never load-bearing: a missed notification is a
  delay, not a bug.
- The runtime is opt-in; nothing changes for consumers that do not import it.

## Considered options

1. **An opt-in `subscriptions` package**: checkpoint per subscriber name,
   advanced in the same database transaction the handler writes in; per-event
   retry with backoff then a parking lot; `FOR UPDATE SKIP LOCKED` on the
   checkpoint row for replicas; LISTEN/NOTIFY or an in-process notifier for wake
   signals with polling as the floor.
2. **At-least-once with idempotent handlers** — checkpoint after the batch,
   outside the handler's transaction; consumers dedupe.
3. **Leader-elected single consumer** — one replica reads; the rest stand by.

## Decision outcome

Chosen option: **option 1**. The mechanisms and the reasons each took its shape:

- **Checkpoint in the handler's transaction.** Each GORM batch is one transaction,
  exposed to handlers through `TxFromContext`, so a projection write and the
  checkpoint advance commit or roll back together. The transaction is begun on a
  **non-cancellable** context: `database/sql` auto-rollback on cancellation
  silently broke drain-on-shutdown, which review caught and a drain test pins.
- **Conditional advance.** The checkpoint moves only if its position has not
  changed under the batch, so a concurrent `ResetCheckpoint` aborts the in-flight
  batch instead of being clobbered. A deferred rollback guard keeps a handler
  panic from leaking the row lock.
- **Poison events.** With `WithParkingLot`, an event is retried with doubling
  backoff (default five attempts after the first), each attempt in a savepoint so
  a failed partial write is discarded; after exhaustion it lands in
  `parked_events` in the same transaction as the checkpoint advance, with an
  error-level log, and the feed continues. `ReplayParked` re-runs the handler and
  clears the row in one transaction. `Park` joins a batch transaction only when
  the connection pool matches its own — a foreign transaction could hide a
  skipped event in the wrong database.
- **Replicas.** The checkpoint row is taken `FOR UPDATE SKIP LOCKED` on Postgres,
  so N same-name processes are active/passive with no election.
- **Wake signals.** The Postgres store fires `NOTIFY pericarp_events` inside the
  append transaction (failure logged, never aborts the append). `PostgresListener`
  (pgx, dedicated connection, reconnect loop) and `InProcessNotifier` fan point-
  to-point signals out per subscriber. `WithWakeSignal` selects alongside the
  poll timer; a wake that finds nothing re-checks after 200 ms; a closed channel
  degrades to pure polling rather than a hot loop.

Option 2 pushes the hardest part — idempotent writes — onto every consumer, and a
crash between handler commit and checkpoint write replays the batch. Option 3 adds
a coordination service to avoid a database feature that already does the job.

### Consequences

- Good: exactly-once for same-database projections, with the consumer's own
  transaction as the only mechanism.
- Good: the synchronous dispatch path is untouched; the runtime is a separate
  import.
- Bad: exactly-once holds only for writes through `TxFromContext`; a handler that
  writes elsewhere (another database, an HTTP call) is at-least-once and must be
  idempotent.
- Bad: Postgres-only for `SKIP LOCKED` and `NOTIFY`; SQLite gets the in-process
  notifier and a single replica.
- Neutral: consumers own CLI and projection concerns; pericarp exposes the
  runtime and nothing above it.

### Confirmation

Tests in `pkg/eventsourcing/subscriptions/`: the GORM-backed drain test, the
two-replica exactly-once test, parking and replay tests, and the Postgres
notifier tests via testcontainers (`postgres_subscriptions_test.go`, which fails
rather than skips under `PERICARP_REQUIRE_DOCKER_TESTS`).

## Pros and cons of the options

### Option 1 — transactional checkpoint, parking lot, SKIP LOCKED

- Good, because every guarantee rests on one database transaction.
- Bad, because the runtime is Postgres-shaped; other backends get less.

### Option 2 — at-least-once

- Good, because the runtime is simpler.
- Bad, because every consumer writes dedup logic, and gets it wrong once.

### Option 3 — leader election

- Good, because only one reader exists at a time.
- Bad, because it needs a lock service or a lease table, and a stale leader
  still double-reads.

## More information

`pkg/eventsourcing/subscriptions/subscriber.go`, `checkpoint.go`,
`gorm_checkpoint.go` (`TxFromContext`), `parking.go`, `notify.go`,
`postgres_listener.go`. Epic #51, stories #53–#55; motivating consumer
wepala/weos#365.

Reconstructed from the journal on 2026-09-03.
