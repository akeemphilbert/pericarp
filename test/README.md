# Pericarp Test Suite

This directory contains comprehensive tests for the Pericarp Go library, implementing Domain-Driven Design (DDD), Command Query Responsibility Segregation (CQRS), and Event Sourcing patterns.

## Test Structure

```
test/
├── bdd/                    # Behavior-Driven Development tests
│   ├── user_management_test.go      # Main BDD scenarios
│   ├── database_sqlite_test.go      # SQLite-specific scenarios
│   └── database_postgres_test.go    # PostgreSQL-specific scenarios
├── config/                 # Test configuration utilities
│   └── test_config.go
├── fixtures/               # Test data builders and fixtures
│   └── user_fixtures.go
├── integration/            # Integration tests
│   ├── eventstore_integration_test.go
│   ├── eventdispatcher_integration_test.go
│   ├── end_to_end_test.go
│   └── performance_test.go
└── mocks/                  # Mock implementations and helpers
    ├── mock_helpers.go
    └── test_doubles.go
```

## Test Categories

### 1. Unit Tests
Located in `pkg/` and `internal/` directories alongside the source code.

- **Domain Tests**: Pure domain logic testing with no external dependencies
- **Application Tests**: Command/query handlers with mocked dependencies
- **Infrastructure Tests**: Infrastructure components with test containers

### 2. BDD Tests (Behavior-Driven Development)
Located in `test/bdd/` directory.

- **User Management**: Complete user lifecycle scenarios
- **Database Specific**: SQLite and PostgreSQL specific behaviors
- **Error Handling**: Edge cases and error scenarios
- **CQRS & Event Sourcing**: Pattern validation scenarios

### 3. Integration Tests
Located in `test/integration/` directory.

- **EventStore Integration**: Real database connections and operations
- **EventDispatcher Integration**: Watermill channel testing
- **End-to-End Flows**: Complete command and query flows
- **Performance Tests**: Load testing and concurrency validation

## Running Tests

### Prerequisites

1. **Go 1.21+** installed
2. **Optional**: PostgreSQL for database-specific tests
3. **Optional**: golangci-lint for code quality checks
4. **Optional**: gosec for security scanning

### Environment Variables

```bash
# Optional: PostgreSQL connection for integration tests
export POSTGRES_TEST_DSN="host=localhost user=postgres password=postgres dbname=pericarp_test port=5432 sslmode=disable"
```

### Quick Start

```bash
# Run all tests
make test

# Run specific test categories
make test-unit          # Unit tests only
make test-bdd           # BDD tests only
make test-integration   # Integration tests only

# Database-specific tests
make test-bdd-sqlite    # BDD tests with SQLite
make test-bdd-postgres  # BDD tests with PostgreSQL (requires POSTGRES_TEST_DSN)

# Performance tests
make test-performance   # Performance and load tests
```

### Comprehensive Test Suite

Run the complete test suite with detailed reporting:

```bash
# Full test suite
./scripts/run-comprehensive-tests.sh

# Quick test suite (skip performance tests)
./scripts/run-comprehensive-tests.sh --short
```

## Test Features

### BDD Scenarios

The BDD tests cover comprehensive scenarios including:

- **User Lifecycle**: Creation, updates, activation/deactivation
- **Input Validation**: Email format, name length, empty values
- **Error Handling**: Duplicate emails, non-existent users, validation failures
- **Concurrency**: Optimistic locking, concurrent modifications
- **Event Sourcing**: State reconstruction, event ordering, version control
- **Database Support**: Both SQLite and PostgreSQL configurations
- **Performance**: Bulk operations, large datasets, query optimization

### Integration Testing

Integration tests validate:

- **Real Database Operations**: SQLite and PostgreSQL
- **Event Store Persistence**: Large event streams, concurrent access
- **Event Dispatcher**: Watermill channels, multiple handlers
- **End-to-End Flows**: Complete CQRS operations
- **Performance Metrics**: Throughput, latency, concurrency limits

### Performance Testing

Performance tests measure:

- **Throughput**: Operations per second under load
- **Concurrency**: Multiple goroutines, race conditions
- **Scalability**: Large datasets, memory usage
- **Latency**: Response times, query performance

## Test Data Management

### Fixtures and Builders

The test suite uses builder patterns for creating test data:

```go
// User builder
user := fixtures.NewUserBuilder().
    WithEmail("test@example.com").
    WithName("Test User").
    WithActive(true).
    Build()

// Command builder
cmd := fixtures.NewCommandBuilder().
    CreateUserCommand("test@example.com", "Test User")
```

### Mock Configuration

Comprehensive mock setup for testing:

```go
mocks := NewMockConfiguration()
mocks.ConfigureAllForSuccess()

// Or configure specific scenarios
mocks.WithUserRepositoryError(errors.New("database error"))
```

## Test Coverage

The test suite aims for high coverage across all layers:

- **Domain Layer**: 90%+ coverage (pure business logic)
- **Application Layer**: 85%+ coverage (handlers and services)
- **Infrastructure Layer**: 80%+ coverage (database and messaging)

Generate coverage reports:

```bash
make test-coverage
open coverage.html
```

## Continuous Integration

The test suite is designed for CI/CD pipelines:

```bash
# CI-friendly test execution
make ci

# Docker-based testing
make docker-test
```

## Best Practices

### Writing BDD Scenarios

1. **Focus on Business Behavior**: Write scenarios from user perspective
2. **Use Given-When-Then**: Clear scenario structure
3. **Avoid Implementation Details**: Test behavior, not implementation
4. **Keep Scenarios Independent**: Each scenario should be self-contained

### Integration Testing

1. **Use Real Dependencies**: Test with actual databases when possible
2. **Clean State**: Ensure clean state between tests
3. **Performance Assertions**: Include performance expectations
4. **Error Scenarios**: Test failure modes and recovery

### Performance Testing

1. **Realistic Load**: Use realistic data volumes and concurrency
2. **Measure Key Metrics**: Throughput, latency, resource usage
3. **Set Thresholds**: Define acceptable performance limits
4. **Monitor Trends**: Track performance over time

## Troubleshooting

### Common Issues

1. **PostgreSQL Connection**: Ensure POSTGRES_TEST_DSN is set correctly
2. **Port Conflicts**: Check for conflicting database instances
3. **Memory Issues**: Increase limits for large dataset tests
4. **Timeout Errors**: Adjust test timeouts for slower systems

### Debug Mode

Enable verbose logging for debugging:

```bash
go test -v -tags=integration ./test/integration/... -run=TestName
```

### Test Isolation

If tests interfere with each other:

```bash
# Run tests sequentially
go test -p 1 ./test/...

# Clean test cache
go clean -testcache
```

## Contributing

When adding new tests:

1. **Follow Naming Conventions**: Use descriptive test names
2. **Add Documentation**: Document complex test scenarios
3. **Update Coverage**: Ensure new code is tested
4. **Performance Impact**: Consider performance implications
5. **Cross-Platform**: Ensure tests work on different platforms

## Metrics and Reporting

The test suite provides detailed metrics:

- **Test Execution Time**: Per test and total duration
- **Coverage Percentage**: Line and branch coverage
- **Performance Metrics**: Throughput and latency measurements
- **Error Rates**: Success/failure ratios
- **Resource Usage**: Memory and CPU utilization

These metrics help ensure the library meets performance and reliability requirements.