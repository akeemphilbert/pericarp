package application

import (
	"context"

	"github.com/example/pericarp/pkg/domain"
)

// commandBus implements CommandBus with middleware support
type commandBus struct {
	handlers    map[string]CommandHandler[Command]
	middlewares []CommandMiddleware
}

// NewCommandBus creates a new command bus instance
func NewCommandBus() CommandBus {
	return &commandBus{
		handlers:    make(map[string]CommandHandler[Command]),
		middlewares: make([]CommandMiddleware, 0),
	}
}

// Use registers middleware to be applied to all command handlers
func (b *commandBus) Use(middleware ...CommandMiddleware) {
	b.middlewares = append(b.middlewares, middleware...)
}

// Handle processes a command through the middleware chain and handler
func (b *commandBus) Handle(ctx context.Context, logger domain.Logger, cmd Command) error {
	handler, exists := b.handlers[cmd.CommandType()]
	if !exists {
		return NewHandlerNotFoundError(cmd.CommandType(), "command")
	}

	// Build middleware chain
	handlerFunc := func(ctx context.Context, logger domain.Logger, cmd Command) error {
		return handler.Handle(ctx, logger, cmd)
	}

	// Apply middleware in reverse order (like Echo framework)
	for i := len(b.middlewares) - 1; i >= 0; i-- {
		handlerFunc = b.middlewares[i](handlerFunc)
	}

	return handlerFunc(ctx, logger, cmd)
}

// Register associates a command type with its handler
func (b *commandBus) Register(cmdType string, handler CommandHandler[Command]) {
	b.handlers[cmdType] = handler
}

// queryBus implements QueryBus with middleware support
type queryBus struct {
	handlers    map[string]QueryHandler[Query, interface{}]
	middlewares []QueryMiddleware
}

// NewQueryBus creates a new query bus instance
func NewQueryBus() QueryBus {
	return &queryBus{
		handlers:    make(map[string]QueryHandler[Query, interface{}]),
		middlewares: make([]QueryMiddleware, 0),
	}
}

// Use registers middleware to be applied to all query handlers
func (q *queryBus) Use(middleware ...QueryMiddleware) {
	q.middlewares = append(q.middlewares, middleware...)
}

// Handle processes a query through the middleware chain and handler
func (q *queryBus) Handle(ctx context.Context, logger domain.Logger, query Query) (interface{}, error) {
	handler, exists := q.handlers[query.QueryType()]
	if !exists {
		return nil, NewHandlerNotFoundError(query.QueryType(), "query")
	}

	// Build middleware chain
	handlerFunc := func(ctx context.Context, logger domain.Logger, query Query) (interface{}, error) {
		return handler.Handle(ctx, logger, query)
	}

	// Apply middleware in reverse order (like Echo framework)
	for i := len(q.middlewares) - 1; i >= 0; i-- {
		handlerFunc = q.middlewares[i](handlerFunc)
	}

	return handlerFunc(ctx, logger, query)
}

// Register associates a query type with its handler
func (q *queryBus) Register(queryType string, handler QueryHandler[Query, interface{}]) {
	q.handlers[queryType] = handler
}
