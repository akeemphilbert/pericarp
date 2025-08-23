package application

import (
	"github.com/example/pericarp/pkg/domain"
	"go.uber.org/fx"
)

// ApplicationModule provides all application layer dependencies
var ApplicationModule = fx.Options(
	fx.Provide(
		CommandBusProvider,
		QueryBusProvider,
		ApplicationServiceProvider,
		// Named middleware providers
		fx.Annotate(LoggingCommandMiddlewareProvider, fx.ResultTags(`name:"logging_command"`)),
		fx.Annotate(LoggingQueryMiddlewareProvider, fx.ResultTags(`name:"logging_query"`)),
		fx.Annotate(ValidationCommandMiddlewareProvider, fx.ResultTags(`name:"validation_command"`)),
		fx.Annotate(ValidationQueryMiddlewareProvider, fx.ResultTags(`name:"validation_query"`)),
		fx.Annotate(ErrorHandlingCommandMiddlewareProvider, fx.ResultTags(`name:"error_command"`)),
		fx.Annotate(ErrorHandlingQueryMiddlewareProvider, fx.ResultTags(`name:"error_query"`)),
		// Metrics middleware (optional - only if MetricsCollector is available)
		fx.Annotate(MetricsCommandMiddlewareProvider, fx.ResultTags(`name:"metrics_command"`)),
		fx.Annotate(MetricsQueryMiddlewareProvider, fx.ResultTags(`name:"metrics_query"`)),
	),
	fx.Invoke(
		// Configure middleware on buses
		fx.Annotate(configureCommandBusMiddleware, fx.ParamTags(``,
			`name:"logging_command"`,
			`name:"validation_command"`,
			`name:"error_command"`,
			`name:"metrics_command"`)),
		fx.Annotate(configureQueryBusMiddleware, fx.ParamTags(``,
			`name:"logging_query"`,
			`name:"validation_query"`,
			`name:"error_query"`,
			`name:"metrics_query"`)),
	),
)

// configureCommandBusMiddleware sets up middleware for command bus
func configureCommandBusMiddleware(
	bus CommandBus,
	loggingMiddleware CommandMiddleware,
	validationMiddleware CommandMiddleware,
	errorMiddleware CommandMiddleware,
	metricsMiddleware CommandMiddleware,
) {
	middlewares := []CommandMiddleware{
		errorMiddleware,      // Error handling should be outermost
		loggingMiddleware,    // Logging should wrap everything
		validationMiddleware, // Validation before business logic
	}

	// Add metrics middleware if available
	if metricsMiddleware != nil {
		middlewares = append(middlewares, metricsMiddleware)
	}

	bus.Use(middlewares...)
}

// configureQueryBusMiddleware sets up middleware for query bus
func configureQueryBusMiddleware(
	bus QueryBus,
	loggingMiddleware QueryMiddleware,
	validationMiddleware QueryMiddleware,
	errorMiddleware QueryMiddleware,
	metricsMiddleware QueryMiddleware,
) {
	middlewares := []QueryMiddleware{
		errorMiddleware,      // Error handling should be outermost
		loggingMiddleware,    // Logging should wrap everything
		validationMiddleware, // Validation before business logic
	}

	// Add metrics middleware if available
	if metricsMiddleware != nil {
		middlewares = append(middlewares, metricsMiddleware)
	}

	bus.Use(middlewares...)
}

// CommandBusProvider creates a command bus
func CommandBusProvider() CommandBus {
	return NewCommandBus()
}

// QueryBusProvider creates a query bus
func QueryBusProvider() QueryBus {
	return NewQueryBus()
}

// LoggingCommandMiddlewareProvider creates logging middleware for commands
func LoggingCommandMiddlewareProvider() CommandMiddleware {
	return LoggingCommandMiddleware()
}

// LoggingQueryMiddlewareProvider creates logging middleware for queries
func LoggingQueryMiddlewareProvider() QueryMiddleware {
	return LoggingQueryMiddleware()
}

// ValidationCommandMiddlewareProvider creates validation middleware for commands
func ValidationCommandMiddlewareProvider() CommandMiddleware {
	return ValidationCommandMiddleware()
}

// ValidationQueryMiddlewareProvider creates validation middleware for queries
func ValidationQueryMiddlewareProvider() QueryMiddleware {
	return ValidationQueryMiddleware()
}

// ErrorHandlingCommandMiddlewareProvider creates error handling middleware for commands
func ErrorHandlingCommandMiddlewareProvider() CommandMiddleware {
	return ErrorHandlingCommandMiddleware()
}

// ErrorHandlingQueryMiddlewareProvider creates error handling middleware for queries
func ErrorHandlingQueryMiddlewareProvider() QueryMiddleware {
	return ErrorHandlingQueryMiddleware()
}

// MetricsCommandMiddlewareProvider creates metrics middleware for commands
func MetricsCommandMiddlewareProvider(metrics MetricsCollector) CommandMiddleware {
	return MetricsCommandMiddleware(metrics)
}

// MetricsQueryMiddlewareProvider creates metrics middleware for queries
func MetricsQueryMiddlewareProvider(metrics MetricsCollector) QueryMiddleware {
	return MetricsQueryMiddleware(metrics)
}

// ApplicationServiceProvider creates an application service
func ApplicationServiceProvider(unitOfWork domain.UnitOfWork, logger domain.Logger) *ApplicationService {
	return NewApplicationService(unitOfWork, logger)
}
