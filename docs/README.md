# Pericarp Documentation

Welcome to the Pericarp Go library documentation. This documentation follows the [Diátaxis framework](https://diataxis.fr/) to provide you with the right information at the right time.

Pericarp is a Go library that implements Domain-Driven Design (DDD), Command Query Responsibility Segregation (CQRS), and Event Sourcing patterns. It provides a clean, testable architecture for building scalable microservices and applications.

## Documentation Structure

### 📚 [Tutorial](tutorial/README.md)
**Learning-oriented** - Start here if you're new to Pericarp or DDD/CQRS/Event Sourcing patterns.

- [Getting Started](tutorial/getting-started.md) - Your first Pericarp application
- [Building a User Management System](tutorial/user-management.md) - Complete walkthrough with CLI demo
- [Adding Event Sourcing](tutorial/event-sourcing.md) - Implementing event sourcing
- [Testing Your Application](tutorial/testing.md) - Writing comprehensive tests

### 🔧 [How-to Guides](how-to/README.md)
**Problem-oriented** - Practical solutions for specific implementation challenges.

- [Implementing Custom Aggregates](how-to/custom-aggregates.md) - Create domain aggregates with business logic
- [Creating Middleware](how-to/middleware.md) - Add cross-cutting concerns to handlers
- [Database Configuration](how-to/database-setup.md) - Configure SQLite and PostgreSQL
- [Performance Optimization](how-to/performance.md) - Optimize your application for production
- [Error Handling Patterns](how-to/error-handling.md) - Handle errors gracefully across layers
- [Testing Strategies](how-to/testing-strategies.md) - Comprehensive testing approaches

### 📖 [Reference](reference/README.md)
**Information-oriented** - Complete API documentation and technical specifications.

- [API Reference](reference/api.md) - Complete API documentation
- [Configuration Options](reference/configuration.md) - All configuration parameters
- [Middleware Reference](reference/middleware.md) - Built-in middleware documentation
- [Error Types](reference/errors.md) - Error handling reference
- [Examples](reference/examples.md) - Code examples and snippets

### 💡 [Explanation](explanation/README.md)
**Understanding-oriented** - Deep dive into concepts, patterns, and design decisions.

- [Domain-Driven Design](explanation/ddd.md) - DDD principles and implementation
- [CQRS Pattern](explanation/cqrs.md) - Command Query Responsibility Segregation
- [Event Sourcing](explanation/event-sourcing.md) - Event sourcing concepts and benefits
- [Clean Architecture](explanation/clean-architecture.md) - Architectural principles
- [Design Decisions](explanation/design-decisions.md) - Why we made certain choices

## Quick Navigation

### I want to...

- **Learn Pericarp from scratch** → Start with the [Tutorial](tutorial/README.md)
- **Try the demo application** → Run `pericarp-demo` CLI commands
- **Solve a specific problem** → Check the [How-to Guides](how-to/README.md)
- **Look up API details** → Use the [Reference](reference/README.md)
- **Understand the concepts** → Read the [Explanation](explanation/README.md)

### By Experience Level

- **Beginner** → [Tutorial](tutorial/README.md) → [How-to Guides](how-to/README.md)
- **Intermediate** → [How-to Guides](how-to/README.md) → [Reference](reference/README.md)
- **Advanced** → [Reference](reference/README.md) → [Explanation](explanation/README.md)

## Demo Application

Pericarp includes a comprehensive CLI demo application that showcases all the library's features:

```bash
# Build and run the demo
go build -o pericarp-demo cmd/demo/main.go
./pericarp-demo --help

# Create a user
./pericarp-demo create-user john@example.com "John Doe"

# List users
./pericarp-demo list-users

# Get user by GetID
./pericarp-demo get-user by-id <user-id>
```

The demo includes:
- Complete user management (create, update, activate/deactivate)
- CQRS pattern implementation
- Event sourcing with SQLite/PostgreSQL
- Read model projections
- Comprehensive error handling

## Contributing to Documentation

We welcome contributions to improve our documentation:

1. **Tutorials** - Help beginners learn step-by-step
2. **How-to Guides** - Share solutions to common problems
3. **Reference** - Keep API docs accurate and complete
4. **Explanations** - Clarify concepts and design decisions

See our [Contributing Guide](../CONTRIBUTING.md) for more details.