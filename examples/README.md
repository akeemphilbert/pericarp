# Entity Usage Examples

This directory contains examples showing how to use the `domain.Entity` struct as a base for creating event-sourced aggregates in your applications.

## Overview

The `domain.Entity` struct provides a concrete implementation of the `AggregateRoot` interface that handles common event sourcing concerns:

- **Identity Management**: Unique ID for each aggregate
- **Version Control**: Tracks the number of applied events
- **Sequence Tracking**: Incremental sequence number for event ordering
- **Event Management**: Stores uncommitted events and manages their lifecycle
- **Thread Safety**: Concurrent access protection with RWMutex

## Key Features

### Properties
- `id`: Unique identifier for the aggregate
- `version`: Number of events applied (used for optimistic concurrency control)
- `sequenceNo`: Incremental sequence number (useful for event ordering)
- `events`: Array of uncommitted domain events

### Methods
- `AddEvent(event)`: Adds an event and increments version/sequence
- `UncommittedEvents()`: Returns copy of uncommitted events
- `MarkEventsAsCommitted()`: Clears uncommitted events after persistence
- `LoadFromHistory(events)`: Reconstructs aggregate state from events
- `HasUncommittedEvents()`: Checks if there are unpersisted events
- `Reset()`: Resets aggregate to initial state (useful for testing)
- `Clone()`: Creates independent copy of the aggregate

## Usage Pattern

### 1. Embed Entity in Your Aggregate

```go
type User struct {
    domain.Entity  // Embed the Entity struct
    email    string
    name     string
    isActive bool
}
```

### 2. Create Constructor with Initial Event

```go
func NewUser(id, email, name string) (*User, error) {
    // Validation
    if id == "" {
        return nil, errors.New("user ID cannot be empty")
    }
    
    // Create aggregate
    user := &User{
        Entity:   domain.NewEntity(id),
        email:    email,
        name:     name,
        isActive: true,
    }
    
    // Generate initial event
    event := UserCreatedEvent{
        UserID:    id,
        Email:     email,
        Name:      name,
        CreatedAt: time.Now(),
    }
    
    user.AddEvent(event)  // This increments version and sequence
    return user, nil
}
```

### 3. Implement Business Methods

```go
func (u *User) ChangeEmail(newEmail string) error {
    // Business validation
    if newEmail == "" {
        return errors.New("email cannot be empty")
    }
    
    if newEmail == u.email {
        return nil // No change needed
    }
    
    // Apply change
    oldEmail := u.email
    u.email = newEmail
    
    // Generate event
    event := UserEmailChangedEvent{
        UserID:    u.ID(),
        OldEmail:  oldEmail,
        NewEmail:  newEmail,
        ChangedAt: time.Now(),
    }
    
    u.AddEvent(event)  // Automatically increments version and sequence
    return nil
}
```

### 4. Implement Event Sourcing Reconstruction

```go
func (u *User) LoadFromHistory(events []domain.Event) {
    // Apply each event to rebuild state
    for _, event := range events {
        u.applyEvent(event)
    }
    
    // Call base implementation to update version and sequence
    u.Entity.LoadFromHistory(events)
}

func (u *User) applyEvent(event domain.Event) {
    switch e := event.(type) {
    case UserCreatedEvent:
        u.email = e.Email
        u.name = e.Name
        u.isActive = true
        
    case UserEmailChangedEvent:
        u.email = e.NewEmail
        
    case UserDeactivatedEvent:
        u.isActive = false
        
    // Handle other event types...
    }
}
```

## Complete Example

See `user_aggregate.go` for a complete implementation showing:

- ✅ Proper aggregate construction with validation
- ✅ Business methods that generate events
- ✅ Event sourcing reconstruction
- ✅ Domain event definitions
- ✅ Thread-safe operations

## Testing

The `user_aggregate_test.go` file demonstrates:

- Unit testing aggregate behavior
- Testing event generation
- Testing event sourcing reconstruction
- Testing complete aggregate lifecycle
- Validation error handling

## Benefits of Using Entity

### 1. **Reduced Boilerplate**
No need to implement basic event sourcing mechanics in every aggregate.

### 2. **Consistency**
All aggregates follow the same patterns for event management.

### 3. **Thread Safety**
Built-in protection against concurrent access issues.

### 4. **Performance Optimized**
- Pre-allocated slices to reduce memory allocations
- RWMutex for better read performance
- Efficient event copying

### 5. **Testing Support**
Built-in methods like `Reset()` and `Clone()` make testing easier.

## Best Practices

### 1. **Always Validate in Constructors**
```go
func NewUser(id, email string) (*User, error) {
    if id == "" {
        return nil, errors.New("user ID cannot be empty")
    }
    // ... validation logic
}
```

### 2. **Generate Events After State Changes**
```go
func (u *User) ChangeEmail(newEmail string) error {
    // Apply change first
    u.email = newEmail
    
    // Then generate event
    u.AddEvent(UserEmailChangedEvent{...})
    return nil
}
```

### 3. **Handle Unknown Events Gracefully**
```go
func (u *User) applyEvent(event domain.Event) {
    switch e := event.(type) {
    case UserCreatedEvent:
        // Handle known event
    default:
        // Ignore unknown events for forward compatibility
    }
}
```

### 4. **Use Meaningful Event Names**
- Use past tense: `UserCreated`, not `CreateUser`
- Be specific: `UserEmailChanged`, not `UserUpdated`
- Include context: `OrderShipped`, not `StatusChanged`

### 5. **Keep Events Immutable**
```go
type UserCreatedEvent struct {
    UserID    string    `json:"user_id"`    // No setters
    Email     string    `json:"email"`      // Read-only
    CreatedAt time.Time `json:"created_at"` // Immutable
}
```

## Integration with Pericarp

The Entity struct works seamlessly with Pericarp's infrastructure:

```go
// Repository usage
user, err := NewUser("user-123", "john@example.com", "John Doe")
err = userRepository.Save(ctx, user)  // Persists uncommitted events

// Loading from event store
user, err := userRepository.Load(ctx, "user-123")  // Reconstructs from events

// Command handler usage
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    user, err := NewUser(cmd.ID, cmd.Email, cmd.Name)
    if err != nil {
        return err
    }
    
    return h.userRepo.Save(ctx, user)  // Entity handles event management
}
```

## Performance Considerations

- **Memory**: Entity pre-allocates small event slices to reduce allocations
- **Concurrency**: RWMutex allows multiple concurrent readers
- **Event Copying**: `UncommittedEvents()` returns copies to prevent external modification
- **Sequence Tracking**: Efficient increment operations for version and sequence

## Thread Safety

The Entity struct is fully thread-safe:

```go
// Safe to call concurrently
go user.ChangeEmail("new@example.com")
go user.ChangeName("New Name")
go func() { events := user.UncommittedEvents() }()
```

All public methods use appropriate locking to ensure data consistency.