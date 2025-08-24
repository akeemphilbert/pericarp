// Package application implements the application layer of the clean architecture,
// providing CQRS (Command Query Responsibility Segregation) patterns with unified
// handler signatures and middleware support.
//
// This package provides:
//   - Command and Query handling with unified signatures
//   - Middleware support for cross-cutting concerns
//   - Event handling for projections and sagas
//   - Application services for coordinating domain operations
//
// The application layer coordinates between the domain layer (business logic)
// and infrastructure layer (technical concerns) without containing business
// logic itself.
//
// Key patterns implemented:
//   - CQRS with separate command and query buses
//   - Unified handler signatures for middleware reusability
//   - Decorator pattern for middleware composition
//   - Application services for use case orchestration
package application

import (
	"context"

	"github.com/example/pericarp/pkg/domain"
)

// Payload wraps request data with metadata for unified handler signatures.
// This enables the same middleware to work with both commands and queries
// by providing a consistent structure for request data and metadata.
//
// The payload includes:
//   - Data: The actual command or query
//   - Metadata: Additional context information
//   - TraceID: For distributed tracing
//   - UserID: For authorization and auditing
//
// Example usage:
//
//	payload := Payload[CreateUserCommand]{
//	    Data: CreateUserCommand{Email: "user@example.com", Name: "John"},
//	    Metadata: map[string]any{"source": "web"},
//	    TraceID: "trace-123",
//	    UserID: "admin-456",
//	}
type Payload[T any] struct {
	// Data contains the actual command or query being processed
	Data T

	// Metadata contains additional context information that may be
	// useful for middleware or handlers (e.g., correlation IDs, feature flags)
	Metadata map[string]any

	// TraceID is used for distributed tracing to track requests across services
	TraceID string

	// UserID identifies the user making the request for authorization and auditing
	UserID string
}

// Response wraps response data with metadata for unified handler signatures.
// This provides a consistent structure for handler responses and enables
// middleware to add metadata to responses.
//
// The response includes:
//   - Data: The actual response data (empty struct{} for commands)
//   - Metadata: Additional information about the response
//   - Error: Any error that occurred during processing
//
// Example usage:
//
//	// Command response (no data)
//	return Response[struct{}]{
//	    Data: struct{}{},
//	    Metadata: map[string]any{"version": aggregate.Version()},
//	}, nil
//
//	// Query response (with data)
//	return Response[UserView]{
//	    Data: UserView{ID: user.ID(), Email: user.Email()},
//	    Metadata: map[string]any{"cached": false},
//	}, nil
type Response[T any] struct {
	// Data contains the response data. For commands, this is typically struct{}.
	// For queries, this contains the requested data.
	Data T

	// Metadata contains additional information about the response that may be
	// useful for clients or middleware (e.g., cache status, version info)
	Metadata map[string]any

	// Error contains any error that occurred during processing.
	// This allows middleware to inspect and potentially modify errors.
	Error error
}

// Command represents an intention to change the system state.
// Commands are imperative and should use verb-noun naming (CreateUser, UpdateEmail).
//
// Commands should:
//   - Represent business intentions, not technical operations
//   - Be immutable once created
//   - Contain all data needed to perform the operation
//   - Use validation to ensure data integrity
//
// Example implementation:
//
//	type CreateUserCommand struct {
//	    Email string `json:"email" validate:"required,email"`
//	    Name  string `json:"name" validate:"required,min=1,max=100"`
//	}
//
//	func (c CreateUserCommand) CommandType() string { return "CreateUser" }
//
//	func (c CreateUserCommand) Validate() error {
//	    if c.Email == "" {
//	        return errors.New("email is required")
//	    }
//	    if c.Name == "" {
//	        return errors.New("name is required")
//	    }
//	    return nil
//	}
type Command interface {
	// CommandType returns a unique identifier for this command type.
	// This is used by the command bus to route commands to appropriate handlers.
	// Should be stable across versions (e.g., "CreateUser", "UpdateUserEmail").
	CommandType() string
}

// Query represents a request for information from the system.
// Queries are interrogative and should use question-like naming (GetUser, FindOrders).
//
// Queries should:
//   - Not change system state (read-only operations)
//   - Be immutable once created
//   - Contain all parameters needed to retrieve the data
//   - Use validation to ensure parameter correctness
//
// Example implementation:
//
//	type GetUserQuery struct {
//	    UserID string `json:"user_id" validate:"required"`
//	}
//
//	func (q GetUserQuery) QueryType() string { return "GetUser" }
//
//	func (q GetUserQuery) Validate() error {
//	    if q.UserID == "" {
//	        return errors.New("user ID is required")
//	    }
//	    return nil
//	}
type Query interface {
	// QueryType returns a unique identifier for this query type.
	// This is used by the query bus to route queries to appropriate handlers.
	// Should be stable across versions (e.g., "GetUser", "ListOrders").
	QueryType() string
}

// Handler is the unified handler signature for both commands and queries.
// This unified approach enables the same middleware to work with both command
// and query handlers, reducing code duplication and improving consistency.
//
// The handler signature includes:
//   - Context for cancellation and timeouts
//   - Logger for structured logging
//   - Payload wrapper with request data and metadata
//   - Response wrapper with result data and metadata
//
// Example command handler:
//
//	func (h *CreateUserHandler) Handle(
//	    ctx context.Context, 
//	    log domain.Logger, 
//	    p Payload[CreateUserCommand],
//	) (Response[struct{}], error) {
//	    log.Info("Creating user", "email", p.Data.Email)
//	    
//	    user, err := domain.NewUser(p.Data.Email, p.Data.Name)
//	    if err != nil {
//	        return Response[struct{}]{Error: err}, err
//	    }
//	    
//	    if err := h.userRepo.Save(ctx, user); err != nil {
//	        return Response[struct{}]{Error: err}, err
//	    }
//	    
//	    return Response[struct{}]{
//	        Data: struct{}{},
//	        Metadata: map[string]any{"userId": user.ID()},
//	    }, nil
//	}
//
// Example query handler:
//
//	func (h *GetUserHandler) Handle(
//	    ctx context.Context, 
//	    log domain.Logger, 
//	    p Payload[GetUserQuery],
//	) (Response[UserView], error) {
//	    log.Debug("Getting user", "userId", p.Data.UserID)
//	    
//	    user, err := h.userRepo.Load(ctx, p.Data.UserID)
//	    if err != nil {
//	        return Response[UserView]{Error: err}, err
//	    }
//	    
//	    view := UserView{ID: user.ID(), Email: user.Email()}
//	    return Response[UserView]{
//	        Data: view,
//	        Metadata: map[string]any{"version": user.Version()},
//	    }, nil
//	}
type Handler[Req any, Res any] func(ctx context.Context, log domain.Logger, p Payload[Req]) (Response[Res], error)

// EventHandler processes events (projectors/sagas)
// This interface is defined here for application layer event handling
type EventHandler interface {
	// Handle processes a single event envelope
	Handle(ctx context.Context, envelope domain.Envelope) error

	// EventTypes returns the list of event types this handler can process
	EventTypes() []string
}

// Unified middleware that works for both commands and queries
type Middleware[Req any, Res any] func(next Handler[Req, Res]) Handler[Req, Res]

// Handler function types for bus registration
type CommandHandlerFunc Handler[Command, struct{}]
type QueryHandlerFunc Handler[Query, any]

// CommandBus with unified middleware support
type CommandBus interface {
	Handle(ctx context.Context, logger domain.Logger, cmd Command) error
	Register(cmdType string, handler Handler[Command, struct{}], middleware ...Middleware[Command, struct{}])
}

// QueryBus with unified middleware support
type QueryBus interface {
	Handle(ctx context.Context, logger domain.Logger, query Query) (any, error)
	Register(queryType string, handler Handler[Query, any], middleware ...Middleware[Query, any])
}
