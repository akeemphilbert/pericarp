# Building a User Management System

In this tutorial, you'll build a complete user management system using Pericarp's DDD, CQRS, and Event Sourcing patterns. This tutorial is based on the actual demo application included with Pericarp.

## Objective

By the end of this tutorial, you'll have:
- A complete user management system with CRUD operations
- CQRS implementation with separate command and query handlers
- Event sourcing with domain events
- Read model projections for queries
- A CLI interface to interact with the system

## Prerequisites

- Completed the [Getting Started](getting-started.md) tutorial
- Go 1.21+ installed
- Basic understanding of DDD, CQRS, and Event Sourcing concepts

## Architecture Overview

Our user management system will follow this architecture:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   CLI Interface │    │  HTTP API       │    │  Web Interface  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────┐
                    │ Application     │
                    │ Layer (CQRS)    │
                    └─────────────────┘
                                 │
                    ┌─────────────────┐
                    │ Domain Layer    │
                    │ (Aggregates)    │
                    └─────────────────┘
                                 │
                    ┌─────────────────┐
                    │ Infrastructure  │
                    │ Layer           │
                    └─────────────────┘
```

## Step 1: Domain Model

Let's start by defining our User aggregate:

```go
// examples/user_aggregate.go
package domain

import (
    "encoding/json"
    "fmt"
    "time"

    "github.com/akeemphilbert/pericarp/pkg/domain"
    "github.com/segmentio/ksuid"
)

// User represents a user aggregate
type User struct {
    *domain.Entity
    Email     string    `json:"email"`
    Name      string    `json:"name"`
    Active    bool      `json:"active"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// WithEmail creates a new user aggregate
func (u *User) WithEmail(email, name string) *User {
    // Initialize Entity first
    u.Entity = new(domain.Entity).WithID(ksuid.New().String())
    
    if email == "" {
        u.AddError(fmt.Errorf("must specify valid email address"))
        return u
    }
    if name == "" {
        u.AddError(fmt.Errorf("must specify user name"))
        return u
    }

    u.Email = email
    u.Name = name
    u.Active = true
    u.CreatedAt = time.Now()
    u.UpdatedAt = time.Now()
    
    // Add domain event
    u.AddEvent(domain.NewEntityEvent("User", "created", u.ID(), "", "", u))
    
    return u
}

// UpdateEmail updates the user's email
func (u *User) UpdateEmail(newEmail string) *User {
    if newEmail == "" {
        u.AddError(fmt.Errorf("email cannot be empty"))
        return u
    }
    
    u.Email = newEmail
    u.UpdatedAt = time.Now()
    
    // Add domain event
    u.AddEvent(domain.NewEntityEvent("User", "email_updated", u.ID(), "", "", map[string]string{
        "old_email": u.Email,
        "new_email": newEmail,
    }))
    
    return u
}

// Activate activates the user
func (u *User) Activate() *User {
    if u.Active {
        u.AddError(fmt.Errorf("user is already active"))
        return u
    }
    
    u.Active = true
    u.UpdatedAt = time.Now()
    
    // Add domain event
    u.AddEvent(domain.NewEntityEvent("User", "activated", u.ID(), "", "", u))
    
    return u
}

// Deactivate deactivates the user
func (u *User) Deactivate() *User {
    if !u.Active {
        u.AddError(fmt.Errorf("user is already inactive"))
        return u
    }
    
    u.Active = false
    u.UpdatedAt = time.Now()
    
    // Add domain event
    u.AddEvent(domain.NewEntityEvent("User", "deactivated", u.ID(), "", "", u))
    
    return u
}

// LoadFromHistory reconstructs the aggregate state from events
func (u *User) LoadFromHistory(events []domain.Event) {
    // Call base Entity LoadFromHistory to update sequence number
    u.Entity.LoadFromHistory(events)

    for _, event := range events {
        switch e := event.(type) {
        case *domain.EntityEvent:
            switch e.EventType {
            case "created":
                if data, ok := e.Data.(*User); ok {
                    u.Email = data.Email
                    u.Name = data.Name
                    u.Active = data.Active
                    u.CreatedAt = data.CreatedAt
                    u.UpdatedAt = data.UpdatedAt
                }
            case "email_updated":
                if data, ok := e.Data.(map[string]string); ok {
                    u.Email = data["new_email"]
                    u.UpdatedAt = time.Now()
                }
            case "activated":
                u.Active = true
                u.UpdatedAt = time.Now()
            case "deactivated":
                u.Active = false
                u.UpdatedAt = time.Now()
            }
        }
    }
}

// UserRepository defines the interface for user persistence
type UserRepository interface {
    Save(ctx context.Context, user *User) error
    FindByID(ctx context.Context, id string) (*User, error)
    FindByEmail(ctx context.Context, email string) (*User, error)
    List(ctx context.Context, page, pageSize int, active *bool) ([]*User, int, error)
}
```

## Step 2: Commands and Command Handlers

Now let's define our commands and their handlers:

```go
// examples/user_aggregate.go (command methods)
package application

import (
    "regexp"
    "strings"

    "github.com/akeemphilbert/pericarp/pkg/application"
)

// CreateUserCommand represents a command to create a new user
type CreateUserCommand struct {
    ID    string `json:"id"`
    Email string `json:"email"`
    Name  string `json:"name"`
}

func (c CreateUserCommand) CommandType() string {
    return "CreateUser"
}

func (c CreateUserCommand) Validate() error {
    if c.ID == "" {
        return application.NewValidationError("id", "GetID cannot be empty")
    }
    if err := validateEmail(c.Email); err != nil {
        return application.NewValidationError("email", err.Error())
    }
    if err := validateName(c.Name); err != nil {
        return application.NewValidationError("name", err.Error())
    }
    return nil
}

// UpdateUserEmailCommand represents a command to update a user's email
type UpdateUserEmailCommand struct {
    ID       string `json:"id"`
    NewEmail string `json:"new_email"`
}

func (c UpdateUserEmailCommand) CommandType() string {
    return "UpdateUserEmail"
}

func (c UpdateUserEmailCommand) Validate() error {
    if c.ID == "" {
        return application.NewValidationError("id", "GetID cannot be empty")
    }
    if err := validateEmail(c.NewEmail); err != nil {
        return application.NewValidationError("new_email", err.Error())
    }
    return nil
}

// ActivateUserCommand represents a command to activate a user
type ActivateUserCommand struct {
    ID string `json:"id"`
}

func (c ActivateUserCommand) CommandType() string {
    return "ActivateUser"
}

func (c ActivateUserCommand) Validate() error {
    if c.ID == "" {
        return application.NewValidationError("id", "GetID cannot be empty")
    }
    return nil
}

// DeactivateUserCommand represents a command to deactivate a user
type DeactivateUserCommand struct {
    ID string `json:"id"`
}

func (c DeactivateUserCommand) CommandType() string {
    return "DeactivateUser"
}

func (c DeactivateUserCommand) Validate() error {
    if c.ID == "" {
        return application.NewValidationError("id", "GetID cannot be empty")
    }
    return nil
}

// Validation functions
func validateEmail(email string) error {
    if email == "" {
        return application.NewValidationError("email", "email cannot be empty")
    }
    
    email = strings.TrimSpace(email)
    if len(email) > 254 {
        return application.NewValidationError("email", "email cannot exceed 254 characters")
    }
    
    emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
    if !emailRegex.MatchString(email) {
        return application.NewValidationError("email", "invalid email format")
    }
    
    return nil
}

func validateName(name string) error {
    if name == "" {
        return application.NewValidationError("name", "name cannot be empty")
    }
    
    name = strings.TrimSpace(name)
    if len(name) < 2 {
        return application.NewValidationError("name", "name must be at least 2 characters long")
    }
    
    if len(name) > 100 {
        return application.NewValidationError("name", "name cannot exceed 100 characters")
    }
    
    return nil
}
```

Now let's create the command handlers:

```go
// examples/user_aggregate.go (business methods)
package application

import (
    "context"

    "github.com/akeemphilbert/pericarp/examples"
    pkgapp "github.com/akeemphilbert/pericarp/pkg/application"
    pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
    pkginfra "github.com/akeemphilbert/pericarp/pkg/infrastructure"
)

// CreateUserHandler handles CreateUserCommand
type CreateUserHandler struct {
    userRepo internaldomain.UserRepository
}

func NewCreateUserHandler(userRepo internaldomain.UserRepository) *CreateUserHandler {
    return &CreateUserHandler{userRepo: userRepo}
}

func (h *CreateUserHandler) Handle(ctx context.Context, logger pkgdomain.Logger, eventStore pkgdomain.EventStore, eventDispatcher pkgdomain.EventDispatcher, payload pkgapp.Payload[pkgapp.Command]) (pkgapp.Response[any], error) {
    cmd, ok := payload.Data.(CreateUserCommand)
    if !ok {
        return pkgapp.Response[any]{
            Error: pkgapp.NewApplicationError("INVALID_COMMAND", "Expected CreateUserCommand", nil),
        }, nil
    }
    
    logger.Debug("Processing CreateUserCommand", "id", cmd.ID, "email", cmd.Email, "name", cmd.Name)

    // Check if user already exists by email
    existingUser, err := h.userRepo.FindByEmail(ctx, cmd.Email)
    if err == nil && existingUser != nil {
        logger.Warn("User with email already exists", "email", cmd.Email)
        return pkgapp.Response[any]{
            Error: pkgapp.NewApplicationError("EMAIL_ALREADY_EXISTS", "Email is already in use", nil),
        }, nil
    }

    unitOfWork := pkginfra.NewUnitOfWork(eventStore, eventDispatcher)

    // Create new user aggregate
    user := new(internaldomain.User).WithEmail(cmd.Email, cmd.Name)
    if user.IsValid() {
        // Register events with unit of work
        unitOfWork.RegisterEvents(user.UncommittedEvents())
        // Commit unit of work (persist and dispatch events)
        var envelopes []pkgdomain.Envelope
        envelopes, err = unitOfWork.Commit(ctx)
        if err != nil {
            logger.Error("Failed to commit unit of work", "id", cmd.ID, "error", err)
            unitOfWork.Rollback()
            return pkgapp.Response[any]{
                Error: err,
            }, nil
        }
        eventDispatcher.Dispatch(ctx, envelopes)
    } else {
        // Handle validation errors
        errors := user.Errors()
        if len(errors) > 0 {
            logger.Error("User validation failed", "id", cmd.ID, "errors", errors)
            return pkgapp.Response[any]{
                Error: pkgapp.NewApplicationError("USER_VALIDATION_FAILED", "User validation failed", errors[0]),
            }, nil
        }
    }

    return pkgapp.Response[any]{
        Data: pkgapp.CommandResponse{
            Code:    200,
            Message: "User created successfully",
            Payload: map[string]string{"user_id": user.ID()},
        },
    }, nil
}

// Similar handlers for UpdateUserEmailHandler, ActivateUserHandler, DeactivateUserHandler
// ... (implementation details similar to CreateUserHandler)
```

## Step 3: Queries and Query Handlers

Now let's define our queries and their handlers:

```go
// examples/user_aggregate.go (query methods)
package application

import (
    "time"

    "github.com/akeemphilbert/pericarp/pkg/application"
    "github.com/segmentio/ksuid"
)

// GetUserQuery represents a query to get a single user by GetID
type GetUserQuery struct {
    ID string `json:"id"`
}

func (q GetUserQuery) QueryType() string {
    return "GetUser"
}

func (q GetUserQuery) Validate() error {
    if q.ID == "" {
        return application.NewValidationError("id", "GetID cannot be empty")
    }
    return nil
}

// GetUserByEmailQuery represents a query to get a single user by email
type GetUserByEmailQuery struct {
    Email string `json:"email"`
}

func (q GetUserByEmailQuery) QueryType() string {
    return "GetUserByEmail"
}

func (q GetUserByEmailQuery) Validate() error {
    if q.Email == "" {
        return application.NewValidationError("email", "email cannot be empty")
    }
    return nil
}

// ListUsersQuery represents a query to list users with pagination
type ListUsersQuery struct {
    Page     int   `json:"page"`
    PageSize int   `json:"page_size"`
    Active   *bool `json:"active,omitempty"` // Filter by active status
}

func (q ListUsersQuery) QueryType() string {
    return "ListUsers"
}

func (q ListUsersQuery) Validate() error {
    if q.Page < 1 {
        return application.NewValidationError("page", "page must be greater than 0")
    }
    if q.PageSize < 1 {
        return application.NewValidationError("page_size", "page_size must be greater than 0")
    }
    if q.PageSize > 100 {
        return application.NewValidationError("page_size", "page_size cannot exceed 100")
    }
    return nil
}

// UserDTO represents a user data transfer object for queries
type UserDTO struct {
    ID        ksuid.KSUID `json:"id"`
    Email     string      `json:"email"`
    Name      string      `json:"name"`
    IsActive  bool        `json:"is_active"`
    CreatedAt time.Time   `json:"created_at"`
    UpdatedAt time.Time   `json:"updated_at"`
}

// ListUsersResult represents the result of a list users query
type ListUsersResult struct {
    Users      []UserDTO `json:"users"`
    Page       int       `json:"page"`
    PageSize   int       `json:"page_size"`
    TotalCount int       `json:"total_count"`
    TotalPages int       `json:"total_pages"`
}
```

## Step 4: Read Model and Projections

For queries, we'll use a read model that's updated by event projections:

```go
// examples/user_aggregate.go (read model methods)
package application

import (
    "context"
    "time"

    "github.com/segmentio/ksuid"
)

// UserReadModel represents the read model for user queries
type UserReadModel struct {
    ID        ksuid.KSUID `json:"id"`
    Email     string      `json:"email"`
    Name      string      `json:"name"`
    IsActive  bool        `json:"is_active"`
    CreatedAt time.Time   `json:"created_at"`
    UpdatedAt time.Time   `json:"updated_at"`
}

func (u *UserReadModel) ToDTO() UserDTO {
    return UserDTO{
        ID:        u.ID,
        Email:     u.Email,
        Name:      u.Name,
        IsActive:  u.IsActive,
        CreatedAt: u.CreatedAt,
        UpdatedAt: u.UpdatedAt,
    }
}

// UserReadModelRepository defines the interface for read model persistence
type UserReadModelRepository interface {
    GetByID(ctx context.Context, id string) (*UserReadModel, error)
    GetByEmail(ctx context.Context, email string) (*UserReadModel, error)
    List(ctx context.Context, page, pageSize int, active *bool) ([]*UserReadModel, int, error)
    Save(ctx context.Context, user *UserReadModel) error
    Update(ctx context.Context, user *UserReadModel) error
}
```

## Step 5: Event Projections

We need to update the read model when domain events occur:

```go
// examples/user_aggregate.go (projection methods)
package application

import (
    "context"
    "time"

    "github.com/akeemphilbert/pericarp/pkg/domain"
    "github.com/segmentio/ksuid"
)

// UserProjector handles user domain events and updates the read model
type UserProjector struct {
    readModelRepo UserReadModelRepository
}

func NewUserProjector(readModelRepo UserReadModelRepository) *UserProjector {
    return &UserProjector{readModelRepo: readModelRepo}
}

func (p *UserProjector) Handle(ctx context.Context, event domain.Event) error {
    switch e := event.(type) {
    case *domain.EntityEvent:
        switch e.EventType {
        case "created":
            return p.handleUserCreated(ctx, e)
        case "email_updated":
            return p.handleUserEmailUpdated(ctx, e)
        case "activated":
            return p.handleUserActivated(ctx, e)
        case "deactivated":
            return p.handleUserDeactivated(ctx, e)
        }
    }
    return nil
}

func (p *UserProjector) handleUserCreated(ctx context.Context, event *domain.EntityEvent) error {
    if userData, ok := event.Data.(*User); ok {
        readModel := &UserReadModel{
            ID:        ksuid.MustParse(event.AggregateID()),
            Email:     userData.Email,
            Name:      userData.Name,
            IsActive:  userData.Active,
            CreatedAt: userData.CreatedAt,
            UpdatedAt: userData.UpdatedAt,
        }
        return p.readModelRepo.Save(ctx, readModel)
    }
    return nil
}

// Similar handlers for other events...
```

## Step 6: CLI Interface

Now let's create a CLI interface to interact with our system:

```go
// cmd/demo/main.go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    "github.com/akeemphilbert/pericarp/examples"
    "github.com/akeemphilbert/pericarp/pkg"
    pkgapp "github.com/akeemphilbert/pericarp/pkg/application"
    "github.com/akeemphilbert/pericarp/pkg/domain"
    "github.com/segmentio/ksuid"
    "github.com/spf13/cobra"
    "go.uber.org/fx"
)

func main() {
    rootCmd := &cobra.Command{
        Use:   "pericarp-demo",
        Short: "Pericarp library demonstration CLI",
        Long: `A demonstration CLI application showcasing the Pericarp library's
Domain-Driven Design, CQRS, and Event Sourcing capabilities.`,
    }

    // Add commands
    rootCmd.AddCommand(createUserCmd())
    rootCmd.AddCommand(updateUserCmd())
    rootCmd.AddCommand(activateUserCmd())
    rootCmd.AddCommand(deactivateUserCmd())
    rootCmd.AddCommand(getUserCmd())
    rootCmd.AddCommand(listUsersCmd())

    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}

// createUserCmd creates a new user
func createUserCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "create-user <email> <name>",
        Short: "Create a new user",
        Long:  "Create a new user with the specified email and name",
        Args:  cobra.ExactArgs(2),
        RunE: func(cmd *cobra.Command, args []string) error {
            email := args[0]
            name := args[1]

            return runWithApp(func(ctx context.Context, logger domain.Logger, commandBus pkgapp.CommandBus, eventStore pkgdomain.EventStore, eventDispatcher pkgdomain.EventDispatcher) error {
                userID := ksuid.New().String()
                command := application.CreateUserCommand{
                    ID:    userID,
                    Email: email,
                    Name:  name,
                }

                if err := command.Validate(); err != nil {
                    return fmt.Errorf("validation failed: %w", err)
                }

                logger.Info("Creating user", "id", userID, "email", email, "name", name)

                payload := pkgapp.Payload[pkgapp.Command]{
                    Data: command,
                }

                if _, err := commandBus.Handle(ctx, logger, eventStore, eventDispatcher, payload); err != nil {
                    return fmt.Errorf("failed to create user: %w", err)
                }

                fmt.Printf("✅ User created successfully!\n")
                fmt.Printf("   GetID: %s\n", userID)
                fmt.Printf("   Email: %s\n", email)
                fmt.Printf("   Name: %s\n", name)
                return nil
            })
        },
    }
    return cmd
}

// Similar commands for other operations...
```

## Step 7: Running the Application

Build and run the demo application:

```bash
# Build the demo
go build -o pericarp-demo cmd/demo/main.go

# Initialize the database
./pericarp-demo init-db

# Create a user
./pericarp-demo create-user john@example.com "John Doe"

# List users
./pericarp-demo list-users

# Get user by GetID
./pericarp-demo get-user by-id <user-id>

# Update user email
./pericarp-demo update-user email <user-id> newemail@example.com

# Activate/deactivate user
./pericarp-demo activate-user <user-id>
./pericarp-demo deactivate-user <user-id>
```

## What You've Built

Congratulations! You've built a complete user management system that demonstrates:

### 1. Domain-Driven Design
- **User Aggregate**: Contains business logic and invariants
- **Domain Events**: Capture what happened in the domain
- **Repository Pattern**: Abstracts data access

### 2. CQRS (Command Query Responsibility Segregation)
- **Commands**: Change state (CreateUser, UpdateUserEmail, etc.)
- **Queries**: Read data (GetUser, ListUsers, etc.)
- **Separate Handlers**: Different handlers for commands and queries

### 3. Event Sourcing
- **Event Store**: Persists all domain events
- **Event Replay**: Reconstructs aggregates from events
- **Event Projections**: Updates read models from events

### 4. Clean Architecture
- **Domain Layer**: Business logic and rules
- **Application Layer**: Use cases and orchestration
- **Infrastructure Layer**: Technical concerns
- **Interface Layer**: CLI and API interfaces

## Key Benefits

1. **Testability**: Each layer can be tested independently
2. **Maintainability**: Clear separation of concerns
3. **Scalability**: Read and write models can be scaled independently
4. **Auditability**: Complete event history for all changes
5. **Flexibility**: Easy to add new features and projections

## Next Steps

Now that you have a complete user management system, you can:

1. **Add more features**: User roles, permissions, etc.
2. **Implement HTTP API**: Add REST endpoints
3. **Add more projections**: User statistics, reports, etc.
4. **Implement sagas**: Handle complex workflows
5. **Add monitoring**: Metrics, logging, tracing

Ready to learn more? Check out the [How-to Guides](../how-to/README.md) for specific implementation patterns!
