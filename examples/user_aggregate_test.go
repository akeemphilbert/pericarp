package examples

import (
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

func TestNewUser(t *testing.T) {
	user, err := NewUser("user-123", "john@example.com", "John Doe")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if user.GetID() != "user-123" {
		t.Errorf("Expected GetID 'user-123', got %s", user.GetID())
	}

	if user.Email() != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got %s", user.Email())
	}

	if user.Name() != "John Doe" {
		t.Errorf("Expected name 'John Doe', got %s", user.Name())
	}

	if !user.IsActive() {
		t.Error("Expected user to be active")
	}

	if user.GetSequenceNo() != 1 {
		t.Errorf("Expected sequence number 1, got %d", user.GetSequenceNo())
	}

	if !user.HasUncommittedEvents() {
		t.Error("Expected user to have uncommitted events")
	}

	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType() != "user.created" {
		t.Errorf("Expected event type 'user.created', got %s", events[0].EventType())
	}
}

func TestNewUser_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		email       string
		userName    string
		expectedErr string
	}{
		{
			name:        "empty GetID",
			id:          "",
			email:       "john@example.com",
			userName:    "John Doe",
			expectedErr: "user GetID cannot be empty",
		},
		{
			name:        "empty email",
			id:          "user-123",
			email:       "",
			userName:    "John Doe",
			expectedErr: "email cannot be empty",
		},
		{
			name:        "empty name",
			id:          "user-123",
			email:       "john@example.com",
			userName:    "",
			expectedErr: "name cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := NewUser(tt.id, tt.email, tt.userName)
			if err == nil {
				t.Errorf("Expected error, got nil")
			}
			if user != nil {
				t.Errorf("Expected nil user, got %v", user)
			}
			if err.Error() != tt.expectedErr {
				t.Errorf("Expected error '%s', got '%s'", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestUser_ChangeEmail(t *testing.T) {
	user, _ := NewUser("user-123", "john@example.com", "John Doe")
	user.MarkEventsAsCommitted() // Clear initial event

	err := user.ChangeEmail("newemail@example.com")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if user.Email() != "newemail@example.com" {
		t.Errorf("Expected email 'newemail@example.com', got %s", user.Email())
	}

	if user.GetSequenceNo() != 2 {
		t.Errorf("Expected sequence number 2, got %d", user.GetSequenceNo())
	}

	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType() != "user.email_changed" {
		t.Errorf("Expected event type 'user.email_changed', got %s", events[0].EventType())
	}
}

func TestUser_ChangeEmail_SameEmail(t *testing.T) {
	user, _ := NewUser("user-123", "john@example.com", "John Doe")
	initialSequenceNo := user.GetSequenceNo()
	user.MarkEventsAsCommitted()

	err := user.ChangeEmail("john@example.com") // Same email
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if user.GetSequenceNo() != initialSequenceNo {
		t.Errorf("Expected sequence number to remain %d, got %d", initialSequenceNo, user.GetSequenceNo())
	}

	if user.HasUncommittedEvents() {
		t.Error("Expected no new events for same email")
	}
}

func TestUser_ChangeEmail_EmptyEmail(t *testing.T) {
	user, _ := NewUser("user-123", "john@example.com", "John Doe")

	err := user.ChangeEmail("")
	if err == nil {
		t.Error("Expected error for empty email")
	}

	if err.Error() != "email cannot be empty" {
		t.Errorf("Expected 'email cannot be empty', got '%s'", err.Error())
	}
}

func TestUser_ChangeName(t *testing.T) {
	user, _ := NewUser("user-123", "john@example.com", "John Doe")
	user.MarkEventsAsCommitted()

	err := user.ChangeName("Jane Doe")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if user.Name() != "Jane Doe" {
		t.Errorf("Expected name 'Jane Doe', got %s", user.Name())
	}

	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType() != "user.name_changed" {
		t.Errorf("Expected event type 'user.name_changed', got %s", events[0].EventType())
	}
}

func TestUser_Deactivate(t *testing.T) {
	user, _ := NewUser("user-123", "john@example.com", "John Doe")
	user.MarkEventsAsCommitted()

	err := user.Deactivate()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if user.IsActive() {
		t.Error("Expected user to be inactive")
	}

	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType() != "user.deactivated" {
		t.Errorf("Expected event type 'user.deactivated', got %s", events[0].EventType())
	}
}

func TestUser_Activate(t *testing.T) {
	user, _ := NewUser("user-123", "john@example.com", "John Doe")
	user.Deactivate()
	user.MarkEventsAsCommitted()

	err := user.Activate()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !user.IsActive() {
		t.Error("Expected user to be active")
	}

	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType() != "user.activated" {
		t.Errorf("Expected event type 'user.activated', got %s", events[0].EventType())
	}
}

func TestUser_LoadFromHistory(t *testing.T) {
	// Create events representing user history using EntityEvent
	events := []domain.Event{
		domain.NewEntityEvent(nil, nil, "user", "created", "user-123", map[string]interface{}{
			"email":     "john@example.com",
			"name":      "John Doe",
			"is_active": true,
		}),
		domain.NewEntityEvent(nil, nil, "user", "email_changed", "user-123", map[string]interface{}{
			"old_email": "john@example.com",
			"new_email": "john.doe@example.com",
		}),
		domain.NewEntityEvent(nil, nil, "user", "name_changed", "user-123", map[string]interface{}{
			"old_name": "John Doe",
			"new_name": "John Smith",
		}),
		domain.NewEntityEvent(nil, nil, "user", "deactivated", "user-123", map[string]interface{}{
			"reason": "user_requested",
		}),
	}

	// Create empty user and load from history
	user := &User{BasicEntity: *domain.NewEntity("user-123")}
	user.LoadFromHistory(events)

	// Verify final state
	if user.Email() != "john.doe@example.com" {
		t.Errorf("Expected email 'john.doe@example.com', got %s", user.Email())
	}

	if user.Name() != "John Smith" {
		t.Errorf("Expected name 'John Smith', got %s", user.Name())
	}

	if user.IsActive() {
		t.Error("Expected user to be inactive")
	}

	if user.GetSequenceNo() != 4 {
		t.Errorf("Expected sequence number 4, got %d", user.GetSequenceNo())
	}

	if user.HasUncommittedEvents() {
		t.Error("Expected no uncommitted events after loading from history")
	}
}

func TestUser_CompleteLifecycle(t *testing.T) {
	// Create user
	user, err := NewUser("user-123", "john@example.com", "John Doe")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Simulate persistence
	events := user.UncommittedEvents()
	user.MarkEventsAsCommitted()

	// Change email
	err = user.ChangeEmail("john.doe@example.com")
	if err != nil {
		t.Fatalf("Failed to change email: %v", err)
	}

	// Change name
	err = user.ChangeName("John Smith")
	if err != nil {
		t.Fatalf("Failed to change name: %v", err)
	}

	// Deactivate
	err = user.Deactivate()
	if err != nil {
		t.Fatalf("Failed to deactivate user: %v", err)
	}

	// Collect all events
	newEvents := user.UncommittedEvents()
	allEvents := append(events, newEvents...)

	// Verify we have all expected events
	expectedEventTypes := []string{
		"user.created",
		"user.email_changed",
		"user.name_changed",
		"user.deactivated",
	}

	if len(allEvents) != len(expectedEventTypes) {
		t.Errorf("Expected %d events, got %d", len(expectedEventTypes), len(allEvents))
	}

	for i, expectedType := range expectedEventTypes {
		if allEvents[i].EventType() != expectedType {
			t.Errorf("Expected event %d to be %s, got %s", i, expectedType, allEvents[i].EventType())
		}
	}

	// Reconstruct user from events
	reconstructedUser := &User{BasicEntity: *domain.NewEntity("user-123")}
	reconstructedUser.LoadFromHistory(allEvents)

	// Verify reconstructed state matches current state
	if reconstructedUser.Email() != user.Email() {
		t.Errorf("Reconstructed email mismatch: expected %s, got %s", user.Email(), reconstructedUser.Email())
	}

	if reconstructedUser.Name() != user.Name() {
		t.Errorf("Reconstructed name mismatch: expected %s, got %s", user.Name(), reconstructedUser.Name())
	}

	if reconstructedUser.IsActive() != user.IsActive() {
		t.Errorf("Reconstructed active status mismatch: expected %t, got %t", user.IsActive(), reconstructedUser.IsActive())
	}

	if reconstructedUser.GetSequenceNo() != int64(len(allEvents)) {
		t.Errorf("Reconstructed sequence number mismatch: expected %d, got %d", len(allEvents), reconstructedUser.GetSequenceNo())
	}
}
