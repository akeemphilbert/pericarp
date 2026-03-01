---
layout: default
title: API Reference
nav_order: 4
---

# API Reference

Complete reference for all exported types, functions, and interfaces in Pericarp.

## Package `ddd`

`import "github.com/akeemphilbert/pericarp/pkg/ddd"`

Provides the base aggregate root for domain entities with event sourcing capabilities.

### Types

#### `BaseEntity`

```go
type BaseEntity struct { /* unexported fields */ }
```

Embed in your aggregate root to gain event tracking, sequence number management, and uncommitted event collection. Thread-safe.

New aggregates start at sequence number -1. The first recorded event gets sequence number 0.

#### Sentinel Errors

```go
var ErrWrongAggregate         = errors.New("event does not belong to this aggregate")
var ErrDuplicateEvent         = errors.New("event has already been applied")
var ErrInvalidEventSequenceNo = errors.New("event sequence number is invalid")
```

### Functions

#### `NewBaseEntity`

```go
func NewBaseEntity(aggregateID string) *BaseEntity
```

Creates a new BaseEntity with the given aggregate ID, starting at sequence number -1.

### Methods on `BaseEntity`

#### `GetID`

```go
func (e *BaseEntity) GetID() string
```

Returns the aggregate ID. Thread-safe.

#### `GetSequenceNo`

```go
func (e *BaseEntity) GetSequenceNo() int
```

Returns the last event sequence number. Thread-safe.

#### `GetUncommittedEvents`

```go
func (e *BaseEntity) GetUncommittedEvents() []domain.EventEnvelope[any]
```

Returns a copy of all uncommitted events. Thread-safe.

#### `ClearUncommittedEvents`

```go
func (e *BaseEntity) ClearUncommittedEvents()
```

Removes all uncommitted events. Typically called after successful persistence. Thread-safe.

#### `ApplyEvent`

```go
func (e *BaseEntity) ApplyEvent(ctx context.Context, event domain.EventEnvelope[any]) error
```

Applies a stored event to the entity during replay. Validates the event belongs to this aggregate, checks for duplicate application (idempotency), verifies the sequence number is consecutive, and advances the sequence number.

Returns `ErrWrongAggregate`, `ErrDuplicateEvent`, or `ErrInvalidEventSequenceNo` on validation failure.

#### `RecordEvent`

```go
func (e *BaseEntity) RecordEvent(payload any, eventType string) error
```

Records a new domain event. Creates an `EventEnvelope` internally with the next sequence number, marks the event as applied, and adds it to the uncommitted events list.

---

## Package `domain`

`import "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"`

Core interfaces and types for event sourcing.

### Types

#### `Event` (interface)

```go
type Event interface {
    GetID() string
    GetSequenceNo() int
}
```

Optional interface that event payloads can implement. When a payload implements `Event`, `NewEventEnvelope` extracts the aggregate ID from `GetID()` instead of using the `aggregateID` parameter.

#### `EventEnvelope[T]`

```go
type EventEnvelope[T any] struct {
    ID          string                 `json:"id"`
    AggregateID string                 `json:"aggregate_id"`
    EventType   string                 `json:"event_type"`
    Payload     T                      `json:"payload"`
    Created     time.Time              `json:"timestamp"`
    SequenceNo  int                    `json:"sequence_no"`
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
```

Generic wrapper around event payloads with metadata for transport and persistence. Implements `json.Marshaler` and `json.Unmarshaler`.

#### `BasicTripleEvent`

```go
type BasicTripleEvent struct {
    Subject   string `json:"subject"`
    Predicate string `json:"predicate"`
    Object    string `json:"object"`
    Original  int64  `json:"original"`
}
```

Standard RDF-style subject-predicate-object event payload for modeling relationships.

#### `Entity` (interface)

```go
type Entity interface {
    GetID() string
    GetSequenceNo() int
    GetUncommittedEvents() []EventEnvelope[any]
    ClearUncommittedEvents()
}
```

Interface for entities tracked by `UnitOfWork`. `ddd.BaseEntity` satisfies this interface.

#### `EventStore` (interface)

```go
type EventStore interface {
    Append(ctx context.Context, aggregateID string, expectedVersion int, events ...EventEnvelope[any]) error
    GetEvents(ctx context.Context, aggregateID string) ([]EventEnvelope[any], error)
    GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]EventEnvelope[any], error)
    GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]EventEnvelope[any], error)
    GetEventByID(ctx context.Context, eventID string) (EventEnvelope[any], error)
    GetCurrentVersion(ctx context.Context, aggregateID string) (int, error)
    Close() error
}
```

Interface for event persistence. Implementations must be thread-safe.

**`Append`** — Appends events with optimistic concurrency control. Pass `expectedVersion = -1` to skip the version check.

**`GetEventsRange`** — Pass `-1` for `fromVersion` to start from the beginning. Pass `-1` for `toVersion` to read to the end.

**`GetCurrentVersion`** — Returns `0` if the aggregate doesn't exist.

#### `EventHandler[T]`

```go
type EventHandler[T any] func(ctx context.Context, env EventEnvelope[T]) error
```

Type-safe handler function for processing events.

#### `EventDispatcher`

```go
type EventDispatcher struct { /* unexported fields */ }
```

Registers event handlers and dispatches events to them using dot-separated pattern matching. Handlers execute in parallel via `errgroup`.

#### Sentinel Errors

```go
var ErrEventNotFound      = errors.New("event not found")
var ErrConcurrencyConflict = errors.New("concurrency conflict: expected version mismatch")
var ErrInvalidEvent        = errors.New("invalid event")
```

#### Event Type Constants

```go
const EventTypeCreate = "created"
const EventTypeUpdate = "updated"
const EventTypeDelete = "deleted"
const EventTypeTriple = "triple"
```

### Functions

#### `NewEventEnvelope[T]`

```go
func NewEventEnvelope[T any](payload T, aggregateID, eventType string, sequenceNo int) EventEnvelope[T]
```

Creates a new `EventEnvelope`. Generates a KSUID for the ID. If `payload` implements the `Event` interface, `aggregateID` is extracted from `payload.GetID()` instead of using the parameter.

#### `ToAnyEnvelope[T]`

```go
func ToAnyEnvelope[T any](envelope EventEnvelope[T]) EventEnvelope[any]
```

Converts a typed `EventEnvelope[T]` to `EventEnvelope[any]` for storage in an `EventStore`.

#### `EventTypeFor`

```go
func EventTypeFor(entityType, action string) string
```

Constructs a dot-separated event type string. `EventTypeFor("user", "created")` returns `"user.created"`.

#### `IsStandardEventType`

```go
func IsStandardEventType(eventType string) bool
```

Returns `true` if the event type is one of the four standard constants.

#### `NewEventDispatcher`

```go
func NewEventDispatcher() *EventDispatcher
```

Creates a new `EventDispatcher`.

#### `Subscribe[T]`

```go
func Subscribe[T any](d *EventDispatcher, eventType string, handler EventHandler[T]) error
```

Registers a typed event handler for a specific event type pattern. The handler is wrapped to perform type assertion from `EventEnvelope[any]` to `EventEnvelope[T]`. Also registers a type factory for deserialization support.

Returns an error if `eventType` is empty or `handler` is nil.

#### `SubscribeWildcard` (method)

```go
func (d *EventDispatcher) SubscribeWildcard(handler func(context.Context, EventEnvelope[any]) error) error
```

Registers a catch-all handler invoked for every dispatched event.

#### `Dispatch` (method)

```go
func (d *EventDispatcher) Dispatch(ctx context.Context, envelope EventEnvelope[any]) error
```

Dispatches an event to all matching handlers. Pattern matching resolves exact, entity wildcard (`user.*`), action wildcard (`*.created`), full wildcard (`*.*`), and registered wildcard handlers. All handlers run in parallel. Returns a combined error if any handler fails.

#### `RegisterType[T]`

```go
func RegisterType[T any](d *EventDispatcher, eventType string, factory func() T) error
```

Registers a type factory for deserialization. Used when unmarshaling events from storage.

#### `UnmarshalEvent` (method)

```go
func (d *EventDispatcher) UnmarshalEvent(ctx context.Context, data []byte, eventType string) (EventEnvelope[any], error)
```

Unmarshals a JSON event using the type factory registered for `eventType`.

#### `WrapEvent[T]`

```go
func WrapEvent[T any](payload T, aggregateID, eventType string, sequenceNo int) (EventEnvelope[T], error)
```

Wraps a typed payload in an `EventEnvelope`. Extracts `AggregateID` from payload if it implements `Event`.

#### `MarshalEventToJSON[T]`

```go
func MarshalEventToJSON[T any](envelope EventEnvelope[T]) ([]byte, error)
```

Marshals a typed `EventEnvelope` to JSON bytes.

#### `UnmarshalEventFromJSON[T]`

```go
func UnmarshalEventFromJSON[T any](data []byte) (EventEnvelope[T], error)
```

Unmarshals JSON bytes into a typed `EventEnvelope[T]`.

---

## Package `infrastructure`

`import "github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"`

Concrete `EventStore` implementations.

### `MemoryStore`

```go
type MemoryStore struct { /* unexported fields */ }
```

In-memory event store. Thread-safe. Data is lost when the process exits. Ideal for unit tests.

```go
func NewMemoryStore() *MemoryStore
```

Creates a new in-memory store.

All `EventStore` interface methods are implemented, plus:

```go
func (m *MemoryStore) GetAllAggregateIDs() []string
```

Returns all aggregate IDs in the store. Useful for testing.

### `FileStore`

```go
type FileStore struct { /* unexported fields */ }
```

File-based event store. Stores events as JSON, one file per aggregate. Uses an in-memory cache for reads. Writes are atomic (temp file + rename). Thread-safe.

```go
func NewFileStore(baseDir string) (*FileStore, error)
```

Creates a new file store at the given directory path. Creates the directory if it doesn't exist. Loads all existing events from disk into memory on startup.

All `EventStore` interface methods are implemented.

---

## Package `application`

`import "github.com/akeemphilbert/pericarp/pkg/eventsourcing/application"`

Application-level services.

### Types

#### `UnitOfWork` (interface)

```go
type UnitOfWork interface {
    Track(entities ...domain.Entity) error
    Commit(ctx context.Context) error
    Rollback() error
}
```

Manages atomic persistence of uncommitted events from multiple tracked entities.

#### `SimpleUnitOfWork`

```go
type SimpleUnitOfWork struct { /* unexported fields */ }
```

Default `UnitOfWork` implementation with optimistic concurrency control and optional event dispatch.

### Functions

#### `NewSimpleUnitOfWork`

```go
func NewSimpleUnitOfWork(eventStore domain.EventStore, dispatcher *domain.EventDispatcher) *SimpleUnitOfWork
```

Creates a new unit of work. `dispatcher` is optional — pass `nil` if event dispatch isn't needed.

### Methods on `SimpleUnitOfWork`

#### `Track`

```go
func (uow *SimpleUnitOfWork) Track(entities ...domain.Entity) error
```

Registers entities for the next commit. Validates all entities before tracking any (nil check, non-empty ID, no duplicates). Captures each entity's current sequence number as the expected version for optimistic concurrency.

#### `Commit`

```go
func (uow *SimpleUnitOfWork) Commit(ctx context.Context) error
```

Persists all uncommitted events from tracked entities. For each aggregate, calls `EventStore.Append` with the expected version captured at `Track` time. On success, clears uncommitted events and tracking. If a dispatcher was provided, dispatches all persisted events (dispatch errors are non-fatal). On persistence failure, calls `Rollback`.

#### `Rollback`

```go
func (uow *SimpleUnitOfWork) Rollback() error
```

Clears entity tracking without clearing uncommitted events. The entities can be re-tracked in a new unit of work for retry.

---

## Package `cqrs`

`import "github.com/akeemphilbert/pericarp/pkg/cqrs"`

Command dispatching with typed receivers and observable results.

### Types

#### `CommandEnvelope[T]`

```go
type CommandEnvelope[T any] struct {
    ID          string         `json:"id"`
    CommandType string         `json:"command_type"`
    Payload     T              `json:"payload"`
    Created     time.Time      `json:"timestamp"`
    Metadata    map[string]any `json:"metadata,omitempty"`
}
```

Generic wrapper around command payloads with metadata.

#### `CommandReceiver[T]`

```go
type CommandReceiver[T any] func(ctx context.Context, env CommandEnvelope[T]) (any, error)
```

Type-safe receiver function for processing commands.

#### `CommandResult`

```go
type CommandResult struct {
    Value       any
    Error       error
    CommandType string
}
```

Result from a single receiver execution. When a receiver returns an error, `Value` is `nil`. When a receiver panics, the panic is recovered and wrapped as an `Error`.

#### `Watchable`

```go
type Watchable struct { /* unexported fields */ }
```

Allows the caller to observe results incrementally as receivers complete. The internal results channel is buffered to the number of matched receivers so goroutines never block on send.

#### `CommandDispatcher` (interface)

```go
type CommandDispatcher interface {
    Dispatch(ctx context.Context, envelope CommandEnvelope[any]) *Watchable
    RegisterWildcardReceiver(receiver func(context.Context, CommandEnvelope[any]) (any, error)) error
    Close() error
}
```

Common interface for both async and queued dispatchers.

#### `AsyncCommandDispatcher`

```go
type AsyncCommandDispatcher struct { /* unexported fields */ }
```

Executes all matched receivers concurrently using goroutines. Results are sent to the `Watchable` as each receiver completes. All receivers run to completion regardless of individual errors.

#### `QueuedCommandDispatcher`

```go
type QueuedCommandDispatcher struct { /* unexported fields */ }
```

Executes matched receivers sequentially in registration order. Each result is sent to the `Watchable` before the next receiver is invoked. If the context is cancelled between receivers, subsequent receivers are skipped.

### Functions

#### `NewCommandEnvelope[T]`

```go
func NewCommandEnvelope[T any](payload T, commandType string) CommandEnvelope[T]
```

Creates a new `CommandEnvelope` with a KSUID, current timestamp, and empty metadata map.

#### `ToAnyCommandEnvelope[T]`

```go
func ToAnyCommandEnvelope[T any](env CommandEnvelope[T]) CommandEnvelope[any]
```

Converts a typed `CommandEnvelope[T]` to `CommandEnvelope[any]` for dispatch.

#### `NewAsyncCommandDispatcher`

```go
func NewAsyncCommandDispatcher() *AsyncCommandDispatcher
```

#### `NewQueuedCommandDispatcher`

```go
func NewQueuedCommandDispatcher() *QueuedCommandDispatcher
```

#### `RegisterReceiver[T]`

```go
func RegisterReceiver[T any](d CommandDispatcher, commandType string, receiver CommandReceiver[T]) error
```

Registers a typed receiver for a command type pattern. Supports the same dot-separated pattern matching as the EventDispatcher (`user.create`, `user.*`, `*.create`, `*.*`). Multiple receivers can be registered for the same pattern.

Returns an error if `commandType` is empty, `receiver` is nil, or the dispatcher doesn't support registration.

### Methods on `Watchable`

#### `Results`

```go
func (w *Watchable) Results() <-chan CommandResult
```

Returns a receive-only channel of results. The channel is closed after all receivers complete.

#### `Wait`

```go
func (w *Watchable) Wait() []CommandResult
```

Blocks until all receivers complete and returns all results as a slice.

#### `First`

```go
func (w *Watchable) First() (CommandResult, bool)
```

Blocks until the first result arrives. Returns `(result, true)` on success. Returns `(zero, false)` if no receivers were registered. Remaining receivers continue in the background.

#### `Done`

```go
func (w *Watchable) Done() <-chan struct{}
```

Returns a channel that is closed when all receivers have completed. Use in `select` statements.

### Methods on dispatchers

#### `Dispatch`

```go
func (d *AsyncCommandDispatcher) Dispatch(ctx context.Context, envelope CommandEnvelope[any]) *Watchable
func (d *QueuedCommandDispatcher) Dispatch(ctx context.Context, envelope CommandEnvelope[any]) *Watchable
```

Resolves matching receivers and executes them. Returns a `Watchable` for observing results. If no receivers match, the returned `Watchable` is immediately complete with zero results.

#### `RegisterWildcardReceiver`

```go
func (d *AsyncCommandDispatcher) RegisterWildcardReceiver(receiver func(context.Context, CommandEnvelope[any]) (any, error)) error
func (d *QueuedCommandDispatcher) RegisterWildcardReceiver(receiver func(context.Context, CommandEnvelope[any]) (any, error)) error
```

Registers a catch-all receiver invoked for all command types. Returns an error if `receiver` is nil.

#### `Close`

```go
func (d *AsyncCommandDispatcher) Close() error
func (d *QueuedCommandDispatcher) Close() error
```

Releases resources. Currently a no-op for both implementations.

---

## Package `auth/domain/entities`

`import "github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"`

Domain aggregates for authentication, authorization, and identity management.

### Aggregates

#### `Agent`

```go
type Agent struct {
    *ddd.BaseEntity
    // unexported fields
}
```

Represents an authenticated party in the system. Uses FOAF vocabulary for agent typing.

**Agent Types** (constants):

| Constant | Value | Description |
|----------|-------|-------------|
| `AgentTypePerson` | `foaf:Person` | Human user |
| `AgentTypeOrganization` | `foaf:Organization` | Organizational entity |
| `AgentTypeGroup` | `foaf:Group` | Group of agents |
| `AgentTypeSoftwareAgent` | `foaf:Agent` | Automated software agent |

**Methods:**

- `With(id, name, agentType string) (*Agent, error)` — Factory. Creates a new agent and records `Agent.Created`. Defaults to `foaf:Person` if `agentType` is empty.
- `Restore(id, name, agentType string, active bool, createdAt time.Time) error` — Restores from database without recording events.
- `Activate() error` / `Deactivate() error` — Idempotent state transitions.
- `AssignRole(roleID string) error` — Records `Agent.RoleAssigned` triple event (agent, `org:hasRole`, role).
- `RevokeRole(roleID string) error` — Records `Agent.RoleRevoked` triple event (agent, `org:hadRole`, role).
- `AddToGroup(groupID string) error` — Records `Agent.GroupMembershipAdded` (agent, `foaf:member`, group).
- `RemoveFromGroup(groupID string) error` — Records `Agent.GroupMembershipRemoved`.
- `Name() string`, `AgentType() string`, `Active() bool`, `CreatedAt() time.Time` — Accessors.

#### `Credential`

```go
type Credential struct {
    *ddd.BaseEntity
    // unexported fields
}
```

Links an external identity provider account to an Agent. Uses Schema.org vocabulary.

**Methods:**

- `With(id, agentID, provider, providerUserID, email, displayName string) (*Credential, error)` — Factory. Creates a new credential and records `Credential.Created` triple event (agent, `schema:credential`, credential).
- `Restore(id, agentID, provider, providerUserID, email, displayName string, active bool, createdAt, lastUsedAt time.Time) error` — Restores from database.
- `MarkUsed() error` — Records `Credential.Used`. Updates `LastUsedAt`.
- `Deactivate() error` / `Reactivate() error` — Idempotent state transitions.
- `ApplyEvent(ctx context.Context, envelope domain.EventEnvelope[any]) error` — Replays events for state reconstruction.
- `AgentID() string`, `Provider() string`, `ProviderUserID() string`, `Email() string`, `DisplayName() string`, `Active() bool`, `CreatedAt() time.Time`, `LastUsedAt() time.Time` — Accessors.

#### `AuthSession`

```go
type AuthSession struct {
    *ddd.BaseEntity
    // unexported fields
}
```

Represents an authenticated session with expiration and optional account scoping.

**Methods:**

- `With(id, agentID, credentialID, ipAddress, userAgent string, expiresAt time.Time) (*AuthSession, error)` — Factory. Records `Session.Created` triple event (agent, `schema:session`, session).
- `Restore(id, agentID, accountID, credentialID, ipAddress, userAgent string, active bool, createdAt, expiresAt, lastAccessedAt time.Time) error` — Restores from database.
- `Touch() error` — Updates `LastAccessedAt`. Records `Session.Touched`.
- `Revoke() error` — Idempotent. Records `Session.Revoked`.
- `IsExpired() bool` — Returns true if the current time is past `ExpiresAt`.
- `ScopeToAccount(accountID string) error` — Records `Session.AccountScoped` triple event (session, `schema:authenticator`, account).
- `ApplyEvent(ctx context.Context, envelope domain.EventEnvelope[any]) error` — Replays events.
- `AgentID() string`, `AccountID() string`, `CredentialID() string`, `Active() bool`, `CreatedAt() time.Time`, `ExpiresAt() time.Time`, `LastAccessedAt() time.Time`, `IPAddress() string`, `UserAgent() string` — Accessors.

#### `Account`

```go
type Account struct {
    *ddd.BaseEntity
    // unexported fields
}
```

Tenant/workspace container for multi-tenancy. Uses W3C ORG vocabulary.

**Methods:**

- `With(id, name string) (*Account, error)` — Factory. Records `Account.Created`.
- `Activate() error` / `Deactivate() error` — Idempotent.
- `AddMember(agentID, roleID string) error` — Records `Account.MemberAdded` triple event (account, `org:hasMember`, agent) with role metadata.
- `RemoveMember(agentID string) error` — Records `Account.MemberRemoved` (account, `org:hadMember`, agent).
- `ChangeMemberRole(agentID, newRoleID string) error` — Records `Account.MemberRoleChanged`.

#### `Role`

```go
type Role struct {
    *ddd.BaseEntity
    // unexported fields
}
```

Named role with description. Aligned with W3C ORG ontology.

- `With(id, name, description string) (*Role, error)` — Factory. Records `Role.Created`.
- `Restore(id, name, description string, createdAt time.Time) error` — Restores from database.

#### `Policy`

```go
type Policy struct {
    *ddd.BaseEntity
    // unexported fields
}
```

ODRL (Open Digital Rights Language) policy for access control.

**Policy Types** (constants):

| Constant | Value | Description |
|----------|-------|-------------|
| `PolicyTypeSet` | `odrl:Set` | General-purpose policy |
| `PolicyTypeOffer` | `odrl:Offer` | Policy proposed by the assigner |
| `PolicyTypeAgreement` | `odrl:Agreement` | Policy agreed upon by both parties |

**Methods:**

- `With(id, name, policyType string) (*Policy, error)` — Factory. Defaults to `odrl:Set`.
- `GrantPermission(assignee, action, target string) error` — Records `Policy.PermissionGranted` triple event.
- `RevokePermission(assignee, action, target string) error` — Records `Policy.PermissionRevoked`.
- `SetProhibition(assignee, action, target string) error` — Records `Policy.ProhibitionSet`.
- `RevokeProhibition(assignee, action, target string) error` — Records `Policy.ProhibitionRevoked`.
- `ImposeDuty(assignee, action, target string) error` — Records `Policy.DutyImposed`.
- `DischargeDuty(assignee, action, target string) error` — Records `Policy.DutyDischarged`.
- `Assign(assigneeID string) error` — Records `Policy.Assigned`.

### Ontology Constants

**ODRL Actions:**

| Constant | Value |
|----------|-------|
| `ActionUse` | `odrl:use` |
| `ActionRead` | `odrl:read` |
| `ActionModify` | `odrl:modify` |
| `ActionDelete` | `odrl:delete` |
| `ActionExecute` | `odrl:execute` |
| `ActionAggregate` | `odrl:aggregate` |
| `ActionDistribute` | `odrl:distribute` |
| `ActionTransfer` | `odrl:transfer` |

**Relationship Predicates:**

| Constant | Value | Usage |
|----------|-------|-------|
| `PredicateHasRole` | `org:hasRole` | Agent currently holds a role |
| `PredicateHadRole` | `org:hadRole` | Agent previously held a role (revocation) |
| `PredicateHasMember` | `org:hasMember` | Account has a member agent |
| `PredicateHadMember` | `org:hadMember` | Account had a member (removal) |
| `PredicateMember` | `foaf:member` | Agent belongs to group |
| `PredicateCredential` | `schema:credential` | Agent linked to credential |
| `PredicateSession` | `schema:session` | Agent linked to session |
| `PredicateAuthenticator` | `schema:authenticator` | Session scoped to account |

### Event Type Constants

All event types follow the `Aggregate.Action` naming convention:

```go
// Agent events
EventTypeAgentCreated, EventTypeAgentActivated, EventTypeAgentDeactivated
EventTypeAgentRoleAssigned, EventTypeAgentRoleRevoked
EventTypeAgentGroupMembershipAdded, EventTypeAgentGroupMembershipRemoved

// Credential events
EventTypeCredentialCreated, EventTypeCredentialUsed
EventTypeCredentialDeactivated, EventTypeCredentialReactivated

// Session events
EventTypeSessionCreated, EventTypeSessionTouched
EventTypeSessionRevoked, EventTypeSessionAccountScoped

// Account events
EventTypeAccountCreated, EventTypeAccountActivated, EventTypeAccountDeactivated
EventTypeAccountMemberAdded, EventTypeAccountMemberRemoved, EventTypeAccountMemberRoleChanged

// Policy events
EventTypePolicyCreated, EventTypePolicyActivated, EventTypePolicyDeactivated
EventTypePermissionGranted, EventTypePermissionRevoked
EventTypeProhibitionSet, EventTypeProhibitionRevoked
EventTypeDutyImposed, EventTypeDutyDischarged, EventTypePolicyAssigned

// Role events
EventTypeRoleCreated
```

---

## Package `auth/domain/repositories`

`import "github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"`

Repository interfaces for auth aggregate persistence. All support cursor-based pagination.

### Types

#### `PaginatedResponse[T]`

```go
type PaginatedResponse[T any] struct {
    Data    []T
    Cursor  string
    Limit   int
    HasMore bool
}
```

#### `AgentRepository` (interface)

```go
Save(ctx context.Context, agent *entities.Agent) error
FindByID(ctx context.Context, id string) (*entities.Agent, error)
FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Agent], error)
```

#### `CredentialRepository` (interface)

```go
Save(ctx context.Context, credential *entities.Credential) error
FindByID(ctx context.Context, id string) (*entities.Credential, error)
FindByProvider(ctx context.Context, provider, providerUserID string) (*entities.Credential, error)
FindByAgent(ctx context.Context, agentID string) ([]*entities.Credential, error)
FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.Credential], error)
```

#### `AuthSessionRepository` (interface)

```go
Save(ctx context.Context, session *entities.AuthSession) error
FindByID(ctx context.Context, id string) (*entities.AuthSession, error)
FindByAgent(ctx context.Context, agentID string) ([]*entities.AuthSession, error)
FindActive(ctx context.Context, agentID string) ([]*entities.AuthSession, error)
FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[*entities.AuthSession], error)
RevokeAllForAgent(ctx context.Context, agentID string) error
```

#### `AccountRepository`, `RoleRepository`, `PolicyRepository` (interfaces)

Follow the same pattern with `Save`, `FindByID`, `FindAll`, plus domain-specific finders.

---

## Package `auth/application`

`import "github.com/akeemphilbert/pericarp/pkg/auth/application"`

Application services for authentication and authorization.

### Types

#### `AuthRequest`

```go
type AuthRequest struct {
    AuthURL      string  // Authorization URL to redirect the user to
    State        string  // CSRF protection parameter
    CodeVerifier string  // PKCE code verifier (store server-side)
    Nonce        string  // OpenID Connect nonce
    Provider     string  // Provider name
}
```

#### `AuthResult`

```go
type AuthResult struct {
    AccessToken  string
    RefreshToken string
    IDToken      string
    TokenType    string
    ExpiresIn    int      // seconds
    UserInfo     UserInfo
}
```

#### `UserInfo`

```go
type UserInfo struct {
    ProviderUserID string  // User's ID at the provider
    Email          string
    DisplayName    string
    AvatarURL      string
    Provider       string  // Provider name (e.g., "google")
}
```

#### `SessionInfo`

```go
type SessionInfo struct {
    SessionID   string
    AgentID     string
    AccountID   string        // Empty if not scoped
    Permissions []Permission
    ExpiresAt   time.Time
}
```

#### `Permission`

```go
type Permission struct {
    Assignee string  // Agent or Role ID
    Action   string  // ODRL action IRI
    Target   string  // Resource identifier or "*" wildcard
}
```

### Interfaces

#### `AuthenticationService`

```go
type AuthenticationService interface {
    InitiateAuthFlow(ctx context.Context, provider, redirectURI string) (*AuthRequest, error)
    ExchangeCode(ctx context.Context, code, codeVerifier, provider, redirectURI string) (*AuthResult, error)
    ValidateState(ctx context.Context, receivedState, storedState string) error
    FindOrCreateAgent(ctx context.Context, userInfo UserInfo) (*entities.Agent, *entities.Credential, error)
    CreateSession(ctx context.Context, agentID, credentialID, ipAddress, userAgent string, duration time.Duration) (*entities.AuthSession, error)
    ValidateSession(ctx context.Context, sessionID string) (*SessionInfo, error)
    RefreshTokens(ctx context.Context, credentialID string) (*AuthResult, error)
    RevokeSession(ctx context.Context, sessionID string) error
    RevokeAllSessions(ctx context.Context, agentID string) error
}
```

#### `OAuthProvider`

```go
type OAuthProvider interface {
    Name() string
    AuthCodeURL(state, codeChallenge, nonce, redirectURI string) string
    Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (*AuthResult, error)
    RefreshToken(ctx context.Context, refreshToken string) (*AuthResult, error)
    RevokeToken(ctx context.Context, token string) error
    ValidateIDToken(ctx context.Context, idToken, nonce string) (*UserInfo, error)
}
```

Provider-agnostic interface for OAuth 2.0 / OpenID Connect identity providers. Implement once per provider (Google, GitHub, etc.).

#### `OAuthProviderRegistry`

```go
type OAuthProviderRegistry map[string]OAuthProvider
```

Maps provider names to their implementations.

#### `TokenStore`

```go
type TokenStore interface {
    StoreTokens(ctx context.Context, credentialID string, accessToken, refreshToken, idToken string, expiresAt time.Time) error
    GetTokens(ctx context.Context, credentialID string) (accessToken, refreshToken string, expiresAt time.Time, err error)
    DeleteTokens(ctx context.Context, credentialID string) error
    NeedsRefresh(ctx context.Context, credentialID string) (bool, error)
}
```

Abstraction for encrypted server-side token storage. Consuming applications implement this against their storage layer.

#### `AuthorizationChecker`

```go
type AuthorizationChecker interface {
    IsAuthorized(ctx context.Context, agentID, action, target string) (bool, error)
    IsAuthorizedInAccount(ctx context.Context, agentID, accountID, action, target string) (bool, error)
    GetPermissions(ctx context.Context, agentID string) ([]Permission, error)
    GetProhibitions(ctx context.Context, agentID string) ([]Permission, error)
}
```

#### `PermissionStore`

```go
type PermissionStore interface {
    GetPermissionsForAssignee(ctx context.Context, assigneeID string) ([]Permission, error)
    GetProhibitionsForAssignee(ctx context.Context, assigneeID string) ([]Permission, error)
    GetRolesForAgent(ctx context.Context, agentID string) ([]string, error)
    GetRolesForAgentInAccount(ctx context.Context, agentID, accountID string) ([]string, error)
}
```

Read model for authorization decisions. Consuming applications implement this against their storage layer.

### Implementations

#### `DefaultAuthenticationService`

```go
func NewDefaultAuthenticationService(
    providers OAuthProviderRegistry,
    agents repositories.AgentRepository,
    credentials repositories.CredentialRepository,
    sessions repositories.AuthSessionRepository,
    tokens TokenStore,
    authorization AuthorizationChecker,
) *DefaultAuthenticationService
```

Reference implementation of `AuthenticationService`. The `authorization` parameter is optional (pass `nil` to skip permission resolution in `ValidateSession`).

#### `PolicyDecisionPoint`

```go
func NewPolicyDecisionPoint(store PermissionStore) *PolicyDecisionPoint
```

Implements `AuthorizationChecker` using ODRL semantics: prohibitions override permissions, default deny.

### PKCE Functions

```go
func GenerateCodeVerifier() (string, error)
```

Generates a 43-character base64url-encoded code verifier from 32 random bytes (RFC 7636).

```go
func GenerateCodeChallenge(verifier string) string
```

Generates a S256 code challenge (SHA-256 + base64url) from the code verifier.

```go
func GenerateState() (string, error)
```

Generates a 32-byte base64url-encoded state parameter for CSRF protection.

```go
func GenerateNonce() (string, error)
```

Generates a 32-byte base64url-encoded nonce for OpenID Connect ID token validation.

### Sentinel Errors

```go
var ErrInvalidProvider    = errors.New("authentication: invalid provider")
var ErrInvalidState       = errors.New("authentication: invalid state parameter")
var ErrCodeExchangeFailed = errors.New("authentication: code exchange failed")
var ErrSessionNotFound    = errors.New("authentication: session not found")
var ErrSessionExpired     = errors.New("authentication: session expired")
var ErrSessionRevoked     = errors.New("authentication: session revoked")
var ErrTokenRefreshFailed = errors.New("authentication: token refresh failed")
var ErrCredentialNotFound = errors.New("authentication: credential not found")
```

---

## Package `auth/infrastructure/session`

`import "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"`

HTTP session management for the BFF (Backend-for-Frontend) pattern.

### Types

#### `SessionData`

```go
type SessionData struct {
    SessionID string
    AgentID   string
    AccountID string     // Empty if not scoped
    CreatedAt time.Time
    ExpiresAt time.Time
}
```

#### `FlowData`

```go
type FlowData struct {
    State        string
    CodeVerifier string
    Nonce        string
    Provider     string
    RedirectURI  string
    CreatedAt    time.Time
}
```

Temporary OAuth flow data stored server-side during the authorization code exchange. Automatically cleared after retrieval (one-time use). Maximum TTL: 10 minutes.

#### `SessionOptions`

```go
type SessionOptions struct {
    MaxAge   int            // seconds (default: 86400)
    Domain   string
    Path     string         // default: "/"
    HttpOnly bool           // default: true
    Secure   bool           // default: true
    SameSite http.SameSite  // default: Lax
}
```

### Interface

#### `SessionManager`

```go
type SessionManager interface {
    CreateHTTPSession(w http.ResponseWriter, r *http.Request, sessionInfo SessionData) error
    GetHTTPSession(r *http.Request) (*SessionData, error)
    DestroyHTTPSession(w http.ResponseWriter, r *http.Request) error
    SetFlowData(w http.ResponseWriter, r *http.Request, data FlowData) error
    GetFlowData(w http.ResponseWriter, r *http.Request) (*FlowData, error)
}
```

### Functions

#### `DefaultSessionOptions`

```go
func DefaultSessionOptions() SessionOptions
```

Returns secure defaults: `HttpOnly=true`, `Secure=true`, `SameSite=Lax`, `MaxAge=86400` (24 hours), `Path="/"`.

#### `NewGorillaSessionManager`

```go
func NewGorillaSessionManager(sessionName string, store sessions.Store, options SessionOptions) *GorillaSessionManager
```

Creates a session manager backed by `gorilla/sessions`. The `store` parameter accepts any gorilla session store (cookie, filesystem, Redis, database).

### Sentinel Errors

```go
var ErrSessionNotFound  = errors.New("session: not found")
var ErrFlowDataNotFound = errors.New("session: flow data not found")
```

---

## Package `auth/infrastructure/casbin`

`import "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/casbin"`

Casbin-backed implementation of `AuthorizationChecker` with RBAC, domain support, and deny-override policy effects.

### Types

#### `CasbinAuthorizationChecker`

```go
type CasbinAuthorizationChecker struct { /* unexported fields */ }
```

Implements `application.AuthorizationChecker` using Casbin's enforcement engine. Uses an embedded RBAC model that maps ODRL semantics to Casbin policies with domain-based scoping for multi-tenancy.

### Constructors

#### `NewCasbinAuthorizationChecker`

```go
func NewCasbinAuthorizationChecker(adapter persist.Adapter) (*CasbinAuthorizationChecker, error)
```

Creates a checker with the embedded ODRL-compatible Casbin model and the given adapter for policy persistence. The adapter parameter accepts any `github.com/casbin/casbin/v3/persist.Adapter` implementation (GORM, file, PostgreSQL, etc.). Pass `nil` for in-memory only (no persistence).

Internally, the constructor:
1. Loads the embedded RBAC model with deny-override policy effects
2. Creates a Casbin enforcer with the model and adapter
3. Registers a domain matching function so that `"*"` policies match any request domain

#### `NewCasbinAuthorizationCheckerFromEnforcer`

```go
func NewCasbinAuthorizationCheckerFromEnforcer(enforcer *casbin.Enforcer) *CasbinAuthorizationChecker
```

Wraps a pre-configured Casbin enforcer. The caller is responsible for configuring the model, adapter, and domain matching function. Use this when you need a custom Casbin model or non-standard configuration.

### AuthorizationChecker Methods

These methods implement the `application.AuthorizationChecker` interface.

#### `IsAuthorized`

```go
func (c *CasbinAuthorizationChecker) IsAuthorized(ctx context.Context, agentID, action, target string) (bool, error)
```

Checks whether the agent is authorized using only global roles and policies. Calls `Enforce(agentID, "*", action, target)` internally.

#### `IsAuthorizedInAccount`

```go
func (c *CasbinAuthorizationChecker) IsAuthorizedInAccount(ctx context.Context, agentID, accountID, action, target string) (bool, error)
```

Checks whether the agent is authorized within an account context. Both global and account-scoped roles are considered via the domain matching function. Calls `Enforce(agentID, accountID, action, target)` internally.

#### `GetPermissions`

```go
func (c *CasbinAuthorizationChecker) GetPermissions(ctx context.Context, agentID string) ([]application.Permission, error)
```

Returns all effective permissions (`eft=allow`) for the agent, including permissions inherited through global role assignments.

#### `GetProhibitions`

```go
func (c *CasbinAuthorizationChecker) GetProhibitions(ctx context.Context, agentID string) ([]application.Permission, error)
```

Returns all effective prohibitions (`eft=deny`) for the agent, including prohibitions inherited through global role assignments.

### Convenience Methods

These methods manage Casbin policies and grouping policies directly.

#### `AddPermission`

```go
func (c *CasbinAuthorizationChecker) AddPermission(assignee, action, target string) error
```

Adds a global permission. Creates a Casbin policy: `(assignee, "*", action, target, "allow")`.

#### `AddProhibition`

```go
func (c *CasbinAuthorizationChecker) AddProhibition(assignee, action, target string) error
```

Adds a global prohibition. Creates a Casbin policy: `(assignee, "*", action, target, "deny")`.

#### `RemovePermission`

```go
func (c *CasbinAuthorizationChecker) RemovePermission(assignee, action, target string) error
```

Removes a global permission policy.

#### `RemoveProhibition`

```go
func (c *CasbinAuthorizationChecker) RemoveProhibition(assignee, action, target string) error
```

Removes a global prohibition policy.

#### `AssignRole`

```go
func (c *CasbinAuthorizationChecker) AssignRole(agentID, roleID string) error
```

Assigns a global role to an agent. Creates a Casbin grouping policy: `(agentID, roleID, "*")`. Global roles apply in all domain contexts.

#### `AssignAccountRole`

```go
func (c *CasbinAuthorizationChecker) AssignAccountRole(agentID, roleID, accountID string) error
```

Assigns a role to an agent within a specific account. Creates a Casbin grouping policy: `(agentID, roleID, accountID)`. Account-scoped roles only apply when checking authorization in that account.

#### `RevokeRole`

```go
func (c *CasbinAuthorizationChecker) RevokeRole(agentID, roleID string) error
```

Removes a global role assignment.

#### `RevokeAccountRole`

```go
func (c *CasbinAuthorizationChecker) RevokeAccountRole(agentID, roleID, accountID string) error
```

Removes an account-scoped role assignment.

### Embedded Casbin Model

The checker uses the following embedded model:

```ini
[request_definition]
r = sub, dom, act, obj

[policy_definition]
p = sub, dom, act, obj, eft

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = g(r.sub, p.sub, r.dom) && r.act == p.act && (r.obj == p.obj || p.obj == "*") && (r.dom == p.dom || p.dom == "*")
```
