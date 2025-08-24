# Implementation Plan

- [x] 1. Set up project structure and core domain interfaces
  - Create standard Go project layout with pkg/, cmd/, internal/ directories
  - Define Logger interface with Debug, Info, Warn, Error, Fatal methods and formatted versions
  - Define core domain interfaces: Event, Envelope, EventStore, EventDispatcher, UnitOfWork
  - Create go.mod file with module definition
  - _Requirements: 9.1, 9.2, 3.1, 3.2, 3.3, 3.4, 3.5_

- [x] 2. Implement domain layer foundation
  - [x] 2.1 Create base domain types and interfaces
    - Implement Event interface with EventType, AggregateID, Version, OccurredAt methods
    - Create AggregateRoot interface with ID, Version, UncommittedEvents, MarkEventsAsCommitted, LoadFromHistory
    - Define Repository interface for aggregate persistence
    - _Requirements: 1.2, 3.1, 8.1_

  - [x] 2.2 Implement value objects and domain services
    - Create base value object types with validation
    - Define DomainService interface for cross-aggregate business logic
    - Implement domain error types (DomainError, ValidationError, ConcurrencyError)
    - _Requirements: 1.2, 8.1_

- [x] 3. Create application layer interfaces and base implementations
  - [x] 3.1 Define CQRS interfaces with logger injection
    - Create Command and Query marker interfaces
    - Implement CommandHandler and QueryHandler generic interfaces with logger parameter
    - Define EventHandler interface for projectors and sagas
    - Update handler signatures to accept context and logger as first two parameters
    - _Requirements: 2.1, 2.2, 1.3_

  - [x] 3.2 Implement command and query buses with middleware support and logger injection
    - Create CommandBus and QueryBus interfaces with middleware registration and logger parameter
    - Implement middleware chain execution similar to Echo framework
    - Use unified Middleware type for both commands and queries
    - Update bus Handle methods to accept context, logger, and command/query parameters
    - Create base application service with UnitOfWork integration
    - Add application-level error handling and validation
    - _Requirements: 2.1, 2.2, 1.3_

  - [x] 3.3 Build core middleware implementations with logger integration
    - Implement unified LoggingMiddleware using injected logger
    - Create unified ValidationMiddleware for command validation with logging
    - Add MetricsMiddleware for performance monitoring with debug logging
    - Implement error handling middleware for consistent error responses
    - Update all middleware to use logger parameter from handler function signature
    - _Requirements: 2.1, 2.2, 1.3_

- [x] 4. Build infrastructure layer core components
  - [x] 4.1 Implement Event Store with GORM
    - Create EventRecord struct for GORM persistence
    - Implement EventStore interface with Save, Load, LoadFromVersion methods
    - Add JSON serialization for events and metadata
    - Configure database migrations for event storage
    - _Requirements: 4.3, 5.1, 5.2, 5.3, 8.3_

  - [x] 4.2 Create Event Dispatcher with Watermill
    - Implement EventDispatcher interface using Watermill Go channels
    - Add support for event subscription and handler registration
    - Implement Envelope wrapper with metadata support
    - Create internal event routing and delivery mechanisms
    - _Requirements: 4.4, 4.5, 3.5, 2.4_

  - [x] 4.3 Implement Unit of Work pattern
    - Create UnitOfWork implementation with event registration
    - Integrate with EventStore for transactional event persistence
    - Implement Persist-then-Dispatch pattern in Commit method
    - Add rollback functionality for failed transactions
    - _Requirements: 3.4, 2.3, 2.4_

- [x] 5. Add configuration and dependency injection
  - [x] 5.1 Implement configuration with Viper
    - Create Config structs for database, events, and logging
    - Implement LoadConfig function with YAML and environment variable support
    - Add validation for configuration values
    - Support both SQLite and PostgreSQL connection strings
    - _Requirements: 4.1, 5.1, 5.2, 5.3_

  - [x] 5.2 Set up dependency injection with Fx
    - Create Fx modules for domain, application, and infrastructure layers
    - Implement provider functions for all major components
    - Configure lifecycle management for database connections and event dispatchers
    - Add graceful shutdown handling
    - _Requirements: 4.2_

- [x] 6. Create demo application foundation
  - [x] 6.1 Implement User aggregate for demo
    - Create User aggregate with ID, Email, Name properties
    - Implement CreateUser and UpdateUserEmail business methods
    - Generate UserCreated and UserEmailUpdated domain events
    - Add aggregate validation and business rules
    - _Requirements: 6.2, 6.3, 1.2_

  - [x] 6.2 Build demo command handlers with middleware integration
    - Implement CreateUserHandler for user creation commands
    - Create UpdateUserEmailHandler for email update commands
    - Add command validation and error handling
    - Integrate with User aggregate and repository
    - Configure command bus with logging, validation, and metrics middleware
    - _Requirements: 6.2, 2.1, 1.3_

  - [x] 6.3 Create demo query handlers with middleware integration
    - Implement GetUserHandler for single user queries
    - Create ListUsersHandler with pagination support
    - Build UserReadModel for query optimization
    - Add query result DTOs and mapping
    - Configure query bus with logging, caching, and metrics middleware
    - _Requirements: 6.2, 2.1, 1.3_

- [x] 7. Implement unified handler architecture
  - [x] 7.1 Create unified handler signature and wrapper types
    - Define `Payload[T any]` struct with Data, Metadata, TraceID, and UserID fields
    - Create `Response[T any]` struct with Data, Metadata, and Error fields
    - Implement unified `Handler[Req any, Res any]` function type signature
    - Update CommandHandler and QueryHandler interfaces to use unified signature
    - _Requirements: 10.1, 10.2, 10.3, 10.4_

  - [x] 7.2 Refactor middleware to use unified signature
    - Converted existing CommandMiddleware and QueryMiddleware to unified Middleware[Req, Res] type
    - Update LoggingMiddleware to work with both commands and queries using unified signature
    - Refactor ValidationMiddleware to use Payload wrapper and Response wrapper
    - Update MetricsMiddleware to handle both command and query metrics uniformly
    - _Requirements: 10.5, 10.6, 10.7_

  - [x] 7.3 Update command and query buses for unified handlers
    - Modify CommandBus.Register to accept unified middleware types
    - Update QueryBus.Register to accept unified middleware types
    - Ensure bus Handle methods properly wrap/unwrap Payload and Response types
    - Add type safety while maintaining unified middleware compatibility
    - _Requirements: 10.1, 10.5_

- [ ] 8. Implement event handling and projections
  - [ ] 8.1 Create user projection event handler
    - Implement UserProjector to handle UserCreated and UserEmailUpdated events
    - Build read model tables and GORM models for projections
    - Add event handler registration and subscription
    - Implement idempotent event processing
    - _Requirements: 6.3, 2.4, 1.3_

  - [ ] 8.2 Add repository implementations
    - Create UserRepository implementation using event sourcing
    - Implement UserReadModelRepository for query operations
    - Add repository error handling and concurrency control
    - Integrate with EventStore for aggregate reconstruction
    - _Requirements: 1.4, 6.2, 6.3_

- [ ] 9. Build testing infrastructure
  - [ ] 9.1 Set up BDD testing with Cucumber
    - Create feature files with Gherkin scenarios for user management
    - Implement step definitions for Given/When/Then steps
    - Set up test database and cleanup between scenarios
    - Add test fixtures and data builders
    - _Requirements: 7.1, 7.2, 7.3_

  - [ ] 9.2 Generate mocks with moq
    - Generate mocks for Repository, EventStore, EventDispatcher interfaces
    - Create mock implementations for testing command and query handlers
    - Add mock configuration helpers for test scenarios
    - Implement in-memory test doubles for integration testing
    - _Requirements: 7.4_

  - [ ] 9.3 Write unit tests for domain layer
    - Test User aggregate business logic and event generation
    - Validate domain rules and invariants
    - Test value object validation and equality
    - Ensure pure domain logic with no external dependencies
    - _Requirements: 8.1, 7.5_

- [ ] 10. Complete demo application
  - [ ] 10.1 Build demo CLI application
    - Create main.go with Viper configuration loading
    - Implement CLI commands for user creation and querying
    - Add database initialization and migration
    - Configure logging and error handling
    - _Requirements: 6.1, 4.1, 5.1, 5.2_

  - [ ] 10.2 Add database configuration support
    - Implement SQLite configuration for development
    - Add PostgreSQL configuration for production
    - Create database migration scripts
    - Add connection pooling and health checks
    - _Requirements: 5.1, 5.2, 5.3_

- [ ] 11. Write comprehensive tests and documentation
  - [ ] 11.1 Complete BDD scenario coverage
    - Write Gherkin scenarios for all user management features
    - Test both SQLite and PostgreSQL configurations
    - Add error handling and edge case scenarios
    - Validate event sourcing and CQRS behavior
    - _Requirements: 6.4, 7.1, 7.2, 7.3_

  - [ ] 11.2 Add integration tests
    - Test EventStore with real database connections
    - Validate EventDispatcher with Watermill channels
    - Test end-to-end command and query flows
    - Add performance and concurrency tests
    - _Requirements: 6.4, 8.4_

- [ ] 12. Finalize library and demo
  - [ ] 12.1 Add library documentation using Diátaxis framework
    - Create tutorial documentation for getting started with the library
    - Write how-to guides for common implementation patterns
    - Add reference documentation with complete API documentation and godoc comments
    - Create explanation documentation covering DDD, CQRS, and Event Sourcing concepts
    - Structure documentation following Diátaxis principles (tutorial, how-to, reference, explanation)
    - _Requirements: 9.3, 9.4_

  - [ ] 12.2 Optimize performance and clean up code
    - Review and optimize JSON serialization performance
    - Ensure no reflection in hot paths
    - Add proper error handling and logging throughout
    - Validate clean architecture boundaries and dependencies
    - _Requirements: 8.3, 8.4, 8.5_