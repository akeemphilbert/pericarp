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
