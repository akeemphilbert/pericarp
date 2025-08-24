package examples

import (
	"testing"

	"github.com/example/pericarp/pkg/domain"
)

func TestNewUserWithStandardEvents(t *testing.T) {
	user, err := NewUserWithStandardEvents("user-123", "john@example.com", "John Doe")
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

	standardEvent, ok := events[0].(*domain.StandardEvent)
	if !ok {
		t.Error("Expected StandardEvent")
	}

	if standardEvent.EventType() != "User.Created" {
		t.Errorf("Expected event type 'User.Created', got %s", standardEvent.EventType())
	}

	if standardEvent.GetDataString("email") != "john@example.com" {
		t.Errorf("Expected event email 'john@example.com', got %s", standardEvent.GetDataString("email"))
	}
}

func TestUserWithStandardEvents_ChangeEmail(t *testing.T) {
	user, _ := NewUserWithStandardEvents("user-123", "john@example.com", "John Doe")
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

	standardEvent, ok := events[0].(*domain.StandardEvent)
	if !ok {
		t.Error("Expected StandardEvent")
	}

	if standardEvent.EventType() != "User.EmailUpdated" {
		t.Errorf("Expected event type 'User.EmailUpdated', got %s", standardEvent.EventType())
	}

	if standardEvent.GetDataString("old_value") != "john@example.com" {
		t.Errorf("Expected old_value 'john@example.com', got %s", standardEvent.GetDataString("old_value"))
	}

	if standardEvent.GetDataString("new_value") != "newemail@example.com" {
		t.Errorf("Expected new_value 'newemail@example.com', got %s", standardEvent.GetDataString("new_value"))
	}
}

func TestUserWithStandardEvents_Deactivate(t *testing.T) {
	user, _ := NewUserWithStandardEvents("user-123", "john@example.com", "John Doe")
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

	standardEvent, ok := events[0].(*domain.StandardEvent)
	if !ok {
		t.Error("Expected StandardEvent")
	}

	if standardEvent.EventType() != "User.StatusChanged" {
		t.Errorf("Expected event type 'User.StatusChanged', got %s", standardEvent.EventType())
	}

	if standardEvent.GetDataString("old_status") != "active" {
		t.Errorf("Expected old_status 'active', got %s", standardEvent.GetDataString("old_status"))
	}

	if standardEvent.GetDataString("new_status") != "inactive" {
		t.Errorf("Expected new_status 'inactive', got %s", standardEvent.GetDataString("new_status"))
	}
}

func TestUserWithStandardEvents_UpdateProfile(t *testing.T) {
	user, _ := NewUserWithStandardEvents("user-123", "john@example.com", "John Doe")
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

	standardEvent, ok := events[0].(*domain.StandardEvent)
	if !ok {
		t.Error("Expected StandardEvent")
	}

	if standardEvent.EventType() != "User.Updated" {
		t.Errorf("Expected event type 'User.Updated', got %s", standardEvent.EventType())
	}

	// Check that both email and name changes are captured
	if standardEvent.GetDataString("old_email") != "john@example.com" {
		t.Errorf("Expected old_email 'john@example.com', got %s", standardEvent.GetDataString("old_email"))
	}

	if standardEvent.GetDataString("new_email") != "john.doe@example.com" {
		t.Errorf("Expected new_email 'john.doe@example.com', got %s", standardEvent.GetDataString("new_email"))
	}

	if standardEvent.GetDataString("old_name") != "John Doe" {
		t.Errorf("Expected old_name 'John Doe', got %s", standardEvent.GetDataString("old_name"))
	}

	if standardEvent.GetDataString("new_name") != "John Smith" {
		t.Errorf("Expected new_name 'John Smith', got %s", standardEvent.GetDataString("new_name"))
	}
}

func TestUserWithStandardEvents_LoadFromHistory(t *testing.T) {
	// Create events representing user history using StandardEvents
	events := []domain.Event{
		domain.NewEvent("user-123", "User", "Created", map[string]interface{}{
			"email":     "john@example.com",
			"name":      "John Doe",
			"is_active": true,
		}),
		domain.NewEvent("user-123", "User", "EmailUpdated", map[string]interface{}{
			"field":     "Email",
			"old_value": "john@example.com",
			"new_value": "john.doe@example.com",
		}),
		domain.NewEvent("user-123", "User", "NameUpdated", map[string]interface{}{
			"field":     "Name",
			"old_value": "John Doe",
			"new_value": "John Smith",
		}),
		domain.NewEvent("user-123", "User", "StatusChanged", map[string]interface{}{
			"old_status": "active",
			"new_status": "inactive",
			"reason":     "user_requested",
		}),
	}

	// Create empty user and load from history
	user := &UserWithStandardEvents{Entity: domain.NewEntity("user-123")}
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

	if user.Version() != 4 {
		t.Errorf("Expected version 4, got %d", user.Version())
	}
}

func TestUserWithStandardEvents_GetEventSummary(t *testing.T) {
	user, _ := NewUserWithStandardEvents("user-123", "john@example.com", "John Doe")

	// Perform various operations
	user.ChangeEmail("new@example.com")
	user.ChangeName("New Name")
	user.Deactivate()

	summary := user.GetEventSummary()
	expectedSummary := []string{
		"User.Created",
		"User.EmailUpdated",
		"User.NameUpdated",
		"User.StatusChanged",
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

func TestUserWithStandardEvents_Delete(t *testing.T) {
	user, _ := NewUserWithStandardEvents("user-123", "john@example.com", "John Doe")
	user.MarkEventsAsCommitted()

	err := user.Delete("account_closure")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	events := user.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	standardEvent, ok := events[0].(*domain.StandardEvent)
	if !ok {
		t.Error("Expected StandardEvent")
	}

	if standardEvent.EventType() != "User.Deleted" {
		t.Errorf("Expected event type 'User.Deleted', got %s", standardEvent.EventType())
	}

	if standardEvent.GetDataString("reason") != "account_closure" {
		t.Errorf("Expected reason 'account_closure', got %s", standardEvent.GetDataString("reason"))
	}

	if !standardEvent.GetDataBool("soft_delete") {
		t.Error("Expected soft_delete to be true")
	}
}
