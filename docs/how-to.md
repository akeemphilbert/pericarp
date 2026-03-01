---
layout: default
title: How-To Guides
nav_order: 3
---

# How-To Guides

Practical recipes for common tasks with Pericarp.

## Event Sourcing

### How to subscribe to events using pattern matching

The EventDispatcher supports dot-separated pattern matching. Given an event type like `user.created`, handlers registered for any of these patterns will fire:

| Pattern | Matches |
|---------|---------|
| `user.created` | Exact match |
| `user.*` | All user events |
| `*.created` | All creation events across entities |
| `*.*` | Every event |

```go
dispatcher := domain.NewEventDispatcher()

// All order events (order.created, order.shipped, order.cancelled, ...)
domain.Subscribe(dispatcher, "order.*", func(ctx context.Context, env domain.EventEnvelope[any]) error {
    log.Printf("order event: %s for aggregate %s", env.EventType, env.AggregateID)
    return nil
})

// All creation events across the system
domain.Subscribe(dispatcher, "*.created", func(ctx context.Context, env domain.EventEnvelope[any]) error {
    log.Printf("something was created: %s", env.AggregateID)
    return nil
})
```

Use `SubscribeWildcard` for a true catch-all that doesn't rely on dot structure:

```go
dispatcher.SubscribeWildcard(func(ctx context.Context, env domain.EventEnvelope[any]) error {
    log.Printf("[audit] event=%s aggregate=%s", env.EventType, env.AggregateID)
    return nil
})
```

### How to construct event type strings

Use `EventTypeFor` with the standard constants to build consistent type strings:

```go
domain.EventTypeFor("user", domain.EventTypeCreate)  // "user.created"
domain.EventTypeFor("order", domain.EventTypeUpdate)  // "order.updated"
domain.EventTypeFor("product", domain.EventTypeDelete) // "product.deleted"
domain.EventTypeFor("graph", domain.EventTypeTriple)   // "graph.triple"

// Custom actions
domain.EventTypeFor("user", "deactivated")  // "user.deactivated"
domain.EventTypeFor("order", "shipped")     // "order.shipped"
```

### How to persist events for multiple aggregates atomically

Track all aggregates in a single `SimpleUnitOfWork`:

```go
uow := application.NewSimpleUnitOfWork(store, dispatcher)

uow.Track(user, order, invoice)

// All three aggregates' events are persisted.
// If any one fails (e.g. concurrency conflict), the commit fails.
err := uow.Commit(ctx)
```

### How to handle concurrency conflicts

The EventStore uses optimistic concurrency control via `expectedVersion`. When a conflict is detected, reload and retry:

```go
func updateUser(ctx context.Context, store domain.EventStore, id string) error {
    for retries := 0; retries < 3; retries++ {
        // Load current state
        user, err := LoadUser(ctx, store, id)
        if err != nil {
            return err
        }

        // Make changes
        user.ChangeEmail("new@example.com")

        // Attempt to persist
        events := user.GetUncommittedEvents()
        err = store.Append(ctx, id, user.GetSequenceNo(), events...)
        if errors.Is(err, domain.ErrConcurrencyConflict) {
            continue // Retry with fresh state
        }
        return err
    }
    return fmt.Errorf("failed after 3 retries")
}
```

### How to retrieve events for a specific version range

```go
// All events
events, _ := store.GetEvents(ctx, "user-1")

// Events from version 5 onwards
events, _ := store.GetEventsFromVersion(ctx, "user-1", 5)

// Events between versions 3 and 7 (inclusive)
events, _ := store.GetEventsRange(ctx, "user-1", 3, 7)

// From the start to version 5
events, _ := store.GetEventsRange(ctx, "user-1", -1, 5)

// From version 3 to the end
events, _ := store.GetEventsRange(ctx, "user-1", 3, -1)
```

### How to use the FileStore for local development

```go
store, err := infrastructure.NewFileStore("/tmp/my-events")
if err != nil {
    log.Fatal(err)
}
defer store.Close()

// Use exactly like MemoryStore — same EventStore interface.
// Events are persisted as JSON files, one per aggregate.
```

The FileStore creates the directory if it doesn't exist and loads existing events from disk on startup.

### How to marshal and unmarshal events as JSON

```go
// Marshal a typed envelope to JSON
envelope := domain.NewEventEnvelope(myPayload, "agg-1", "user.created", 0)
data, err := domain.MarshalEventToJSON(envelope)

// Unmarshal back to a typed envelope
restored, err := domain.UnmarshalEventFromJSON[*MyPayloadType](data)
// restored.Payload is now *MyPayloadType — fully type-safe
```

### How to add metadata to events

Attach arbitrary key-value metadata to any event envelope:

```go
envelope := domain.NewEventEnvelope(payload, aggregateID, eventType, seqNo)
envelope.Metadata["correlation_id"] = "req-abc-123"
envelope.Metadata["user_agent"] = "web-client/1.0"
envelope.Metadata["source"] = "api"
```

Metadata is preserved through serialization and dispatch.

---

## Commands (CQRS)

### How to choose between async and queued dispatchers

| Dispatcher | Execution | Best for |
|------------|-----------|----------|
| `AsyncCommandDispatcher` | All receivers run concurrently | Fan-out commands, independent side effects |
| `QueuedCommandDispatcher` | Receivers run sequentially in registration order | Ordered processing, deterministic results |

```go
// Concurrent execution
async := cqrs.NewAsyncCommandDispatcher()

// Sequential execution
queued := cqrs.NewQueuedCommandDispatcher()
```

Both implement the `CommandDispatcher` interface, so consuming code doesn't need to change.

### How to return early from a command dispatch

Use `First()` to get the first result and return an HTTP response while remaining receivers continue in the background:

```go
func handleCreateUser(w http.ResponseWriter, r *http.Request) {
    envelope := cqrs.NewCommandEnvelope(cmd, "user.create")
    watchable := dispatcher.Dispatch(r.Context(), cqrs.ToAnyCommandEnvelope(envelope))

    result, ok := watchable.First()
    if !ok {
        http.Error(w, "no handler", 500)
        return
    }
    if result.Error != nil {
        http.Error(w, result.Error.Error(), 400)
        return
    }

    json.NewEncoder(w).Encode(result.Value)
    // Remaining receivers finish in the background — their results are buffered.
}
```

### How to register a wildcard command receiver

```go
// Audit logging for all commands
dispatcher.RegisterWildcardReceiver(func(ctx context.Context, env cqrs.CommandEnvelope[any]) (any, error) {
    log.Printf("[audit] command=%s id=%s", env.CommandType, env.ID)
    return nil, nil
})
```

Wildcard receivers fire for every command in addition to any pattern-matched receivers.

---

## Authentication

### How to set up the session manager with gorilla/sessions

```go
import (
    "github.com/gorilla/sessions"
    authsession "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/session"
)

// Use a cookie store with a 32-byte key for encryption
store := sessions.NewCookieStore([]byte("your-32-byte-secret-key-here!!!"))

// Secure defaults: HttpOnly, Secure, SameSite=Lax
opts := authsession.DefaultSessionOptions()

sessionManager := authsession.NewGorillaSessionManager("myapp-session", store, opts)
```

For production, use a server-side store (Redis, database) instead of `CookieStore` so that session data isn't stored in the cookie itself.

### How to implement an OAuthProvider

The `OAuthProvider` interface is provider-agnostic. Implement it once per identity provider:

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

Register providers in the registry:

```go
providers := authapp.OAuthProviderRegistry{
    "google": &GoogleProvider{clientID: "...", clientSecret: "..."},
    "github": &GitHubProvider{clientID: "...", clientSecret: "..."},
}
```

### How to check authorization for an agent

Use the `PolicyDecisionPoint` to evaluate ODRL-based permissions:

```go
import authapp "github.com/akeemphilbert/pericarp/pkg/auth/application"

pdp := authapp.NewPolicyDecisionPoint(permissionStore)

// Global check
allowed, err := pdp.IsAuthorized(ctx, "agent-123", "odrl:read", "document-456")

// Account-scoped check (considers both global and account roles)
allowed, err := pdp.IsAuthorizedInAccount(ctx, "agent-123", "account-789", "odrl:modify", "document-456")
```

The evaluation order is: prohibitions first (deny overrides), then permissions, then default deny.

### How to check authorization within a session

The `ValidateSession` method returns permissions alongside session info:

```go
info, err := authService.ValidateSession(ctx, sessionID)
if err != nil {
    // Handle ErrSessionNotFound, ErrSessionExpired, ErrSessionRevoked
}

// info.Permissions contains the agent's effective permissions
// info.AgentID, info.AccountID identify the authenticated context
```

### How to revoke sessions

```go
// Revoke a single session (e.g., user clicks "log out")
err := authService.RevokeSession(ctx, sessionID)

// Revoke all sessions for an agent (e.g., password change, security incident)
err := authService.RevokeAllSessions(ctx, agentID)
```

### How to refresh OAuth tokens

Token refresh happens server-side. The user's session continues seamlessly:

```go
// Check if tokens need refreshing
needsRefresh, err := tokenStore.NeedsRefresh(ctx, credentialID)
if needsRefresh {
    result, err := authService.RefreshTokens(ctx, credentialID)
    // New tokens are stored automatically
}
```

### How to scope a session to an account

For multi-tenant applications, scope a session to a specific account after login:

```go
session, _ := authService.CreateSession(ctx, agentID, credentialID, ip, ua, 24*time.Hour)
session.ScopeToAccount("account-789")
sessionRepo.Save(ctx, session)
```

### How to define roles and policies

```go
import "github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"

// Create a role
role, _ := new(entities.Role).With("role-editor", "Editor", "Can read and modify documents")

// Create an ODRL policy with permissions
policy, _ := new(entities.Policy).With("policy-1", "Editor Policy", entities.PolicyTypeSet)
policy.GrantPermission("role-editor", "odrl:read", "documents")
policy.GrantPermission("role-editor", "odrl:modify", "documents")
policy.SetProhibition("role-editor", "odrl:delete", "documents")

// Assign a role to an agent
agent, _ := new(entities.Agent).With("agent-1", "Alice", entities.AgentTypePerson)
agent.AssignRole("role-editor")

// Add an agent to an account with a role
account, _ := new(entities.Account).With("account-1", "Acme Corp")
account.AddMember("agent-1", "role-editor")
```

### How to link multiple identity providers to one agent

An agent can have multiple credentials from different providers:

```go
// First login creates agent + credential
agent, googleCred, _ := authService.FindOrCreateAgent(ctx, authapp.UserInfo{
    Provider:       "google",
    ProviderUserID: "google-123",
    Email:          "alice@example.com",
    DisplayName:    "Alice",
})

// Later, link a GitHub credential to the same agent
githubCred, _ := new(entities.Credential).With(
    ksuid.New().String(), agent.GetID(),
    "github", "github-456", "alice@example.com", "alice",
)
credentialRepo.Save(ctx, githubCred)
```

---

## Commands (CQRS)

### How to use the Watchable in a select statement

```go
watchable := dispatcher.Dispatch(ctx, envelope)

select {
case <-watchable.Done():
    results := collectRemaining(watchable)
    // All receivers finished
case <-time.After(5 * time.Second):
    // Timeout — receivers may still be running in the background
    log.Println("command timed out")
case <-ctx.Done():
    // Context cancelled
}
```

### How to fire-and-forget a command

For scenarios where receivers must outlive the HTTP request, dispatch with a background context:

```go
// Use background context so receivers aren't cancelled when the request ends
bgCtx := context.Background()
watchable := dispatcher.Dispatch(bgCtx, envelope)
// Don't call Wait() or First() — let receivers run entirely in the background
_ = watchable
```

### How to register multiple typed receivers for the same command

```go
// Validation receiver
cqrs.RegisterReceiver(dispatcher, "order.place", func(
    ctx context.Context, env cqrs.CommandEnvelope[PlaceOrderCommand],
) (any, error) {
    if env.Payload.Total <= 0 {
        return nil, fmt.Errorf("invalid total")
    }
    return "validated", nil
})

// Persistence receiver
cqrs.RegisterReceiver(dispatcher, "order.place", func(
    ctx context.Context, env cqrs.CommandEnvelope[PlaceOrderCommand],
) (any, error) {
    order := createOrder(env.Payload)
    return order.ID, nil
})

// Both receivers fire when "order.place" is dispatched.
// With AsyncCommandDispatcher they run concurrently.
// With QueuedCommandDispatcher they run in registration order.
```
