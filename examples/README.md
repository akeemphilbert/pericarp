# Pericarp Examples

This directory contains practical examples demonstrating how to use Pericarp for building event-sourced applications with Domain-Driven Design (DDD) patterns.

## Overview

The examples showcase different approaches to implementing aggregates and domain events using Pericarp's domain layer:

- **Event Sourcing**: Complete event-sourced aggregates with history reconstruction
- **Domain Events**: Flexible event creation using `EntityEvent` for various business scenarios
- **Aggregate Patterns**: Different approaches to implementing business logic and state management
- **Testing**: Comprehensive test coverage demonstrating proper testing patterns

## Examples

### 1. User Aggregate (`user_aggregate.go`)
A complete event-sourced user aggregate demonstrating:
- Basic aggregate creation with validation
- Business methods that generate domain events
- Event sourcing reconstruction from history
- Proper error handling and validation

### 2. Order Aggregate (`order_aggregate_example.go`)
A more complex aggregate showing:
- Complex business logic with multiple states
- Value objects and embedded types
- Business rules and constraints
- State transitions with events

### 3. User with Standard Events (`user_with_standard_events.go`)
An alternative approach using:
- Standardized event patterns
- Flexible event data structures
- Bulk operations and updates
- Metadata and context handling

### 4. Simple Event Usage (`simple_event_usage.go`)
Demonstrates basic event creation patterns:
- Creating events for different business scenarios
- Event metadata and context
- Various event types and patterns

## Quick Start

### Basic Event Creation

```go
// Create a simple domain event
event := domain.NewEntityEvent("user", "created", "user-123", "", "", map[string]interface{}{
    "email": "john@example.com",
    "name":  "John Doe",
})

// Add metadata
event.SetMetadata("source", "api")
event.SetMetadata("user_id", "admin-456")
```

### Aggregate Implementation

```go
type User struct {
    domain.BasicEntity
    email    string
    name     string
    isActive bool
}

func NewUser(id, email, name string) (*User, error) {
    user := &User{
        BasicEntity: domain.NewEntity(id),
        email:       email,
        name:        name,
        isActive:    true,
    }
    
    // Generate initial event
    eventData := struct {
        Email     string    `json:"email"`
        Name      string    `json:"name"`
        CreatedAt time.Time `json:"created_at"`
    }{
        Email:     email,
        Name:      name,
        CreatedAt: time.Now(),
    }
    
    event := domain.NewEntityEvent("user", "created", id, "", "", eventData)
    user.AddEvent(event)
    return user, nil
}
```

### Running the Examples

```bash
# Run individual examples
go run examples/user_aggregate.go
go run examples/simple_event_usage.go

# Run tests
go test ./examples/...
```

## Key Features

- **Event Sourcing**: Complete event history and reconstruction
- **Domain Events**: Flexible event creation with `EntityEvent`
- **Aggregate Patterns**: Multiple approaches to business logic implementation
- **Testing**: Comprehensive test coverage with proper patterns
- **Validation**: Input validation and error handling
- **Thread Safety**: Concurrent access protection

## Best Practices

1. **Always validate inputs** in constructors and business methods
2. **Generate events after state changes** to maintain consistency
3. **Use meaningful event names** in past tense (e.g., `user.created`)
4. **Handle unknown events gracefully** for forward compatibility
5. **Keep events immutable** once created
6. **Test event sourcing reconstruction** thoroughly

## Integration

These examples work seamlessly with Pericarp's infrastructure:
- Event stores for persistence
- Event dispatchers for publishing
- Repositories for aggregate management
- Command and query handlers for CQRS patterns