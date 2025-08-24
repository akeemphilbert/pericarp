# Application Layer - CQRS with Per-Handler Middleware

This package implements a CQRS (Command Query Responsibility Segregation) pattern with per-handler middleware registration, similar to the Echo framework's approach.

## Key Features

- **Per-Handler Middleware**: Each command/query handler can have its own middleware stack
- **Flexible Registration**: Easy to configure different middleware combinations for different handlers
- **Echo-like Middleware Chain**: Middleware executes in the order specified during registration
- **Type-Safe**: Full Go generics support for type-safe command and query handling
- **Fx Integration Ready**: Designed to work seamlessly with Uber Fx dependency injection

## Basic Usage

### Command Bus Registration

```go
// Create command bus
commandBus := NewCommandBus()

// Register handler with middleware
commandBus.Register("CreateUser", 
    handler,
    ErrorHandlingCommandMiddleware(),    // First - handles panics and wraps errors
    LoggingCommandMiddleware(),          // Second - logs command execution
    ValidationCommandMiddleware(),       // Third - validates commands
    MetricsCommandMiddleware(metrics),   // Last - collects metrics
)
```

### Query Bus Registration

```go
// Create query bus
queryBus := NewQueryBus()

// Register handler with middleware
queryBus.Register("GetUser", 
    handler,
    ErrorHandlingQueryMiddleware(),      // First - handles panics and wraps errors
    LoggingQueryMiddleware(),            // Second - logs query execution
    ValidationQueryMiddleware(),         // Third - validates queries
    CachingQueryMiddleware(cache),       // Fourth - caches query results
    MetricsQueryMiddleware(metrics),     // Last - collects metrics
)
```

## Middleware Execution Order

Middleware executes in the order specified during registration:

1. **First middleware** - Outermost layer (e.g., error handling)
2. **Second middleware** - Next layer (e.g., logging)
3. **Third middleware** - Next layer (e.g., validation)
4. **Handler** - Core business logic
5. **Third middleware** - Cleanup/after logic
6. **Second middleware** - Cleanup/after logic
7. **First middleware** - Cleanup/after logic

## Available Middleware

### Command Middleware
- `ErrorHandlingCommandMiddleware()` - Handles panics and wraps errors
- `LoggingCommandMiddleware()` - Logs command execution
- `ValidationCommandMiddleware()` - Validates commands that implement `Validator`
- `MetricsCommandMiddleware(metrics)` - Collects execution metrics

### Query Middleware
- `ErrorHandlingQueryMiddleware()` - Handles panics and wraps errors
- `LoggingQueryMiddleware()` - Logs query execution
- `ValidationQueryMiddleware()` - Validates queries that implement `Validator`
- `CachingQueryMiddleware(cache)` - Caches query results
- `MetricsQueryMiddleware(metrics)` - Collects execution metrics

## Advanced Usage Examples

### Different Middleware Per Handler

```go
// Public API handler with full middleware stack
commandBus.Register("CreateUserPublic", 
    publicHandler,
    ErrorHandlingCommandMiddleware(),
    AuthenticationMiddleware(),          // Custom auth middleware
    LoggingCommandMiddleware(),
    ValidationCommandMiddleware(),
    RateLimitingMiddleware(),           // Custom rate limiting
    MetricsCommandMiddleware(metrics),
)

// Internal handler with minimal middleware
commandBus.Register("CreateUserInternal", 
    internalHandler,
    ErrorHandlingCommandMiddleware(),    // Only error handling
)

// Admin handler with admin-specific middleware
commandBus.Register("CreateUserAdmin", 
    adminHandler,
    ErrorHandlingCommandMiddleware(),
    AdminAuthMiddleware(),              // Admin-only auth
    LoggingCommandMiddleware(),
    AuditMiddleware(),                  // Admin action auditing
    MetricsCommandMiddleware(metrics),
)
```

### Fx Integration with Tagged Handlers

```go
// In your Fx module
fx.Provide(
    // Tagged handler providers
    fx.Annotate(NewCreateUserHandler, fx.ResultTags(`group:"public_handlers"`)),
    fx.Annotate(NewAdminUserHandler, fx.ResultTags(`group:"admin_handlers"`)),
    fx.Annotate(NewInternalUserHandler, fx.ResultTags(`group:"internal_handlers"`)),
)

fx.Invoke(
    // Register public handlers with standard middleware
    fx.Annotate(registerPublicHandlers, fx.ParamTags(`group:"public_handlers"`)),
    // Register admin handlers with admin middleware
    fx.Annotate(registerAdminHandlers, fx.ParamTags(`group:"admin_handlers"`)),
    // Register internal handlers with minimal middleware
    fx.Annotate(registerInternalHandlers, fx.ParamTags(`group:"internal_handlers"`)),
)

func registerPublicHandlers(handlers []PublicHandler, bus CommandBus) {
    for _, handler := range handlers {
        bus.Register(handler.CommandType(), handler,
            ErrorHandlingCommandMiddleware(),
            AuthenticationMiddleware(),
            LoggingCommandMiddleware(),
            ValidationCommandMiddleware(),
            MetricsCommandMiddleware(metrics),
        )
    }
}
```

### Custom Middleware

```go
// Create custom middleware
func CustomAuthMiddleware(authService AuthService) CommandMiddleware {
    return func(next CommandHandlerFunc) CommandHandlerFunc {
        return func(ctx context.Context, logger domain.Logger, cmd Command) error {
            // Extract auth token from context
            token := extractToken(ctx)
            
            // Validate token
            if !authService.ValidateToken(token) {
                return NewApplicationError("UNAUTHORIZED", "Invalid token", nil)
            }
            
            // Add user info to context
            ctx = context.WithValue(ctx, "user", authService.GetUser(token))
            
            // Continue to next middleware/handler
            return next(ctx, logger, cmd)
        }
    }
}

// Use custom middleware
commandBus.Register("SecureCommand", handler,
    ErrorHandlingCommandMiddleware(),
    CustomAuthMiddleware(authService),
    LoggingCommandMiddleware(),
    ValidationCommandMiddleware(),
)
```

## Benefits

1. **Granular Control**: Each handler can have exactly the middleware it needs
2. **Performance**: No unnecessary middleware execution for handlers that don't need it
3. **Security**: Easy to apply different security policies to different handlers
4. **Maintainability**: Clear separation of concerns and easy to understand middleware chains
5. **Flexibility**: Easy to add, remove, or reorder middleware for specific handlers
6. **Testing**: Easy to test handlers with or without middleware

## Migration from Global Middleware

If you're migrating from a global middleware approach:

```go
// Old approach (global middleware)
commandBus.Use(
    ErrorHandlingCommandMiddleware(),
    LoggingCommandMiddleware(),
    ValidationCommandMiddleware(),
)
commandBus.Register("CreateUser", handler)

// New approach (per-handler middleware)
commandBus.Register("CreateUser", handler,
    ErrorHandlingCommandMiddleware(),
    LoggingCommandMiddleware(),
    ValidationCommandMiddleware(),
)
```

The new approach gives you the same functionality but with much more flexibility for different handlers.