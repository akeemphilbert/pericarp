package application

import (
	"context"

	"github.com/example/pericarp/pkg/domain"
)

// Example showing how to use the unified Middleware type for both commands and queries

// Example command
type ExampleCommand struct {
	Message string
}

func (c ExampleCommand) CommandType() string { return "ExampleCommand" }

// Example query
type ExampleQuery struct {
	ID string
}

func (q ExampleQuery) QueryType() string { return "ExampleQuery" }

// Example command handler using unified Handler type
func ExampleCommandHandler(ctx context.Context, log domain.Logger, p Payload[ExampleCommand]) (Response[struct{}], error) {
	log.Info("Processing example command", "message", p.Data.Message, "traceId", p.TraceID)

	// Process the command...

	return Response[struct{}]{
		Data: struct{}{},
		Metadata: map[string]any{
			"processed_at": "2024-01-01T00:00:00Z",
		},
	}, nil
}

// Example query handler using unified Handler type
func ExampleQueryHandler(ctx context.Context, log domain.Logger, p Payload[ExampleQuery]) (Response[string], error) {
	log.Info("Processing example query", "id", p.Data.ID, "traceId", p.TraceID)

	// Process the query...
	result := "Example result for ID: " + p.Data.ID

	return Response[string]{
		Data: result,
		Metadata: map[string]any{
			"query_time": "2024-01-01T00:00:00Z",
		},
	}, nil
}

// Example of how to register handlers with the buses using unified Middleware
func ExampleUnifiedMiddlewareRegistration() {
	commandBus := NewCommandBus()
	queryBus := NewQueryBus()

	// Register command handler with unified middleware
	commandBus.Register("ExampleCommand",
		Handler[Command, struct{}](func(ctx context.Context, log domain.Logger, p Payload[Command]) (Response[struct{}], error) {
			// Type assertion to get the specific command
			if cmd, ok := p.Data.(ExampleCommand); ok {
				// Create a new payload with the specific command type
				specificPayload := Payload[ExampleCommand]{
					Data:     cmd,
					Metadata: p.Metadata,
					TraceID:  p.TraceID,
					UserID:   p.UserID,
				}
				return ExampleCommandHandler(ctx, log, specificPayload)
			}
			return Response[struct{}]{}, NewApplicationError("INVALID_COMMAND", "Invalid command type", nil)
		}),
		// Using unified Middleware type - no need for CommandMiddleware wrapper
		LoggingMiddleware[Command, struct{}](),
		ValidationMiddleware[Command, struct{}](),
	)

	// Register query handler with unified middleware
	queryBus.Register("ExampleQuery",
		Handler[Query, any](func(ctx context.Context, log domain.Logger, p Payload[Query]) (Response[any], error) {
			// Type assertion to get the specific query
			if query, ok := p.Data.(ExampleQuery); ok {
				// Create a new payload with the specific query type
				specificPayload := Payload[ExampleQuery]{
					Data:     query,
					Metadata: p.Metadata,
					TraceID:  p.TraceID,
					UserID:   p.UserID,
				}
				response, err := ExampleQueryHandler(ctx, log, specificPayload)
				if err != nil {
					return Response[any]{}, err
				}
				return Response[any]{
					Data:     response.Data,
					Metadata: response.Metadata,
					Error:    response.Error,
				}, nil
			}
			return Response[any]{}, NewApplicationError("INVALID_QUERY", "Invalid query type", nil)
		}),
		// Using unified Middleware type - no need for QueryMiddleware wrapper
		LoggingMiddleware[Query, any](),
		ValidationMiddleware[Query, any](),
	)
}
