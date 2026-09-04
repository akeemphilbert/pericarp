---
layout: default
title: "0006. Optional store capabilities"
parent: Decisions
nav_order: 6
status: accepted
date: 2026-06-12
decision-makers: aphilbert
---

# 0006. An optional store capability is a declared interface with a sentinel refusal

## Context and problem statement

Not every backend can do everything. DynamoDB has no global ordering across
partitions; `FileStore` has no transaction in which to delete events and record a
manifest. As the subscriber runtime and then compaction were added, each needed
something from the store that only some stores could give. The question was how a
store says "I cannot do this" in a way that a caller — and a subscriber running
unattended in production — cannot mistake for success.

## Decision drivers

- A subscriber that receives an empty feed from a store with no ordering reads it
  as "caught up" and stops. That is silent data loss, not an error.
- A compaction run that skips an unsupported store and reports success leaves an
  operator believing history was collapsed.
- The domain package cannot know which concrete stores exist
  ([0002](0002-dependencies-point-inward-and-drivers-live-at-the-edge.md)).
- Consumers need a compile-time or startup-time way to know what their store
  supports.

## Considered options

1. **Interface per capability, sentinel error per refusal.** Global ordering is
   `ReadAfter`/`HeadPosition` on `EventStore` itself, returning
   `ErrGlobalOrderingNotSupported` where absent; compaction is a separate
   `domain.CompactableEventStore` that the algorithm type-asserts, returning
   `ErrCompactionNotSupported` when the assertion fails.
2. **Best-effort fallback** — a store without ordering emulates it (scan and sort
   by `Created`); a store without transactions compacts non-atomically.
3. **A `Capabilities()` method** returning a bitset the caller inspects.

## Decision outcome

Chosen option: **option 1**. A refusal is a named error the caller must handle;
there is no code path in which "unsupported" looks like "done". The two shapes
differ deliberately: global ordering went onto `EventStore` because every store
must at least *answer* (the subscriber runtime is built on it), while compaction is
a separate interface because a store that cannot compact should not have to carry
`RetireEvents` and `Compactions` methods that only return an error.

Option 2 was rejected outright: an emulated order over `Created` timestamps is
wrong under concurrent writers, and a non-atomic compaction can lose events on a
crash between delete and manifest. A wrong answer delivered confidently is the
failure mode this record exists to prevent.

The rule for the next capability: **interface, sentinel, refusal.** Declare it in
`domain/`, name the error, and make the unsupported path fail before it reads a
single event.

### Consequences

- Good: `FileStore` and `DynamoEventStore` refuse compaction before touching the
  store; DynamoDB refuses `ReadAfter` before a subscriber starts.
- Good: a consumer can check support at startup with one call or one type
  assertion.
- Bad: the `EventStore` interface grew two methods that some stores implement only
  as `return ErrGlobalOrderingNotSupported`.
- Neutral: capabilities are per store, not per deployment; a GORM store on a
  dialect other than Postgres or SQLite is refused by the migration rather than
  by the interface.

### Confirmation

`pkg/eventsourcing/infrastructure/compaction_store_test.go`, the DynamoDB
support tests in `pkg/eventsourcing/compaction/`, and the `ReadAfter` cases in
`eventstore_readafter_test.go` assert the refusal. Constitution Article IV states
the rule.

## Pros and cons of the options

### Option 1 — interface plus sentinel

- Good, because refusal is loud, typed, and matchable with `errors.Is`.
- Bad, because the caller must handle one more error.

### Option 2 — best-effort fallback

- Good, because everything "works" on every store.
- Bad, because "works" is a lie under concurrency or on crash.

### Option 3 — capabilities bitset

- Good, because it is one call.
- Bad, because a bitset drifts from the methods it describes, and a store can
  claim a bit it does not honour; a sentinel on the actual call cannot.

## More information

`pkg/eventsourcing/domain/eventstore.go` (`ReadAfter`, `HeadPosition`,
`ErrGlobalOrderingNotSupported`), `pkg/eventsourcing/domain/compaction.go`
(`CompactableEventStore`, `ErrCompactionNotSupported`). Established with epic
#51 (2026-06-12) and applied again by #81 (2026-09-01,
[0011](0011-compaction-archive-then-append-then-delete.md)).

Reconstructed from the journal on 2026-09-03.
