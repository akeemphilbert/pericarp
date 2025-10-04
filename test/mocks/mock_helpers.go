package mocks

import (
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/examples"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	pkgdomainmocks "github.com/akeemphilbert/pericarp/pkg/domain/mocks"
	"github.com/google/uuid"
)

// TestEnvelope is a test implementation of the Envelope interface
type TestEnvelope struct {
	event     pkgdomain.Event
	eventID   string
	timestamp time.Time
	metadata  map[string]interface{}
}

func (e *TestEnvelope) Event() pkgdomain.Event {
	return e.event
}

func (e *TestEnvelope) EventID() string {
	return e.eventID
}

func (e *TestEnvelope) Timestamp() time.Time {
	return e.timestamp
}

func (e *TestEnvelope) Metadata() map[string]interface{} {
	return e.metadata
}

// NewTestEnvelope creates a new test envelope
func NewTestEnvelope(event pkgdomain.Event) *TestEnvelope {
	return &TestEnvelope{
		event:     event,
		eventID:   uuid.New().String(),
		timestamp: time.Now(),
		metadata:  make(map[string]interface{}),
	}
}

// TestScenarioBuilder helps build test scenarios
type TestScenarioBuilder struct {
	config *TestScenarioConfig
}

// TestScenarioConfig holds configuration for test scenarios
type TestScenarioConfig struct {
	EventStore      pkgdomain.EventStore
	EventDispatcher pkgdomain.EventDispatcher
	Logger          pkgdomain.Logger
}

// NewTestScenarioBuilder creates a new test scenario builder
func NewTestScenarioBuilder() *TestScenarioBuilder {
	return &TestScenarioBuilder{
		config: &TestScenarioConfig{
			EventStore:      &pkgdomainmocks.EventStoreMock{},
			EventDispatcher: &pkgdomainmocks.EventDispatcherMock{},
			Logger:          &pkgdomainmocks.LoggerMock{},
		},
	}
}

// WithEventStore sets the event store
func (b *TestScenarioBuilder) WithEventStore(eventStore pkgdomain.EventStore) *TestScenarioBuilder {
	b.config.EventStore = eventStore
	return b
}

// WithEventDispatcher sets the event dispatcher
func (b *TestScenarioBuilder) WithEventDispatcher(dispatcher pkgdomain.EventDispatcher) *TestScenarioBuilder {
	b.config.EventDispatcher = dispatcher
	return b
}

// WithLogger sets the logger
func (b *TestScenarioBuilder) WithLogger(logger pkgdomain.Logger) *TestScenarioBuilder {
	b.config.Logger = logger
	return b
}

// Build creates the test scenario
func (b *TestScenarioBuilder) Build() *TestScenarioConfig {
	return b.config
}

// TestUserBuilder helps build test users
type TestUserBuilder struct {
	id       string
	email    string
	name     string
	isActive bool
}

// NewTestUserBuilder creates a new test user builder
func NewTestUserBuilder() *TestUserBuilder {
	return &TestUserBuilder{
		id:       uuid.New().String(),
		email:    "test@example.com",
		name:     "Test User",
		isActive: true,
	}
}

// WithID sets the user GetID
func (b *TestUserBuilder) WithID(id string) *TestUserBuilder {
	b.id = id
	return b
}

// WithEmail sets the user email
func (b *TestUserBuilder) WithEmail(email string) *TestUserBuilder {
	b.email = email
	return b
}

// WithName sets the user name
func (b *TestUserBuilder) WithName(name string) *TestUserBuilder {
	b.name = name
	return b
}

// WithActive sets the user active status
func (b *TestUserBuilder) WithActive(isActive bool) *TestUserBuilder {
	b.isActive = isActive
	return b
}

// Build creates a test user
func (b *TestUserBuilder) Build() (*examples.User, error) {
	user, err := examples.NewUser(b.id, b.email, b.name)
	if err != nil {
		return nil, err
	}

	if !b.isActive {
		if err := user.Deactivate(); err != nil {
			return nil, err
		}
	}

	return user, nil
}

// TestEventBuilder helps build test events
type TestEventBuilder struct{}

// NewTestEventBuilder creates a new test event builder
func NewTestEventBuilder() *TestEventBuilder {
	return &TestEventBuilder{}
}

// UserCreatedEvent creates a user created event
func (b *TestEventBuilder) UserCreatedEvent(userID, email, name string) pkgdomain.Event {
	return pkgdomain.NewEntityEvent(nil, nil, "user", "created", userID, map[string]interface{}{
		"email": email,
		"name":  name,
	})
}

// UserEmailChangedEvent creates a user email changed event
func (b *TestEventBuilder) UserEmailChangedEvent(userID, oldEmail, newEmail string) pkgdomain.Event {
	return pkgdomain.NewEntityEvent(nil, nil, "user", "email_changed", userID, map[string]interface{}{
		"old_email": oldEmail,
		"new_email": newEmail,
	})
}

// UserActivatedEvent creates a user activated event
func (b *TestEventBuilder) UserActivatedEvent(userID string) pkgdomain.Event {
	return pkgdomain.NewEntityEvent(nil, nil, "user", "activated", userID, map[string]interface{}{
		"activated_at": time.Now(),
	})
}

// UserDeactivatedEvent creates a user deactivated event
func (b *TestEventBuilder) UserDeactivatedEvent(userID string) pkgdomain.Event {
	return pkgdomain.NewEntityEvent(nil, nil, "user", "deactivated", userID, map[string]interface{}{
		"deactivated_at": time.Now(),
	})
}

// AssertionHelpers provides helper functions for test assertions
type AssertionHelpers struct{}

// NewAssertionHelpers creates a new assertion helpers instance
func NewAssertionHelpers() *AssertionHelpers {
	return &AssertionHelpers{}
}

// AssertEventStoreSaveCalled checks if EventStore.Save was called
func (h *AssertionHelpers) AssertEventStoreSaveCalled(mock *pkgdomainmocks.EventStoreMock) error {
	if mock.SaveFunc == nil {
		return fmt.Errorf("EventStore.Save was not called")
	}
	return nil
}

// AssertEventStoreGetEventsCalled checks if EventStore.GetEvents was called
func (h *AssertionHelpers) AssertEventStoreGetEventsCalled(mock *pkgdomainmocks.EventStoreMock) error {
	if mock.GetEventsFunc == nil {
		return fmt.Errorf("EventStore.GetEvents was not called")
	}
	return nil
}

// AssertEventDispatcherDispatchCalled checks if EventDispatcher.Dispatch was called
func (h *AssertionHelpers) AssertEventDispatcherDispatchCalled(mock *pkgdomainmocks.EventDispatcherMock) error {
	if mock.DispatchFunc == nil {
		return fmt.Errorf("EventDispatcher.Dispatch was not called")
	}
	return nil
}

// AssertLoggerInfoCalled checks if Logger.Info was called
func (h *AssertionHelpers) AssertLoggerInfoCalled(mock *pkgdomainmocks.LoggerMock) error {
	if mock.InfoFunc == nil {
		return fmt.Errorf("Logger.Info was not called")
	}
	return nil
}

// AssertLoggerErrorCalled checks if Logger.Error was called
func (h *AssertionHelpers) AssertLoggerErrorCalled(mock *pkgdomainmocks.LoggerMock) error {
	if mock.ErrorFunc == nil {
		return fmt.Errorf("Logger.Error was not called")
	}
	return nil
}

// TestData provides common test data
type TestData struct{}

// NewTestData creates a new test data instance
func NewTestData() *TestData {
	return &TestData{}
}

// ValidUser returns a valid user for testing
func (td *TestData) ValidUser() *examples.User {
	user, _ := examples.NewUser(uuid.New().String(), "valid@example.com", "Valid User")
	return user
}

// ValidUserWithID returns a valid user with specific GetID
func (td *TestData) ValidUserWithID(id string) *examples.User {
	user, _ := examples.NewUser(id, "valid@example.com", "Valid User")
	return user
}

// InactiveUser returns an inactive user for testing
func (td *TestData) InactiveUser() *examples.User {
	user, _ := examples.NewUser(uuid.New().String(), "inactive@example.com", "Inactive User")
	user.Deactivate()
	return user
}

// UserWithEmail returns a user with specific email
func (td *TestData) UserWithEmail(email string) *examples.User {
	user, _ := examples.NewUser(uuid.New().String(), email, "Test User")
	return user
}

// MultipleUsers returns multiple users for testing
func (td *TestData) MultipleUsers(count int) []*examples.User {
	users := make([]*examples.User, count)
	for i := 0; i < count; i++ {
		users[i], _ = examples.NewUser(
			uuid.New().String(),
			fmt.Sprintf("user%d@example.com", i),
			fmt.Sprintf("User %d", i),
		)
	}
	return users
}

// EventSequence returns a sequence of events for testing
func (td *TestData) EventSequence(userID string) []pkgdomain.Event {
	eventBuilder := NewTestEventBuilder()
	return []pkgdomain.Event{
		eventBuilder.UserCreatedEvent(userID, "test@example.com", "Test User"),
		eventBuilder.UserEmailChangedEvent(userID, "test@example.com", "newemail@example.com"),
		eventBuilder.UserDeactivatedEvent(userID),
		eventBuilder.UserActivatedEvent(userID),
	}
}

// Helper functions for common test scenarios

// CreateUserWithEvents creates a user and returns it with its events
func CreateUserWithEvents(id, email, name string) (*examples.User, []pkgdomain.Event, error) {
	user, err := examples.NewUser(id, email, name)
	if err != nil {
		return nil, nil, err
	}

	events := user.GetEvents()
	return user, events, nil
}

// CreateUserWithEmailChange creates a user, changes email, and returns all events
func CreateUserWithEmailChange(id, email, name, newEmail string) (*examples.User, []pkgdomain.Event, error) {
	user, err := examples.NewUser(id, email, name)
	if err != nil {
		return nil, nil, err
	}

	// Get initial events
	initialEvents := user.GetEvents()

	// Change email
	if err := user.ChangeEmail(newEmail); err != nil {
		return nil, nil, err
	}

	// Get all events
	allEvents := user.GetEvents()
	return user, allEvents, nil
}

// CreateUserWithLifecycle creates a user and performs full lifecycle operations
func CreateUserWithLifecycle(id, email, name string) (*examples.User, []pkgdomain.Event, error) {
	user, err := examples.NewUser(id, email, name)
	if err != nil {
		return nil, nil, err
	}

	// Change email
	if err := user.ChangeEmail("newemail@example.com"); err != nil {
		return nil, nil, err
	}

	// Deactivate
	if err := user.Deactivate(); err != nil {
		return nil, nil, err
	}

	// Activate
	if err := user.Activate(); err != nil {
		return nil, nil, err
	}

	// Get all events
	allEvents := user.GetEvents()
	return user, allEvents, nil
}
