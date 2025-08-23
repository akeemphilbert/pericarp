package main

import (
	"context"
	"log"

	"github.com/example/pericarp/pkg"
	"github.com/example/pericarp/pkg/application"
	"github.com/example/pericarp/pkg/domain"
	"go.uber.org/fx"
)

// Example command
type ExampleCommand struct {
	Message string
}

func (c ExampleCommand) CommandType() string {
	return "ExampleCommand"
}

// Example command handler
type ExampleCommandHandler struct {
	unitOfWork domain.UnitOfWork
}

func (h *ExampleCommandHandler) Handle(ctx context.Context, logger domain.Logger, cmd ExampleCommand) error {
	logger.Info("Handling example command", "message", cmd.Message)
	return nil
}

// Example query
type ExampleQuery struct {
	ID string
}

func (q ExampleQuery) QueryType() string {
	return "ExampleQuery"
}

// Example query handler
type ExampleQueryHandler struct{}

func (h *ExampleQueryHandler) Handle(ctx context.Context, logger domain.Logger, query ExampleQuery) (string, error) {
	logger.Info("Handling example query", "id", query.ID)
	return "Example result for " + query.ID, nil
}

func main() {
	app := pkg.NewApp(
		// Register our example handlers
		fx.Provide(
			func(unitOfWork domain.UnitOfWork) *ExampleCommandHandler {
				return &ExampleCommandHandler{unitOfWork: unitOfWork}
			},
			func() *ExampleQueryHandler {
				return &ExampleQueryHandler{}
			},
		),
		fx.Invoke(func(
			commandBus application.CommandBus,
			queryBus application.QueryBus,
			logger domain.Logger,
			cmdHandler *ExampleCommandHandler,
			queryHandler *ExampleQueryHandler,
		) {
			// Register handlers with buses
			commandBus.Register("ExampleCommand", func(ctx context.Context, logger domain.Logger, cmd application.Command) error {
				return cmdHandler.Handle(ctx, logger, cmd.(ExampleCommand))
			})

			queryBus.Register("ExampleQuery", func(ctx context.Context, logger domain.Logger, query application.Query) (interface{}, error) {
				return queryHandler.Handle(ctx, logger, query.(ExampleQuery))
			})

			// Example usage
			ctx := context.Background()

			// Execute a command
			exampleCmd := ExampleCommand{Message: "Hello from Pericarp!"}
			if err := commandBus.Handle(ctx, logger, exampleCmd); err != nil {
				logger.Error("Command failed", "error", err)
			}

			// Execute a query
			exampleQuery := ExampleQuery{ID: "123"}
			result, err := queryBus.Handle(ctx, logger, exampleQuery)
			if err != nil {
				logger.Error("Query failed", "error", err)
			} else {
				logger.Info("Query result", "result", result)
			}

			log.Println("Example completed successfully!")
		}),
	)

	app.Run()
}