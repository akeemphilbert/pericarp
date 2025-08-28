package domain

import (
	"testing"

	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/segmentio/ksuid"
)

func TestNewUser(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		userName    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid user creation",
			email:       "john@example.com",
			userName:    "John Doe",
			expectError: false,
		},
		{
			name:        "empty email should fail",
			email:       "",
			userName:    "John Doe",
			expectError: true,
			errorMsg:    "email cannot be empty",
		},
		{
			name:        "empty name should fail",
			email:       "john@example.com",
			userName:    "",
			expectError: true,
			errorMsg:    "name cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := NewUser(tt.email, tt.userName)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify user properties
			if user.Email() != tt.email {
				t.Errorf("expected email '%s', got '%s'", tt.email, user.Email())
			}

			if user.Name() != tt.userName {
				t.Errorf("expected name '%s', got '%s'", tt.userName, user.Name())
			}

			if !user.IsActive() {
				t.Errorf("expected new user to be active")
			}

			if user.Version() != 1 {
				t.Errorf("expected version 1, got %d", user.Version())
			}

			// Verify UserCreated event was generated
			events := user.UncommittedEvents()
			if len(events) != 1 {
				t.Errorf("expected 1 uncommitted event, got %d", len(events))
				return
			}

			event := events[0]
			if event.EventType() != "UserCreated" {
				t.Errorf("expected UserCreated event, got %s", event.EventType())
			}

			if event.AggregateID() != user.ID() {
				t.Errorf("expected aggregate ID '%s', got '%s'", user.ID(), event.AggregateID())
			}

			if event.Version() != 1 {
				t.Errorf("expected event version 1, got %d", event.Version())
			}
		})
	}
}

func TestUser_UpdateEmail(t *testing.T) {
	tests := []struct {
		name         string
		initialEmail string
		newEmail     string
		expectError  bool
		errorMsg     string
		expectEvent  bool
	}{
		{
			name:         "valid email update",
			initialEmail: "john@example.com",
			newEmail:     "john.doe@example.com",
			expectError:  false,
			expectEvent:  true,
		},
		{
			name:         "empty email should fail",
			initialEmail: "john@example.com",
			newEmail:     "",
			expectError:  true,
			errorMsg:     "email cannot be empty",
			expectEvent:  false,
		},
		{
			name:         "same email should not generate event",
			initialEmail: "john@example.com",
			newEmail:     "john@example.com",
			expectError:  false,
			expectEvent:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := NewUser(tt.initialEmail, "John Doe")
			if err != nil {
				t.Fatalf("failed to create user: %v", err)
			}

			// Clear initial events
			user.MarkEventsAsCommitted()

			err = user.UpdateEmail(tt.newEmail)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify email was updated (if different)
			if tt.newEmail != tt.initialEmail {
				if user.Email() != tt.newEmail {
					t.Errorf("expected email '%s', got '%s'", tt.newEmail, user.Email())
				}

				if user.Version() != 2 {
					t.Errorf("expected version 2, got %d", user.Version())
				}
			}

			// Verify event generation
			events := user.UncommittedEvents()
			if tt.expectEvent {
				if len(events) != 1 {
					t.Errorf("expected 1 uncommitted event, got %d", len(events))
					return
				}

				event := events[0]
				if event.EventType() != "UserEmailUpdated" {
					t.Errorf("expected UserEmailUpdated event, got %s", event.EventType())
				}

				if userEmailEvent, ok := event.(UserEmailUpdatedEvent); ok {
					if userEmailEvent.OldEmail != tt.initialEmail {
						t.Errorf("expected old email '%s', got '%s'", tt.initialEmail, userEmailEvent.OldEmail)
					}
					if userEmailEvent.NewEmail != tt.newEmail {
						t.Errorf("expected new email '%s', got '%s'", tt.newEmail, userEmailEvent.NewEmail)
					}
				} else {
					t.Errorf("event is not UserEmailUpdatedEvent")
				}
			} else {
				if len(events) != 0 {
					t.Errorf("expected no uncommitted events, got %d", len(events))
				}
			}
		})
	}
}

func TestUser_UpdateName(t *testing.T) {
	tests := []struct {
		name        string
		initialName string
		newName     string
		expectError bool
		errorMsg    string
		expectEvent bool
	}{
		{
			name:        "valid name update",
			initialName: "John Doe",
			newName:     "John Smith",
			expectError: false,
			expectEvent: true,
		},
		{
			name:        "empty name should fail",
			initialName: "John Doe",
			newName:     "",
			expectError: true,
			errorMsg:    "name cannot be empty",
			expectEvent: false,
		},
		{
			name:        "same name should not generate event",
			initialName: "John Doe",
			newName:     "John Doe",
			expectError: false,
			expectEvent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := NewUser("john@example.com", tt.initialName)
			if err != nil {
				t.Fatalf("failed to create user: %v", err)
			}

			// Clear initial events
			user.MarkEventsAsCommitted()

			err = user.UpdateName(tt.newName)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify name was updated (if different)
			if tt.newName != tt.initialName {
				if user.Name() != tt.newName {
					t.Errorf("expected name '%s', got '%s'", tt.newName, user.Name())
				}

				if user.Version() != 2 {
					t.Errorf("expected version 2, got %d", user.Version())
				}
			}

			// Verify event generation
			events := user.UncommittedEvents()
			if tt.expectEvent {
				if len(events) != 1 {
					t.Errorf("expected 1 uncommitted event, got %d", len(events))
					return
				}

				event := events[0]
				if event.EventType() != "UserNameUpdated" {
					t.Errorf("expected UserNameUpdated event, got %s", event.EventType())
				}

				if userNameEvent, ok := event.(UserNameUpdatedEvent); ok {
					if userNameEvent.OldName != tt.initialName {
						t.Errorf("expected old name '%s', got '%s'", tt.initialName, userNameEvent.OldName)
					}
					if userNameEvent.NewName != tt.newName {
						t.Errorf("expected new name '%s', got '%s'", tt.newName, userNameEvent.NewName)
					}
				} else {
					t.Errorf("event is not UserNameUpdatedEvent")
				}
			} else {
				if len(events) != 0 {
					t.Errorf("expected no uncommitted events, got %d", len(events))
				}
			}
		})
	}
}

func TestUser_Deactivate(t *testing.T) {
	t.Run("deactivate active user", func(t *testing.T) {
		user, err := NewUser("john@example.com", "John Doe")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// Clear initial events
		user.MarkEventsAsCommitted()

		err = user.Deactivate()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}

		// Verify user is deactivated
		if user.IsActive() {
			t.Errorf("expected user to be deactivated")
		}

		if user.Version() != 2 {
			t.Errorf("expected version 2, got %d", user.Version())
		}

		// Verify event generation
		events := user.UncommittedEvents()
		if len(events) != 1 {
			t.Errorf("expected 1 uncommitted event, got %d", len(events))
			return
		}

		event := events[0]
		if event.EventType() != "UserDeactivated" {
			t.Errorf("expected UserDeactivated event, got %s", event.EventType())
		}
	})

	t.Run("deactivate already inactive user", func(t *testing.T) {
		user, err := NewUser("john@example.com", "John Doe")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// Deactivate first time
		user.Deactivate()
		user.MarkEventsAsCommitted()

		// Try to deactivate again
		err = user.Deactivate()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}

		// Should not generate new event
		events := user.UncommittedEvents()
		if len(events) != 0 {
			t.Errorf("expected no uncommitted events, got %d", len(events))
		}
	})
}

func TestUser_Activate(t *testing.T) {
	t.Run("activate inactive user", func(t *testing.T) {
		user, err := NewUser("john@example.com", "John Doe")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// Deactivate first
		user.Deactivate()
		user.MarkEventsAsCommitted()

		err = user.Activate()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}

		// Verify user is activated
		if !user.IsActive() {
			t.Errorf("expected user to be activated")
		}

		if user.Version() != 3 {
			t.Errorf("expected version 3, got %d", user.Version())
		}

		// Verify event generation
		events := user.UncommittedEvents()
		if len(events) != 1 {
			t.Errorf("expected 1 uncommitted event, got %d", len(events))
			return
		}

		event := events[0]
		if event.EventType() != "UserActivated" {
			t.Errorf("expected UserActivated event, got %s", event.EventType())
		}
	})

	t.Run("activate already active user", func(t *testing.T) {
		user, err := NewUser("john@example.com", "John Doe")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// Clear initial events
		user.MarkEventsAsCommitted()

		// Try to activate already active user
		err = user.Activate()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}

		// Should not generate new event
		events := user.UncommittedEvents()
		if len(events) != 0 {
			t.Errorf("expected no uncommitted events, got %d", len(events))
		}
	})
}

func TestUser_LoadFromHistory(t *testing.T) {
	t.Run("reconstruct user from events", func(t *testing.T) {
		userID := ksuid.New()

		// Create events that represent user lifecycle
		events := []pkgdomain.Event{
			NewUserCreatedEvent(userID, "john@example.com", "John Doe", userID.String(), 1),
			NewUserEmailUpdatedEvent(userID, "john@example.com", "john.doe@example.com", userID.String(), 2),
			NewUserNameUpdatedEvent(userID, "John Doe", "John Smith", userID.String(), 3),
			NewUserDeactivatedEvent(userID, userID.String(), 4),
		}

		// Create empty user and load from history
		user := &User{}
		user.LoadFromHistory(events)

		// Verify final state
		if user.UserID() != userID {
			t.Errorf("expected user ID '%s', got '%s'", userID, user.UserID())
		}

		if user.Email() != "john.doe@example.com" {
			t.Errorf("expected email 'john.doe@example.com', got '%s'", user.Email())
		}

		if user.Name() != "John Smith" {
			t.Errorf("expected name 'John Smith', got '%s'", user.Name())
		}

		if user.IsActive() {
			t.Errorf("expected user to be inactive")
		}

		if user.Version() != 4 {
			t.Errorf("expected version 4, got %d", user.Version())
		}

		// Should have no uncommitted events after loading from history
		if len(user.UncommittedEvents()) != 0 {
			t.Errorf("expected no uncommitted events, got %d", len(user.UncommittedEvents()))
		}
	})
}

func TestUser_MarkEventsAsCommitted(t *testing.T) {
	user, err := NewUser("john@example.com", "John Doe")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Should have uncommitted events initially
	if len(user.UncommittedEvents()) == 0 {
		t.Errorf("expected uncommitted events")
	}

	// Mark events as committed
	user.MarkEventsAsCommitted()

	// Should have no uncommitted events
	if len(user.UncommittedEvents()) != 0 {
		t.Errorf("expected no uncommitted events after marking as committed, got %d", len(user.UncommittedEvents()))
	}
}

func TestUser_AggregateRootInterface(t *testing.T) {
	user, err := NewUser("john@example.com", "John Doe")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Test ID() method
	if user.ID() == "" {
		t.Errorf("expected non-empty ID")
	}

	if user.ID() != user.UserID().String() {
		t.Errorf("ID() should return string representation of UserID()")
	}

	// Test Version() method
	if user.Version() != 1 {
		t.Errorf("expected version 1, got %d", user.Version())
	}

	// Test UncommittedEvents() method
	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 uncommitted event, got %d", len(events))
	}

	// Test MarkEventsAsCommitted() method
	user.MarkEventsAsCommitted()
	if len(user.UncommittedEvents()) != 0 {
		t.Errorf("expected no uncommitted events after marking as committed")
	}
}

func TestUser_BusinessInvariants(t *testing.T) {
	t.Run("user ID should be immutable", func(t *testing.T) {
		user, err := NewUser("john@example.com", "John Doe")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		originalID := user.UserID()

		// Perform various operations
		user.UpdateEmail("new@example.com")
		user.UpdateName("New Name")
		user.Deactivate()
		user.Activate()

		// ID should remain the same
		if user.UserID() != originalID {
			t.Errorf("user ID changed from %s to %s", originalID, user.UserID())
		}
	})

	t.Run("version should increment with each operation", func(t *testing.T) {
		user, err := NewUser("john@example.com", "John Doe")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		expectedVersion := 1
		if user.Version() != expectedVersion {
			t.Errorf("expected version %d, got %d", expectedVersion, user.Version())
		}

		user.UpdateEmail("new@example.com")
		expectedVersion++
		if user.Version() != expectedVersion {
			t.Errorf("expected version %d after email update, got %d", expectedVersion, user.Version())
		}

		user.UpdateName("New Name")
		expectedVersion++
		if user.Version() != expectedVersion {
			t.Errorf("expected version %d after name update, got %d", expectedVersion, user.Version())
		}

		user.Deactivate()
		expectedVersion++
		if user.Version() != expectedVersion {
			t.Errorf("expected version %d after deactivation, got %d", expectedVersion, user.Version())
		}

		user.Activate()
		expectedVersion++
		if user.Version() != expectedVersion {
			t.Errorf("expected version %d after activation, got %d", expectedVersion, user.Version())
		}
	})

	t.Run("events should have correct aggregate ID and version", func(t *testing.T) {
		user, err := NewUser("john@example.com", "John Doe")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		user.UpdateEmail("new@example.com")
		user.UpdateName("New Name")

		events := user.UncommittedEvents()
		for _, event := range events {
			if event.AggregateID() != user.ID() {
				t.Errorf("event aggregate ID '%s' does not match user ID '%s'", event.AggregateID(), user.ID())
			}

			if event.Version() < 1 || event.Version() > user.Version() {
				t.Errorf("event version %d is not within valid range [1, %d]", event.Version(), user.Version())
			}
		}
	})
}
