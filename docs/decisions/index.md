---
layout: default
title: Decisions
nav_order: 6
has_children: true
---

# Architecture Decision Records

This directory is the record of the architectural decisions that shape pericarp:
what was decided, which options were on the table, why one won, and what it costs.
Each record is a [MADR](https://adr.github.io/madr/) document. Records are numbered
in the order they are written, and a record is never edited once it is accepted —
a change of mind is a new record that supersedes the old one.

[Article XII](../../CONSTITUTION.md#article-xii--an-architectural-decision-is-recorded-as-an-adr)
of the constitution says when a record is required. In short: a new package, a
change to the event or store contract, a new store capability, a swapped
foundational dependency, a breaking change to the exported surface, or a reversed
decision.

## Index

| # | Title | Status | Date |
|---|---|---|---|
| [0001](0001-event-sourcing-append-contract.md) | The event store is the source of truth, and the append contract is fixed | accepted | 2026-09-03 (retroactive) |
| [0002](0002-dependencies-point-inward-and-drivers-live-at-the-edge.md) | Dependencies point inward, and drivers live at the edge | accepted | 2026-09-03 (retroactive) |
| [0003](0003-transaction-id-correlates-a-unit-of-work.md) | A transaction ID correlates every event committed in one unit of work | accepted | 2026-04-04 |
| [0004](0004-token-claims-are-extended-through-typed-seams.md) | Token claims are extended through typed seams, not by reimplementing the JWT service | accepted | 2026-05-09 |
| [0005](0005-retire-the-bigquery-event-store.md) | Retire the BigQuery event store rather than gate its tests | accepted | 2026-05-30 |
| [0006](0006-optional-store-capabilities-are-declared-and-refused.md) | An optional store capability is a declared interface with a sentinel refusal | accepted | 2026-06-12 |
| [0007](0007-global-ordered-event-feed.md) | The event store exposes a global ordered feed with store-assigned positions | accepted | 2026-06-12 |
| [0008](0008-crash-safe-subscriber-runtime.md) | Background subscribers checkpoint in the handler's transaction, park poison events, and coordinate without a leader | accepted | 2026-06-12 |
| [0009](0009-event-store-migration-via-portable-file.md) | Event-store migration is export → portable file → import | accepted | 2026-07-24 |
| [0010](0010-session-account-scoping-flows-from-sign-in.md) | A session's account is fixed at sign-in, and the compiler forces every caller to say which | accepted | 2026-08-09 |
| [0011](0011-compaction-archive-then-append-then-delete.md) | Compaction archives, fsyncs, appends the snapshot, and only then deletes | accepted | 2026-09-01 |

## Writing a record

1. Copy [`0000-adr-template.md`](0000-adr-template.md) to the next number:
   `NNNN-short-kebab-title.md`.
2. Fill every section. "Considered options" lists the options that were actually
   weighed, including the one chosen; a record with one option is a note, not a
   decision.
3. Set `status` to `proposed` while the pull request is open, `accepted` when it
   merges. Later, `deprecated` or `superseded by [NNNN](NNNN-....md)` — change the
   status line only; the body stays as written.
4. Add the row to the index above, in the same pull request.
5. Name the record in the pull request body and, when it answers a review
   question, in the code comment at the site the decision governs.

Dates are the date the decision was made, not the date the record was written. A
record written after the fact says so under *More information*, and names the
source it was reconstructed from.

## History

Before 2026-09-03 the decision record was an append-only journal at
`.claude/journal.md`. It is frozen, not deleted: records 0001–0011 were
reconstructed from it, and the entries it holds that did not become records —
feature logs, review fixes, provider additions — remain readable there.
