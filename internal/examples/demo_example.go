package examples

import (
	"github.com/akeemphilbert/pericarp/internal/application"
	"github.com/akeemphilbert/pericarp/internal/domain"
	"github.com/akeemphilbert/pericarp/internal/infrastructure"
	pkgapp "github.com/akeemphilbert/pericarp/pkg/application"
	pkginfra "github.com/akeemphilbert/pericarp/pkg/infrastructure"
)

// DemoRegistrationExample shows how to register handlers with different middleware combinations
func DemoRegistrationExample() {
	// Create buses
	commandBus := pkgapp.NewCommandBus()
	queryBus := pkgapp.NewQueryBus()

	// Mock dependencies
	var userRepo domain.UserRepository
	var unitOfWork pkginfra.UnitOfWork
	var readModelRepo application.UserReadModelRepository
	var metrics pkgapp.MetricsCollector
	var cache pkgapp.CacheProvider

	// Create handlers
	createUserHandler := application.NewCreateUserHandler(userRepo, unitOfWork)
	getUserHandler := application.NewGetUserHandler(readModelRepo)

	// Example 1: Register a command handler with full middleware stack
	commandBus.Register("CreateUser",
		&createUserCommandHandlerAdapter{handler: createUserHandler},
		pkgapp.ErrorHandlingCommandMiddleware(),  // First - handles panics and wraps errors
		pkgapp.LoggingCommandMiddleware(),        // Second - logs command execution
		pkgapp.ValidationCommandMiddleware(),     // Third - validates commands
		pkgapp.MetricsCommandMiddleware(metrics), // Last - collects metrics
	)

	// Example 2: Register a command handler with only basic middleware (no metrics)
	commandBus.Register("CreateUserBasic",
		&createUserCommandHandlerAdapter{handler: createUserHandler},
		pkgapp.ErrorHandlingCommandMiddleware(), // First - handles panics and wraps errors
		pkgapp.LoggingCommandMiddleware(),       // Second - logs command execution
		pkgapp.ValidationCommandMiddleware(),    // Third - validates commands
	)

	// Example 3: Register a query handler with caching for public queries
	queryBus.Register("GetUser",
		&getUserQueryHandlerAdapter{handler: getUserHandler},
		pkgapp.ErrorHandlingQueryMiddleware(),  // First - handles panics and wraps errors
		pkgapp.LoggingQueryMiddleware(),        // Second - logs query execution
		pkgapp.ValidationQueryMiddleware(),     // Third - validates queries
		pkgapp.CachingQueryMiddleware(cache),   // Fourth - caches query results
		pkgapp.MetricsQueryMiddleware(metrics), // Last - collects metrics
	)

	// Example 4: Register a query handler without caching for admin queries
	queryBus.Register("GetUserAdmin",
		&getUserQueryHandlerAdapter{handler: getUserHandler},
		pkgapp.ErrorHandlingQueryMiddleware(),  // First - handles panics and wraps errors
		pkgapp.LoggingQueryMiddleware(),        // Second - logs query execution
		pkgapp.ValidationQueryMiddleware(),     // Third - validates queries
		pkgapp.MetricsQueryMiddleware(metrics), // Last - collects metrics
	)

	// Example 5: Register a query handler with minimal middleware for internal use
	queryBus.Register("GetUserInternal",
		&getUserQueryHandlerAdapter{handler: getUserHandler},
		pkgapp.ErrorHandlingQueryMiddleware(), // Only error handling
	)
}

// FxRegistrationExample shows how this could work with Fx dependency injection
func FxRegistrationExample(
	commandBus pkgapp.CommandBus,
	queryBus pkgapp.QueryBus,
	userRepo domain.UserRepository,
	unitOfWork pkginfra.UnitOfWork,
	readModelRepo application.UserReadModelRepository,
	metrics pkgapp.MetricsCollector,
	cache pkgapp.CacheProvider,
) {
	// Create handlers
	createUserHandler := application.NewCreateUserHandler(userRepo, unitOfWork)
	getUserHandler := application.NewGetUserHandler(readModelRepo)

	// Standard middleware stack for most handlers
	standardCommandMiddleware := []pkgapp.CommandMiddleware{
		pkgapp.ErrorHandlingCommandMiddleware(),
		pkgapp.LoggingCommandMiddleware(),
		pkgapp.ValidationCommandMiddleware(),
		pkgapp.MetricsCommandMiddleware(metrics),
	}

	standardQueryMiddleware := []pkgapp.QueryMiddleware{
		pkgapp.ErrorHandlingQueryMiddleware(),
		pkgapp.LoggingQueryMiddleware(),
		pkgapp.ValidationQueryMiddleware(),
		pkgapp.CachingQueryMiddleware(cache),
		pkgapp.MetricsQueryMiddleware(metrics),
	}

	// Register with standard middleware
	commandBus.Register("CreateUser",
		&createUserCommandHandlerAdapter{handler: createUserHandler},
		standardCommandMiddleware...,
	)

	queryBus.Register("GetUser",
		&getUserQueryHandlerAdapter{handler: getUserHandler},
		standardQueryMiddleware...,
	)
}

// SecureHandlerRegistration shows how you could register handlers with security middleware
func SecureHandlerRegistration(
	commandBus pkgapp.CommandBus,
	queryBus pkgapp.QueryBus,
	userRepo domain.UserRepository,
	unitOfWork pkginfra.UnitOfWork,
	readModelRepo application.UserReadModelRepository,
	metrics pkgapp.MetricsCollector,
	authMiddleware pkgapp.CommandMiddleware, // Hypothetical auth middleware
	adminAuthMiddleware pkgapp.QueryMiddleware, // Hypothetical admin auth middleware
) {
	// Create handlers
	createUserHandler := application.NewCreateUserHandler(userRepo, unitOfWork)
	getUserHandler := application.NewGetUserHandler(readModelRepo)

	// Register command with authentication
	commandBus.Register("CreateUserSecure",
		&createUserCommandHandlerAdapter{handler: createUserHandler},
		pkgapp.ErrorHandlingCommandMiddleware(),
		authMiddleware, // Authentication middleware
		pkgapp.LoggingCommandMiddleware(),
		pkgapp.ValidationCommandMiddleware(),
		pkgapp.MetricsCommandMiddleware(metrics),
	)

	// Register query with admin authentication
	queryBus.Register("GetUserAdmin",
		&getUserQueryHandlerAdapter{handler: getUserHandler},
		pkgapp.ErrorHandlingQueryMiddleware(),
		adminAuthMiddleware, // Admin authentication middleware
		pkgapp.LoggingQueryMiddleware(),
		pkgapp.ValidationQueryMiddleware(),
		pkgapp.MetricsQueryMiddleware(metrics),
	)
}

// Handler adapters for the internal handlers
type createUserCommandHandlerAdapter struct {
	handler *application.CreateUserHandler
}

func (a *createUserCommandHandlerAdapter) Handle(ctx pkgapp.CommandContext) error {
	cmd := ctx.Command().(application.CreateUserCommand)
	return a.handler.Handle(ctx.Context(), ctx.Logger(), cmd)
}

type getUserQueryHandlerAdapter struct {
	handler *application.GetUserHandler
}

func (a *getUserQueryHandlerAdapter) Handle(ctx pkgapp.QueryContext) (interface{}, error) {
	query := ctx.Query().(application.GetUserQuery)
	return a.handler.Handle(ctx.Context(), ctx.Logger(), query)
}
