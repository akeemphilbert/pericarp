package examples

import (
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

func StandardEventExample() {
	// Example 1: Creating a StandardEvent from a map
	fmt.Println("=== StandardEvent Example ===")

	// Create event data from a map (e.g., from API payload or database)
	eventData := map[string]interface{}{
		"event_type":   "user.created",
		"aggregate_id": "user-123",
		"user_id":      "admin-456",
		"account_id":   "account-789",
		"email":        "john@example.com",
		"name":         "John Doe",
		"age":          30,
		"is_active":    true,
		"created_at":   time.Now(),
		"metadata": map[string]interface{}{
			"source":  "api",
			"version": "1.0",
		},
	}

	// Create the event using the factory function
	event := domain.NewStandardEventFromMap(eventData)

	// Access event properties using the Event interface
	fmt.Printf("Event Type: %s\n", event.EventType())
	fmt.Printf("Aggregate ID: %s\n", event.AggregateID())
	fmt.Printf("User ID: %s\n", event.User())
	fmt.Printf("Account ID: %s\n", event.Account())
	fmt.Printf("Sequence No: %d\n", event.SequenceNo())
	fmt.Printf("Created At: %s\n", event.CreatedAt().Format(time.RFC3339))

	// Access custom data using helper methods
	if email, ok := event.GetString("email"); ok {
		fmt.Printf("Email: %s\n", email)
	}
	if name, ok := event.GetString("name"); ok {
		fmt.Printf("Name: %s\n", name)
	}
	if age, ok := event.GetInt("age"); ok {
		fmt.Printf("Age: %d\n", age)
	}
	if isActive, ok := event.GetBool("is_active"); ok {
		fmt.Printf("Is Active: %t\n", isActive)
	}

	// Access nested data
	if metadata, ok := event.GetInterface("metadata"); ok {
		if metaMap, ok := metadata.(map[string]interface{}); ok {
			if source, ok := metaMap["source"].(string); ok {
				fmt.Printf("Source: %s\n", source)
			}
		}
	}

	// Get the entire payload as JSON
	payload := event.Payload()
	fmt.Printf("Payload (JSON): %s\n", string(payload))

	fmt.Println("\n=== Modifying Event Data ===")

	// Modify event data
	event.SetData("email", "john.doe@updated.com")
	event.SetData("last_updated", time.Now())

	// Update sequence number
	event.SetSequenceNo(1)

	// Get updated data
	if email, ok := event.GetString("email"); ok {
		fmt.Printf("Updated Email: %s\n", email)
	}
	fmt.Printf("Updated Sequence No: %d\n", event.SequenceNo())

	fmt.Println("\n=== Creating Event with Minimal Data ===")

	// Create event with minimal required data
	minimalData := map[string]interface{}{
		"event_type":      "order.shipped",
		"aggregate_id":    "order-456",
		"user_id":         "customer-789",
		"account_id":      "account-123",
		"tracking_number": "TRK123456789",
		"shipped_at":      time.Now(),
	}

	minimalEvent := domain.NewStandardEventFromMap(minimalData)
	fmt.Printf("Minimal Event Type: %s\n", minimalEvent.EventType())
	fmt.Printf("Minimal Event Aggregate ID: %s\n", minimalEvent.AggregateID())
	fmt.Printf("Minimal Event Sequence No: %d\n", minimalEvent.SequenceNo())

	if trackingNumber, ok := minimalEvent.GetString("tracking_number"); ok {
		fmt.Printf("Tracking Number: %s\n", trackingNumber)
	}
}
