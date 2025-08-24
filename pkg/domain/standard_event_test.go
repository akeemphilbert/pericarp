package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewEvent(t *testing.T) {
	data := map[string]interface{}{
		"email":  "john@example.com",
		"name":   "John Doe",
		"age":    30,
		"active": true,
	}

	event := NewEvent("user-123", "User", "Created", data)

	if event.AggregateID() != "user-123" {
		t.Errorf("Expected aggregate ID 'user-123', got %s", event.AggregateID())
	}

	if event.EntityType() != "User" {
		t.Errorf("Expected entity type 'User', got %s", event.EntityType())
	}

	if event.ActionType() != "Created" {
		t.Errorf("Expected action type 'Created', got %s", event.ActionType())
	}

	if event.EventType() != "User.Created" {
		t.Errorf("Expected event type 'User.Created', got %s", event.EventType())
	}

	if event.Version() != 1 {
		t.Errorf("Expected version 1, got %d", event.Version())
	}

	// Check that occurred at is recent
	if time.Since(event.OccurredAt()) > time.Second {
		t.Error("Event occurred at should be recent")
	}
}

func TestStandardEvent_DataAccess(t *testing.T) {
	data := map[string]interface{}{
		"email":      "john@example.com",
		"name":       "John Doe",
		"age":        30,
		"active":     true,
		"created_at": time.Now(),
	}

	event := NewEvent("user-123", "User", "Created", data)

	// Test GetDataString
	if email := event.GetDataString("email"); email != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got %s", email)
	}

	if name := event.GetDataString("nonexistent"); name != "" {
		t.Errorf("Expected empty string for nonexistent key, got %s", name)
	}

	// Test GetDataInt
	if age := event.GetDataInt("age"); age != 30 {
		t.Errorf("Expected age 30, got %d", age)
	}

	if missing := event.GetDataInt("missing"); missing != 0 {
		t.Errorf("Expected 0 for missing key, got %d", missing)
	}

	// Test GetDataBool
	if active := event.GetDataBool("active"); !active {
		t.Error("Expected active to be true")
	}

	if inactive := event.GetDataBool("inactive"); inactive {
		t.Error("Expected inactive to be false")
	}

	// Test GetDataValue
	if value := event.GetDataValue("email"); value != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got %v", value)
	}

	if value := event.GetDataValue("missing"); value != nil {
		t.Errorf("Expected nil for missing key, got %v", value)
	}
}

func TestStandardEvent_SetDataValue(t *testing.T) {
	event := NewEvent("user-123", "User", "Created", nil)

	event.SetDataValue("email", "john@example.com")
	event.SetDataValue("age", 30)

	if email := event.GetDataString("email"); email != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got %s", email)
	}

	if age := event.GetDataInt("age"); age != 30 {
		t.Errorf("Expected age 30, got %d", age)
	}
}

func TestStandardEvent_Metadata(t *testing.T) {
	event := NewEvent("user-123", "User", "Created", nil)

	event.SetMetadata("correlation_id", "corr-123")
	event.SetMetadata("user_id", "admin-456")

	if corrID := event.GetMetadata("correlation_id"); corrID != "corr-123" {
		t.Errorf("Expected correlation_id 'corr-123', got %v", corrID)
	}

	if userID := event.GetMetadata("user_id"); userID != "admin-456" {
		t.Errorf("Expected user_id 'admin-456', got %v", userID)
	}

	metadata := event.Metadata()
	if len(metadata) != 2 {
		t.Errorf("Expected 2 metadata items, got %d", len(metadata))
	}

	// Verify that modifying returned metadata doesn't affect event
	metadata["new_key"] = "new_value"
	if event.GetMetadata("new_key") != nil {
		t.Error("Modifying returned metadata should not affect event")
	}
}

func TestStandardEvent_WithMethods(t *testing.T) {
	event := NewEvent("user-123", "User", "Created", nil)

	// Test method chaining
	event.WithVersion(5).WithMetadata("source", "api")

	if event.Version() != 5 {
		t.Errorf("Expected version 5, got %d", event.Version())
	}

	if source := event.GetMetadata("source"); source != "api" {
		t.Errorf("Expected source 'api', got %v", source)
	}
}

func TestStandardEvent_JSONSerialization(t *testing.T) {
	data := map[string]interface{}{
		"email": "john@example.com",
		"name":  "John Doe",
		"age":   30,
	}

	originalEvent := NewEvent("user-123", "User", "Created", data)
	originalEvent.SetMetadata("correlation_id", "corr-123")
	originalEvent.SetVersion(2)

	// Marshal to JSON
	jsonData, err := json.Marshal(originalEvent)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	// Unmarshal from JSON
	var deserializedEvent StandardEvent
	err = json.Unmarshal(jsonData, &deserializedEvent)
	if err != nil {
		t.Fatalf("Failed to unmarshal event: %v", err)
	}

	// Verify all fields are preserved
	if deserializedEvent.AggregateID() != originalEvent.AggregateID() {
		t.Errorf("AggregateID mismatch: expected %s, got %s", originalEvent.AggregateID(), deserializedEvent.AggregateID())
	}

	if deserializedEvent.EntityType() != originalEvent.EntityType() {
		t.Errorf("EntityType mismatch: expected %s, got %s", originalEvent.EntityType(), deserializedEvent.EntityType())
	}

	if deserializedEvent.ActionType() != originalEvent.ActionType() {
		t.Errorf("ActionType mismatch: expected %s, got %s", originalEvent.ActionType(), deserializedEvent.ActionType())
	}

	if deserializedEvent.Version() != originalEvent.Version() {
		t.Errorf("Version mismatch: expected %d, got %d", originalEvent.Version(), deserializedEvent.Version())
	}

	if deserializedEvent.GetDataString("email") != "john@example.com" {
		t.Errorf("Data email mismatch: expected john@example.com, got %s", deserializedEvent.GetDataString("email"))
	}

	if deserializedEvent.GetMetadata("correlation_id") != "corr-123" {
		t.Errorf("Metadata correlation_id mismatch: expected corr-123, got %v", deserializedEvent.GetMetadata("correlation_id"))
	}
}

func TestStandardEvent_DifferentEventTypes(t *testing.T) {
	data := map[string]interface{}{
		"email": "john@example.com",
		"name":  "John Doe",
	}

	// Test different event types using NewEvent
	createdEvent := NewEvent("user-123", "User", "Created", data)
	if createdEvent.EventType() != "User.Created" {
		t.Errorf("Expected event type 'User.Created', got %s", createdEvent.EventType())
	}

	updatedEvent := NewEvent("user-123", "User", "Updated", data)
	if updatedEvent.EventType() != "User.Updated" {
		t.Errorf("Expected event type 'User.Updated', got %s", updatedEvent.EventType())
	}

	deletedEvent := NewEvent("user-123", "User", "Deleted", data)
	if deletedEvent.EventType() != "User.Deleted" {
		t.Errorf("Expected event type 'User.Deleted', got %s", deletedEvent.EventType())
	}

	// Test status change event
	statusEvent := NewEvent("order-456", "Order", "StatusChanged", map[string]interface{}{
		"old_status": "pending",
		"new_status": "confirmed",
		"reason":     "payment_received",
	})
	if statusEvent.EventType() != "Order.StatusChanged" {
		t.Errorf("Expected event type 'Order.StatusChanged', got %s", statusEvent.EventType())
	}
	if statusEvent.GetDataString("old_status") != "pending" {
		t.Errorf("Expected old_status 'pending', got %s", statusEvent.GetDataString("old_status"))
	}
	if statusEvent.GetDataString("new_status") != "confirmed" {
		t.Errorf("Expected new_status 'confirmed', got %s", statusEvent.GetDataString("new_status"))
	}
	if statusEvent.GetDataString("reason") != "payment_received" {
		t.Errorf("Expected reason 'payment_received', got %s", statusEvent.GetDataString("reason"))
	}

	// Test field update event
	fieldEvent := NewEvent("user-123", "User", "EmailUpdated", map[string]interface{}{
		"field":     "Email",
		"old_value": "old@example.com",
		"new_value": "new@example.com",
	})
	if fieldEvent.EventType() != "User.EmailUpdated" {
		t.Errorf("Expected event type 'User.EmailUpdated', got %s", fieldEvent.EventType())
	}
	if fieldEvent.GetDataString("field") != "Email" {
		t.Errorf("Expected field 'Email', got %s", fieldEvent.GetDataString("field"))
	}
}

func TestStandardEvent_String(t *testing.T) {
	event := NewEvent("user-123", "User", "Created", nil)
	event.SetVersion(2)

	str := event.String()
	expectedPrefix := "StandardEvent{Type: User.Created, AggregateID: user-123, Version: 2"

	if !eventContains(str, expectedPrefix) {
		t.Errorf("Expected string to contain '%s', got %s", expectedPrefix, str)
	}
}

func TestStandardEvent_DataCopy(t *testing.T) {
	originalData := map[string]interface{}{
		"email": "john@example.com",
		"name":  "John Doe",
	}

	event := NewEvent("user-123", "User", "Created", originalData)

	// Get data copy
	dataCopy := event.Data()

	// Modify the copy
	dataCopy["email"] = "modified@example.com"
	dataCopy["new_field"] = "new_value"

	// Verify original data is unchanged
	if event.GetDataString("email") != "john@example.com" {
		t.Error("Original event data should not be modified when copy is changed")
	}

	if event.GetDataValue("new_field") != nil {
		t.Error("Original event should not have new field added to copy")
	}
}

func TestStandardEvent_TypeConversions(t *testing.T) {
	event := NewEvent("test-123", "Test", "Created", nil)

	// Test int conversions
	event.SetDataValue("int_value", 42)
	event.SetDataValue("int64_value", int64(64))
	event.SetDataValue("float64_value", 3.14)
	event.SetDataValue("string_number", "not_a_number")

	if event.GetDataInt("int_value") != 42 {
		t.Errorf("Expected int_value 42, got %d", event.GetDataInt("int_value"))
	}

	if event.GetDataInt("int64_value") != 64 {
		t.Errorf("Expected int64_value 64, got %d", event.GetDataInt("int64_value"))
	}

	if event.GetDataInt("float64_value") != 3 {
		t.Errorf("Expected float64_value 3, got %d", event.GetDataInt("float64_value"))
	}

	if event.GetDataInt("string_number") != 0 {
		t.Errorf("Expected string_number 0, got %d", event.GetDataInt("string_number"))
	}

	// Test time conversions
	now := time.Now()
	event.SetDataValue("time_value", now)
	event.SetDataValue("time_string", now.Format(time.RFC3339))
	event.SetDataValue("invalid_time", "not_a_time")

	retrievedTime := event.GetDataTime("time_value")
	if !retrievedTime.Equal(now) {
		t.Errorf("Expected time_value %v, got %v", now, retrievedTime)
	}

	parsedTime := event.GetDataTime("time_string")
	if parsedTime.IsZero() {
		t.Error("Expected time_string to be parsed successfully")
	}

	invalidTime := event.GetDataTime("invalid_time")
	if !invalidTime.IsZero() {
		t.Error("Expected invalid_time to return zero time")
	}
}

func TestNewEventWithTime(t *testing.T) {
	specificTime := time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC)
	data := map[string]interface{}{
		"message": "test",
	}

	event := NewEventWithTime("test-123", "Test", "Created", data, specificTime)

	if !event.OccurredAt().Equal(specificTime) {
		t.Errorf("Expected occurred at %v, got %v", specificTime, event.OccurredAt())
	}
}

// Helper function for string contains check
func eventContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark tests
func BenchmarkNewEvent(b *testing.B) {
	data := map[string]interface{}{
		"email": "john@example.com",
		"name":  "John Doe",
		"age":   30,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewEvent("user-123", "User", "Created", data)
	}
}

func BenchmarkStandardEvent_GetDataString(b *testing.B) {
	data := map[string]interface{}{
		"email": "john@example.com",
		"name":  "John Doe",
	}

	event := NewEvent("user-123", "User", "Created", data)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = event.GetDataString("email")
	}
}

func BenchmarkStandardEvent_JSONMarshal(b *testing.B) {
	data := map[string]interface{}{
		"email": "john@example.com",
		"name":  "John Doe",
		"age":   30,
	}

	event := NewEvent("user-123", "User", "Created", data)
	event.SetMetadata("correlation_id", "corr-123")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(event)
	}
}
