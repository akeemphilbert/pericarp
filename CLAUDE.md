# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

Pericarp is a Go library implementing Event Sourcing and DDD primitives. It provides base types for aggregates, event envelopes, event stores, a unit of work, and an event dispatcher. It is the core library used by vine-os microservices.

## Commands

```bash
# Development cycle (format → lint → test)
make dev-test

# Run all tests with race detection
make test

# Run a single test
go test -v -race -run TestEventEnvelope ./pkg/eventsourcing/domain/

# Lint
make lint

# Build
make build

# Full CI pipeline (deps → format → lint → test)
make ci

# Install golangci-lint
make install-tools
```

## Architecture

```
pkg/
├── ddd/                              # BaseEntity — embed in aggregates
├── eventsourcing/
│   ├── domain/                       # Interfaces and core types
│   │   ├── event.go                  # Event interface, EventEnvelope[T], BasicTripleEvent
│   │   ├── eventstore.go             # EventStore interface, sentinel errors, ToAnyEnvelope
│   │   ├── event_dispatcher.go       # EventDispatcher with pattern-matching subscriptions
│   │   ├── event_type.go             # EventTypeFor() helper, standard type constants
│   │   └── entity.go                 # Entity interface (for UnitOfWork tracking)
│   ├── infrastructure/               # EventStore implementations
│   │   ├── memory_store.go           # In-memory store (testing)
│   │   ├── file_store.go             # File-based JSON store (development)
│   │   ├── gorm_store.go             # GORM store (SQLite/Postgres)
│   │   ├── bigquery_store.go         # BigQuery store (analytics at scale)
│   │   ├── bigtable_store.go         # Bigtable store (managed NoSQL on GCP)
│   │   ├── dynamo_store.go           # DynamoDB store
│   │   └── composite_store.go        # Primary-sync + secondaries-async wrapper
│   └── application/                  # UnitOfWork
│       └── unit_of_work.go           # SimpleUnitOfWork — tracks entities, atomic commit
```

### Key Types and Their Relationships

**BaseEntity** (`pkg/ddd/entity.go`) — Embed in your aggregate root. Manages aggregate ID, sequence numbers, uncommitted events. Call `RecordEvent(payload, eventType)` to record new events, `ApplyEvent(ctx, envelope)` to replay from store.

**EventEnvelope[T]** (`domain/event.go`) — Generic wrapper around event payloads. Fields: `ID`, `AggregateID`, `EventType`, `Payload`, `Created`, `SequenceNo`, `Metadata`. Created via `NewEventEnvelope(payload, aggregateID, eventType, sequenceNo)`.

**EventStore** (`domain/eventstore.go`) — Interface for persistence. `Append()` uses optimistic concurrency (`expectedVersion`). Two implementations: `MemoryStore` (tests) and `FileStore` (dev).

**SimpleUnitOfWork** (`application/unit_of_work.go`) — Tracks multiple entities, commits their uncommitted events atomically to an EventStore. Optionally dispatches events to an EventDispatcher after commit.

**EventDispatcher** (`domain/event_dispatcher.go`) — Subscribe to event types with pattern matching (`user.created`, `user.*`, `*.created`, `*.*`). Handlers run in parallel via `errgroup`.

### Event Flow

1. Aggregate calls `BaseEntity.RecordEvent(payload, eventType)` — creates envelope, increments sequence number, adds to uncommitted list
2. `SimpleUnitOfWork.Track(entity)` registers the aggregate
3. `SimpleUnitOfWork.Commit(ctx)` calls `EventStore.Append()` with optimistic concurrency, then optionally dispatches to `EventDispatcher`

### Sequence Numbers

- New aggregates start at sequence 0 (no events); first event gets sequence 1
- Strict ordering enforced (no gaps)
- `expectedVersion` in `Append()` uses the sequence number before the new events

### Generics Pattern

`EventEnvelope[T]` is generic, but `EventStore` operates on `EventEnvelope[any]`. Use `ToAnyEnvelope[T]()` to convert typed envelopes for storage.

## Dependencies

Only two: `github.com/segmentio/ksuid` (event IDs) and `golang.org/x/sync` (errgroup for parallel dispatch). Go 1.24+.

## Testing Conventions

- Table-driven tests with `t.Parallel()`
- Race detection enabled (`-race` flag)
- Tests colocated with source files
- `MemoryStore` used as the default test EventStore

## Project Journal

An append-only journal at `.claude/journal.md` tracks major changes to Pericarp.

**When to read it:** At the start of any major task (new feature, architectural change, new package) to understand recent context and avoid contradicting prior decisions.

**When to append:** After completing a major change — new packages, architectural decisions, significant feature additions, design pivots, or scope changes. Do not log routine bug fixes, test additions, or minor refactors.

**Entry format:**
- Heading: `### YYYY-MM-DD: Short description`
- A few bullets covering what changed, why, and key design decisions
- Keep entries concise (3-6 bullets)