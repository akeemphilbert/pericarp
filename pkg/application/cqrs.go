package application

import (
	"context"

	"github.com/example/pericarp/pkg/domain"
)

// Payload wraps request data with metadata for unified handler signature
type Payload[T any] struct {
	Data     T
	Metadata map[string]any
	TraceID  string
	UserID   string
}

// Response wraps response data with metadata for unified handler signature
type Response[T any] struct {
	Data     T
	Metadata map[string]any
	Error    error
}

// Command marker interface
type Command interface {
	CommandType() string
}

// Query marker interface
type Query interface {
	QueryType() string
}

// Handler is the unified handler signature for both commands and queries
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
