# Design Document

## Overview

Pericarp is a Go library implementing Domain-Driven Design (DDD), Command Query Responsibility Segregation (CQRS), and Event Sourcing patterns. The library provides a clean, layered architecture that enables developers to build scalable server applications with clear separation of concerns. The design follows the Persist-then-Dispatch pattern for event handling and maintains domain purity by isolating infrastructure concerns.

## Architecture

The library follows a three-layer DDD architecture:

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
│  - DTOs                                │
│  - Platform (Logger, Config)           │
└─────────────────────────────────────────┘
```

### Package Structure

Following standard Go project layout:

```
pericarp/
├── cmd/                    # Demo applications
│   └── demo/
├── pkg/                    # Public library code
│   ├── domain/            # Domain layer
│   ├── application/       # Application layer
│   └── infrastructure/    # Infrastructure layer
├── internal/              # Private library code
├── examples/              # Example usage
├── docs/                  # Documentation
└── scripts/               # Build and utility scripts
```

## Components and Interfaces

### Core Domain Interfaces

```go
// Logger interface for structured logging
type Logger interface {
    Debug(msg string, keysAndValues ...interface{})
    Debugf(format string, args ...interface{})
    Info(msg string, keysAndValues ...interface{})
    Infof(format string, args ...interface{})
    Warn(msg string, keysAndValues ...interface{})
    Warnf(format string, args ...interface{})
    Error(msg string, keysAndValues ...interface{})
    Errorf(format string, args ...interface{})
    Fatal(msg string, keysAndValues ...interface{})
    Fatalf(format string, args ...interface{})
}

// Event represents a domain event
type Event interface {
    EventType() string
    AggregateID() string
    Version() int
    OccurredAt() time.Time
}

// Envelope wraps events with metadata
type Envelope interface {
    Event() Event
    Metadata() map[string]interface{}
    EventID() string
    Timestamp() time.Time
}

// EventStore handles event persistence
type EventStore interface {
    Save(ctx context.Context, events []Event) ([]Envelope, error)
    Load(ctx context.Context, aggregateID string) ([]Envelope, error)
    LoadFromVersion(ctx context.Context, aggregateID string, version int) ([]Envelope, error)
}

// EventDispatcher handles event distribution
type EventDispatcher interface {
    Dispatch(ctx context.Context, envelopes []Envelope) error
    Subscribe(eventType string, handler EventHandler) error
}

// UnitOfWork manages transactional event persistence
type UnitOfWork interface {
    RegisterEvents(events []Event)
    Commit(ctx context.Context) ([]Envelope, error)
    Rollback() error
}
```

### Application Layer Components

```go
// Payload wraps request data with metadata
type Payload[T any] struct {
    Data     T
    Metadata map[string]any
    TraceID  string
    UserID   string
}

// Response wraps response data with metadata
type Response[T any] struct {
    Data     T
    Metadata map[string]any
    Error    error
}

// Unified handler signature for both commands and queries
type Handler[Req any, Res any] func(ctx context.Context, log Logger, p Payload[Req]) (Response[Res], error)

// CommandHandler processes commands using unified signature
type CommandHandler[T Command] interface {
    Handle(ctx context.Context, log Logger, p Payload[T]) (Response[struct{}], error)
}

// QueryHandler processes queries using unified signature
type QueryHandler[T Query, R any] interface {
    Handle(ctx context.Context, log Logger, p Payload[T]) (Response[R], error)
}

// EventHandler processes events (projectors/sagas)
type EventHandler interface {
    Handle(ctx context.Context, envelope Envelope) error
    EventTypes() []string
}

// Command marker interface
type Command interface {
    CommandType() string
}

// Query marker interface
type Query interface {
    QueryType() string
}

// Unified middleware that works for both commands and queries
type Middleware[Req any, Res any] func(next Handler[Req, Res]) Handler[Req, Res]

// Using unified Middleware type for both commands and queries

// Handler function types for bus registration
type CommandHandlerFunc func(ctx context.Context, log Logger, p Payload[Command]) (Response[struct{}], error)
type QueryHandlerFunc func(ctx context.Context, log Logger, p Payload[Query]) (Response[any], error)

// CommandBus with unified middleware support
type CommandBus interface {
    Handle(ctx context.Context, logger Logger, cmd Command) error
    Register(cmdType string, handler Handler[Command, struct{}], middleware ...Middleware[Command, struct{}])
}

// QueryBus with unified middleware support
type QueryBus interface {
    Handle(ctx context.Context, logger Logger, query Query) (any, error)
    Register(queryType string, handler Handler[Query, any], middleware ...Middleware[Query, any])
}
```

### Domain Layer Components

```go
// AggregateRoot base for domain aggregates
type AggregateRoot interface {
    ID() string
    Version() int
    UncommittedEvents() []Event
    MarkEventsAsCommitted()
    LoadFromHistory(events []Event)
}

// Repository interface for aggregate persistence
type Repository[T AggregateRoot] interface {
    Save(ctx context.Context, aggregate T) error
    Load(ctx context.Context, id string) (T, error)
}

// DomainService for domain logic that doesn't belong to aggregates
type DomainService interface {
    // Domain-specific methods
}
```

## Data Models

### Event Storage Schema

Using GORM for event persistence with the following schema:

```go
type EventRecord struct {
    ID          string    `gorm:"primaryKey"`
    AggregateID string    `gorm:"index"`
    EventType   string    `gorm:"index"`
    Version     int       `gorm:"index"`
    Data        string    `gorm:"type:text"` // JSON serialized event
    Metadata    string    `gorm:"type:text"` // JSON serialized metadata
    Timestamp   time.Time `gorm:"index"`
    CreatedAt   time.Time
}
```

### Configuration Models

```go
type Config struct {
    Database DatabaseConfig `mapstructure:"database"`
    Events   EventsConfig   `mapstructure:"events"`
    Logging  LoggingConfig  `mapstructure:"logging"`
}

type DatabaseConfig struct {
    Driver string `mapstructure:"driver"` // sqlite, postgres
    DSN    string `mapstructure:"dsn"`
}

type EventsConfig struct {
    Publisher string `mapstructure:"publisher"` // channel, pubsub
}
```

## Error Handling

### Error Types

```go
// Domain errors
type DomainError struct {
    Code    string
    Message string
    Cause   error
}

// Application errors
type ValidationError struct {
    Field   string
    Message string
}

type ConcurrencyError struct {
    AggregateID string
    Expected    int
    Actual      int
}

// Infrastructure errors
type PersistenceError struct {
    Operation string
    Cause     error
}
```

### Error Handling Strategy

1. **Domain Layer**: Returns domain-specific errors without infrastructure details
2. **Application Layer**: Translates domain errors to application errors, handles validation
3. **Infrastructure Layer**: Wraps infrastructure errors with context, handles retries and circuit breaking

## Testing Strategy

### BDD Testing with Cucumber
- Use Cucumber for behavior-driven development with Gherkin scenarios
- Follow best practices from https://automationpanda.com/2017/01/30/bdd-101-writing-good-gherkin/
- Write scenarios that focus on business behavior rather than implementation details

### Test Levels
- **BDD/Acceptance Tests**: Cucumber scenarios testing end-to-end behavior
- **Unit Testing**: Pure unit tests for domain logic and individual components
- **Integration Testing**: Test infrastructure components with real dependencies

### Test Structure
```go
// Domain tests - no external dependencies
func TestAggregate_BusinessLogic(t *testing.T) {
    // Pure unit tests
}

// Application tests - with mocks generated by moq
func TestCommandHandler_Handle(t *testing.T) {
    // Use moq-generated mocks for repositories and services
    mockRepo := &RepositoryMock{}
    // Configure mock behavior and test handler
}

// Infrastructure tests - with test containers
func TestEventStore_Save(t *testing.T) {
    // Use testcontainers for database testing
}
```

### BDD Scenario Examples
```gherkin
Feature: User Management
  As a system administrator
  I want to manage users in the system
  So that I can control access and maintain user data

  Scenario: Creating a new user
    Given the system is running
    When I create a user with email "john@example.com"
    Then the user should be created successfully
    And a UserCreated event should be published
    And the user should appear in the read model

  Scenario: Updating user email
    Given a user exists with email "john@example.com"
    When I update the user's email to "john.doe@example.com"
    Then the email should be updated successfully
    And a UserEmailUpdated event should be published
    And the read model should reflect the new email
```

### Testing Utilities
- In-memory implementations for testing
- Test fixtures for common scenarios
- Custom assertions without external frameworks
- Use moq (https://github.com/matryer/moq) for generating mocks from interfaces
- Cucumber step definitions for BDD scenarios

## Implementation Details

### Event Sourcing Flow

```mermaid
sequenceDiagram
    participant C as Command Handler
    participant A as Aggregate
    participant UoW as Unit of Work
    participant ES as Event Store
    participant ED as Event Dispatcher
    participant EH as Event Handler

    C->>A: Execute business logic
    A->>A: Generate domain events
    C->>UoW: Register events
    C->>UoW: Commit
    UoW->>ES: Save events
    ES->>UoW: Return envelopes
    UoW->>ED: Dispatch envelopes
    ED->>EH: Handle events
```

### Middleware Examples

```go
// Unified logging middleware that works for both commands and queries
func LoggingMiddleware[Req any, Res any]() Middleware[Req, Res] {
    return func(next Handler[Req, Res]) Handler[Req, Res] {
        return func(ctx context.Context, log Logger, p Payload[Req]) (Response[Res], error) {
            start := time.Now()
            
            // Extract request type for logging
            var requestType string
            if cmd, ok := any(p.Data).(Command); ok {
                requestType = cmd.CommandType()
            } else if query, ok := any(p.Data).(Query); ok {
                requestType = query.QueryType()
            } else {
                requestType = fmt.Sprintf("%T", p.Data)
            }
            
            log.Info("Processing request", 
                "type", requestType, 
                "traceId", p.TraceID,
                "userId", p.UserID)
            
            response, err := next(ctx, log, p)
            
            duration := time.Since(start)
            if err != nil {
                log.Error("Request failed", 
                    "type", requestType, 
                    "duration", duration, 
                    "error", err,
                    "traceId", p.TraceID)
            } else {
                log.Info("Request completed", 
                    "type", requestType, 
                    "duration", duration,
                    "traceId", p.TraceID)
            }
            
            return response, err
        }
    }
}

// Unified validation middleware
func ValidationMiddleware[Req any, Res any]() Middleware[Req, Res] {
    return func(next Handler[Req, Res]) Handler[Req, Res] {
        return func(ctx context.Context, log Logger, p Payload[Req]) (Response[Res], error) {
            if validator, ok := any(p.Data).(Validator); ok {
                if err := validator.Validate(); err != nil {
                    log.Warn("Request validation failed", 
                        "error", err,
                        "traceId", p.TraceID)
                    
                    var zero Res
                    return Response[Res]{
                        Data:  zero,
                        Error: ValidationError{Message: err.Error()},
                        Metadata: map[string]any{
                            "validation_failed": true,
                        },
                    }, err
                }
            }
            return next(ctx, log, p)
        }
    }
}

// Unified metrics middleware
func MetricsMiddleware[Req any, Res any](metrics MetricsCollector) Middleware[Req, Res] {
    return func(next Handler[Req, Res]) Handler[Req, Res] {
        return func(ctx context.Context, log Logger, p Payload[Req]) (Response[Res], error) {
            start := time.Now()
            
            var requestType string
            if cmd, ok := any(p.Data).(Command); ok {
                requestType = cmd.CommandType()
            } else if query, ok := any(p.Data).(Query); ok {
                requestType = query.QueryType()
            } else {
                requestType = fmt.Sprintf("%T", p.Data)
            }
            
            response, err := next(ctx, log, p)
            duration := time.Since(start)
            
            metrics.RecordRequestDuration(requestType, duration)
            if err != nil {
                metrics.IncrementRequestErrors(requestType)
            }
            
            return response, err
        }
    }
}

// Usage examples showing unified middleware application
func ConfigureCommandBus(bus CommandBus, metrics MetricsCollector) {
    // Same middleware can be used for all command types
    loggingMW := LoggingMiddleware[CreateUserCommand, struct{}]()
    validationMW := ValidationMiddleware[CreateUserCommand, struct{}]()
    metricsMW := MetricsMiddleware[CreateUserCommand, struct{}](metrics)
    
    bus.Register("CreateUser", createUserHandler, loggingMW, validationMW, metricsMW)
}

func ConfigureQueryBus(bus QueryBus, metrics MetricsCollector) {
    // Same middleware implementations work for queries too
    loggingMW := LoggingMiddleware[GetUserQuery, UserView]()
    metricsMW := MetricsMiddleware[GetUserQuery, UserView](metrics)
    
    bus.Register("GetUser", getUserHandler, loggingMW, metricsMW)
}
```

### Dependency Injection with Fx

```go
var Module = fx.Options(
    fx.Provide(
        NewEventStore,
        NewEventDispatcher,
        NewUnitOfWork,
        NewCommandBus,
        NewQueryBus,
        NewLogger,
        // Command handlers
        NewCreateUserHandler,
        // Query handlers
        NewGetUserHandler,
        // Event handlers
        NewUserProjector,
        // Middleware
        NewLoggingMiddleware,
        NewValidationMiddleware,
        NewMetricsMiddleware,
    ),
)
```

### Configuration with Viper

```go
func LoadConfig() (*Config, error) {
    viper.SetConfigName("config")
    viper.SetConfigType("yaml")
    viper.AddConfigPath(".")
    viper.AddConfigPath("./configs")
    
    // Environment variable support
    viper.AutomaticEnv()
    viper.SetEnvPrefix("PERICARP")
    
    var config Config
    if err := viper.ReadInConfig(); err != nil {
        return nil, err
    }
    
    return &config, viper.Unmarshal(&config)
}
```

### Performance Considerations

1. **JSON Serialization**: Use standard library encoding/json for event serialization
2. **Connection Pooling**: Configure GORM with appropriate connection pool settings
3. **Event Batching**: Process events in batches to reduce database round trips
4. **Indexing**: Proper database indexes on aggregate_id, event_type, and version
5. **Memory Management**: Avoid reflection in hot paths, use concrete types where possible

### Database Support

#### SQLite Configuration
```go
dsn := "file:events.db?cache=shared&mode=rwc"
db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
```

#### PostgreSQL Configuration
```go
dsn := "host=localhost user=user password=pass dbname=events port=5432 sslmode=disable"
db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
```

## Demo Application

The demo will showcase:

1. **User Management Aggregate**: Create, update user operations
2. **CQRS Implementation**: Separate command and query models
3. **Event Sourcing**: User events persisted and replayed
4. **Event Handlers**: User projection for read models
5. **Database Flexibility**: Configuration for SQLite and PostgreSQL

### Demo Commands
- `CreateUser`: Creates a new user aggregate
- `UpdateUserEmail`: Updates user email address

### Demo Queries
- `GetUser`: Retrieves user by ID from read model
- `ListUsers`: Lists all users with pagination

### Demo Events
- `UserCreated`: Fired when user is created
- `UserEmailUpdated`: Fired when email is updated

### Demo Unified Handler Configuration
```go
// Example command handler using unified signature
type CreateUserHandler struct {
    userRepo UserRepository
}

func (h *CreateUserHandler) Handle(ctx context.Context, log Logger, p Payload[CreateUserCommand]) (Response[struct{}], error) {
    log.Info("Creating user", "email", p.Data.Email, "traceId", p.TraceID)
    
    user, err := NewUser(p.Data.Email, p.Data.Name)
    if err != nil {
        return Response[struct{}]{Error: err}, err
    }
    
    if err := h.userRepo.Save(ctx, user); err != nil {
        return Response[struct{}]{Error: err}, err
    }
    
    return Response[struct{}]{
        Data: struct{}{},
        Metadata: map[string]any{
            "userId": user.ID(),
            "version": user.Version(),
        },
    }, nil
}

// Example query handler using unified signature
type GetUserHandler struct {
    readRepo UserReadRepository
}

func (h *GetUserHandler) Handle(ctx context.Context, log Logger, p Payload[GetUserQuery]) (Response[UserView], error) {
    log.Debug("Getting user", "userId", p.Data.UserID, "traceId", p.TraceID)
    
    user, err := h.readRepo.GetByID(ctx, p.Data.UserID)
    if err != nil {
        return Response[UserView]{Error: err}, err
    }
    
    return Response[UserView]{
        Data: UserView{
            ID:    user.ID,
            Email: user.Email,
            Name:  user.Name,
        },
        Metadata: map[string]any{
            "cached": false,
            "version": user.Version,
        },
    }, nil
}

// Configure buses with unified middleware
func ConfigureBuses(commandBus CommandBus, queryBus QueryBus, metrics MetricsCollector) {
    // Same middleware works for both commands and queries
    loggingMW := LoggingMiddleware[any, any]()
    validationMW := ValidationMiddleware[any, any]()
    metricsMW := MetricsMiddleware[any, any](metrics)
    
    // Register command handlers
    commandBus.Register("CreateUser", createUserHandler, loggingMW, validationMW, metricsMW)
    commandBus.Register("UpdateUserEmail", updateUserHandler, loggingMW, validationMW, metricsMW)
    
    // Register query handlers with same middleware
    queryBus.Register("GetUser", getUserHandler, loggingMW, metricsMW)
    queryBus.Register("ListUsers", listUsersHandler, loggingMW, metricsMW)
}

// Usage example with Payload wrapper
logger := NewLogger()
payload := Payload[CreateUserCommand]{
    Data: CreateUserCommand{Email: "user@example.com", Name: "John Doe"},
    Metadata: map[string]any{"source": "api"},
    TraceID: "trace-123",
    UserID: "admin-456",
}

err := commandBus.Handle(ctx, logger, payload.Data)
```