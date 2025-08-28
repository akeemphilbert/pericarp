package domain

import (
	"testing"
	"time"

	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/segmentio/ksuid"
)

func TestUserCreatedEvent(t *testing.T) {
	userID := ksuid.New()
	email := "john@example.com"
	name := "John Doe"
	aggregateID := userID.String()
	version := 1

	event := NewUserCreatedEvent(userID, email, name, aggregateID, version)

	// Test event interface methods
	if event.EventType() != "UserCreated" {
		t.Errorf("expected event type 'UserCreated', got '%s'", event.EventType())
	}

	if event.AggregateID() != aggregateID {
		t.Errorf("expected aggregate ID '%s', got '%s'", aggregateID, event.AggregateID())
	}

	if event.Version() != version {
		t.Errorf("expected version %d, got %d", version, event.Version())
	}

	if event.OccurredAt().IsZero() {
		t.Errorf("expected non-zero occurred at timestamp")
	}

	if time.Since(event.OccurredAt()) > time.Second {
		t.Errorf("occurred at timestamp should be recent")
	}

	// Test event-specific fields
	if event.UserID != userID {
		t.Errorf("expected user ID '%s', got '%s'", userID, event.UserID)
	}

	if event.Email != email {
		t.Errorf("expected email '%s', got '%s'", email, event.Email)
	}

	if event.Name != name {
		t.Errorf("expected name '%s', got '%s'", name, event.Name)
	}
}

func TestUserEmailUpdatedEvent(t *testing.T) {
	userID := ksuid.New()
	oldEmail := "john@example.com"
	newEmail := "john.doe@example.com"
	aggregateID := userID.String()
	version := 2

	event := NewUserEmailUpdatedEvent(userID, oldEmail, newEmail, aggregateID, version)

	// Test event interface methods
	if event.EventType() != "UserEmailUpdated" {
		t.Errorf("expected event type 'UserEmailUpdated', got '%s'", event.EventType())
	}

	if event.AggregateID() != aggregateID {
		t.Errorf("expected aggregate ID '%s', got '%s'", aggregateID, event.AggregateID())
	}

	if event.Version() != version {
		t.Errorf("expected version %d, got %d", version, event.Version())
	}

	if event.OccurredAt().IsZero() {
		t.Errorf("expected non-zero occurred at timestamp")
	}

	// Test event-specific fields
	if event.UserID != userID {
		t.Errorf("expected user ID '%s', got '%s'", userID, event.UserID)
	}

	if event.OldEmail != oldEmail {
		t.Errorf("expected old email '%s', got '%s'", oldEmail, event.OldEmail)
	}

	if event.NewEmail != newEmail {
		t.Errorf("expected new email '%s', got '%s'", newEmail, event.NewEmail)
	}
}

func TestUserNameUpdatedEvent(t *testing.T) {
	userID := ksuid.New()
	oldName := "John Doe"
	newName := "John Smith"
	aggregateID := userID.String()
	version := 3

	event := NewUserNameUpdatedEvent(userID, oldName, newName, aggregateID, version)

	// Test event interface methods
	if event.EventType() != "UserNameUpdated" {
		t.Errorf("expected event type 'UserNameUpdated', got '%s'", event.EventType())
	}

	if event.AggregateID() != aggregateID {
		t.Errorf("expected aggregate ID '%s', got '%s'", aggregateID, event.AggregateID())
	}

	if event.Version() != version {
		t.Errorf("expected version %d, got %d", version, event.Version())
	}

	if event.OccurredAt().IsZero() {
		t.Errorf("expected non-zero occurred at timestamp")
	}

	// Test event-specific fields
	if event.UserID != userID {
		t.Errorf("expected user ID '%s', got '%s'", userID, event.UserID)
	}

	if event.OldName != oldName {
		t.Errorf("expected old name '%s', got '%s'", oldName, event.OldName)
	}

	if event.NewName != newName {
		t.Errorf("expected new name '%s', got '%s'", newName, event.NewName)
	}
}

func TestUserDeactivatedEvent(t *testing.T) {
	userID := ksuid.New()
	aggregateID := userID.String()
	version := 4

	event := NewUserDeactivatedEvent(userID, aggregateID, version)

	// Test event interface methods
	if event.EventType() != "UserDeactivated" {
		t.Errorf("expected event type 'UserDeactivated', got '%s'", event.EventType())
	}

	if event.AggregateID() != aggregateID {
		t.Errorf("expected aggregate ID '%s', got '%s'", aggregateID, event.AggregateID())
	}

	if event.Version() != version {
		t.Errorf("expected version %d, got %d", version, event.Version())
	}

	if event.OccurredAt().IsZero() {
		t.Errorf("expected non-zero occurred at timestamp")
	}

	// Test event-specific fields
	if event.UserID != userID {
		t.Errorf("expected user ID '%s', got '%s'", userID, event.UserID)
	}
}

func TestUserActivatedEvent(t *testing.T) {
	userID := ksuid.New()
	aggregateID := userID.String()
	version := 5

	event := NewUserActivatedEvent(userID, aggregateID, version)

	// Test event interface methods
	if event.EventType() != "UserActivated" {
		t.Errorf("expected event type 'UserActivated', got '%s'", event.EventType())
	}

	if event.AggregateID() != aggregateID {
		t.Errorf("expected aggregate ID '%s', got '%s'", aggregateID, event.AggregateID())
	}

	if event.Version() != version {
		t.Errorf("expected version %d, got %d", version, event.Version())
	}

	if event.OccurredAt().IsZero() {
		t.Errorf("expected non-zero occurred at timestamp")
	}

	// Test event-specific fields
	if event.UserID != userID {
		t.Errorf("expected user ID '%s', got '%s'", userID, event.UserID)
	}
}

func TestEventVersioning(t *testing.T) {
	userID := ksuid.New()
	aggregateID := userID.String()

	tests := []struct {
		name            string
		createEvent     func() pkgdomain.Event
		expectedVersion int
	}{
		{
			name: "UserCreated should be version 1",
			createEvent: func() pkgdomain.Event {
				return NewUserCreatedEvent(userID, "john@example.com", "John Doe", aggregateID, 1)
			},
			expectedVersion: 1,
		},
		{
			name: "UserEmailUpdated should be version 2",
			createEvent: func() pkgdomain.Event {
				return NewUserEmailUpdatedEvent(userID, "old@example.com", "new@example.com", aggregateID, 2)
			},
			expectedVersion: 2,
		},
		{
			name: "UserNameUpdated should be version 3",
			createEvent: func() pkgdomain.Event {
				return NewUserNameUpdatedEvent(userID, "Old Name", "New Name", aggregateID, 3)
			},
			expectedVersion: 3,
		},
		{
			name: "UserDeactivated should be version 4",
			createEvent: func() pkgdomain.Event {
				return NewUserDeactivatedEvent(userID, aggregateID, 4)
			},
			expectedVersion: 4,
		},
		{
			name: "UserActivated should be version 5",
			createEvent: func() pkgdomain.Event {
				return NewUserActivatedEvent(userID, aggregateID, 5)
			},
			expectedVersion: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := tt.createEvent()
			if event.Version() != tt.expectedVersion {
				t.Errorf("expected version %d, got %d", tt.expectedVersion, event.Version())
			}
		})
	}
}

func TestEventAggregateIDConsistency(t *testing.T) {
	userID := ksuid.New()
	aggregateID := userID.String()

	events := []pkgdomain.Event{
		NewUserCreatedEvent(userID, "john@example.com", "John Doe", aggregateID, 1),
		NewUserEmailUpdatedEvent(userID, "john@example.com", "john.doe@example.com", aggregateID, 2),
		NewUserNameUpdatedEvent(userID, "John Doe", "John Smith", aggregateID, 3),
		NewUserDeactivatedEvent(userID, aggregateID, 4),
		NewUserActivatedEvent(userID, aggregateID, 5),
	}

	for i, event := range events {
		if event.AggregateID() != aggregateID {
			t.Errorf("event %d: expected aggregate ID '%s', got '%s'", i, aggregateID, event.AggregateID())
		}

		// For user events, the aggregate ID should match the user ID string
		if event.AggregateID() != userID.String() {
			t.Errorf("event %d: aggregate ID should match user ID string", i)
		}
	}
}
