package application

import (
	"context"
	"time"

	"github.com/example/pericarp/pkg/domain"
)

// DemoCommandBusSetup configures a command bus with middleware for demo purposes
func DemoCommandBusSetup(
	userRepo domain.UserRepository,
	unitOfWork domain.UnitOfWork,
	metrics MetricsCollector,
) CommandBus {
	// Create command bus
	commandBus := NewCommandBus()

	// Configure middleware stack (applied in reverse order)
	commandBus.Use(
		ErrorHandlingCommandMiddleware(),  // Outermost - handles panics and wraps errors
		LoggingCommandMiddleware(),        // Logs command execution
		ValidationCommandMiddleware(),     // Validates commands
		MetricsCommandMiddleware(metrics), // Collects metrics
	)

	// Register command handlers
	createUserHandler := NewCreateUserHandler(userRepo, unitOfWork)
	updateEmailHandler := NewUpdateUserEmailHandler(userRepo, unitOfWork)

	// Register handlers with the bus (using type assertion to work around Go's type system)
	commandBus.Register("CreateUser", &createUserCommandHandlerAdapter{handler: createUserHandler})
	commandBus.Register("UpdateUserEmail", &updateEmailCommandHandlerAdapter{handler: updateEmailHandler})

	return commandBus
}

// SimpleMetricsCollector provides a basic implementation of MetricsCollector for demo
type SimpleMetricsCollector struct {
	commandDurations map[string][]int64 // nanoseconds
	queryDurations   map[string][]int64 // nanoseconds
	commandErrors    map[string]int
	queryErrors      map[string]int
}

// NewSimpleMetricsCollector creates a new simple metrics collector
func NewSimpleMetricsCollector() *SimpleMetricsCollector {
	return &SimpleMetricsCollector{
		commandDurations: make(map[string][]int64),
		queryDurations:   make(map[string][]int64),
		commandErrors:    make(map[string]int),
		queryErrors:      make(map[string]int),
	}
}

// RecordCommandDuration records the duration of a command execution
func (m *SimpleMetricsCollector) RecordCommandDuration(commandType string, duration time.Duration) {
	if m.commandDurations[commandType] == nil {
		m.commandDurations[commandType] = make([]int64, 0)
	}
	m.commandDurations[commandType] = append(m.commandDurations[commandType], duration.Nanoseconds())
}

// RecordQueryDuration records the duration of a query execution
func (m *SimpleMetricsCollector) RecordQueryDuration(queryType string, duration time.Duration) {
	if m.queryDurations[queryType] == nil {
		m.queryDurations[queryType] = make([]int64, 0)
	}
	m.queryDurations[queryType] = append(m.queryDurations[queryType], duration.Nanoseconds())
}

// IncrementCommandErrors increments the error count for a command type
func (m *SimpleMetricsCollector) IncrementCommandErrors(commandType string) {
	m.commandErrors[commandType]++
}

// IncrementQueryErrors increments the error count for a query type
func (m *SimpleMetricsCollector) IncrementQueryErrors(queryType string) {
	m.queryErrors[queryType]++
}

// GetCommandMetrics returns command metrics for a given type
func (m *SimpleMetricsCollector) GetCommandMetrics(commandType string) (durations []int64, errors int) {
	return m.commandDurations[commandType], m.commandErrors[commandType]
}

// GetQueryMetrics returns query metrics for a given type
func (m *SimpleMetricsCollector) GetQueryMetrics(queryType string) (durations []int64, errors int) {
	return m.queryDurations[queryType], m.queryErrors[queryType]
}

// Adapter types to bridge specific command handlers to generic CommandHandler interface

type createUserCommandHandlerAdapter struct {
	handler *CreateUserHandler
}

func (a *createUserCommandHandlerAdapter) Handle(ctx context.Context, logger domain.Logger, cmd Command) error {
	createCmd, ok := cmd.(CreateUserCommand)
	if !ok {
		return NewApplicationError("INVALID_COMMAND_TYPE", "Expected CreateUserCommand", nil)
	}
	return a.handler.Handle(ctx, logger, createCmd)
}

type updateEmailCommandHandlerAdapter struct {
	handler *UpdateUserEmailHandler
}

func (a *updateEmailCommandHandlerAdapter) Handle(ctx context.Context, logger domain.Logger, cmd Command) error {
	updateCmd, ok := cmd.(UpdateUserEmailCommand)
	if !ok {
		return NewApplicationError("INVALID_COMMAND_TYPE", "Expected UpdateUserEmailCommand", nil)
	}
	return a.handler.Handle(ctx, logger, updateCmd)
}

// DemoQueryBusSetup configures a query bus with middleware for demo purposes
func DemoQueryBusSetup(
	readModelRepo UserReadModelRepository,
	cache CacheProvider,
	metrics MetricsCollector,
) QueryBus {
	// Create query bus
	queryBus := NewQueryBus()

	// Configure middleware stack (applied in reverse order)
	queryBus.Use(
		ErrorHandlingQueryMiddleware(),  // Outermost - handles panics and wraps errors
		LoggingQueryMiddleware(),        // Logs query execution
		ValidationQueryMiddleware(),     // Validates queries
		CachingQueryMiddleware(cache),   // Caches query results
		MetricsQueryMiddleware(metrics), // Collects metrics
	)

	// Register query handlers
	getUserHandler := NewGetUserHandler(readModelRepo)
	listUsersHandler := NewListUsersHandler(readModelRepo)

	// Register handlers with the bus
	queryBus.Register("GetUser", &getUserQueryHandlerAdapter{handler: getUserHandler})
	queryBus.Register("ListUsers", &listUsersQueryHandlerAdapter{handler: listUsersHandler})

	return queryBus
}

// SimpleCache provides a basic in-memory cache implementation for demo
type SimpleCache struct {
	data map[string]interface{}
}

// NewSimpleCache creates a new simple cache
func NewSimpleCache() *SimpleCache {
	return &SimpleCache{
		data: make(map[string]interface{}),
	}
}

// Get retrieves a value from the cache
func (c *SimpleCache) Get(key string) (interface{}, bool) {
	value, exists := c.data[key]
	return value, exists
}

// Set stores a value in the cache
func (c *SimpleCache) Set(key string, value interface{}) {
	c.data[key] = value
}

// Delete removes a value from the cache
func (c *SimpleCache) Delete(key string) {
	delete(c.data, key)
}

// Clear removes all values from the cache
func (c *SimpleCache) Clear() {
	c.data = make(map[string]interface{})
}

// Adapter types for query handlers

type getUserQueryHandlerAdapter struct {
	handler *GetUserHandler
}

func (a *getUserQueryHandlerAdapter) Handle(ctx context.Context, logger domain.Logger, query Query) (interface{}, error) {
	getUserQuery, ok := query.(GetUserQuery)
	if !ok {
		return nil, NewApplicationError("INVALID_QUERY_TYPE", "Expected GetUserQuery", nil)
	}
	return a.handler.Handle(ctx, logger, getUserQuery)
}

type listUsersQueryHandlerAdapter struct {
	handler *ListUsersHandler
}

func (a *listUsersQueryHandlerAdapter) Handle(ctx context.Context, logger domain.Logger, query Query) (interface{}, error) {
	listUsersQuery, ok := query.(ListUsersQuery)
	if !ok {
		return nil, NewApplicationError("INVALID_QUERY_TYPE", "Expected ListUsersQuery", nil)
	}
	return a.handler.Handle(ctx, logger, listUsersQuery)
}
