package application

import (
	"context"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// commandBus implements CommandBus with unified handler support
type commandBus struct {
	handlers map[string]CommandHandlerFunc
}

// NewCommandBus creates a new command bus instance
func NewCommandBus() CommandBus {
	return &commandBus{
		handlers: make(map[string]CommandHandlerFunc),
	}
}

// Handle processes a command through the registered handler with its middleware chain
func (b *commandBus) Handle(ctx context.Context, logger domain.Logger, eventStore domain.EventStore, eventDispatcher domain.EventDispatcher, cmd Command) error {
	handlerFunc, exists := b.handlers[cmd.CommandType()]
	if !exists {
		return NewHandlerNotFoundError(cmd.CommandType(), "command")
	}

	// Wrap command in Payload
	payload := Payload[Command]{
		Data:     cmd,
		Metadata: make(map[string]any),
		TraceID:  "", // Could be extracted from context
		UserID:   "", // Could be extracted from context
	}

	response, err := handlerFunc(ctx, logger, eventStore, eventDispatcher, payload)
	if err != nil {
		return err
	}

	// Check if response contains an error
	if response.Error != nil {
		return response.Error
	}

	return nil
}

// Register associates a command type with its handler and applies middleware in the order provided
func (b *commandBus) Register(cmdType string, handler Handler[Command, any], middleware ...Middleware[Command, any]) {
	// Start with the base handler function
	handlerFunc := handler

	// Apply middleware in reverse order (like Echo framework) so they execute in the order provided
	for i := len(middleware) - 1; i >= 0; i-- {
		handlerFunc = middleware[i](handlerFunc)
	}

	b.handlers[cmdType] = CommandHandlerFunc(handlerFunc)
}

// queryBus implements QueryBus with unified handler support
type queryBus struct {
	handlers map[string]QueryHandlerFunc
}

// NewQueryBus creates a new query bus instance
func NewQueryBus() QueryBus {
	return &queryBus{
		handlers: make(map[string]QueryHandlerFunc),
	}
}

// Handle processes a query through the registered handler with its middleware chain
func (q *queryBus) Handle(ctx context.Context, logger domain.Logger, eventStore domain.EventStore, eventDispatcher domain.EventDispatcher, query Query) (any, error) {
	handlerFunc, exists := q.handlers[query.QueryType()]
	if !exists {
		return nil, NewHandlerNotFoundError(query.QueryType(), "query")
	}

	// Wrap query in Payload
	payload := Payload[Query]{
		Data:     query,
		Metadata: make(map[string]any),
		TraceID:  "", // Could be extracted from context
		UserID:   "", // Could be extracted from context
	}

	response, err := handlerFunc(ctx, logger, eventStore, eventDispatcher, payload)
	if err != nil {
		return nil, err
	}

	// Check if response contains an error
	if response.Error != nil {
		return nil, response.Error
	}

	return response.Data, nil
}

// Register associates a query type with its handler and applies middleware in the order provided
func (q *queryBus) Register(queryType string, handler Handler[Query, any], middleware ...Middleware[Query, any]) {
	// Start with the base handler function
	handlerFunc := handler

	// Apply middleware in reverse order (like Echo framework) so they execute in the order provided
	for i := len(middleware) - 1; i >= 0; i-- {
		handlerFunc = middleware[i](handlerFunc)
	}

	q.handlers[queryType] = QueryHandlerFunc(handlerFunc)
}
