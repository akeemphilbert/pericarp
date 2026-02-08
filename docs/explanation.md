# Explanation: Design Decisions in Pericarp

This document explains the architectural decisions and trade-offs behind Pericarp's design.

## Why Event Sourcing?

Traditional CRUD persistence overwrites state. Event sourcing records every state change as an immutable event. This gives you:

- **Complete audit trail** — every change is captured with its timestamp and metadata
- **Temporal queries** — reconstruct the state of any aggregate at any point in time
- **Event-driven integration** — events are a natural integration boundary between services
- **Debugging** — reproduce bugs by replaying the exact sequence of events

Pericarp provides the building blocks (envelopes, stores, dispatchers) without prescribing your domain model. You define your events and aggregates; Pericarp handles the plumbing.

## Package Structure

Pericarp is organized into layers following clean architecture:

```
pkg/
├── ddd/                            Domain-Driven Design primitives
│   └── entity.go                   BaseEntity for aggregate roots
├── eventsourcing/
│   ├── domain/                     Interfaces and core types
│   │   ├── event.go                EventEnvelope[T], Event interface
│   │   ├── eventstore.go           EventStore interface, sentinel errors
│   │   ├── event_dispatcher.go     EventDispatcher (subscribe + dispatch)
│   │   ├── event_types.go          Standard event type constants and helpers
│   │   ├── entity.go               Entity interface (for UnitOfWork)
│   │   └── marshal.go              JSON marshaling helpers
│   ├── infrastructure/             Concrete EventStore implementations
│   │   ├── memory_eventstore.go    In-memory (testing)
│   │   └── file_eventstore.go      File-based JSON (development)
│   └── application/                Application services
│       └── unitofwork.go           SimpleUnitOfWork
└── cqrs/                           Command dispatching
    └── command_dispatcher.go       CommandDispatcher, Watchable
```

The dependency rule flows inward: infrastructure and application depend on domain, never the reverse. The `cqrs` package is independent — it does not import from `eventsourcing`.

## Generics Strategy

### The envelope pattern

Both `EventEnvelope[T]` and `CommandEnvelope[T]` use Go generics to preserve type safety at the boundary where developers write handlers:

```go
// Your handler receives a strongly-typed envelope — no casting required.
func(ctx context.Context, env domain.EventEnvelope[UserCreatedPayload]) error {
    fmt.Println(env.Payload.Email)  // Compiler-checked field access
    return nil
}
```

### The `any` erasure boundary

The `EventStore` interface operates on `EventEnvelope[any]` because a single store holds events of many different payload types. The conversion happens at the edges:

```
                   type-safe                    type-erased
    EventEnvelope[UserCreated]  ──ToAnyEnvelope──>  EventEnvelope[any]
                                                        │
                                                   EventStore.Append()
                                                        │
    EventEnvelope[UserCreated]  <──type assert──   EventEnvelope[any]
```

`ToAnyEnvelope[T]()` and `ToAnyCommandEnvelope[T]()` handle the conversion. On the read side, the handler wrappers created by `Subscribe[T]` and `RegisterReceiver[T]` perform the type assertion automatically.

### Why package-level generic functions?

Go does not support generic methods on non-generic types. Since `EventDispatcher` and `CommandDispatcher` are concrete (non-generic) types, `Subscribe[T]` and `RegisterReceiver[T]` must be package-level functions:

```go
// This is not valid Go:
// func (d *EventDispatcher) Subscribe[T any](eventType string, handler EventHandler[T]) error

// So we use a package-level function instead:
func Subscribe[T any](d *EventDispatcher, eventType string, handler EventHandler[T]) error
```

The `RegisterReceiver[T]` function in the `cqrs` package accepts the `CommandDispatcher` interface and internally type-asserts to the unexported `receiverRegistrar` interface. This keeps the public API clean while allowing generic registration across both dispatcher variants.

## Sequence Numbers and Optimistic Concurrency

### How sequence numbers work

- A new aggregate starts at sequence number **-1**
- The first event recorded gets sequence number **0**
- Each subsequent event increments by 1
- `BaseEntity.RecordEvent` manages this automatically

### How optimistic concurrency works

When you call `EventStore.Append(ctx, aggregateID, expectedVersion, events...)`:

1. The store checks that the aggregate's current version equals `expectedVersion`
2. If it matches, the events are appended and the version advances
3. If it doesn't match, the store returns `ErrConcurrencyConflict`

Passing `-1` as `expectedVersion` skips the version check entirely — use this for brand-new aggregates.

The `SimpleUnitOfWork` captures the expected version when you call `Track()`, so the check is automatic at commit time.

## Event Dispatch Design

### Pattern matching

Event types follow a `entity.action` convention (e.g., `user.created`, `order.shipped`). The dispatcher builds a set of candidate patterns for each dispatched event:

For `user.created`, it checks registrations for:
1. `user.created` (exact)
2. `user.*` (entity wildcard)
3. `*.created` (action wildcard)
4. `*.*` (full wildcard)

Plus any wildcard handlers registered via `SubscribeWildcard`.

This same pattern matching is used by both the EventDispatcher and the CommandDispatcher.

### Parallel execution

The EventDispatcher runs all matching handlers concurrently using `errgroup`. All handlers complete regardless of individual errors — errors are collected and returned together.

The CommandDispatcher offers two strategies:
- **AsyncCommandDispatcher** — same concurrent approach, but results stream to a `Watchable`
- **QueuedCommandDispatcher** — sequential execution in registration order, with context cancellation checks between receivers

### Why dispatch errors are non-fatal in the UnitOfWork

When `SimpleUnitOfWork.Commit()` succeeds in persisting events, it then dispatches them. If dispatch fails, the commit still succeeds. This is intentional:

1. Events are already durably stored — the system of record is consistent
2. Dispatch failures can be retried by replaying events from the store
3. Making dispatch fatal would mean transient handler errors could prevent persistence

This follows the eventual consistency model: persistence is the source of truth, and dispatch is a best-effort notification.

## The Watchable Pattern

The `Watchable` returned by `CommandDispatcher.Dispatch()` solves a specific problem: a REST controller needs to return a response, but a command might trigger multiple receivers (validation, persistence, notifications, analytics).

Key design decisions:

- **Buffered channel** — sized to the number of matched receivers, so goroutines never block on send even if nobody reads the results
- **`First()`** — returns the first result immediately; the controller can respond to the client while other receivers (logging, analytics) continue in the background
- **`Wait()`** — collects all results when you need them all
- **`Done()`** — channel-based completion signal for use in `select` statements

The buffer size is critical: if a controller calls `First()` and returns, the remaining receivers write to the buffered channel without blocking. No goroutine leaks.

## BaseEntity vs Entity Interface

Pericarp separates these concerns:

- **`ddd.BaseEntity`** (struct) — the concrete implementation you embed in your aggregates. Manages sequence numbers, uncommitted events, idempotency checks, and thread safety.
- **`domain.Entity`** (interface) — the contract the `UnitOfWork` depends on. Any type that implements `GetID()`, `GetSequenceNo()`, `GetUncommittedEvents()`, and `ClearUncommittedEvents()` can be tracked.

`BaseEntity` satisfies the `Entity` interface, so embedding it is the simplest path. But you could implement `Entity` yourself if you need custom behavior.

## Why Two EventStore Implementations?

- **`MemoryStore`** — fast, zero-setup, no disk I/O. Ideal for unit tests. Data is lost when the process exits.
- **`FileStore`** — persists to JSON files on disk. Useful for local development and debugging (you can inspect the files). Not suitable for production due to lack of transactional guarantees across aggregates.

Both implement the `EventStore` interface, so switching between them is a one-line change. Production implementations (PostgreSQL, DynamoDB, etc.) would also implement this interface.

## Thread Safety Model

Every stateful component in Pericarp is protected by `sync.RWMutex`:

- `BaseEntity` — protects aggregate state and uncommitted events
- `MemoryStore` / `FileStore` — protects event storage
- `EventDispatcher` — protects handler registry (lock released before invoking handlers)
- `CommandDispatcher` — protects receiver registry (lock released before invoking receivers)
- `SimpleUnitOfWork` — protects entity tracking

The dispatchers specifically release the registry lock before invoking handlers/receivers. This prevents deadlocks when a handler registers new handlers during execution.

## Dependencies

Pericarp has only two external dependencies:

- **`github.com/segmentio/ksuid`** — generates time-sortable unique IDs for events and commands. KSUIDs are preferred over UUIDs because they sort chronologically, which is valuable in an event log.
- **`golang.org/x/sync`** — provides `errgroup` for the EventDispatcher's parallel handler execution.
