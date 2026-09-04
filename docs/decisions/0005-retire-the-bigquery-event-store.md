---
layout: default
title: "0005. Retire the BigQuery store"
parent: Decisions
nav_order: 5
status: accepted
date: 2026-05-30
decision-makers: aphilbert
---

# 0005. Retire the BigQuery event store rather than gate its tests

## Context and problem statement

A `BigQueryEventStore` was added on 2026-03-21 as the fifth store, on the theory
that an append-optimized analytics warehouse is a natural fit for event streams at
scale. Its integration tests ran the `goccy/bigquery-emulator` container whenever
Docker was present, and under CI the emulator flaked with TCP `i/o timeout`s —
each subtest stalling 30–48 seconds and turning the whole `Test` job red. The
store had no in-repo consumer and no downstream service used it. The question was
whether to gate the tests, keep the store untested, or remove it.

## Decision drivers

- A red CI job that everyone learns to re-run is a gate that no longer gates
  (Constitution Article XIV).
- A store that is in the tree but not in the shared suite is a store nobody has
  verified against the append contract ([0001](0001-event-sourcing-append-contract.md)).
- The store carried `cloud.google.com/go/bigquery`, `google.golang.org/api`, and
  roughly twenty transitive dependencies into every consumer's module graph.
- Nothing used it.

## Considered options

1. **Remove the store, its tests, and its dependency tree.**
2. **Gate its tests behind an opt-in environment variable** and keep the code.
3. **Keep the store, mark its tests as skipped** until the emulator stabilises.

## Decision outcome

Chosen option: **option 1**. The cost of carrying the store was real (CI
instability, twenty dependencies) and the benefit was hypothetical. Option 2
would keep code in the tree whose correctness no gate checks, which is the
situation Article VII forbids; option 3 is option 2 without the honesty. If a
consumer needs BigQuery later, the store can be rebuilt against the then-current
contract and join the suite properly — the git history holds the old
implementation.

The wider rule this sets: **pericarp carries a store only while a gate exercises
it against the shared suite.** Optional backends earn their place by being
tested, not by being plausible.

### Consequences

- Good: `go mod tidy` dropped the BigQuery tree; CI stopped flaking.
- Good: the surviving stores — Memory, File, GORM (SQLite/Postgres), DynamoDB —
  are all in the shared suite, and the container-backed ones fail rather than
  skip when `PERICARP_REQUIRE_DOCKER_TESTS` is set.
- Bad: anyone who wanted BigQuery gets nothing today.
- Neutral: this reverses the 2026-03-21 decision. That is what this record is for.

### Confirmation

`go.mod` contains no `cloud.google.com/go/bigquery`; the CI `Build` job runs
`make build` with no private credentials (Article XI).

## Pros and cons of the options

### Option 1 — remove

- Good, because the tree only contains what is verified.
- Bad, because the work of 2026-03-21 is discarded (recoverable from history).

### Option 2 — opt-in gate

- Good, because the code survives.
- Bad, because an opt-in test is a test that does not run, and the dependencies
  remain in every consumer's graph.

### Option 3 — skip

- Bad, because it is option 2 with the appearance of coverage.

## More information

Journal entries of 2026-03-21 (addition) and 2026-05-30 (removal). Done on the
#43 auth branch at the maintainer's request rather than as a separate PR.

Reconstructed from the journal on 2026-09-03.
