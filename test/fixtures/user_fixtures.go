package fixtures

import (
	"time"

	internalapp "github.com/example/pericarp/internal/application"
	i
)

// UserBuilder provides a fluent interface for building test users
type UserBuilder struct {
	id       uuid.UUID
	email    string
	name     string
	isActive bool
	version  int
}

// NewUserBuilder creates a new UserBuilder with default values
func NewUserBuilder() *UserBuilder {
	return &UserBuilder{
		id:       uuid.New(),
		email:    "test@example.com",
		name:     "Test User",
		isActive: true,
		version:  1,
	}
}

// WithID sets the user ID
func (b *UserBuilder) WithID(id uuid.UUID) *UserBuilder {
	b.id = id
	return b
}

// WithEmail sets the user email
func (b *UserBuilder) WithEmail(email string) *UserBuilder {
	b.email = email
	return b
}

// WithName sets the user name
func (b *UserBuilder) WithName(name string) *UserBuilder {
	b.name = name
	return b
}

// WithActive sets the user active status
func (b *UserBuilder) WithActive(isActive bool) *UserBuilder {
	b.isActive = isActive
	return b
}

// WithVersion sets the user version
func (b *UserBuilder) WithVersion(version int) *UserBuilder {
	b.version = version
	return b
}

// Build creates a User instance with the configured values
func (b *UserBuilder) Build() (*internaldomain.User, error) {
	user, err := internaldomain.NewUser(b.email, b.name)
	if err != nil {
		return nil, err
	}

	// Clear uncommitted events to avoid side effects in tests
	user.MarkEventsAsCommitted()

	return user, nil
}

// CommandBuilder provides a fluent interface for building test commands
type CommandBuilder struct{}

// NewCommandBuilder creates a new CommandBuilder
func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{}
}

// CreateUserCommand creates a CreateUserCommand with the given parameters
func (b *CommandBuilder) CreateUserCommand(email, name string) internalapp.CreateUserCommand {
	return internalapp.CreateUserCommand{
		ID:    uuid.New(),
		Email: email,
		Name:  name,
	}
}

// UpdateUserEmailCommand creates an UpdateUserEmailCommand
func (b *CommandBuilder) UpdateUserEmailCommand(userID uuid.UUID, newEmail string) internalapp.UpdateUserEmailCommand {
	return internalapp.UpdateUserEmailCommand{
		ID:       userID,
		NewEmail: newEmail,
	}
}

// UpdateUserNameCommand creates an UpdateUserNameCommand
func (b *CommandBuilder) UpdateUserNameCommand(userID uuid.UUID, newName string) internalapp.UpdateUserNameCommand {
	return internalapp.UpdateUserNameCommand{
		ID:      userID,
		NewName: newName,
	}
}

// DeactivateUserCommand creates a DeactivateUserCommand
func (b *CommandBuilder) DeactivateUserCommand(userID uuid.UUID) internalapp.DeactivateUserCommand {
	return internalapp.DeactivateUserCommand{
		ID: userID,
	}
}

// ActivateUserCommand creates an ActivateUserCommand
func (b *CommandBuilder) ActivateUserCommand(userID uuid.UUID) internalapp.ActivateUserCommand {
	return internalapp.ActivateUserCommand{
		ID: userID,
	}
}

// QueryBuilder provides a fluent interface for building test queries
type QueryBuilder struct{}

// NewQueryBuilder creates a new QueryBuilder
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{}
}

// GetUserQuery creates a GetUserQuery
func (b *QueryBuilder) GetUserQuery(userID uuid.UUID) internalapp.GetUserQuery {
	return internalapp.GetUserQuery{
		ID: userID,
	}
}

// GetUserByEmailQuery creates a GetUserByEmailQuery
func (b *QueryBuilder) GetUserByEmailQuery(email string) internalapp.GetUserByEmailQuery {
	return internalapp.GetUserByEmailQuery{
		Email: email,
	}
}

// ListUsersQuery creates a ListUsersQuery
func (b *QueryBuilder) ListUsersQuery(page, pageSize int, active *bool) internalapp.ListUsersQuery {
	return internalapp.ListUsersQuery{
		Page:     page,
		PageSize: pageSize,
		Active:   active,
	}
}

// ReadModelBuilder provides a fluent interface for building test read models
type ReadModelBuilder struct {
	id        uuid.UUID
	email     string
	name      string
	isActive  bool
	createdAt time.Time
	updatedAt time.Time
}

// NewReadModelBuilder creates a new ReadModelBuilder with default values
func NewReadModelBuilder() *ReadModelBuilder {
	now := time.Now()
	return &ReadModelBuilder{
		id:        uuid.New(),
		email:     "test@example.com",
		name:      "Test User",
		isActive:  true,
		createdAt: now,
		updatedAt: now,
	}
}

// WithID sets the read model ID
func (b *ReadModelBuilder) WithID(id uuid.UUID) *ReadModelBuilder {
	b.id = id
	return b
}

// WithEmail sets the read model email
func (b *ReadModelBuilder) WithEmail(email string) *ReadModelBuilder {
	b.email = email
	return b
}

// WithName sets the read model name
func (b *ReadModelBuilder) WithName(name string) *ReadModelBuilder {
	b.name = name
	return b
}

// WithActive sets the read model active status
func (b *ReadModelBuilder) WithActive(isActive bool) *ReadModelBuilder {
	b.isActive = isActive
	return b
}

// WithCreatedAt sets the read model creation time
func (b *ReadModelBuilder) WithCreatedAt(createdAt time.Time) *ReadModelBuilder {
	b.createdAt = createdAt
	return b
}

// WithUpdatedAt sets the read model update time
func (b *ReadModelBuilder) WithUpdatedAt(updatedAt time.Time) *ReadModelBuilder {
	b.updatedAt = updatedAt
	return b
}

// Build creates a UserReadModel instance with the configured values
func (b *ReadModelBuilder) Build() *internalapp.UserReadModel {
	return &internalapp.UserReadModel{
		ID:        b.id,
		Email:     b.email,
		Name:      b.name,
		IsActive:  b.isActive,
		CreatedAt: b.createdAt,
		UpdatedAt: b.updatedAt,
	}
}

// TestDataCleaner provides utilities for cleaning test data
type TestDataCleaner struct{}

// NewTestDataCleaner creates a new TestDataCleaner
func NewTestDataCleaner() *TestDataCleaner {
	return &TestDataCleaner{}
}

// CleanupTestData removes all test data from the provided repositories
func (c *TestDataCleaner) CleanupTestData() {
	// This would be implemented to clean up test data
	// For now, it's a placeholder as cleanup is handled in the BDD test context
}

// EventBuilder provides utilities for building test events
type EventBuilder struct{}

// NewEventBuilder creates a new EventBuilder
func NewEventBuilder() *EventBuilder {
	return &EventBuilder{}
}

// UserCreatedEvent creates a UserCreated event for testing
func (b *EventBuilder) UserCreatedEvent(userID uuid.UUID, email, name string) internaldomain.UserCreatedEvent {
	return internaldomain.NewUserCreatedEvent(userID, email, name, userID.String(), 1)
}

// UserEmailUpdatedEvent creates a UserEmailUpdated event for testing
func (b *EventBuilder) UserEmailUpdatedEvent(userID uuid.UUID, oldEmail, newEmail string, version int) internaldomain.UserEmailUpdatedEvent {
	return internaldomain.NewUserEmailUpdatedEvent(userID, oldEmail, newEmail, userID.String(), version)
}

// UserNameUpdatedEvent creates a UserNameUpdated event for testing
func (b *EventBuilder) UserNameUpdatedEvent(userID uuid.UUID, oldName, newName string, version int) internaldomain.UserNameUpdatedEvent {
	return internaldomain.NewUserNameUpdatedEvent(userID, oldName, newName, userID.String(), version)
}

// UserDeactivatedEvent creates a UserDeactivated event for testing
func (b *EventBuilder) UserDeactivatedEvent(userID uuid.UUID, version int) internaldomain.UserDeactivatedEvent {
	return internaldomain.NewUserDeactivatedEvent(userID, userID.String(), version)
}

// UserActivatedEvent creates a UserActivated event for testing
func (b *EventBuilder) UserActivatedEvent(userID uuid.UUID, version int) internaldomain.UserActivatedEvent {
	return internaldomain.NewUserActivatedEvent(userID, userID.String(), version)
}