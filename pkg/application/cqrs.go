package application

import (
	"context"

	"github.com/example/pericarp/pkg/domain"
)

// Command marker interface
type Command interface {
	CommandType() string
}

// Query marker interface
type Query interface {
	QueryType() string
}

// CommandHandler processes commands with logger injection
// Context and logger are the first two parameters as required
type CommandHandler[T Command] interface {
	Handle(ctx context.Context, logger domain.Logger, cmd T) error
}

// QueryHandler processes queries with logger injection
// Context and logger are the first two parameters as required
type QueryHandler[T Query, R any] interface {
	Handle(ctx context.Context, logger domain.Logger, query T) (R, error)
}

// EventHandler processes events (projectors/sagas)
// This interface is defined here for application layer event handling
type EventHandler interface {
	// Handle processes a single event envelope
	Handle(ctx context.Context, envelope domain.Envelope) error

	// EventTypes returns the list of event types this handler can process
	EventTypes() []string
}

// Middleware function types with logger parameter
type CommandHandlerFunc func(ctx context.Context, logger domain.Logger, cmd Command) error
type QueryHandlerFunc func(ctx context.Context, logger domain.Logger, query Query) (interface{}, error)

type CommandMiddleware func(next CommandHandlerFunc) CommandHandlerFunc
type QueryMiddleware func(next QueryHandlerFunc) QueryHandlerFunc

// CommandBus with middleware support and logger injection
type CommandBus interface {
	Use(middleware ...CommandMiddleware)
	Handle(ctx context.Context, logger domain.Logger, cmd Command) error
	Register(cmdType string, handler CommandHandler[Command])
}

// QueryBus with middleware support and logger injection
type QueryBus interface {
	Use(middleware ...QueryMiddleware)
	Handle(ctx context.Context, logger domain.Logger, query Query) (interface{}, error)
	Register(queryType string, handler QueryHandler[Query, interface{}])
}
