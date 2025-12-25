# Pericarp Architecture

This document describes the architecture and design decisions of the Pericarp library.

## Overview

Pericarp is a Go library implementing Domain-Driven Design (DDD), Command Query Responsibility Segregation (CQRS), and Event Sourcing patterns with clean architecture principles.

## Project Structure

```
pericarp/
├── pkg/                    # Public API packages
│   ├── domain/             # Domain entities, events, value objects, repository interfaces
│   ├── application/        # Command/query handlers, application services, CQRS bus
│   └── infrastructure/     # Event store implementations, database access, external integrations
├── internal/               # Private implementation packages
│   ├── store/              # Private store implementations
│   ├── handler/            # Private handler logic
│   └── examples/           # Example implementations (not for export)
├── cmd/pericarp/           # CLI tools/demos
├── examples/               # Runnable examples for users
├── test/                   # Integration tests
├── docs/                   # Documentation
├── scripts/                # Build/automation scripts
└── configs/                # Configuration templates
```

## Architecture Layers

### Domain Layer (`pkg/domain/`)

The domain layer contains:
- Aggregate roots and entities
- Domain events
- Value objects
- Domain services
- Repository interfaces

### Application Layer (`pkg/application/`)

The application layer contains:
- Command and query handlers
- Application services
- DTOs and data transfer objects
- Use case orchestration
- CQRS bus implementation

### Infrastructure Layer (`pkg/infrastructure/`)

The infrastructure layer contains:
- Event store implementations
- Repository implementations
- Database access
- External service integrations
- Configuration management

## Design Patterns

### Event Sourcing

All state changes are captured as events. The event store persists events, and aggregates are reconstructed by replaying events.

### CQRS

Commands and queries are separated:
- Commands modify state and return success/failure
- Queries read state and return data
- Separate handlers for commands and queries

### Clean Architecture

Dependencies point inward:
- Domain has no dependencies
- Application depends only on domain
- Infrastructure depends on application and domain

## Key Components

### Event Store

The event store is responsible for:
- Persisting domain events
- Retrieving events for aggregate reconstruction
- Supporting multiple database backends (SQLite, PostgreSQL)

### Event Dispatcher

The event dispatcher:
- Publishes events to subscribers
- Supports async event processing
- Integrates with Watermill for event streaming

### Aggregate Root

The base aggregate root provides:
- Event management
- Version tracking
- Sequence number management
- Event sourcing capabilities

## Testing Strategy

- **Unit Tests**: Fast, isolated tests for individual components
- **Integration Tests**: End-to-end testing with real databases
- **Performance Tests**: Benchmarking and profiling

## Database Support

- **SQLite**: Development and testing
- **PostgreSQL**: Production deployments

## Future Enhancements

- [ ] Add more database backends
- [ ] Improve performance optimizations
- [ ] Add more middleware options
- [ ] Enhance documentation

## References

- [Domain-Driven Design](https://martinfowler.com/bliki/DomainDrivenDesign.html)
- [CQRS Pattern](https://martinfowler.com/bliki/CQRS.html)
- [Event Sourcing](https://martinfowler.com/eaaDev/EventSourcing.html)
- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
