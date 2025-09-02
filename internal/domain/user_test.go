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
			errorMsg:    "must specify valid email address",
		},
		{
			name:        "empty name should fail",
			email:       "john@example.com",
			userName:    "",
			expectError: true,
			errorMsg:    "must specify user name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := new(User).WithEmail(tt.email, tt.userName)
			var err error
			if len(user.Errors()) > 0 {
				err = user.Errors()[0]
			}
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
			if user.Email != tt.email {
				t.Errorf("expected email '%s', got '%s'", tt.email, user.Email)
			}

			if user.Name != tt.userName {
				t.Errorf("expected name '%s', got '%s'", tt.userName, user.Name)
			}

			if user.SequenceNo() != 1 {
				t.Errorf("expected version 1, got %d", user.SequenceNo())
			}

			// Verify User.created event was generated
			events := user.UncommittedEvents()
			if len(events) != 1 {
				t.Errorf("expected 1 uncommitted event, got %d", len(events))
				return
			}

			event := events[0]
			if event.EventType() != "User.created" {
				t.Errorf("expected User.created event, got %s", event.EventType())
			}

			if event.AggregateID() != user.ID() {
				t.Errorf("expected aggregate ID '%s', got '%s'", user.ID(), event.AggregateID())
			}

			if event.SequenceNo() != 1 {
				t.Errorf("expected event sequence 1, got %d", event.SequenceNo())
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
			errorMsg:     "invalid email address provided",
			expectEvent:  false,
		},
		{
			name:         "same email should generate error",
			initialEmail: "john@example.com",
			newEmail:     "john@example.com",
			expectError:  true,
			errorMsg:     "no change provided",
			expectEvent:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := new(User).WithEmail(tt.initialEmail, "John Doe")
			var err error
			if len(user.Errors()) > 0 {
				err = user.Errors()[0]
			}
			if err != nil {
				t.Fatalf("failed to create user: %v", err)
			}

			// Clear initial events
			user.MarkEventsAsCommitted()

			user.UpdateEmail(tt.newEmail)
			if len(user.Errors()) > 0 {
				err = user.Errors()[len(user.Errors())-1] // Get the latest error
			}

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
				if user.Email != tt.newEmail {
					t.Errorf("expected email '%s', got '%s'", tt.newEmail, user.Email)
				}

				if user.SequenceNo() != 2 {
					t.Errorf("expected version 2, got %d", user.SequenceNo())
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
				if event.EventType() != "User.updated" {
					t.Errorf("expected User.updated event, got %s", event.EventType())
				}

				if event.AggregateID() != user.ID() {
					t.Errorf("expected aggregate ID '%s', got '%s'", user.ID(), event.AggregateID())
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
		user := new(User).WithEmail("john@example.com", "John Doe")
		user.Active = true // Set user as active initially
		var err error
		if len(user.Errors()) > 0 {
			err = user.Errors()[0]
		}
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// Clear initial events
		user.MarkEventsAsCommitted()

		user.Deactivate()
		if len(user.Errors()) > 0 {
			err = user.Errors()[0]
			t.Errorf("unexpected error: %v", err)
			return
		}

		// Verify user is deactivated
		if user.Active {
			t.Errorf("expected user to be deactivated")
		}

		if user.SequenceNo() != 2 {
			t.Errorf("expected version 2, got %d", user.SequenceNo())
		}

		// Verify event generation
		events := user.UncommittedEvents()
		if len(events) != 1 {
			t.Errorf("expected 1 uncommitted event, got %d", len(events))
			return
		}

		event := events[0]
		if event.EventType() != "User.updated" {
			t.Errorf("expected User.updated event, got %s", event.EventType())
		}
	})

	t.Run("deactivate already inactive user", func(t *testing.T) {
		user := new(User).WithEmail("john@example.com", "John Doe")
		user.Active = false // Set user as inactive initially

		// Clear initial events
		user.MarkEventsAsCommitted()

		// Try to deactivate again
		user.Deactivate()

		// Should generate error
		if len(user.Errors()) == 0 {
			t.Errorf("expected error for deactivating already inactive user")
			return
		}

		err := user.Errors()[0]
		if err.Error() != "user already deactivated" {
			t.Errorf("expected 'user already deactivated' error, got '%s'", err.Error())
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
		user := new(User).WithEmail("john@example.com", "John Doe")
		user.Active = false // Set user as inactive initially
		var err error
		if len(user.Errors()) > 0 {
			err = user.Errors()[0]
		}
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// Clear initial events
		user.MarkEventsAsCommitted()

		user.Activate()
		if len(user.Errors()) > 0 {
			err = user.Errors()[0]
			t.Errorf("unexpected error: %v", err)
			return
		}

		// Verify user is activated
		if !user.Active {
			t.Errorf("expected user to be activated")
		}

		if user.SequenceNo() != 2 {
			t.Errorf("expected version 2, got %d", user.SequenceNo())
		}

		// Verify event generation
		events := user.UncommittedEvents()
		if len(events) != 1 {
			t.Errorf("expected 1 uncommitted event, got %d", len(events))
			return
		}

		event := events[0]
		if event.EventType() != "User.updated" {
			t.Errorf("expected User.updated event, got %s", event.EventType())
		}
	})

	t.Run("activate already active user", func(t *testing.T) {
		user := new(User).WithEmail("john@example.com", "John Doe")
		user.Active = true // Set user as active initially

		// Clear initial events
		user.MarkEventsAsCommitted()

		// Try to activate already active user
		user.Activate()

		// Should generate error
		if len(user.Errors()) == 0 {
			t.Errorf("expected error for activating already active user")
			return
		}

		err := user.Errors()[0]
		if err.Error() != "user already activated" {
			t.Errorf("expected 'user already activated' error, got '%s'", err.Error())
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
		userID := ksuid.New().String()

		// Create test user data for events
		userData1 := &User{Email: "john@example.com", Name: "John Doe", Active: true}
		userData2 := &User{Email: "john.doe@example.com", Name: "John Doe", Active: true}
		userData3 := &User{Email: "john.doe@example.com", Name: "John Smith", Active: true}
		userData4 := &User{Email: "john.doe@example.com", Name: "John Smith", Active: false}

		// Create events that represent user lifecycle
		events := []pkgdomain.Event{
			pkgdomain.NewEntityEvent("User", "created", userID, "", "", userData1),
			pkgdomain.NewEntityEvent("User", "updated", userID, "", "", userData2),
			pkgdomain.NewEntityEvent("User", "updated", userID, "", "", userData3),
			pkgdomain.NewEntityEvent("User", "updated", userID, "", "", userData4),
		}

		// Create empty user and load from history
		user := &User{}
		user.Entity = new(pkgdomain.Entity).WithID(userID)
		user.LoadFromHistory(events)

		// Verify final state
		if user.ID() != userID {
			t.Errorf("expected user ID '%s', got '%s'", userID, user.ID())
		}

		if user.Email != "john.doe@example.com" {
			t.Errorf("expected email 'john.doe@example.com', got '%s'", user.Email)
		}

		if user.Name != "John Smith" {
			t.Errorf("expected name 'John Smith', got '%s'", user.Name)
		}

		if user.Active {
			t.Errorf("expected user to be inactive")
		}

		if user.SequenceNo() != 4 {
			t.Errorf("expected version 4, got %d", user.SequenceNo())
		}

		// Should have no uncommitted events after loading from history
		if len(user.UncommittedEvents()) != 0 {
			t.Errorf("expected no uncommitted events, got %d", len(user.UncommittedEvents()))
		}
	})
}

func TestUser_MarkEventsAsCommitted(t *testing.T) {
	user := new(User).WithEmail("john@example.com", "John Doe")
	var err error
	if len(user.Errors()) > 0 {
		err = user.Errors()[0]
	}
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
	user := new(User).WithEmail("john@example.com", "John Doe")
	var err error
	if len(user.Errors()) > 0 {
		err = user.Errors()[0]
	}
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Test ID() method
	if user.ID() == "" {
		t.Errorf("expected non-empty ID")
	}

	// Test Version() method
	if user.SequenceNo() != 1 {
		t.Errorf("expected version 1, got %d", user.SequenceNo())
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
		user := new(User).WithEmail("john@example.com", "John Doe")
		user.Active = true
		var err error
		if len(user.Errors()) > 0 {
			err = user.Errors()[0]
		}
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		originalID := user.ID()

		// Perform various operations
		user.UpdateEmail("new@example.com")
		user.Deactivate()
		user.Activate()

		// ID should remain the same
		if user.ID() != originalID {
			t.Errorf("user ID changed from %s to %s", originalID, user.ID())
		}
	})

	t.Run("version should increment with each operation", func(t *testing.T) {
		user := new(User).WithEmail("john@example.com", "John Doe")
		user.Active = true
		var err error
		if len(user.Errors()) > 0 {
			err = user.Errors()[0]
		}
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		expectedVersion := 1
		if user.SequenceNo() != int64(expectedVersion) {
			t.Errorf("expected version %d, got %d", expectedVersion, user.SequenceNo())
		}

		user.UpdateEmail("new@example.com")
		expectedVersion++
		if user.SequenceNo() != int64(expectedVersion) {
			t.Errorf("expected version %d after email update, got %d", expectedVersion, user.SequenceNo())
		}

		user.Deactivate()
		expectedVersion++
		if user.SequenceNo() != int64(expectedVersion) {
			t.Errorf("expected version %d after deactivation, got %d", expectedVersion, user.SequenceNo())
		}

		user.Activate()
		expectedVersion++
		if user.SequenceNo() != int64(expectedVersion) {
			t.Errorf("expected version %d after activation, got %d", expectedVersion, user.SequenceNo())
		}
	})

	t.Run("events should have correct aggregate ID and sequence", func(t *testing.T) {
		user := new(User).WithEmail("john@example.com", "John Doe")
		user.Active = true
		var err error
		if len(user.Errors()) > 0 {
			err = user.Errors()[0]
		}
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		user.UpdateEmail("new@example.com")

		events := user.UncommittedEvents()
		for _, event := range events {
			if event.AggregateID() != user.ID() {
				t.Errorf("event aggregate ID '%s' does not match user ID '%s'", event.AggregateID(), user.ID())
			}

			if event.SequenceNo() < 1 || event.SequenceNo() > user.SequenceNo() {
				t.Errorf("event sequence %d is not within valid range [1, %d]", event.SequenceNo(), user.SequenceNo())
			}
		}
	})
}
