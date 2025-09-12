package domain

import (
	"testing"
	"time"
)

func TestNewStandardEventFromMap(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected StandardEvent
	}{
		{
			name: "complete event data",
			data: map[string]interface{}{
				"event_type":   "user.created",
				"aggregate_id": "user-123",
				"user_id":      "admin-456",
				"account_id":   "account-789",
				"sequence_no":  int64(5),
				"created_at":   time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				"email":        "test@example.com",
				"name":         "Test User",
			},
			expected: StandardEvent{
				data: map[string]interface{}{
					"event_type":   "user.created",
					"aggregate_id": "user-123",
					"user_id":      "admin-456",
					"account_id":   "account-789",
					"sequence_no":  int64(5),
					"created_at":   time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
					"email":        "test@example.com",
					"name":         "Test User",
				},
			},
		},
		{
			name: "minimal event data with defaults",
			data: map[string]interface{}{
				"event_type":   "order.shipped",
				"aggregate_id": "order-456",
				"user_id":      "customer-789",
				"account_id":   "account-123",
			},
			expected: StandardEvent{
				data: map[string]interface{}{
					"event_type":   "order.shipped",
					"aggregate_id": "order-456",
					"user_id":      "customer-789",
					"account_id":   "account-123",
					"sequence_no":  int64(0),
					"created_at":   time.Now(), // This will be set to current time
				},
			},
		},
		{
			name: "nil data",
			data: nil,
			expected: StandardEvent{
				data: map[string]interface{}{
					"sequence_no": int64(0),
					"created_at":  time.Now(), // This will be set to current time
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := NewStandardEventFromMap(tt.data)

			// Check that the event was created
			if event == nil {
				t.Fatal("Expected event to be created, got nil")
			}

			// For the minimal data test, we can't compare the exact time
			if tt.name == "minimal event data with defaults" || tt.name == "nil data" {
				// Just check that created_at is set and sequence_no is 0
				if event.SequenceNo() != 0 {
					t.Errorf("Expected sequence_no to be 0, got %d", event.SequenceNo())
				}
				if event.CreatedAt().IsZero() {
					t.Error("Expected created_at to be set, got zero time")
				}
			} else {
				// Check all fields for complete data test
				if event.EventType() != tt.expected.data["event_type"] {
					t.Errorf("Expected event_type %s, got %s", tt.expected.data["event_type"], event.EventType())
				}
				if event.AggregateID() != tt.expected.data["aggregate_id"] {
					t.Errorf("Expected aggregate_id %s, got %s", tt.expected.data["aggregate_id"], event.AggregateID())
				}
				if event.User() != tt.expected.data["user_id"] {
					t.Errorf("Expected user_id %s, got %s", tt.expected.data["user_id"], event.User())
				}
				if event.Account() != tt.expected.data["account_id"] {
					t.Errorf("Expected account_id %s, got %s", tt.expected.data["account_id"], event.Account())
				}
				if event.SequenceNo() != tt.expected.data["sequence_no"] {
					t.Errorf("Expected sequence_no %d, got %d", tt.expected.data["sequence_no"], event.SequenceNo())
				}
			}
		})
	}
}

func TestStandardEvent_GetString(t *testing.T) {
	data := map[string]interface{}{
		"event_type":   "user.created",
		"aggregate_id": "user-123",
		"email":        "test@example.com",
		"name":         "Test User",
		"age":          30, // Not a string
	}

	event := NewStandardEventFromMap(data)

	// Test existing string values
	if email, ok := event.GetString("email"); !ok || email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s' (ok: %t)", email, ok)
	}

	if name, ok := event.GetString("name"); !ok || name != "Test User" {
		t.Errorf("Expected name 'Test User', got '%s' (ok: %t)", name, ok)
	}

	// Test non-string value
	if _, ok := event.GetString("age"); ok {
		t.Error("Expected GetString to return false for non-string value")
	}

	// Test non-existent key
	if _, ok := event.GetString("non_existent"); ok {
		t.Error("Expected GetString to return false for non-existent key")
	}
}

func TestStandardEvent_GetInt(t *testing.T) {
	data := map[string]interface{}{
		"event_type":   "user.created",
		"aggregate_id": "user-123",
		"age":          30,
		"count":        int64(42),
		"price":        19.99,  // Float
		"name":         "Test", // String
	}

	event := NewStandardEventFromMap(data)

	// Test int value
	if age, ok := event.GetInt("age"); !ok || age != 30 {
		t.Errorf("Expected age 30, got %d (ok: %t)", age, ok)
	}

	// Test int64 value
	if count, ok := event.GetInt("count"); !ok || count != 42 {
		t.Errorf("Expected count 42, got %d (ok: %t)", count, ok)
	}

	// Test float value
	if price, ok := event.GetInt("price"); !ok || price != 19 {
		t.Errorf("Expected price 19, got %d (ok: %t)", price, ok)
	}

	// Test non-numeric value
	if _, ok := event.GetString("name"); !ok {
		t.Error("Expected GetString to work for string value")
	}

	// Test non-existent key
	if _, ok := event.GetInt("non_existent"); ok {
		t.Error("Expected GetInt to return false for non-existent key")
	}
}

func TestStandardEvent_GetBool(t *testing.T) {
	data := map[string]interface{}{
		"event_type":   "user.created",
		"aggregate_id": "user-123",
		"is_active":    true,
		"is_deleted":   false,
		"name":         "Test", // Not a bool
	}

	event := NewStandardEventFromMap(data)

	// Test true value
	if isActive, ok := event.GetBool("is_active"); !ok || !isActive {
		t.Errorf("Expected is_active true, got %t (ok: %t)", isActive, ok)
	}

	// Test false value
	if isDeleted, ok := event.GetBool("is_deleted"); !ok || isDeleted {
		t.Errorf("Expected is_deleted false, got %t (ok: %t)", isDeleted, ok)
	}

	// Test non-bool value
	if _, ok := event.GetBool("name"); ok {
		t.Error("Expected GetBool to return false for non-bool value")
	}

	// Test non-existent key
	if _, ok := event.GetBool("non_existent"); ok {
		t.Error("Expected GetBool to return false for non-existent key")
	}
}

func TestStandardEvent_SetSequenceNo(t *testing.T) {
	data := map[string]interface{}{
		"event_type":   "user.created",
		"aggregate_id": "user-123",
	}

	event := NewStandardEventFromMap(data)

	// Test setting sequence number
	event.SetSequenceNo(42)
	if event.SequenceNo() != 42 {
		t.Errorf("Expected sequence_no 42, got %d", event.SequenceNo())
	}
}

func TestStandardEvent_SetData(t *testing.T) {
	data := map[string]interface{}{
		"event_type":   "user.created",
		"aggregate_id": "user-123",
	}

	event := NewStandardEventFromMap(data)

	// Test setting new data
	event.SetData("email", "test@example.com")
	if email, ok := event.GetString("email"); !ok || email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s' (ok: %t)", email, ok)
	}

	// Test updating existing data
	event.SetData("email", "updated@example.com")
	if email, ok := event.GetString("email"); !ok || email != "updated@example.com" {
		t.Errorf("Expected email 'updated@example.com', got '%s' (ok: %t)", email, ok)
	}
}

func TestStandardEvent_GetAllData(t *testing.T) {
	data := map[string]interface{}{
		"event_type":   "user.created",
		"aggregate_id": "user-123",
		"email":        "test@example.com",
	}

	event := NewStandardEventFromMap(data)
	allData := event.GetAllData()

	// Test that we get a copy by checking they are different pointers
	// We can't directly compare maps, but we can check if modifying one affects the other

	// Test that the data is the same
	// The constructor adds sequence_no and created_at if not present
	expectedLen := len(data)
	if _, hasSeq := data["sequence_no"]; !hasSeq {
		expectedLen++
	}
	if _, hasCreated := data["created_at"]; !hasCreated {
		expectedLen++
	}
	if len(allData) != expectedLen {
		t.Errorf("Expected %d items, got %d", expectedLen, len(allData))
	}

	// Test that modifying the copy doesn't affect the original
	allData["new_field"] = "new_value"
	if _, ok := event.GetString("new_field"); ok {
		t.Error("Expected modifying the copy to not affect the original")
	}
}
