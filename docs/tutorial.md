---
layout: default
title: Tutorial
nav_order: 2
---

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

## 8. Add Authentication with OAuth 2.0 / OIDC

Pericarp's auth package provides a complete OAuth 2.0 Authorization Code Flow with PKCE. This section walks through integrating it into a backend service using the Backend-for-Frontend (BFF) pattern.

### 8.1 Register an OAuth Provider

Implement the `OAuthProvider` interface for your identity provider. The library is provider-agnostic — you supply the provider, it handles the flow.

```go
package auth

import (
    authapp "github.com/akeemphilbert/pericarp/pkg/auth/application"
)

// GoogleProvider implements authapp.OAuthProvider for Google.
type GoogleProvider struct {
    clientID     string
    clientSecret string
    tokenURL     string
    authURL      string
}

func (g *GoogleProvider) Name() string { return "google" }

func (g *GoogleProvider) AuthCodeURL(state, codeChallenge, nonce, redirectURI string) string {
    // Build the Google authorization URL with PKCE and nonce parameters
    return fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code"+
        "&scope=openid+email+profile&state=%s&code_challenge=%s"+
        "&code_challenge_method=S256&nonce=%s",
        g.authURL, g.clientID, redirectURI, state, codeChallenge, nonce)
}

// Exchange, RefreshToken, RevokeToken, ValidateIDToken — implement per provider docs
```

### 8.2 Create the Authentication Service

Wire the provider registry and repositories into the `DefaultAuthenticationService`:

```go
import (
    authapp "github.com/akeemphilbert/pericarp/pkg/auth/application"
)

providers := authapp.OAuthProviderRegistry{
    "google": &GoogleProvider{...},
    "github": &GitHubProvider{...},
}

authService := authapp.NewDefaultAuthenticationService(
    providers,
    agentRepo,       // repositories.AgentRepository
    credentialRepo,  // repositories.CredentialRepository
    sessionRepo,     // repositories.AuthSessionRepository
    tokenStore,      // authapp.TokenStore
    authzChecker,    // authapp.AuthorizationChecker (or nil)
)
```

### 8.3 Implement the Login Handler

The login handler initiates the OAuth flow, stores PKCE data server-side, and redirects:

```go
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
    provider := r.URL.Query().Get("provider")

    // 1. Generate PKCE parameters and authorization URL
    authReq, err := h.authService.InitiateAuthFlow(r.Context(), provider, h.callbackURL)
    if err != nil {
        http.Error(w, "Failed to initiate auth flow", http.StatusBadRequest)
        return
    }

    // 2. Store flow data server-side (state, code_verifier, nonce)
    h.sessionManager.SetFlowData(w, r, session.FlowData{
        State:        authReq.State,
        CodeVerifier: authReq.CodeVerifier,
        Nonce:        authReq.Nonce,
        Provider:     authReq.Provider,
        RedirectURI:  h.callbackURL,
        CreatedAt:    time.Now(),
    })

    // 3. Redirect to identity provider
    http.Redirect(w, r, authReq.AuthURL, http.StatusFound)
}
```

### 8.4 Handle the Callback

The callback handler validates the state, exchanges the code, and creates a session:

```go
func (h *AuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // 1. Retrieve and clear flow data (one-time use)
    flowData, err := h.sessionManager.GetFlowData(w, r)
    if err != nil {
        http.Error(w, "Invalid or expired flow", http.StatusBadRequest)
        return
    }

    // 2. Validate state parameter (constant-time comparison)
    if err := h.authService.ValidateState(ctx, r.URL.Query().Get("state"), flowData.State); err != nil {
        http.Error(w, "Invalid state", http.StatusForbidden)
        return
    }

    // 3. Exchange authorization code for tokens (server-to-server)
    result, err := h.authService.ExchangeCode(ctx, r.URL.Query().Get("code"),
        flowData.CodeVerifier, flowData.Provider, flowData.RedirectURI)
    if err != nil {
        http.Error(w, "Code exchange failed", http.StatusInternalServerError)
        return
    }

    // 4. Find or create the agent and credential
    agent, credential, err := h.authService.FindOrCreateAgent(ctx, result.UserInfo)
    if err != nil {
        http.Error(w, "Failed to resolve user", http.StatusInternalServerError)
        return
    }

    // 5. Create authenticated session
    authSession, err := h.authService.CreateSession(ctx, agent.GetID(),
        credential.GetID(), r.RemoteAddr, r.UserAgent(), 24*time.Hour)
    if err != nil {
        http.Error(w, "Failed to create session", http.StatusInternalServerError)
        return
    }

    // 6. Set HTTP session cookie (opaque session ID only)
    h.sessionManager.CreateHTTPSession(w, r, session.SessionData{
        SessionID: authSession.GetID(),
        AgentID:   agent.GetID(),
        ExpiresAt: authSession.ExpiresAt(),
        CreatedAt: authSession.CreatedAt(),
    })

    http.Redirect(w, r, "/dashboard", http.StatusFound)
}
```

### 8.5 Protect API Routes

Validate the session on every authenticated request:

```go
func (h *AuthHandler) RequireAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        sessionData, err := h.sessionManager.GetHTTPSession(r)
        if err != nil {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }

        info, err := h.authService.ValidateSession(r.Context(), sessionData.SessionID)
        if err != nil {
            http.Error(w, "Session invalid", http.StatusUnauthorized)
            return
        }

        // Add session info to context for downstream handlers
        ctx := context.WithValue(r.Context(), "session", info)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## 9. Add Authorization with Casbin

Now that authentication is in place, let's add authorization so we can control what authenticated agents are allowed to do. Pericarp offers two implementations of the `AuthorizationChecker` interface:

- **`PolicyDecisionPoint`** — bring-your-own `PermissionStore`. You implement the read model; the PDP evaluates it using ODRL semantics.
- **`CasbinAuthorizationChecker`** — batteries-included. Uses the [Casbin](https://casbin.org/) enforcement engine with an embedded RBAC model that maps ODRL semantics directly to Casbin policies.

This tutorial uses the Casbin path because it requires no additional infrastructure.

### 9.1 Set Up the Casbin Authorization Checker

Create a checker by passing a Casbin adapter for policy persistence. For learning, we'll use a nil adapter (in-memory only). In production, use a GORM or database adapter.

```go
import (
    casbinauth "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/casbin"
)

// In-memory only (no persistence) — fine for learning
checker, err := casbinauth.NewCasbinAuthorizationChecker(nil)
if err != nil {
    log.Fatal(err)
}
```

Under the hood, the constructor loads an embedded RBAC model with deny-override policy effects, configures domain matching for global vs account-scoped roles, and creates a Casbin enforcer.

### 9.2 Define Roles and Assign Permissions

Use convenience methods to build up a role with permissions and a prohibition:

```go
// Grant the "editor" role permission to read and modify documents
checker.AddPermission("editor", "odrl:read", "documents")
checker.AddPermission("editor", "odrl:modify", "documents")

// Prohibit the "editor" role from deleting documents
checker.AddProhibition("editor", "odrl:delete", "documents")

// Assign the "editor" role to our agent
checker.AssignRole("agent-1", "editor")
```

### 9.3 Check Authorization

Call `IsAuthorized` to evaluate whether the agent can perform an action:

```go
ctx := context.Background()

// Agent can read (via the editor role)
allowed, _ := checker.IsAuthorized(ctx, "agent-1", "odrl:read", "documents")
fmt.Println("read:", allowed) // true

// Agent cannot delete (prohibition overrides any permission)
allowed, _ = checker.IsAuthorized(ctx, "agent-1", "odrl:delete", "documents")
fmt.Println("delete:", allowed) // false

// Agent cannot transfer (no permission granted — default deny)
allowed, _ = checker.IsAuthorized(ctx, "agent-1", "odrl:transfer", "documents")
fmt.Println("transfer:", allowed) // false
```

The evaluation order follows ODRL semantics: prohibitions override permissions, and ungranted actions are denied by default.

### 9.4 Add Account-Scoped Roles

For multi-tenant applications, assign roles within a specific account:

```go
// Give agent-1 the "admin" role only within account-42
checker.AssignAccountRole("agent-1", "admin", "account-42")
checker.AddPermission("admin", "odrl:delete", "documents")

// Check authorization within account-42 — considers both global and account roles
allowed, _ = checker.IsAuthorizedInAccount(ctx, "agent-1", "account-42", "odrl:delete", "documents")
fmt.Println("delete in account-42:", allowed) // true (admin can delete, editor prohibition is on editor role)

// Global check still uses only global roles
allowed, _ = checker.IsAuthorized(ctx, "agent-1", "odrl:delete", "documents")
fmt.Println("delete globally:", allowed) // false (editor prohibition)
```

### 9.5 Wire Authorization into the Auth Service

Inject the checker into `DefaultAuthenticationService` so that `ValidateSession` returns the agent's effective permissions:

```go
authService := authapp.NewDefaultAuthenticationService(
    providers,
    agentRepo,
    credentialRepo,
    sessionRepo,
    tokenStore,
    checker, // CasbinAuthorizationChecker implements AuthorizationChecker
)

// When validating a session, permissions are resolved automatically
info, err := authService.ValidateSession(ctx, sessionID)
// info.Permissions now contains the agent's effective permissions
```

If you pass `nil` instead of a checker, `ValidateSession` skips permission resolution and returns an empty permissions slice.

## Next Steps

- Read the [How-To Guides](how-to.md) for specific recipes (pattern matching, file store setup, OAuth providers, authorization checks)
- Read the [Explanation](explanation.md) to understand the design decisions behind Pericarp
- Browse the [Reference](reference.md) for complete API documentation
