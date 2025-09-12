package examples

import (
	"encoding/json"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

func TestNewUserWithEntityEvents(t *testing.T) {
	user, err := NewUserWithEntityEvents("user-123", "john@example.com", "John Doe")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if user.ID() != "user-123" {
		t.Errorf("Expected ID 'user-123', got %s", user.ID())
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

	// Check that a Created event was generated
	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	entityEvent, ok := events[0].(*domain.EntityEvent)
	if !ok {
		t.Error("Expected EntityEvent")
	}

	if entityEvent.EventType() != "user.created" {
		t.Errorf("Expected event type 'user.created', got %s", entityEvent.EventType())
	}

	// Parse payload to access data
	var data map[string]interface{}
	if err := json.Unmarshal(entityEvent.Payload(), &data); err != nil {
		t.Fatalf("Failed to parse payload: %v", err)
	}

	if email, ok := data["email"].(string); !ok || email != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got %v", data["email"])
	}
}

func TestUserWithEntityEvents_ChangeEmail(t *testing.T) {
	user, _ := NewUserWithEntityEvents("user-123", "john@example.com", "John Doe")
	user.MarkEventsAsCommitted() // Clear initial event

	err := user.ChangeEmail("newemail@example.com")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if user.Email() != "newemail@example.com" {
		t.Errorf("Expected email 'newemail@example.com', got %s", user.Email())
	}

	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	entityEvent, ok := events[0].(*domain.EntityEvent)
	if !ok {
		t.Error("Expected EntityEvent")
	}

	if entityEvent.EventType() != "user.email_updated" {
		t.Errorf("Expected event type 'user.email_updated', got %s", entityEvent.EventType())
	}

	// Parse payload to access data
	var data map[string]interface{}
	if err := json.Unmarshal(entityEvent.Payload(), &data); err != nil {
		t.Fatalf("Failed to parse payload: %v", err)
	}

	if oldValue, ok := data["old_value"].(string); !ok || oldValue != "john@example.com" {
		t.Errorf("Expected old_value 'john@example.com', got %v", data["old_value"])
	}

	if newValue, ok := data["new_value"].(string); !ok || newValue != "newemail@example.com" {
		t.Errorf("Expected new_value 'newemail@example.com', got %v", data["new_value"])
	}
}

func TestUserWithEntityEvents_Deactivate(t *testing.T) {
	user, _ := NewUserWithEntityEvents("user-123", "john@example.com", "John Doe")
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

	entityEvent, ok := events[0].(*domain.EntityEvent)
	if !ok {
		t.Error("Expected EntityEvent")
	}

	if entityEvent.EventType() != "user.status_changed" {
		t.Errorf("Expected event type 'user.status_changed', got %s", entityEvent.EventType())
	}

	// Parse payload to access data
	var data map[string]interface{}
	if err := json.Unmarshal(entityEvent.Payload(), &data); err != nil {
		t.Fatalf("Failed to parse payload: %v", err)
	}

	if oldStatus, ok := data["old_status"].(string); !ok || oldStatus != "active" {
		t.Errorf("Expected old_status 'active', got %v", data["old_status"])
	}

	if newStatus, ok := data["new_status"].(string); !ok || newStatus != "inactive" {
		t.Errorf("Expected new_status 'inactive', got %v", data["new_status"])
	}
}

func TestUserWithEntityEvents_UpdateProfile(t *testing.T) {
	user, _ := NewUserWithEntityEvents("user-123", "john@example.com", "John Doe")
	user.MarkEventsAsCommitted()

	err := user.UpdateProfile("john.doe@example.com", "John Smith")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if user.Email() != "john.doe@example.com" {
		t.Errorf("Expected email 'john.doe@example.com', got %s", user.Email())
	}

	if user.Name() != "John Smith" {
		t.Errorf("Expected name 'John Smith', got %s", user.Name())
	}

	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	entityEvent, ok := events[0].(*domain.EntityEvent)
	if !ok {
		t.Error("Expected EntityEvent")
	}

	if entityEvent.EventType() != "user.updated" {
		t.Errorf("Expected event type 'user.updated', got %s", entityEvent.EventType())
	}

	// Parse payload to access data
	var data map[string]interface{}
	if err := json.Unmarshal(entityEvent.Payload(), &data); err != nil {
		t.Fatalf("Failed to parse payload: %v", err)
	}

	// Check that both email and name changes are captured
	if oldEmail, ok := data["old_email"].(string); !ok || oldEmail != "john@example.com" {
		t.Errorf("Expected old_email 'john@example.com', got %v", data["old_email"])
	}

	if newEmail, ok := data["new_email"].(string); !ok || newEmail != "john.doe@example.com" {
		t.Errorf("Expected new_email 'john.doe@example.com', got %v", data["new_email"])
	}

	if oldName, ok := data["old_name"].(string); !ok || oldName != "John Doe" {
		t.Errorf("Expected old_name 'John Doe', got %v", data["old_name"])
	}

	if newName, ok := data["new_name"].(string); !ok || newName != "John Smith" {
		t.Errorf("Expected new_name 'John Smith', got %v", data["new_name"])
	}
}

func TestUserWithEntityEvents_LoadFromHistory(t *testing.T) {
	// Create events representing user history using EntityEvents
	events := []domain.Event{
		domain.NewEntityEvent("user", "created", "user-123", "", "", map[string]interface{}{
			"email":     "john@example.com",
			"name":      "John Doe",
			"is_active": true,
		}),
		domain.NewEntityEvent("user", "email_updated", "user-123", "", "", map[string]interface{}{
			"field":     "Email",
			"old_value": "john@example.com",
			"new_value": "john.doe@example.com",
		}),
		domain.NewEntityEvent("user", "name_updated", "user-123", "", "", map[string]interface{}{
			"field":     "Name",
			"old_value": "John Doe",
			"new_value": "John Smith",
		}),
		domain.NewEntityEvent("user", "status_changed", "user-123", "", "", map[string]interface{}{
			"old_status": "active",
			"new_status": "inactive",
			"reason":     "user_requested",
		}),
	}

	// Create empty user and load from history
	user := &UserWithEntityEvents{BasicEntity: domain.NewEntity("user-123")}
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

	if user.SequenceNo() != 4 {
		t.Errorf("Expected sequence number 4, got %d", user.SequenceNo())
	}
}

func TestUserWithEntityEvents_GetEventSummary(t *testing.T) {
	user, _ := NewUserWithEntityEvents("user-123", "john@example.com", "John Doe")

	// Perform various operations
	user.ChangeEmail("new@example.com")
	user.ChangeName("New Name")
	user.Deactivate()

	summary := user.GetEventSummary()
	expectedSummary := []string{
		"user.created",
		"user.email_updated",
		"user.name_updated",
		"user.status_changed",
	}

	if len(summary) != len(expectedSummary) {
		t.Errorf("Expected %d events in summary, got %d", len(expectedSummary), len(summary))
	}

	for i, expected := range expectedSummary {
		if i < len(summary) && summary[i] != expected {
			t.Errorf("Expected event %d to be %s, got %s", i, expected, summary[i])
		}
	}
}

func TestUserWithEntityEvents_Delete(t *testing.T) {
	user, _ := NewUserWithEntityEvents("user-123", "john@example.com", "John Doe")
	user.MarkEventsAsCommitted()

	err := user.Delete("account_closure")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	entityEvent, ok := events[0].(*domain.EntityEvent)
	if !ok {
		t.Error("Expected EntityEvent")
	}

	if entityEvent.EventType() != "user.deleted" {
		t.Errorf("Expected event type 'user.deleted', got %s", entityEvent.EventType())
	}

	// Parse payload to access data
	var data map[string]interface{}
	if err := json.Unmarshal(entityEvent.Payload(), &data); err != nil {
		t.Fatalf("Failed to parse payload: %v", err)
	}

	if reason, ok := data["reason"].(string); !ok || reason != "account_closure" {
		t.Errorf("Expected reason 'account_closure', got %v", data["reason"])
	}

	if softDelete, ok := data["soft_delete"].(bool); !ok || !softDelete {
		t.Error("Expected soft_delete to be true")
	}
}
