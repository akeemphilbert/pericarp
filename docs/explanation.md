---
layout: default
title: Design Decisions
nav_order: 5
---

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

## Authentication Architecture

### The BFF (Backend-for-Frontend) Pattern

Pericarp's authentication module is built around the Backend-for-Frontend pattern, where the backend serves as a secure proxy between the browser and identity providers. This is a deliberate architectural choice over alternatives like implicit flow or client-side token handling.

The key insight: **the browser never sees tokens.** The backend initiates the OAuth flow, exchanges the authorization code for tokens server-to-server, stores tokens encrypted server-side, and gives the browser only an opaque session ID in an `HttpOnly` cookie. Even if an XSS vulnerability exists in the frontend, there are no tokens to steal.

### Why PKCE for Authorization Code Flow

The authorization code flow with PKCE (Proof Key for Code Exchange) prevents authorization code interception attacks. Without PKCE, an attacker who intercepts the authorization code (via a malicious browser extension or open redirect) could exchange it for tokens. PKCE binds the code to the client that initiated the flow:

1. The backend generates a random `code_verifier` (32 bytes, base64url) and its SHA-256 hash as the `code_challenge`
2. The `code_challenge` goes in the authorization URL, the `code_verifier` stays server-side
3. At exchange time, the backend sends the `code_verifier` — the provider verifies it matches the original challenge
4. An attacker who intercepts the code cannot exchange it without the `code_verifier`

### State Validation and Timing Attacks

The OAuth `state` parameter prevents CSRF attacks by binding the authorization request to the user's session. Pericarp uses `crypto/subtle.ConstantTimeCompare` for state validation rather than `==` or `!=`. Regular string comparison leaks timing information — an attacker could determine the correct state value byte-by-byte by measuring response times. Constant-time comparison takes the same amount of time regardless of where strings differ.

### Flow Data Lifecycle

The temporary OAuth flow data (state, code_verifier, nonce) follows a strict lifecycle:

1. Created when the login handler initiates the flow
2. Stored server-side in a separate short-lived session (10-minute TTL)
3. Retrieved and **immediately cleared** during the callback (one-time use)
4. If the callback never happens, the data expires automatically

This prevents replay attacks and ensures stale PKCE data cannot accumulate.

### Session Cookie Security

The HTTP session cookie uses three security flags that work together:

- **`HttpOnly=true`** — JavaScript cannot read the cookie. Prevents XSS from stealing session IDs.
- **`Secure=true`** — Cookie is only sent over HTTPS. Prevents network sniffing.
- **`SameSite=Lax`** — Cookie is sent on top-level navigations but not on cross-origin subrequests. Prevents CSRF while allowing OAuth redirects to work.

`SameSite=Lax` (not `Strict`) is necessary because the OAuth callback is a cross-origin redirect from the identity provider back to your application. `Strict` would block the cookie on this redirect.

## Authorization Model

### Why ODRL?

Pericarp uses the [Open Digital Rights Language (ODRL)](https://www.w3.org/TR/odrl-vocab/) vocabulary for access control rather than inventing a custom permissions model. ODRL provides:

- **Permissions, prohibitions, and duties** — three distinct rule types. Most RBAC systems only have "allow" rules. ODRL adds "deny" (prohibitions) and "obligations" (duties), enabling richer access control.
- **Prohibition precedence** — prohibitions override permissions. If an agent has both `odrl:read` permission and `odrl:read` prohibition on the same target, the prohibition wins. This is critical for security: deny rules must always take priority.
- **Standard vocabulary** — ODRL actions (`odrl:read`, `odrl:modify`, `odrl:delete`) are a W3C standard. Using standard terms enables interoperability and avoids reinventing terminology.
- **Policy composition** — policies can be typed as Sets (general), Offers (proposed), or Agreements (mutual). This maps naturally to policy lifecycle workflows.

### Ontology Choices

The auth domain uses three established ontologies, each for its strength:

- **FOAF (Friend of a Friend)** for agent typing — `foaf:Person`, `foaf:Organization`, `foaf:Group`, `foaf:Agent`. FOAF is the de facto standard for describing people and organizations on the web.
- **W3C ORG** for organizational relationships — `org:hasRole`, `org:hasMember`, `org:memberOf`. ORG provides precise semantics for role assignment and membership, including temporal tracking (`org:hadRole` for revoked roles).
- **Schema.org** for authentication — `schema:credential`, `schema:session`, `schema:provider`. Schema.org covers identity and authentication concepts that FOAF and ORG do not.

### Triple Events for Cross-Aggregate Relationships

Relationships between aggregates (agent-to-role, agent-to-account, session-to-account) are recorded as enriched triple events using `BasicTripleEvent` (subject-predicate-object). This is deliberate:

1. **Aggregates are independent** — an Agent aggregate should not hold a direct reference to a Role aggregate. That would create tight coupling and complicate eventual consistency.
2. **Full audit trail** — triple events capture who was assigned what role, when, and by whom. Revocations are tracked separately (`org:hadRole`) rather than deleting the assignment.
3. **Semantic precision** — using RDF-style triples with standard predicates gives each relationship a precise meaning that can be reasoned about.

### The PolicyDecisionPoint

The `PolicyDecisionPoint` follows the XACML PDP pattern adapted for ODRL:

1. **Collect assignees** — the agent itself plus all roles they hold (global + account-scoped, deduplicated)
2. **Evaluate prohibitions** — check all assignees for matching prohibitions. If any match, deny immediately.
3. **Evaluate permissions** — check all assignees for matching permissions. If any match, allow.
4. **Default deny** — if no rules match, access is denied.

Account-scoped evaluation (`IsAuthorizedInAccount`) adds account-level roles to the assignee set alongside global roles. This allows a single agent to have different permissions in different accounts without duplicating policies.

## Why Casbin for Enforcement?

The `PolicyDecisionPoint` implements authorization using ODRL semantics, but it requires consumers to build a `PermissionStore` — a read model that resolves permissions, prohibitions, and role assignments from their storage layer. This is flexible but requires non-trivial infrastructure.

`CasbinAuthorizationChecker` offers a batteries-included alternative by delegating enforcement to the [Casbin](https://casbin.org/) engine. Casbin provides a battle-tested policy engine with an adapter ecosystem (GORM, PostgreSQL, Redis, file-based, and more). Rather than building a custom read model, you plug in a Casbin adapter and get persistence for free.

### How ODRL Maps to Casbin

The embedded Casbin model maps ODRL concepts directly:

| ODRL Concept | Casbin Representation |
|---|---|
| Permission (allow) | Policy with `eft=allow` |
| Prohibition (deny) | Policy with `eft=deny` |
| Prohibition overrides permission | `deny-override` policy effect |
| Global role assignment | Grouping policy with `domain="*"` |
| Account-scoped role assignment | Grouping policy with `domain=accountID` |

The policy effect expression `some(where (p.eft == allow)) && !some(where (p.eft == deny))` ensures that if any matching policy denies access, the request is denied — matching ODRL's prohibition-overrides-permission semantics.

### Domain Matching Semantics

The `*` wildcard enables global roles. A custom domain matching function is registered so that:

- A policy/grouping with domain `"*"` matches any request domain — global roles and policies apply everywhere.
- A policy/grouping with a specific domain (e.g., `"account-42"`) matches only when the request domain is `"account-42"`.

When calling `IsAuthorized`, the request domain is `"*"` (global context). When calling `IsAuthorizedInAccount`, the request domain is the specific account ID, and the matcher considers both global (`"*"`) and account-specific policies.

### Two Implementation Paths

| Aspect | PolicyDecisionPoint | CasbinAuthorizationChecker |
|---|---|---|
| **Setup** | Implement `PermissionStore` | Pass a Casbin adapter |
| **Role resolution** | Manual (via `GetRolesForAgent`) | Built-in (Casbin's RBAC) |
| **Persistence** | You build the read model | Casbin adapter ecosystem |
| **Flexibility** | Full control over data model | Constrained to Casbin's model |
| **Best for** | Custom projections, CQRS read models | Standard RBAC, rapid prototyping |

Choose `PolicyDecisionPoint` when you already have a CQRS projection that stores permissions or need a custom data model. Choose `CasbinAuthorizationChecker` when you want a working authorization system with minimal setup.

## Dependencies

Pericarp has three external dependencies:

- **`github.com/segmentio/ksuid`** — generates time-sortable unique IDs for events, commands, and session IDs. KSUIDs are preferred over UUIDs because they sort chronologically, which is valuable in an event log. As session IDs, they are opaque and leak no information.
- **`golang.org/x/sync`** — provides `errgroup` for the EventDispatcher's parallel handler execution.
- **`github.com/gorilla/sessions`** — HTTP session management for the auth infrastructure layer. Provides cookie-based session handling with pluggable backends (cookie, filesystem, Redis, database).
