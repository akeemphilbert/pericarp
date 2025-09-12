# Reference Documentation

This section provides comprehensive technical reference for the Pericarp Go library. Use this when you need to look up specific API details, configuration options, or implementation specifications.

## API Reference

### Core Interfaces
- [Domain Interfaces](api/domain.md) - Event, AggregateRoot, Repository, Logger
- [Application Interfaces](api/application.md) - Command, Query, Handler, Bus, Middleware
- [Infrastructure Interfaces](api/infrastructure.md) - EventStore, EventDispatcher, UnitOfWork

### Implementations
- [Event Store](api/event-store.md) - GORM-based event persistence
- [Event Dispatcher](api/event-dispatcher.md) - Watermill-based event dispatching
- [Command Bus](api/command-bus.md) - Command handling and middleware
- [Query Bus](api/query-bus.md) - Query handling and middleware

## Configuration Reference

### [Configuration Options](configuration.md)
Complete reference for all configuration parameters:
- Database configuration (SQLite, PostgreSQL)
- Event system configuration
- Logging configuration
- Fx module configuration

### [Environment Variables](environment-variables.md)
All supported environment variables and their effects.

## Middleware Reference

### [Built-in Middleware](middleware.md)
Documentation for all built-in middleware:
- LoggingMiddleware
- ValidationMiddleware
- MetricsMiddleware
- ErrorHandlingMiddleware

### [Custom Middleware](custom-middleware.md)
How to create and register custom middleware.

## Error Reference

### [Error Types](errors.md)
Complete reference for all error types:
- Domain errors
- Application errors
- Infrastructure errors
- Validation errors

### [Error Handling](error-handling.md)
Error handling patterns and best practices.

## Examples

### [Code Examples](examples.md)
Complete, working code examples:
- Basic usage patterns
- Advanced scenarios
- Integration examples
- Testing examples

### [Configuration Examples](config-examples.md)
Example configurations for different scenarios:
- Development setup
- Production setup
- Testing setup
- Docker setup

## Package Documentation

### Core Packages
- [`pkg/domain`](packages/domain.md) - Domain layer interfaces and types
- [`pkg/application`](packages/application.md) - Application layer CQRS implementation
- [`pkg/infrastructure`](packages/infrastructure.md) - Infrastructure implementations

### Internal Packages
- [`examples/`](packages/examples.md) - Example implementations and usage patterns

## Testing Reference

### [Testing Utilities](testing.md)
Built-in testing utilities and helpers:
- Mock implementations
- Test fixtures
- BDD step definitions
- Integration test helpers

### [Test Configuration](test-configuration.md)
Configuration options for testing:
- Test database setup
- Mock configuration
- Performance test settings

## Performance Reference

### [Performance Characteristics](performance.md)
Performance characteristics and benchmarks:
- Throughput measurements
- Latency characteristics
- Memory usage patterns
- Scalability limits

### [Optimization Guide](optimization.md)
Performance optimization techniques:
- Event store optimization
- Query optimization
- Memory optimization
- Concurrency optimization

## Migration Guide

### [Version Migration](migration.md)
Guide for migrating between versions:
- Breaking changes
- Migration steps
- Compatibility notes

## Quick Reference

### Common Patterns
```go
// Create aggregate
user := new(User).WithEmail(email, name)

// Handle command
func (h *Handler) Handle(ctx context.Context, logger Logger, eventStore EventStore, eventDispatcher EventDispatcher, payload Payload[Command]) (Response[any], error)

// Handle query
func (h *Handler) Handle(ctx context.Context, logger Logger, query Query) (Result, error)

// Register handler
commandBus.Register("CommandType", handler)
queryBus.Register("QueryType", handler)

// Configure with Fx
fx.New(pkg.Module, yourModule)
```

### Configuration Template
```yaml
database:
  driver: postgres
  dsn: "host=localhost user=user dbname=db sslmode=disable"
events:
  publisher: channel
logging:
  level: info
```

### Error Handling
```go
if err != nil {
    return Response[T]{Error: err}, err
}
```

## API Stability

### Stable APIs
These APIs are considered stable and will maintain backward compatibility:
- Core domain interfaces (Event, AggregateRoot, Repository)
- Application interfaces (Command, Query, Handler)
- Configuration structure

### Experimental APIs
These APIs may change in future versions:
- Advanced middleware features
- Performance optimization utilities
- Internal implementation details

### Deprecated APIs
Currently no deprecated APIs.

## Contributing to Reference

Help us keep the reference documentation accurate:

1. **API Changes** - Update docs when changing APIs
2. **New Features** - Document new features completely
3. **Examples** - Add working examples for complex features
4. **Corrections** - Fix errors and improve clarity

See our [Contributing Guide](../../CONTRIBUTING.md) for more details.