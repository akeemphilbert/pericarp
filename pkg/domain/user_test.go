package domain

import (
	"testing"
)

func TestNewUser_ValidInput_CreatesUserSuccessfully(t *testing.T) {
	// Arrange
	id := "user-123"
	email := "john@example.com"
	name := "John Doe"

	// Act
	user, err := NewUser(id, email, name)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if user.ID() != id {
		t.Errorf("Expected ID %s, got %s", id, user.ID())
	}

	if user.Email() != email {
		t.Errorf("Expected email %s, got %s", email, user.Email())
	}

	if user.Name() != name {
		t.Errorf("Expected name %s, got %s", name, user.Name())
	}

	if user.Version() != 1 {
		t.Errorf("Expected version 1, got %d", user.Version())
	}

	// Check that UserCreated event was generated
	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 uncommitted event, got %d", len(events))
	}

	event, ok := events[0].(*UserCreatedEvent)
	if !ok {
		t.Fatalf("Expected UserCreatedEvent, got %T", events[0])
	}

	if event.EventType() != "UserCreated" {
		t.Errorf("Expected event type 'UserCreated', got %s", event.EventType())
	}

	if event.AggregateID() != id {
		t.Errorf("Expected aggregate ID %s, got %s", id, event.AggregateID())
	}
}

func TestNewUser_InvalidEmail_ReturnsError(t *testing.T) {
	// Arrange
	id := "user-123"
	invalidEmail := "invalid-email"
	name := "John Doe"

	// Act
	user, err := NewUser(id, invalidEmail, name)

	// Assert
	if err == nil {
		t.Fatal("Expected error for invalid email, got nil")
	}

	if user != nil {
		t.Error("Expected nil user for invalid input")
	}

	validationErr, ok := err.(ValidationError)
	if !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}

	if validationErr.Field != "email" {
		t.Errorf("Expected field 'email', got %s", validationErr.Field)
	}
}

func TestNewUser_EmptyName_ReturnsError(t *testing.T) {
	// Arrange
	id := "user-123"
	email := "john@example.com"
	emptyName := ""

	// Act
	user, err := NewUser(id, email, emptyName)

	// Assert
	if err == nil {
		t.Fatal("Expected error for empty name, got nil")
	}

	if user != nil {
		t.Error("Expected nil user for invalid input")
	}

	validationErr, ok := err.(ValidationError)
	if !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}

	if validationErr.Field != "name" {
		t.Errorf("Expected field 'name', got %s", validationErr.Field)
	}
}

func TestUpdateUserEmail_ValidEmail_UpdatesSuccessfully(t *testing.T) {
	// Arrange
	user, _ := NewUser("user-123", "john@example.com", "John Doe")
	user.MarkEventsAsCommitted() // Clear initial events
	newEmail := "john.doe@example.com"

	// Act
	err := user.UpdateUserEmail(newEmail)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if user.Email() != newEmail {
		t.Errorf("Expected email %s, got %s", newEmail, user.Email())
	}

	if user.Version() != 2 {
		t.Errorf("Expected version 2, got %d", user.Version())
	}

	// Check that UserEmailUpdated event was generated
	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 uncommitted event, got %d", len(events))
	}

	event, ok := events[0].(*UserEmailUpdatedEvent)
	if !ok {
		t.Fatalf("Expected UserEmailUpdatedEvent, got %T", events[0])
	}

	if event.EventType() != "UserEmailUpdated" {
		t.Errorf("Expected event type 'UserEmailUpdated', got %s", event.EventType())
	}

	if event.OldEmail != "john@example.com" {
		t.Errorf("Expected old email 'john@example.com', got %s", event.OldEmail)
	}

	if event.NewEmail != newEmail {
		t.Errorf("Expected new email %s, got %s", newEmail, event.NewEmail)
	}
}

func TestUpdateUserEmail_SameEmail_ReturnsError(t *testing.T) {
	// Arrange
	email := "john@example.com"
	user, _ := NewUser("user-123", email, "John Doe")

	// Act
	err := user.UpdateUserEmail(email)

	// Assert
	if err == nil {
		t.Fatal("Expected error for same email, got nil")
	}

	validationErr, ok := err.(ValidationError)
	if !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}

	if validationErr.Field != "email" {
		t.Errorf("Expected field 'email', got %s", validationErr.Field)
	}
}

func TestLoadUserFromHistory_ReconstructsCorrectly(t *testing.T) {
	// Arrange
	id := "user-123"
	originalEmail := "john@example.com"
	newEmail := "john.doe@example.com"
	name := "John Doe"

	events := []Event{
		NewUserCreatedEvent(id, originalEmail, name, 1),
		NewUserEmailUpdatedEvent(id, originalEmail, newEmail, 2),
	}

	// Act
	user := LoadUserFromHistory(id, events)

	// Assert
	if user.ID() != id {
		t.Errorf("Expected ID %s, got %s", id, user.ID())
	}

	if user.Email() != newEmail {
		t.Errorf("Expected email %s, got %s", newEmail, user.Email())
	}

	if user.Name() != name {
		t.Errorf("Expected name %s, got %s", name, user.Name())
	}

	if user.Version() != 2 {
		t.Errorf("Expected version 2, got %d", user.Version())
	}

	// Should have no uncommitted events when loaded from history
	if len(user.UncommittedEvents()) != 0 {
		t.Errorf("Expected 0 uncommitted events, got %d", len(user.UncommittedEvents()))
	}
}

func TestUser_MarkEventsAsCommitted_ClearsUncommittedEvents(t *testing.T) {
	// Arrange
	user, _ := NewUser("user-123", "john@example.com", "John Doe")

	// Verify we have uncommitted events
	if len(user.UncommittedEvents()) == 0 {
		t.Fatal("Expected uncommitted events before marking as committed")
	}

	// Act
	user.MarkEventsAsCommitted()

	// Assert
	if len(user.UncommittedEvents()) != 0 {
		t.Errorf("Expected 0 uncommitted events after marking as committed, got %d", len(user.UncommittedEvents()))
	}
}
