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
- **Database Flexibility**: Support for SQLite (development) and PostgreSQL (production)
- **Dependency Injection**: Built-in Fx modules for easy configuration
- **Comprehensive Testing**: BDD scenarios, unit tests, and integration tests
- **Performance Optimized**: No reflection in hot paths, efficient JSON serialization

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
type User struct {
    id       string
    email    string
    name     string
    version  int
    events   []domain.Event
}

func (u *User) UpdateEmail(newEmail string) error {
    if newEmail == u.email {
        return nil
    }
    
    event := UserEmailUpdatedEvent{
        UserID:   u.id,
        OldEmail: u.email,
        NewEmail: newEmail,
    }
    
    u.email = newEmail
    u.version++
    u.events = append(u.events, event)
    
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