package fixtures

import (
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/examples"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/google/uuid"
)

// UserBuilder provides a fluent interface for building test users
type UserBuilder struct {
	id       string
	email    string
	name     string
	isActive bool
}

// NewUserBuilder creates a new UserBuilder with default values
func NewUserBuilder() *UserBuilder {
	return &UserBuilder{
		id:       uuid.New().String(),
		email:    "test@example.com",
		name:     "Test User",
		isActive: true,
	}
}

// WithID sets the user ID
func (b *UserBuilder) WithID(id string) *UserBuilder {
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

// Build creates a User instance with the configured values
func (b *UserBuilder) Build() (*examples.User, error) {
	user, err := examples.NewUser(b.id, b.email, b.name)
	if err != nil {
		return nil, err
	}

	// Set active status if needed
	if !b.isActive {
		if err := user.Deactivate(); err != nil {
			return nil, err
		}
	}

	return user, nil
}

// EventBuilder provides a fluent interface for building test events
type EventBuilder struct{}

// NewEventBuilder creates a new EventBuilder
func NewEventBuilder() *EventBuilder {
	return &EventBuilder{}
}

// UserCreatedEvent creates a user created event
func (b *EventBuilder) UserCreatedEvent(userID, email, name string) pkgdomain.Event {
	user, err := examples.NewUser(userID, email, name)
	if err != nil {
		// Return a basic event if user creation fails
		return pkgdomain.NewEntityEvent("user", "created", userID, "", "", map[string]interface{}{
			"email": email,
			"name":  name,
		})
	}

	events := user.GetEvents()
	if len(events) > 0 {
		return events[0]
	}

	// Fallback to basic event
	return pkgdomain.NewEntityEvent("user", "created", userID, "", "", map[string]interface{}{
		"email": email,
		"name":  name,
	})
}

// UserEmailChangedEvent creates a user email changed event
func (b *EventBuilder) UserEmailChangedEvent(userID, oldEmail, newEmail string) pkgdomain.Event {
	return pkgdomain.NewEntityEvent("user", "email_changed", userID, "", "", map[string]interface{}{
		"old_email": oldEmail,
		"new_email": newEmail,
	})
}

// UserActivatedEvent creates a user activated event
func (b *EventBuilder) UserActivatedEvent(userID string) pkgdomain.Event {
	return pkgdomain.NewEntityEvent("user", "activated", userID, "", "", map[string]interface{}{
		"activated_at": time.Now(),
		"reason":       "test",
	})
}

// UserDeactivatedEvent creates a user deactivated event
func (b *EventBuilder) UserDeactivatedEvent(userID string) pkgdomain.Event {
	return pkgdomain.NewEntityEvent("user", "deactivated", userID, "", "", map[string]interface{}{
		"deactivated_at": time.Now(),
		"reason":         "test",
	})
}

// TestData provides common test data
type TestData struct{}

// NewTestData creates a new TestData instance
func NewTestData() *TestData {
	return &TestData{}
}

// ValidUser returns a valid user for testing
func (td *TestData) ValidUser() *examples.User {
	user, _ := examples.NewUser(uuid.New().String(), "valid@example.com", "Valid User")
	return user
}

// ValidUserWithID returns a valid user with specific ID
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
	eventBuilder := NewEventBuilder()
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
