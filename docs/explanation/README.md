# Explanation: Understanding Pericarp

This section provides deep understanding of the concepts, patterns, and design decisions behind Pericarp. Read this to understand the "why" behind the library's architecture and implementation.

## Core Concepts

### [Domain-Driven Design (DDD)](ddd.md)
Understanding the principles and patterns that guide Pericarp's architecture:
- Strategic design and bounded contexts
- Tactical patterns: aggregates, entities, value objects
- Domain services and repositories
- Ubiquitous language and modeling

### [CQRS (Command Query Responsibility Segregation)](cqrs.md)
Why and how Pericarp separates commands from queries:
- Benefits of command/query separation
- Unified handler architecture
- Middleware patterns
- Read/write model separation

### [Event Sourcing](event-sourcing.md)
How Pericarp implements event sourcing and why it matters:
- Event-first thinking
- Persist-then-dispatch pattern
- Event store design
- Aggregate reconstruction

### [Clean Architecture](clean-architecture.md)
The architectural principles that keep Pericarp maintainable:
- Dependency inversion
- Layer separation
- Testability
- Framework independence

## Design Decisions

### [Unified Handler Signature](design-decisions.md#unified-handlers)
Why Pericarp uses the same handler signature for commands and queries:
- Middleware reusability
- Simplified architecture
- Type safety considerations
- Performance implications

### [Persist-then-Dispatch](design-decisions.md#persist-then-dispatch)
Why events are persisted before being dispatched:
- Consistency guarantees
- Failure recovery
- Event ordering
- Transactional boundaries

### [No Reflection in Hot Paths](design-decisions.md#no-reflection)
How Pericarp achieves performance without sacrificing type safety:
- Compile-time type checking
- Generic-based approach
- Performance characteristics
- Memory efficiency

### [JSON over Gob](design-decisions.md#json-serialization)
Why Pericarp uses JSON for event serialization:
- Cross-language compatibility
- Schema evolution
- Debugging and tooling
- Performance considerations

## Architectural Patterns

### [Layered Architecture](layered-architecture.md)
How Pericarp organizes code into layers:
- Domain layer purity
- Application layer coordination
- Infrastructure layer implementation
- Dependency flow

### [Repository Pattern](repository-pattern.md)
How Pericarp abstracts data access:
- Interface segregation
- Event sourcing repositories
- Testing strategies
- Implementation patterns

### [Unit of Work Pattern](unit-of-work.md)
How Pericarp manages transactions:
- Event registration
- Transactional boundaries
- Rollback strategies
- Performance optimization

### [Middleware Pattern](middleware-pattern.md)
How Pericarp implements cross-cutting concerns:
- Chain of responsibility
- Decorator pattern
- Composition over inheritance
- Testability

## Event-Driven Architecture

### [Event Design](event-design.md)
How to design effective domain events:
- Event naming conventions
- Event granularity
- Event versioning
- Event metadata

### [Event Handlers](event-handlers.md)
Different types of event handlers and their purposes:
- Projectors for read models
- Sagas for process management
- Integration handlers
- Notification handlers

### [Event Ordering](event-ordering.md)
How Pericarp ensures event ordering:
- Aggregate-level ordering
- Global ordering considerations
- Concurrency handling
- Consistency models

## Performance Considerations

### [Scalability Patterns](scalability.md)
How Pericarp scales with your application:
- Horizontal scaling strategies
- Event store partitioning
- Read model optimization
- Caching strategies

### [Memory Management](memory-management.md)
How Pericarp manages memory efficiently:
- Event batching
- Connection pooling
- Garbage collection considerations
- Memory profiling

### [Concurrency Model](concurrency.md)
How Pericarp handles concurrent operations:
- Optimistic concurrency control
- Event ordering guarantees
- Deadlock prevention
- Performance implications

## Testing Philosophy

### [Testing Strategy](testing-strategy.md)
Pericarp's approach to testing:
- Test pyramid application
- BDD for behavior verification
- Unit testing for domain logic
- Integration testing for infrastructure

### [Test Doubles](test-doubles.md)
How Pericarp uses mocks and stubs:
- Mock generation with moq
- In-memory implementations
- Test data builders
- Fixture management

## Comparison with Other Approaches

### [vs. Traditional CRUD](vs-crud.md)
How Pericarp differs from traditional CRUD applications:
- State vs. behavior focus
- Event-driven vs. data-driven
- Complexity trade-offs
- When to choose each approach

### [vs. Other DDD Frameworks](vs-other-frameworks.md)
How Pericarp compares to other DDD/CQRS frameworks:
- Design philosophy differences
- Performance characteristics
- Learning curve
- Ecosystem considerations

### [vs. Event Streaming Platforms](vs-event-streaming.md)
How Pericarp relates to Kafka, EventStore, etc.:
- Complementary vs. competing
- Use case differences
- Integration patterns
- Architectural considerations

## Evolution and Future

### [Design Evolution](design-evolution.md)
How Pericarp's design has evolved:
- Original design goals
- Lessons learned
- Community feedback
- Future directions

### [Roadmap](roadmap.md)
Where Pericarp is heading:
- Planned features
- Performance improvements
- Ecosystem expansion
- Community priorities

## Learning Path

### For Beginners
1. Start with [Domain-Driven Design](ddd.md)
2. Understand [CQRS](cqrs.md) concepts
3. Learn about [Event Sourcing](event-sourcing.md)
4. Explore [Clean Architecture](clean-architecture.md)

### For Experienced Developers
1. Review [Design Decisions](design-decisions.md)
2. Study [Performance Considerations](scalability.md)
3. Understand [Testing Philosophy](testing-strategy.md)
4. Compare with [Other Approaches](vs-crud.md)

### For Architects
1. Analyze [Architectural Patterns](layered-architecture.md)
2. Study [Scalability Patterns](scalability.md)
3. Review [Evolution and Future](design-evolution.md)
4. Consider [Integration Patterns](vs-event-streaming.md)

## Contributing to Understanding

Help others understand Pericarp better:

1. **Clarify Concepts** - Improve explanations of complex topics
2. **Add Examples** - Provide real-world scenarios and use cases
3. **Share Experience** - Document lessons learned from using Pericarp
4. **Ask Questions** - Help identify areas that need better explanation

The best explanations come from the community's collective experience. Share your insights!