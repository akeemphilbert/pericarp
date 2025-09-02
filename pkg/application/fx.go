package application

import (
	"github.com/akeemphilbert/pericarp/pkg/domain"
	"go.uber.org/fx"
)

// ApplicationModule provides all application layer dependencies
var ApplicationModule = fx.Options(
	fx.Provide(
		CommandBusProvider,
		QueryBusProvider,
		ApplicationServiceProvider,
		HandlerRegistrarProvider,
		MetricsCollectorProvider,
		CacheProviderProvider,

		// Standard middleware providers (tagged for different use cases)
		fx.Annotate(ErrorHandlingCommandMiddlewareProvider, fx.ResultTags(`group:"admin_command_middleware" group:"public_command_middleware" group:"internal_command_middleware"`)),
		fx.Annotate(ErrorHandlingQueryMiddlewareProvider, fx.ResultTags(`group:"admin_query_middleware" group:"public_query_middleware" group:"internal_query_middleware"`)),

		fx.Annotate(LoggingCommandMiddlewareProvider, fx.ResultTags(`group:"admin_command_middleware" group:"public_command_middleware"`)),
		fx.Annotate(LoggingQueryMiddlewareProvider, fx.ResultTags(`group:"admin_query_middleware" group:"public_query_middleware"`)),

		fx.Annotate(ValidationCommandMiddlewareProvider, fx.ResultTags(`group:"admin_command_middleware" group:"public_command_middleware"`)),
		fx.Annotate(ValidationQueryMiddlewareProvider, fx.ResultTags(`group:"admin_query_middleware" group:"public_query_middleware"`)),

		fx.Annotate(MetricsCommandMiddlewareProvider, fx.ResultTags(`group:"admin_command_middleware" group:"public_command_middleware"`)),
		fx.Annotate(MetricsQueryMiddlewareProvider, fx.ResultTags(`group:"admin_query_middleware" group:"public_query_middleware"`)),

		fx.Annotate(CachingQueryMiddlewareProvider, fx.ResultTags(`group:"public_query_middleware"`)),
	),
	fx.Invoke(
		// Setup functions for different handler groups
		fx.Annotate(setupAdminCommandHandlers, fx.ParamTags(``, ``, `group:"admin_command_handlers"`, `group:"admin_command_middleware"`)),
		fx.Annotate(setupAdminQueryHandlers, fx.ParamTags(``, ``, `group:"admin_query_handlers"`, `group:"admin_query_middleware"`)),
		fx.Annotate(setupPublicCommandHandlers, fx.ParamTags(``, ``, `group:"public_command_handlers"`, `group:"public_command_middleware"`)),
		fx.Annotate(setupPublicQueryHandlers, fx.ParamTags(``, ``, `group:"public_query_handlers"`, `group:"public_query_middleware"`)),
		fx.Annotate(setupInternalCommandHandlers, fx.ParamTags(``, ``, `group:"internal_command_handlers"`, `group:"internal_command_middleware"`)),
		fx.Annotate(setupInternalQueryHandlers, fx.ParamTags(``, ``, `group:"internal_query_handlers"`, `group:"internal_query_middleware"`)),
	),
)

// CommandBusProvider creates a command bus
func CommandBusProvider() CommandBus {
	return NewCommandBus()
}

// QueryBusProvider creates a query bus
func QueryBusProvider() QueryBus {
	return NewQueryBus()
}

// HandlerRegistrarProvider creates a handler registrar
func HandlerRegistrarProvider() HandlerRegistrar {
	return &DefaultHandlerRegistrar{}
}

// Tagged middleware providers

// ErrorHandlingCommandMiddlewareProvider creates error handling middleware for commands
func ErrorHandlingCommandMiddlewareProvider() TaggedCommandMiddleware {
	return TaggedCommandMiddleware{
		Name:       "error_handling",
		Middleware: ErrorHandlingMiddleware[Command, any](),
	}
}

// ErrorHandlingQueryMiddlewareProvider creates error handling middleware for queries
func ErrorHandlingQueryMiddlewareProvider() TaggedQueryMiddleware {
	return TaggedQueryMiddleware{
		Name:       "error_handling",
		Middleware: ErrorHandlingMiddleware[Query, any](),
	}
}

// LoggingCommandMiddlewareProvider creates logging middleware for commands
func LoggingCommandMiddlewareProvider() TaggedCommandMiddleware {
	return TaggedCommandMiddleware{
		Name:       "logging",
		Middleware: LoggingMiddleware[Command, any](),
	}
}

// LoggingQueryMiddlewareProvider creates logging middleware for queries
func LoggingQueryMiddlewareProvider() TaggedQueryMiddleware {
	return TaggedQueryMiddleware{
		Name:       "logging",
		Middleware: LoggingMiddleware[Query, any](),
	}
}

// ValidationCommandMiddlewareProvider creates validation middleware for commands
func ValidationCommandMiddlewareProvider() TaggedCommandMiddleware {
	return TaggedCommandMiddleware{
		Name:       "validation",
		Middleware: ValidationMiddleware[Command, any](),
	}
}

// ValidationQueryMiddlewareProvider creates validation middleware for queries
func ValidationQueryMiddlewareProvider() TaggedQueryMiddleware {
	return TaggedQueryMiddleware{
		Name:       "validation",
		Middleware: ValidationMiddleware[Query, any](),
	}
}

// MetricsCommandMiddlewareProvider creates metrics middleware for commands
func MetricsCommandMiddlewareProvider(metrics MetricsCollector) TaggedCommandMiddleware {
	return TaggedCommandMiddleware{
		Name:       "metrics",
		Middleware: MetricsMiddleware[Command, any](metrics),
	}
}

// MetricsQueryMiddlewareProvider creates metrics middleware for queries
func MetricsQueryMiddlewareProvider(metrics MetricsCollector) TaggedQueryMiddleware {
	return TaggedQueryMiddleware{
		Name:       "metrics",
		Middleware: MetricsMiddleware[Query, any](metrics),
	}
}

// ApplicationServiceProvider creates an application service
func ApplicationServiceProvider(unitOfWork domain.UnitOfWork, logger domain.Logger) *ApplicationService {
	return NewApplicationService(unitOfWork, logger)
}

// CachingQueryMiddlewareProvider creates caching middleware for queries
func CachingQueryMiddlewareProvider(cache CacheProvider) TaggedQueryMiddleware {
	return TaggedQueryMiddleware{
		Name:       "caching",
		Middleware: CachingMiddleware[Query, any](cache),
	}
}

// Setup functions for different handler groups

// setupAdminCommandHandlers registers all admin command handlers with admin middleware
func setupAdminCommandHandlers(
	registrar HandlerRegistrar,
	commandBus CommandBus,
	handlers []TaggedCommandHandler,
	middleware []TaggedCommandMiddleware,
) {
	registrar.RegisterCommandHandlers(commandBus, handlers, middleware)
}

// setupAdminQueryHandlers registers all admin query handlers with admin middleware
func setupAdminQueryHandlers(
	registrar HandlerRegistrar,
	queryBus QueryBus,
	handlers []TaggedQueryHandler,
	middleware []TaggedQueryMiddleware,
) {
	registrar.RegisterQueryHandlers(queryBus, handlers, middleware)
}

// setupPublicCommandHandlers registers all public command handlers with public middleware
func setupPublicCommandHandlers(
	registrar HandlerRegistrar,
	commandBus CommandBus,
	handlers []TaggedCommandHandler,
	middleware []TaggedCommandMiddleware,
) {
	registrar.RegisterCommandHandlers(commandBus, handlers, middleware)
}

// setupPublicQueryHandlers registers all public query handlers with public middleware
func setupPublicQueryHandlers(
	registrar HandlerRegistrar,
	queryBus QueryBus,
	handlers []TaggedQueryHandler,
	middleware []TaggedQueryMiddleware,
) {
	registrar.RegisterQueryHandlers(queryBus, handlers, middleware)
}

// setupInternalCommandHandlers registers all internal command handlers with internal middleware
func setupInternalCommandHandlers(
	registrar HandlerRegistrar,
	commandBus CommandBus,
	handlers []TaggedCommandHandler,
	middleware []TaggedCommandMiddleware,
) {
	registrar.RegisterCommandHandlers(commandBus, handlers, middleware)
}

// setupInternalQueryHandlers registers all internal query handlers with internal middleware
func setupInternalQueryHandlers(
	registrar HandlerRegistrar,
	queryBus QueryBus,
	handlers []TaggedQueryHandler,
	middleware []TaggedQueryMiddleware,
) {
	registrar.RegisterQueryHandlers(queryBus, handlers, middleware)
}

// MetricsCollectorProvider creates a metrics collector
func MetricsCollectorProvider() MetricsCollector {
	return NewInMemoryMetricsCollector()
}

// CacheProviderProvider creates a cache provider
func CacheProviderProvider() CacheProvider {
	return NewInMemoryCache()
}
