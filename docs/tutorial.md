# Tutorial: Building an Event-Sourced Aggregate with Pericarp

This tutorial walks you through building a complete event-sourced User aggregate from scratch. By the end, you will have a working aggregate that records events, persists them to a store, dispatches them to handlers, and processes commands through a command dispatcher.

## Prerequisites

- Go 1.24+
- `go get github.com/akeemphilbert/pericarp`

## 1. Define Your Domain Events

Start by defining the event payloads your aggregate will produce. These are plain Go structs.

```go
package user

import "time"

type UserCreatedPayload struct {
    UserID string `json:"user_id"`
    Email  string `json:"email"`
    Name   string `json:"name"`
}

type EmailChangedPayload struct {
    UserID   string `json:"user_id"`
    OldEmail string `json:"old_email"`
    NewEmail string `json:"new_email"`
}

type UserDeactivatedPayload struct {
    UserID     string    `json:"user_id"`
    Reason     string    `json:"reason"`
    OccurredAt time.Time `json:"occurred_at"`
}
```

Use `domain.EventTypeFor` to build consistent event type strings:

```go
import "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"

var (
    UserCreated     = domain.EventTypeFor("user", domain.EventTypeCreate)  // "user.created"
    UserUpdated     = domain.EventTypeFor("user", domain.EventTypeUpdate)  // "user.updated"
    UserDeactivated = domain.EventTypeFor("user", "deactivated")           // "user.deactivated"
)
```

## 2. Build the Aggregate

Embed `ddd.BaseEntity` to gain event sourcing capabilities. Your aggregate holds its current state in regular fields and mutates them by recording events.

```go
package user

import (
    "fmt"

    "github.com/akeemphilbert/pericarp/pkg/ddd"
)

type User struct {
    *ddd.BaseEntity
    email  string
    name   string
    active bool
}

// NewUser creates a brand-new User aggregate and records the creation event.
func NewUser(id, email, name string) (*User, error) {
    u := &User{
        BaseEntity: ddd.NewBaseEntity(id),
    }

    // Record the creation event — this adds it to the uncommitted events list
    // and increments the sequence number.
    err := u.RecordEvent(
        UserCreatedPayload{UserID: id, Email: email, Name: name},
        UserCreated,
    )
    if err != nil {
        return nil, fmt.Errorf("failed to record creation event: %w", err)
    }

    // Apply state locally
    u.email = email
    u.name = name
    u.active = true

    return u, nil
}

// ChangeEmail records an email change event.
func (u *User) ChangeEmail(newEmail string) error {
    if !u.active {
        return fmt.Errorf("cannot change email on deactivated user")
    }

    err := u.RecordEvent(
        EmailChangedPayload{UserID: u.GetID(), OldEmail: u.email, NewEmail: newEmail},
        UserUpdated,
    )
    if err != nil {
        return err
    }

    u.email = newEmail
    return nil
}
```

At this point, `u.GetUncommittedEvents()` returns the events that haven't been persisted yet.

## 3. Persist Events with the EventStore

Pericarp ships with two EventStore implementations. Use `MemoryStore` for testing and `FileStore` for local development.

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

func main() {
    store := infrastructure.NewMemoryStore()
    defer store.Close()

    ctx := context.Background()

    // Create the aggregate (from step 2)
    user, _ := NewUser("user-1", "alice@example.com", "Alice")

    // Persist uncommitted events manually
    events := user.GetUncommittedEvents()
    err := store.Append(ctx, user.GetID(), -1, events...)
    //                                      ^^ -1 means "no version check" (new aggregate)
    if err != nil {
        log.Fatal(err)
    }
    user.ClearUncommittedEvents()

    // Later, retrieve the events
    stored, _ := store.GetEvents(ctx, "user-1")
    fmt.Printf("Stored %d event(s) for user-1\n", len(stored))
}
```

### Optimistic Concurrency

Pass the aggregate's current sequence number as `expectedVersion` to detect concurrent writes:

```go
// This will fail if another process appended events since we last read.
currentVersion := user.GetSequenceNo()
err := store.Append(ctx, user.GetID(), currentVersion, newEvents...)
if errors.Is(err, domain.ErrConcurrencyConflict) {
    // Reload the aggregate and retry
}
```

## 4. Use the Unit of Work

The `SimpleUnitOfWork` handles persistence and dispatch for you, including optimistic concurrency control.

```go
import (
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing/application"
    "github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

func handleCommand(ctx context.Context) error {
    store := infrastructure.NewMemoryStore()

    // UnitOfWork without a dispatcher (nil is fine)
    uow := application.NewSimpleUnitOfWork(store, nil)

    user, _ := NewUser("user-1", "alice@example.com", "Alice")
    user.ChangeEmail("newalice@example.com")

    // Track registers the entity and captures its expected version
    if err := uow.Track(user); err != nil {
        return err
    }

    // Commit persists all uncommitted events atomically
    return uow.Commit(ctx)
}
```

You can track multiple aggregates in a single unit of work:

```go
uow.Track(user1, user2, order)
uow.Commit(ctx)  // persists all three atomically
```

## 5. Subscribe to Events with the EventDispatcher

The EventDispatcher lets you react to events after they are persisted. Handlers are type-safe via generics.

```go
import "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"

dispatcher := domain.NewEventDispatcher()

// Subscribe to a specific event type with a typed handler
domain.Subscribe(dispatcher, "user.created", func(
    ctx context.Context,
    env domain.EventEnvelope[UserCreatedPayload],
) error {
    fmt.Printf("User created: %s (%s)\n", env.Payload.Name, env.Payload.Email)
    return nil
})

// Subscribe with a wildcard — receives all user.* events
domain.Subscribe(dispatcher, "user.*", func(
    ctx context.Context,
    env domain.EventEnvelope[any],
) error {
    fmt.Printf("User event: %s\n", env.EventType)
    return nil
})
```

Wire the dispatcher into the UnitOfWork so events are dispatched automatically after commit:

```go
uow := application.NewSimpleUnitOfWork(store, dispatcher)
// After uow.Commit(), the dispatcher fires for each persisted event
```

## 6. Dispatch Commands with the CommandDispatcher

The `cqrs` package provides a command dispatcher for the write side. Commands are routed to receivers and return results through a `Watchable`.

```go
import "github.com/akeemphilbert/pericarp/pkg/cqrs"

// Define a command payload
type CreateUserCommand struct {
    Email string
    Name  string
}

// Create a dispatcher — choose async (concurrent) or queued (sequential)
dispatcher := cqrs.NewAsyncCommandDispatcher()

// Register a typed receiver
cqrs.RegisterReceiver(dispatcher, "user.create", func(
    ctx context.Context,
    env cqrs.CommandEnvelope[CreateUserCommand],
) (any, error) {
    user, err := NewUser("user-1", env.Payload.Email, env.Payload.Name)
    if err != nil {
        return nil, err
    }
    return user.GetID(), nil
})

// Dispatch a command
envelope := cqrs.NewCommandEnvelope(
    CreateUserCommand{Email: "alice@example.com", Name: "Alice"},
    "user.create",
)
watchable := dispatcher.Dispatch(ctx, cqrs.ToAnyCommandEnvelope(envelope))

// Option A: Wait for all receivers
results := watchable.Wait()

// Option B: Get just the first result (e.g. for a REST controller)
result, ok := watchable.First()

// Option C: Stream results as they arrive
for result := range watchable.Results() {
    fmt.Printf("Got result: %v (err: %v)\n", result.Value, result.Error)
}
```

## 7. Replay Events to Rebuild State

To reconstitute an aggregate from stored events, create an empty entity and apply each event:

```go
func LoadUser(ctx context.Context, store domain.EventStore, id string) (*User, error) {
    events, err := store.GetEvents(ctx, id)
    if err != nil {
        return nil, err
    }

    u := &User{
        BaseEntity: ddd.NewBaseEntity(id),
    }

    for _, event := range events {
        if err := u.ApplyEvent(ctx, event); err != nil {
            return nil, err
        }

        // Apply state based on event type
        switch event.EventType {
        case UserCreated:
            if p, ok := event.Payload.(UserCreatedPayload); ok {
                u.email = p.Email
                u.name = p.Name
                u.active = true
            }
        case UserUpdated:
            if p, ok := event.Payload.(EmailChangedPayload); ok {
                u.email = p.NewEmail
            }
        }
    }

    return u, nil
}
```

## Next Steps

- Read the [How-To Guides](how-to.md) for specific recipes (pattern matching, file store setup, error handling)
- Read the [Explanation](explanation.md) to understand the design decisions behind Pericarp
- Browse the [Reference](reference.md) for complete API documentation
