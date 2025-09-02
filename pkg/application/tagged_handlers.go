package application

// No imports needed for the interface definitions

// TaggedCommandHandler represents a command handler with metadata for registration
type TaggedCommandHandler struct {
	CommandType string
	Handler     Handler[Command, any]
}

// TaggedQueryHandler represents a query handler with metadata for registration
type TaggedQueryHandler struct {
	QueryType string
	Handler   Handler[Query, any]
}

// TaggedCommandMiddleware represents middleware with metadata
type TaggedCommandMiddleware struct {
	Name       string
	Middleware Middleware[Command, any]
}

// TaggedQueryMiddleware represents query middleware with metadata
type TaggedQueryMiddleware struct {
	Name       string
	Middleware Middleware[Query, any]
}

// HandlerRegistrar defines the interface for registering tagged handlers
type HandlerRegistrar interface {
	RegisterCommandHandlers(bus CommandBus, handlers []TaggedCommandHandler, middleware []TaggedCommandMiddleware)
	RegisterQueryHandlers(bus QueryBus, handlers []TaggedQueryHandler, middleware []TaggedQueryMiddleware)
}

// DefaultHandlerRegistrar provides default registration logic
type DefaultHandlerRegistrar struct{}

// RegisterCommandHandlers registers command handlers with their middleware
func (r *DefaultHandlerRegistrar) RegisterCommandHandlers(
	bus CommandBus,
	handlers []TaggedCommandHandler,
	middleware []TaggedCommandMiddleware,
) {
	// Convert tagged middleware to middleware slice
	middlewares := make([]Middleware[Command, any], len(middleware))
	for i, mw := range middleware {
		middlewares[i] = mw.Middleware
	}

	// Register each handler with the middleware stack
	for _, handler := range handlers {
		bus.Register(handler.CommandType, handler.Handler, middlewares...)
	}
}

// RegisterQueryHandlers registers query handlers with their middleware
func (r *DefaultHandlerRegistrar) RegisterQueryHandlers(
	bus QueryBus,
	handlers []TaggedQueryHandler,
	middleware []TaggedQueryMiddleware,
) {
	// Convert tagged middleware to middleware slice
	middlewares := make([]Middleware[Query, any], len(middleware))
	for i, mw := range middleware {
		middlewares[i] = mw.Middleware
	}

	// Register each handler with the middleware stack
	for _, handler := range handlers {
		bus.Register(handler.QueryType, handler.Handler, middlewares...)
	}
}
