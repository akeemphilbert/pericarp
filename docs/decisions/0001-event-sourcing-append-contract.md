---
layout: default
title: "0001. The append contract"
parent: Decisions
nav_order: 1
status: accepted
date: 2026-09-03
decision-makers: aphilbert
---

# 0001. The event store is the source of truth, and the append contract is fixed

## Context and problem statement

Pericarp exists so that vine-os services can be event-sourced without each one
inventing its own store. That only works if every service can trust the same three
things from every `EventStore` implementation: how events are numbered, how a lost
update is detected, and what replay is allowed to assume. Those rules had been
implied by `MemoryStore` and repeated in doc comments, but never stated as the
contract that every store — present and future — must meet identically.

## Decision drivers

- Aggregates rebuild by replaying events in order; a numbering difference between
  stores changes the state a service reconstructs.
- Two writers on one aggregate must be detectable by the caller, with one error
  they can match and retry on.
- A store implementation added later (GORM, DynamoDB, and any next one) must be
  interchangeable with `MemoryStore` in tests.
- Consumers hold the events for years; the rules cannot drift once data exists.

## Considered options

1. **Fix the contract in the `EventStore` interface and prove it with one shared
   suite** — every store passes the same table-driven tests.
2. **Let each store document its own semantics** — consumers read the store they
   chose.
3. **Push ordering and concurrency into the aggregate layer** — the store is a dumb
   append log and `BaseEntity` enforces the rest.

## Decision outcome

Chosen option: **option 1**. The contract is:

- Events are immutable once created. A projection is a view of the store, never a
  second source.
- A new aggregate is at sequence 0; its first event is sequence 1. Sequence numbers
  are strictly ordered with no gaps.
- `Append(ctx, aggregateID, expectedVersion, events...)` takes the aggregate's
  sequence number *before* the new events as `expectedVersion`; `-1` skips the
  check. A mismatch returns `domain.ErrConcurrencyConflict`.
- `GetEventByID` on a miss returns `domain.ErrEventNotFound`; range reads return
  an empty slice, not an error.
- `EventEnvelope[T]` is the unit of storage: `ID` (KSUID), `AggregateID`,
  `EventType`, `Payload`, `Created`, `SequenceNo`, `Metadata`. Stores operate on
  `EventEnvelope[any]`; typed envelopes convert through `ToAnyEnvelope`.

Option 3 was rejected because a concurrency check that lives outside the store's
own write transaction is not a check — two processes can both pass it.

### Consequences

- Good: a store is interchangeable. The compaction and migration packages depend
  on the interface alone and are unit-tested against two `MemoryStore`s.
- Good: consumers branch on `errors.Is(err, domain.ErrConcurrencyConflict)` and
  nothing else.
- Bad: every new store must implement optimistic concurrency inside its own
  transaction, which is the hardest part of a store (see DynamoDB's conditional
  writes and the GORM store's per-dialect position assignment).
- Neutral: the contract says nothing about global order across aggregates. That
  is a separate capability ([0007](0007-global-ordered-event-feed.md)).

### Confirmation

The table-driven suites in `pkg/eventsourcing/infrastructure/` —
`eventstore_test.go`, `eventstore_range_test.go`, `eventstore_readafter_test.go`
— run the same assertions against every store under `make test`. Constitution
Articles I, III, and V state the rule.

## Pros and cons of the options

### Option 1 — one contract, one shared suite

- Good, because a store either passes the suite or is not a store.
- Good, because the contract is written once, in `domain/eventstore.go`.
- Bad, because the suite must grow with the contract, and a store added without
  joining it silently escapes the check (Article VII exists for this).

### Option 2 — per-store semantics

- Good, because each store can play to its backend's strengths.
- Bad, because a service that switches backends reconstructs different state.

### Option 3 — aggregate-layer enforcement

- Good, because stores become trivial.
- Bad, because the check races. It is not a concurrency control.

## More information

`pkg/eventsourcing/domain/eventstore.go`, `event.go`; `pkg/ddd/entity.go`
(`RecordEvent`, `ApplyEvent`); `pkg/eventsourcing/application/unitofwork.go`.

Recorded retroactively on 2026-09-03. The contract predates the journal; this
record states what the code and CLAUDE.md had established by then, so that later
records can cite it.
