package mocks

import (
	"context"
	"time"

	internalapp "github.com/akeemphilbert/pericarp/internal/application"
	internalappmocks "github.com/akeemphilbert/pericarp/internal/application/mocks"
	internaldomain "github.com/akeemphilbert/pericarp/internal/domain"
	internaldomainmocks "github.com/akeemphilbert/pericarp/in
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	pkgdomainmocks "github.com/akeemphilbert/pericarp/pkg/domain/mocks"
p
)

// MockConfiguration provides utilities for configuring mocks in tests
type MockConfiguration struct {
	EventStore              *pkgdomainmocks.EventStoreMock
	EventDispatcher         *pkgdomainmocks.EventDispatcherMock
	UnitOfWork              *pkgdomainmocks.UnitOfWorkMock
	UserRepository          *internaldomainmocks.UserRepositoryMock
	UserReadModelRepository *internalappmocks.UserReadModelRepositoryMock
}

// NewMockConfiguration creates a new mock configuration with default behaviors
func NewMockConfiguration() *MockConfiguration {
	return &MockConfiguration{
		EventStore:              &pkgdomainmocks.EventStoreMock{},
		EventDispatcher:         &pkgdomainmocks.EventDispatcherMock{},
		UnitOfWork:              &pkgdomainmocks.UnitOfWorkMock{},
		UserRepository:          &internaldomainmocks.UserRepositoryMock{},
		UserReadModelRepository: &internalappmocks.UserReadModelRepositoryMock{},
	}
}

// ConfigureSuccessfulEventStore configures the event store mock for successful operations
func (m *MockConfiguration) ConfigureSuccessfulEventStore() {
	m.EventStore.SaveFunc = func(ctx context.Context, events []pkgdomain.Event) ([]pkgdomain.Envelope, error) {
		envelopes := make([]pkgdomain.Envelope, len(events))
		for i, event := range events {
			envelopes[i] = &TestEnvelope{
				event:     event,
				eventID:   uuid.New().String(),
				timestamp: time.Now(),
				metadata: map[string]interface{}{
					"aggregate_id": event.AggregateID(),
					"event_type":   event.EventType(),
					"version":      event.Version(),
				},
			}
		}
		return envelopes, nil
	}

	m.EventStore.LoadFunc = func(ctx context.Context, aggregateID string) ([]pkgdomain.Envelope, error) {
		return []pkgdomain.Envelope{}, nil
	}

	m.EventStore.LoadFromVersionFunc = func(ctx context.Context, aggregateID string, version int) ([]pkgdomain.Envelope, error) {
		return []pkgdomain.Envelope{}, nil
	}
}

// ConfigureSuccessfulEventDispatcher configures the event dispatcher mock for successful operations
func (m *MockConfiguration) ConfigureSuccessfulEventDispatcher() {
	m.EventDispatcher.DispatchFunc = func(ctx context.Context, envelopes []pkgdomain.Envelope) error {
		return nil
	}

	m.EventDispatcher.SubscribeFunc = func(eventType string, handler pkgdomain.EventHandler) error {
		return nil
	}
}

// ConfigureSuccessfulUnitOfWork configures the unit of work mock for successful operations
func (m *MockConfiguration) ConfigureSuccessfulUnitOfWork() {
	var registeredEvents []pkgdomain.Event

	m.UnitOfWork.RegisterEventsFunc = func(events []pkgdomain.Event) {
		registeredEvents = append(registeredEvents, events...)
	}

	m.UnitOfWork.CommitFunc = func(ctx context.Context) ([]pkgdomain.Envelope, error) {
		envelopes := make([]pkgdomain.Envelope, len(registeredEvents))
		for i, event := range registeredEvents {
			envelopes[i] = &TestEnvelope{
				event:     event,
				eventID:   uuid.New().String(),
				timestamp: time.Now(),
				metadata: map[string]interface{}{
					"aggregate_id": event.AggregateID(),
					"event_type":   event.EventType(),
					"version":      event.Version(),
				},
			}
		}
		registeredEvents = nil // Clear after commit
		return envelopes, nil
	}

	m.UnitOfWork.RollbackFunc = func() error {
		registeredEvents = nil
		return nil
	}
}

// ConfigureSuccessfulUserRepository configures the user repository mock for successful operations
func (m *MockConfiguration) ConfigureSuccessfulUserRepository() {
	users := make(map[uuid.UUID]*internaldomain.User)

	m.UserRepository.SaveFunc = func(user *internaldomain.User) error {
		users[user.UserID()] = user
		return nil
	}

	m.UserRepository.FindByIDFunc = func(id uuid.UUID) (*internaldomain.User, error) {
		if user, exists := users[id]; exists {
			return user, nil
		}
		return nil, &internalapp.ApplicationError{Code: "USER_NOT_FOUND", Message: "User not found"}
	}

	m.UserRepository.FindByEmailFunc = func(email string) (*internaldomain.User, error) {
		for _, user := range users {
			if user.Email() == email {
				return user, nil
			}
		}
		return nil, &internalapp.ApplicationError{Code: "USER_NOT_FOUND", Message: "User not found"}
	}

	m.UserRepository.DeleteFunc = func(id uuid.UUID) error {
		delete(users, id)
		return nil
	}

	m.UserRepository.LoadFromVersionFunc = func(id uuid.UUID, version int) (*internaldomain.User, error) {
		if user, exists := users[id]; exists {
			return user, nil
		}
		return nil, &internalapp.ApplicationError{Code: "USER_NOT_FOUND", Message: "User not found"}
	}
}

// ConfigureSuccessfulUserReadModelRepository configures the user read model repository mock for successful operations
func (m *MockConfiguration) ConfigureSuccessfulUserReadModelRepository() {
	users := make(map[uuid.UUID]*internalapp.UserReadModel)

	m.UserReadModelRepository.GetByIDFunc = func(ctx context.Context, id uuid.UUID) (*internalapp.UserReadModel, error) {
		if user, exists := users[id]; exists {
			return user, nil
		}
		return nil, &internalapp.ApplicationError{Code: "USER_NOT_FOUND", Message: "User not found"}
	}

	m.UserReadModelRepository.GetByEmailFunc = func(ctx context.Context, email string) (*internalapp.UserReadModel, error) {
		for _, user := range users {
			if user.Email == email {
				return user, nil
			}
		}
		return nil, &internalapp.ApplicationError{Code: "USER_NOT_FOUND", Message: "User not found"}
	}

	m.UserReadModelRepository.ListFunc = func(ctx context.Context, page, pageSize int, active *bool) ([]internalapp.UserReadModel, int, error) {
		var filteredUsers []internalapp.UserReadModel
		for _, user := range users {
			if active == nil || user.IsActive == *active {
				filteredUsers = append(filteredUsers, *user)
			}
		}

		totalCount := len(filteredUsers)
		start := (page - 1) * pageSize
		end := start + pageSize

		if start >= totalCount {
			return []internalapp.UserReadModel{}, totalCount, nil
		}

		if end > totalCount {
			end = totalCount
		}

		return filteredUsers[start:end], totalCount, nil
	}

	m.UserReadModelRepository.SaveFunc = func(ctx context.Context, user *internalapp.UserReadModel) error {
		users[user.ID] = user
		return nil
	}

	m.UserReadModelRepository.DeleteFunc = func(ctx context.Context, id uuid.UUID) error {
		delete(users, id)
		return nil
	}

	m.UserReadModelRepository.CountFunc = func(ctx context.Context, active *bool) (int, error) {
		count := 0
		for _, user := range users {
			if active == nil || user.IsActive == *active {
				count++
			}
		}
		return count, nil
	}
}

// ConfigureAllForSuccess configures all mocks for successful operations
func (m *MockConfiguration) ConfigureAllForSuccess() {
	m.ConfigureSuccessfulEventStore()
	m.ConfigureSuccessfulEventDispatcher()
	m.ConfigureSuccessfulUnitOfWork()
	m.ConfigureSuccessfulUserRepository()
	m.ConfigureSuccessfulUserReadModelRepository()
}

// TestScenarioBuilder provides utilities for building test scenarios with mocks
type TestScenarioBuilder struct {
	config *MockConfiguration
}

// NewTestScenarioBuilder creates a new test scenario builder
func NewTestScenarioBuilder() *TestScenarioBuilder {
	return &TestScenarioBuilder{
		config: NewMockConfiguration(),
	}
}

// WithSuccessfulUserCreation configures mocks for successful user creation scenario
func (b *TestScenarioBuilder) WithSuccessfulUserCreation() *TestScenarioBuilder {
	b.config.ConfigureAllForSuccess()
	return b
}

// WithUserRepositoryError configures the user repository to return an error
func (b *TestScenarioBuilder) WithUserRepositoryError(err error) *TestScenarioBuilder {
	b.config.UserRepository.SaveFunc = func(user *internaldomain.User) error {
		return err
	}
	return b
}

// WithEventStoreError configures the event store to return an error
func (b *TestScenarioBuilder) WithEventStoreError(err error) *TestScenarioBuilder {
	b.config.EventStore.SaveFunc = func(ctx context.Context, events []pkgdomain.Event) ([]pkgdomain.Envelope, error) {
		return nil, err
	}
	return b
}

// WithEventDispatcherError configures the event dispatcher to return an error
func (b *TestScenarioBuilder) WithEventDispatcherError(err error) *TestScenarioBuilder {
	b.config.EventDispatcher.DispatchFunc = func(ctx context.Context, envelopes []pkgdomain.Envelope) error {
		return err
	}
	return b
}

// WithExistingUser configures the repository to return an existing user for email lookup
func (b *TestScenarioBuilder) WithExistingUser(email string, user *internaldomain.User) *TestScenarioBuilder {
	b.config.UserRepository.FindByEmailFunc = func(searchEmail string) (*internaldomain.User, error) {
		if searchEmail == email {
			return user, nil
		}
		return nil, &internalapp.ApplicationError{Code: "USER_NOT_FOUND", Message: "User not found"}
	}
	return b
}

// Build returns the configured mock configuration
func (b *TestScenarioBuilder) Build() *MockConfiguration {
	return b.config
}

// AssertionHelpers provides utilities for making assertions about mock calls
type AssertionHelpers struct{}

// NewAssertionHelpers creates a new assertion helpers instance
func NewAssertionHelpers() *AssertionHelpers {
	return &AssertionHelpers{}
}

// AssertEventStoreSaveCalled verifies that EventStore.Save was called with expected events
func (h *AssertionHelpers) AssertEventStoreSaveCalled(mock *pkgdomainmocks.EventStoreMock, expectedEventCount int) error {
	calls := mock.SaveCalls()
	if len(calls) == 0 {
		return &AssertionError{Message: "EventStore.Save was not called"}
	}

	lastCall := calls[len(calls)-1]
	if len(lastCall.Events) != expectedEventCount {
		return &AssertionError{
			Message:  "EventStore.Save called with unexpected number of events",
			Expected: expectedEventCount,
			Actual:   len(lastCall.Events),
		}
	}

	return nil
}

// AssertEventDispatcherDispatchCalled verifies that EventDispatcher.Dispatch was called
func (h *AssertionHelpers) AssertEventDispatcherDispatchCalled(mock *pkgdomainmocks.EventDispatcherMock) error {
	calls := mock.DispatchCalls()
	if len(calls) == 0 {
		return &AssertionError{Message: "EventDispatcher.Dispatch was not called"}
	}
	return nil
}

// AssertUserRepositorySaveCalled verifies that UserRepository.Save was called
func (h *AssertionHelpers) AssertUserRepositorySaveCalled(mock *internaldomainmocks.UserRepositoryMock) error {
	calls := mock.SaveCalls()
	if len(calls) == 0 {
		return &AssertionError{Message: "UserRepository.Save was not called"}
	}
	return nil
}

// AssertionError represents an assertion failure
type AssertionError struct {
	Message  string
	Expected interface{}
	Actual   interface{}
}

func (e *AssertionError) Error() string {
	if e.Expected != nil && e.Actual != nil {
		return fmt.Sprintf("%s: expected %v, got %v", e.Message, e.Expected, e.Actual)
	}
	return e.Message
}