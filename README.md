# Pericarp Go Library

A comprehensive Go library implementing Domain-Driven Design (DDD), Command Query Responsibility Segregation (CQRS), and Event Sourcing patterns with clean architecture principles.

## Quick Start

```go
package main

import (
    "context"
    "log"
    
    "github.com/your-org/pericarp"
    "go.uber.org/fx"
)

func main() {
    app := fx.New(
        pericarp.Module,
        fx.Invoke(func(bus pericarp.CommandBus) {
            cmd := CreateUserCommand{Email: "user@example.com", Name: "John Doe"}
            if err := bus.Handle(context.Background(), cmd); err != nil {
                log.Fatal(err)
            }
        }),
    )
    app.Run()
}
```

## Features

- **Clean Architecture**: Strict separation between domain, application, and infrastructure layers
- **CQRS Pattern**: Separate command and query handling with unified middleware support
- **Event Sourcing**: Persist-then-dispatch pattern with event store and dispatcher
- **Ready-to-Use Entity**: Concrete aggregate root implementation with built-in event management
- **EntityEvent**: Generic event implementation that eliminates the need for specific event types
- **Database Flexibility**: Support for SQLite (development) and PostgreSQL (production)
- **Dependency Injection**: Built-in Fx modules for easy configuration
- **Comprehensive Testing**: BDD scenarios, unit tests, and integration tests
- **Performance Optimized**: No reflection in hot paths, efficient JSON serialization
- **Thread-Safe**: Concurrent access protection for all core components

## EntityEvent - Flexible Event Creation

The `EntityEvent` provides a single, flexible way to create domain events without needing to define specific event types for each use case.

```go
import "github.com/akeemphilbert/pericarp/pkg/domain"

// User creation event
user := &User{ID: "user-123", Email: "john@example.com", Name: "John Doe", Role: "admin"}
event := domain.NewEntityEvent("user", "created", "user-123", "", "", user)

// Order status change
statusChange := struct {
    OldStatus string `json:"old_status"`
    NewStatus string `json:"new_status"`
    Tracking  string `json:"tracking"`
}{
    OldStatus: "pending",
    NewStatus: "shipped",
    Tracking:  "TRACK123",
}
event := domain.NewEntityEvent("order", "status_changed", "order-456", "", "", statusChange)

// Custom business event
payment := struct {
    Amount        float64 `json:"amount"`
    Currency      string  `json:"currency"`
    TransactionID string  `json:"transaction_id"`
}{
    Amount:        99.99,
    Currency:      "USD",
    TransactionID: "txn_abc123",
}
event := domain.NewEntityEvent("payment", "processing_completed", "payment-999", "", "", payment)

// Add metadata for cross-cutting concerns
event.SetMetadata("correlation_id", "req-abc123")
event.SetMetadata("user_id", "user-123")
```

### Benefits

- **Single Factory**: Just use `domain.NewEntityEvent()` for all event types
- **Type-Safe Data**: Pass any serializable struct or object as event data
- **Consistent Format**: All events follow the same `entitytype.eventtype` naming
- **Metadata Support**: Add correlation IDs, user context, and other cross-cutting data
- **JSON Serialization**: Built-in marshaling/unmarshaling support

## Documentation

### 📚 [Tutorial](docs/tutorial/README.md)
Learn how to build your first application with Pericarp step-by-step.

### 🔧 [How-to Guides](docs/how-to/README.md)
Practical solutions for common implementation patterns and problems.

### 📖 [Reference](docs/reference/README.md)
Complete API documentation and technical specifications.

### 💡 [Explanation](docs/explanation/README.md)
Deep dive into DDD, CQRS, and Event Sourcing concepts and design decisions.

## Installation

```bash
go get github.com/your-org/pericarp
```

## Architecture Overview

```
┌─────────────────────────────────────────┐
│           Application Layer             │
│  - Command Handlers                     │
│  - Query Handlers                       │
│  - Event Handlers (Projectors/Sagas)   │
└─────────────────────────────────────────┘
┌─────────────────────────────────────────┐
│             Domain Layer                │
│  - Aggregates                          │
│  - Value Objects                       │
│  - Domain Services                     │
│  - Repository Interfaces               │
│  - Domain Events                       │
└─────────────────────────────────────────┘
┌─────────────────────────────────────────┐
│          Infrastructure Layer           │
│  - Event Store (GORM)                  │
│  - Event Dispatcher (Watermill)        │
│  - Repository Implementations          │
│  - Configuration (Viper)               │
│  - Dependency Injection (Fx)           │
└─────────────────────────────────────────┘
```

## Example Usage

### Define Your Domain

```go
// Using the built-in Entity struct for event sourcing
type User struct {
    domain.Entity  // Embeds GetID, version, sequenceNo, and events management
    email    string
    name     string
}

func NewUser(id, email, name string) (*User, error) {
    user := &User{
        Entity: domain.NewEntity(id),
        email:  email,
        name:   name,
    }
    
    event := UserCreatedEvent{
        UserID: id,
        Email:  email,
        Name:   name,
    }
    
    user.AddEvent(event)  // Automatically manages version and sequence
    return user, nil
}

func (u *User) UpdateEmail(newEmail string) error {
    if newEmail == u.email {
        return nil
    }
    
    event := UserEmailUpdatedEvent{
        UserID:   u.ID(),
        OldEmail: u.email,
        NewEmail: newEmail,
    }
    
    u.email = newEmail
    u.AddEvent(event)  // Handles version increment and event storage
    
    return nil
}
```

### Create Command Handlers

```go
type UpdateUserEmailHandler struct {
    userRepo UserRepository
}

func (h *UpdateUserEmailHandler) Handle(ctx context.Context, log domain.Logger, p application.Payload[UpdateUserEmailCommand]) (application.Response[struct{}], error) {
    user, err := h.userRepo.Load(ctx, p.Data.UserID)
    if err != nil {
        return application.Response[struct{}]{Error: err}, err
    }
    
    if err := user.UpdateEmail(p.Data.NewEmail); err != nil {
        return application.Response[struct{}]{Error: err}, err
    }
    
    if err := h.userRepo.Save(ctx, user); err != nil {
        return application.Response[struct{}]{Error: err}, err
    }
    
    return application.Response[struct{}]{Data: struct{}{}}, nil
}
```

### Configure with Middleware

```go
commandBus.Register("UpdateUserEmail", updateHandler,
    application.LoggingMiddleware[UpdateUserEmailCommand, struct{}](),
    application.ValidationMiddleware[UpdateUserEmailCommand, struct{}](),
    application.MetricsMiddleware[UpdateUserEmailCommand, struct{}](metrics),
)
```

## Testing

Run the comprehensive test suite:

```bash
# All tests
make test

# BDD scenarios
make test-bdd

# Integration tests
make test-integration

# Performance tests
make test-performance
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for your changes
4. Ensure all tests pass
5. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Support

- 📖 [Documentation](docs/)
- 🐛 [Issues](https://github.com/your-org/pericarp/issues)
- 💬 [Discussions](https://github.com/your-org/pericarp/discussions)