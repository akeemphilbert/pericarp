# Internal Package

This package contains internal implementation details, examples, and testing utilities for the Pericarp Go library. The code in this package is not intended to be used directly by external projects and follows the Go convention of using the `internal` directory to prevent external imports.

## Structure

The internal package follows the same Domain-Driven Design (DDD) layered architecture as the main library:

```
internal/
├── domain/           # Domain models and events for examples
├── application/      # Application services and handlers for examples  
├── infrastructure/   # Infrastructure implementations for examples
├── examples/         # Usage examples and demos
└── fx.go            # Fx modules for dependency injection
```

## Purpose

This package serves several purposes:

1. **Examples**: Provides concrete implementations showing how to use the library
2. **Testing**: Contains utilities and mock implementations for testing
3. **Demos**: Complete working examples of DDD patterns
4. **Internal Tools**: Helper functions and utilities used by the library itself

## User Management Example

The main example in this package is a complete user management system that demonstrates:

- **Domain Layer**: User aggregate with business logic and domain events
- **Application Layer**: Command/query handlers, DTOs, and projectors
- **Infrastructure Layer**: GORM-based read model repository

### Key Components

#### Domain (`internal/domain/`)
- `User` - User aggregate root with business logic
- `UserRepository` - Repository interface for user persistence
- Domain events: `UserCreatedEvent`, `UserEmailUpdatedEvent`, etc.

#### Application (`internal/application/`)
- Commands: `CreateUserCommand`, `UpdateUserEmailCommand`, etc.
- Queries: `GetUserQuery`, `ListUsersQuery`, etc.
- Handlers: Command and query handlers for user operations
- `UserProjector` - Event handler that maintains read models
- `UserReadModelRepository` - Interface for querying user read models

#### Infrastructure (`internal/infrastructure/`)
- `UserReadModelGORMRepository` - GORM implementation of read model repository

## Usage

### With Fx Dependency Injection

```go
package main

import (
    "github.com/pericarp/pericarp-go/internal"
    "github.com/pericarp/pericarp-go/pkg"
    "go.uber.org/fx"
)

func main() {
    fx.New(
        pkg.PericarpModule,           // Core library components
        internal.UserExampleModule,   // User example with auto-setup
        // Your application modules...
    ).Run()
}
```

### Manual Setup

```go
package main

import (
    "github.com/pericarp/pericarp-go/internal/application"
    "github.com/pericarp/pericarp-go/internal/infrastructure"
    // ... other imports
)

func main() {
    // Setup database
    db := setupGormDB()
    
    // Create repositories
    readModelRepo := infrastructure.NewUserReadModelGORMRepository(db)
    
    // Create handlers
    getUserHandler := application.NewGetUserHandler(readModelRepo)
    
    // Use handlers...
}
```

## Testing

The internal package includes comprehensive tests that serve as both validation and examples:

- Unit tests for all components
- Integration tests showing end-to-end flows
- Mock implementations for external dependencies

## Important Notes

1. **Not Public API**: Code in this package is not part of the public API and may change without notice
2. **Examples Only**: These implementations are for demonstration and may not be production-ready
3. **Testing Focus**: Designed primarily for testing and learning the library patterns
4. **No Backwards Compatibility**: Internal APIs may change between versions

## Contributing

When adding new examples or internal utilities:

1. Follow the same DDD layered architecture
2. Include comprehensive tests
3. Add documentation explaining the example's purpose
4. Ensure examples are realistic but not overly complex