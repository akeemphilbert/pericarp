---
layout: default
title: "0003. Transaction ID"
parent: Decisions
nav_order: 3
status: accepted
date: 2026-04-04
decision-makers: aphilbert
---

# 0003. A transaction ID correlates every event committed in one unit of work

## Context and problem statement

`SimpleUnitOfWork.Commit` appends the uncommitted events of several aggregates in
one call. Once stored, nothing tied those events back together: an auditor, a
debugger, or a cross-aggregate consistency check could see that two aggregates
changed at about the same time, but not that they changed *because of the same
command*. The envelope needed a correlation key, and the question was where it is
minted and what it costs existing data.

## Decision drivers

- Auditing and debugging need "everything this commit did" as one query.
- Events already persisted must stay readable; no migration of stored JSON.
- The key must be assigned once per commit, not once per aggregate, or it
  correlates nothing.

## Considered options

1. **A `TransactionID` (KSUID) on `EventEnvelope`, minted by the unit of work at
   `Commit` and stamped on every event before persisting** — `omitempty` in JSON.
2. **Correlate by timestamp window** — no schema change; group events by
   `Created` within a tolerance.
3. **A separate `transactions` table** mapping transaction → event IDs.

## Decision outcome

Chosen option: **option 1**. The unit of work is the only place that knows the
boundary of a commit, so it mints the ID. `omitempty` means events stored before
the field existed unmarshal with an empty `TransactionID`, and nothing rewrites
them. `ToAnyEnvelope` copies the field so typed and untyped envelopes agree.
`EventStore.GetEventsByTransactionID` answers the "everything this commit did"
query, ordered by aggregate ID then sequence.

### Consequences

- Good: one query returns a commit's full footprint across aggregates.
- Good: dispatched events carry the ID, so a handler can correlate its own
  side-effects.
- Bad: a direct `EventStore.Append` that bypasses the unit of work carries no
  transaction ID; the correlation is only as complete as the discipline in
  Constitution Article I.
- Neutral: the ID is a KSUID, so it sorts by mint time — a convenience, not a
  guarantee of commit order.

### Confirmation

Tests in `pkg/eventsourcing/application/`: events in one commit share an ID,
different commits get different IDs, dispatched events carry it.
`GetEventsByTransactionID` is in the shared store suite.

## Pros and cons of the options

### Option 1 — envelope field, minted at commit

- Good, because it is exact, cheap, and travels with the event everywhere.
- Bad, because pre-existing events have no ID and cannot be correlated after the
  fact.

### Option 2 — timestamp window

- Good, because it needs no change.
- Bad, because it is a guess. Two unrelated commits in the same millisecond
  correlate; a slow commit splits.

### Option 3 — side table

- Good, because the envelope stays unchanged.
- Bad, because the table must be written in the same transaction as every store's
  append, which every store must then implement, and it cannot follow events that
  are exported, archived, or dispatched.

## More information

`pkg/eventsourcing/domain/event.go` (`EventEnvelope.TransactionID`),
`pkg/eventsourcing/application/unitofwork.go`,
`pkg/eventsourcing/domain/eventstore.go` (`GetEventsByTransactionID`).

Reconstructed from the journal entry of 2026-04-04 on 2026-09-03.
