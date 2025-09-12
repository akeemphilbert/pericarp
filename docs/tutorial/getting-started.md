# Getting Started with Pericarp

In this tutorial, you'll create your first Pericarp application and learn the basic concepts of Domain-Driven Design, CQRS, and Event Sourcing.

## Objective

By the end of this tutorial, you'll have:
- A working Pericarp application
- Understanding of the basic architecture
- A simple command and query handler
- Basic configuration setup

## Prerequisites

- Go 1.21+ installed
- Basic Go programming knowledge
- A text editor or IDE

## Step 1: Create a New Project

Create a new directory for your project:

```bash
mkdir my-pericarp-app
cd my-pericarp-app
go mod init my-pericarp-app
```

## Step 2: Install Pericarp

Add Pericarp to your project:

```bash
go get github.com/akeemphilbert/pericarp
```

## Step 3: Create Basic Configuration

Create a configuration file `config.yaml`:

```yaml
database:
  driver: sqlite
  dsn: "file:app.db?cache=shared&mode=rwc"

events:
  publisher: channel

logging:
  level: info
  format: json
```

## Step 4: Define Your First Domain Model

Create `domain/greeting.go`:

```go
package domain

import (
    "time"
    "github.com/your-org/pericarp/pkg/domain"
)

// Greeting represents a simple greeting aggregate using the built-in Entity
type Greeting struct {
    domain.Entity  // Embeds ID, version, sequenceNo, and event management
    message string
}

// NewGreeting creates a new greeting
func NewGreeting(id, message string) *Greeting {
    greeting := &Greeting{
        Entity:  domain.NewEntity(id),
        message: message,
    }
    
    // Generate domain event
    event := GreetingCreatedEvent{
        GreetingID: id,
        Message:    message,
        CreatedAt:  time.Now(),
    }
    
    greeting.AddEvent(event)  // Automatically handles version and sequence
    return greeting
}

// Domain methods
func (g *Greeting) Message() string { return g.message }

// LoadFromHistory reconstructs the greeting from events
func (g *Greeting) LoadFromHistory(events []domain.Event) {
    for _, event := range events {
        switch e := event.(type) {
        case GreetingCreatedEvent:
            g.message = e.Message
        }
    }
    
    // Call base implementation to update version and sequence
    g.Entity.LoadFromHistory(events)
}

// GreetingCreatedEvent represents a greeting creation event
type GreetingCreatedEvent struct {
    GreetingID string    `json:"greeting_id"`
    Message    string    `json:"message"`
    CreatedAt  time.Time `json:"created_at"`
}

func (e GreetingCreatedEvent) EventType() string { return "GreetingCreated" }
func (e GreetingCreatedEvent) AggregateID() string { return e.GreetingID }
func (e GreetingCreatedEvent) SequenceNo() int64 { return 1 }
func (e GreetingCreatedEvent) OccurredAt() time.Time { return e.CreatedAt }
```

## Step 5: Create Application Layer

Create `application/commands.go`:

```go
package application

import (
    "context"
    "my-pericarp-app/domain"
    
    "github.com/akeemphilbert/pericarp/pkg/application"
    pericarpdomain "github.com/akeemphilbert/pericarp/pkg/domain"
)

// CreateGreetingCommand represents a command to create a greeting
type CreateGreetingCommand struct {
    ID      string `json:"id" validate:"required"`
    Message string `json:"message" validate:"required,min=1,max=100"`
}

func (c CreateGreetingCommand) CommandType() string { return "CreateGreeting" }

// CreateGreetingHandler handles greeting creation commands
type CreateGreetingHandler struct {
    greetingRepo GreetingRepository
}

func NewCreateGreetingHandler(repo GreetingRepository) *CreateGreetingHandler {
    return &CreateGreetingHandler{greetingRepo: repo}
}

func (h *CreateGreetingHandler) Handle(ctx context.Context, logger pericarpdomain.Logger, eventStore pericarpdomain.EventStore, eventDispatcher pericarpdomain.EventDispatcher, payload application.Payload[application.Command]) (application.Response[any], error) {
    cmd, ok := payload.Data.(CreateGreetingCommand)
    if !ok {
        return application.Response[any]{
            Error: application.NewApplicationError("INVALID_COMMAND", "Expected CreateGreetingCommand", nil),
        }, nil
    }
    
    logger.Info("Creating greeting", "id", cmd.ID, "message", cmd.Message)
    
    // Create domain aggregate
    greeting := domain.NewGreeting(cmd.ID, cmd.Message)
    
    // Save aggregate (this will persist events)
    if err := h.greetingRepo.Save(ctx, greeting); err != nil {
        logger.Error("Failed to save greeting", "error", err)
        return application.Response[any]{Error: err}, nil
    }
    
    logger.Info("Greeting created successfully", "id", cmd.ID)
    return application.Response[any]{
        Data: application.CommandResponse{
            Code:    200,
            Message: "Greeting created successfully",
            Payload: map[string]string{"greeting_id": greeting.ID()},
        },
    }, nil
}

// GreetingRepository defines the interface for greeting persistence
type GreetingRepository interface {
    Save(ctx context.Context, greeting *domain.Greeting) error
    Load(ctx context.Context, id string) (*domain.Greeting, error)
}
```

Create `application/queries.go`:

```go
package application

import (
    "context"
    
    "github.com/akeemphilbert/pericarp/pkg/application"
    "github.com/akeemphilbert/pericarp/pkg/domain"
)

// GetGreetingQuery represents a query to get a greeting
type GetGreetingQuery struct {
    ID string `json:"id" validate:"required"`
}

func (q GetGreetingQuery) QueryType() string { return "GetGreeting" }

// GreetingView represents the read model for greetings
type GreetingView struct {
    ID         string `json:"id"`
    Message    string `json:"message"`
    SequenceNo int64  `json:"sequence_no"`
}

// GetGreetingHandler handles greeting queries
type GetGreetingHandler struct {
    greetingRepo GreetingRepository
}

func NewGetGreetingHandler(repo GreetingRepository) *GetGreetingHandler {
    return &GetGreetingHandler{greetingRepo: repo}
}

func (h *GetGreetingHandler) Handle(ctx context.Context, logger domain.Logger, query GetGreetingQuery) (GreetingView, error) {
    logger.Debug("Getting greeting", "id", query.ID)
    
    greeting, err := h.greetingRepo.Load(ctx, query.ID)
    if err != nil {
        logger.Error("Failed to load greeting", "error", err)
        return GreetingView{}, application.NewApplicationError("GREETING_NOT_FOUND", "Greeting not found", err)
    }
    
    view := GreetingView{
        ID:      greeting.ID(),
        Message: greeting.Message(),
        SequenceNo: greeting.SequenceNo(),
    }
    
    return view, nil
}
```

## Step 6: Create Infrastructure Layer

Create `infrastructure/greeting_repository.go`:

```go
package infrastructure

import (
    "context"
    "my-pericarp-app/application"
    "my-pericarp-app/domain"
    
    pericarpdomain "github.com/your-org/pericarp/pkg/domain"
)

// GreetingEventSourcingRepository implements event sourcing for greetings
type GreetingEventSourcingRepository struct {
    eventStore pericarpdomain.EventStore
    uow        pericarpdomain.UnitOfWork
}

func NewGreetingEventSourcingRepository(eventStore pericarpdomain.EventStore, uow pericarpdomain.UnitOfWork) application.GreetingRepository {
    return &GreetingEventSourcingRepository{
        eventStore: eventStore,
        uow:        uow,
    }
}

func (r *GreetingEventSourcingRepository) Save(ctx context.Context, greeting *domain.Greeting) error {
    // Register events with unit of work
    r.uow.RegisterEvents(greeting.UncommittedEvents())
    
    // Commit will persist events and dispatch them
    _, err := r.uow.Commit(ctx)
    if err != nil {
        return err
    }
    
    // Mark events as committed
    greeting.MarkEventsAsCommitted()
    return nil
}

func (r *GreetingEventSourcingRepository) Load(ctx context.Context, id string) (*domain.Greeting, error) {
    // Load events from event store
    envelopes, err := r.eventStore.Load(ctx, id)
    if err != nil {
        return nil, err
    }
    
    if len(envelopes) == 0 {
        return nil, pericarpdomain.NewDomainError("GREETING_NOT_FOUND", "Greeting not found", nil)
    }
    
    // Extract events from envelopes
    events := make([]pericarpdomain.Event, len(envelopes))
    for i, envelope := range envelopes {
        events[i] = envelope.Event()
    }
    
    // Reconstruct aggregate from events
    greeting := &domain.Greeting{}
    greeting.LoadFromHistory(events)
    
    return greeting, nil
}
```

## Step 7: Create Main Application

Create `main.go`:

```go
package main

import (
    "context"
    "log"
    "my-pericarp-app/application"
    "my-pericarp-app/infrastructure"
    
    "github.com/your-org/pericarp/pkg"
    pericarpdomain "github.com/your-org/pericarp/pkg/domain"
    pericarpapp "github.com/your-org/pericarp/pkg/application"
    pericarpinfra "github.com/your-org/pericarp/pkg/infrastructure"
    "go.uber.org/fx"
)

func main() {
    app := fx.New(
        // Pericarp core modules
        pkg.Module,
        
        // Application modules
        fx.Provide(
            // Repositories
            infrastructure.NewGreetingEventSourcingRepository,
            
            // Handlers
            application.NewCreateGreetingHandler,
            application.NewGetGreetingHandler,
        ),
        
        // Configure buses and run demo
        fx.Invoke(configureBuses),
        fx.Invoke(runDemo),
    )
    
    app.Run()
}

func configureBuses(
    commandBus pericarpapp.CommandBus,
    queryBus pericarpapp.QueryBus,
    createHandler *application.CreateGreetingHandler,
    getHandler *application.GetGreetingHandler,
) {
    // Register command handler
    commandBus.Register("CreateGreeting", createHandler)
    
    // Register query handler
    queryBus.Register("GetGreeting", getHandler)
}

func runDemo(
    commandBus pericarpapp.CommandBus,
    queryBus pericarpapp.QueryBus,
    logger pericarpdomain.Logger,
    eventStore pericarpdomain.EventStore,
    eventDispatcher pericarpdomain.EventDispatcher,
) {
    ctx := context.Background()
    
    // Create a greeting
    createCmd := application.CreateGreetingCommand{
        ID:      "greeting-1",
        Message: "Hello, Pericarp!",
    }
    
    logger.Info("Creating greeting...")
    payload := pericarpapp.Payload[pericarpapp.Command]{
        Data: createCmd,
    }
    
    if _, err := commandBus.Handle(ctx, logger, eventStore, eventDispatcher, payload); err != nil {
        log.Fatalf("Failed to create greeting: %v", err)
    }
    
    // Query the greeting
    getQuery := application.GetGreetingQuery{ID: "greeting-1"}
    
    logger.Info("Querying greeting...")
    result, err := queryBus.Handle(ctx, logger, eventStore, eventDispatcher, getQuery)
    if err != nil {
        log.Fatalf("Failed to get greeting: %v", err)
    }
    
    greeting := result.(application.GreetingView)
    logger.Info("Retrieved greeting", "id", greeting.ID, "message", greeting.Message, "version", greeting.Version)
}
```

## Step 8: Run Your Application

Run your application:

```bash
go mod tidy
go run main.go
```

You should see output similar to:

```
INFO Creating greeting...
INFO Creating greeting id=greeting-1 message=Hello, Pericarp!
INFO Greeting created successfully id=greeting-1
INFO Querying greeting...
DEBUG Getting greeting id=greeting-1
INFO Retrieved greeting id=greeting-1 message=Hello, Pericarp! version=1
```

## What Just Happened?

Congratulations! You've just built your first Pericarp application. Let's break down what happened:

### 1. Domain Layer
- **Greeting Aggregate**: Contains business logic and generates domain events
- **GreetingCreatedEvent**: Domain event that captures what happened
- **Event Sourcing Methods**: Allow the aggregate to be reconstructed from events

### 2. Application Layer
- **Commands**: Represent intentions to change state (`CreateGreetingCommand`)
- **Queries**: Represent requests for information (`GetGreetingQuery`)
- **Handlers**: Process commands and queries with business logic
- **Repository Interface**: Defines how aggregates are persisted

### 3. Infrastructure Layer
- **Event Sourcing Repository**: Persists aggregates as events
- **Unit of Work**: Manages transactional event persistence
- **Event Store**: Stores and retrieves events

### 4. Configuration
- **Fx Dependency Injection**: Wires everything together
- **Middleware**: Adds cross-cutting concerns like logging and validation
- **Bus Registration**: Connects handlers to their respective buses

## Key Concepts Learned

1. **Separation of Concerns**: Each layer has a specific responsibility
2. **Event Sourcing**: State is derived from a sequence of events
3. **CQRS**: Commands and queries are handled separately
4. **Domain Events**: Capture what happened in the domain
5. **Dependency Injection**: Components are loosely coupled

## Next Steps

Now that you have a basic understanding, you're ready to:

1. **Build a more complex system** → [User Management System](user-management.md)
2. **Learn about testing** → [Testing Your Application](testing.md)
3. **Understand the concepts deeper** → [Explanation docs](../explanation/README.md)

## Troubleshooting

### Common Issues

**"Module not found" error**
```bash
go mod tidy
```

**Database connection issues**
- Check that the `config.yaml` file is in the correct location
- Ensure the database directory is writable

**Import path issues**
- Make sure your module name in `go.mod` matches your import paths
- Update import paths to match your actual module name

### Getting Help

If you're stuck:
1. Check the complete example in the [examples directory](../../examples/)
2. Review the [How-to Guides](../how-to/README.md) for specific solutions
3. Ask questions in [GitHub Discussions](https://github.com/your-org/pericarp/discussions)

Ready for the next tutorial? Let's build a [User Management System](user-management.md)!