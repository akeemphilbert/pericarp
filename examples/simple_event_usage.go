package examples

import (
	"fmt"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// SimpleEventUsageExample demonstrates the clean, simple API for creating events
// using just the NewEvent factory function.
func SimpleEventUsageExample() {
	// User creation event
	userCreated := domain.NewEvent("user-123", "User", "Created", map[string]interface{}{
		"email": "john@example.com",
		"name":  "John Doe",
		"role":  "admin",
	})
	fmt.Printf("Created event: %s\n", userCreated.EventType())

	// Order status update event
	orderUpdated := domain.NewEvent("order-456", "Order", "StatusChanged", map[string]interface{}{
		"old_status": "pending",
		"new_status": "shipped",
		"tracking":   "TRACK123",
	})
	fmt.Printf("Order event: %s\n", orderUpdated.EventType())

	// Product deletion event
	productDeleted := domain.NewEvent("product-789", "Product", "Deleted", map[string]interface{}{
		"reason":     "discontinued",
		"deleted_by": "admin",
	})
	fmt.Printf("Product event: %s\n", productDeleted.EventType())

	// Custom business event
	paymentProcessed := domain.NewEvent("payment-999", "Payment", "ProcessingCompleted", map[string]interface{}{
		"amount":         99.99,
		"currency":       "USD",
		"gateway":        "stripe",
		"transaction_id": "txn_abc123",
	})
	fmt.Printf("Payment event: %s\n", paymentProcessed.EventType())

	// Event with metadata
	auditEvent := domain.NewEvent("audit-001", "AuditLog", "AccessGranted", map[string]interface{}{
		"user_id":    "user-123",
		"resource":   "/api/users",
		"ip_address": "192.168.1.1",
	})
	auditEvent.SetMetadata("correlation_id", "req-abc123")
	auditEvent.SetMetadata("source", "api_gateway")
	fmt.Printf("Audit event: %s with metadata: %v\n", auditEvent.EventType(), auditEvent.Metadata())
}

// DemoFlexibleEventTypes shows how the single factory supports any event type
func DemoFlexibleEventTypes() {
	// Standard CRUD operations
	events := []domain.Event{
		domain.NewEvent("item-1", "Item", "Created", map[string]interface{}{"name": "Widget"}),
		domain.NewEvent("item-1", "Item", "Updated", map[string]interface{}{"name": "Super Widget"}),
		domain.NewEvent("item-1", "Item", "Deleted", map[string]interface{}{"reason": "obsolete"}),
	}

	// Business-specific events
	businessEvents := []domain.Event{
		domain.NewEvent("order-1", "Order", "PaymentReceived", map[string]interface{}{"amount": 100.0}),
		domain.NewEvent("order-1", "Order", "InventoryReserved", map[string]interface{}{"items": []string{"item-1", "item-2"}}),
		domain.NewEvent("order-1", "Order", "ShippingLabelGenerated", map[string]interface{}{"tracking": "TRACK123"}),
		domain.NewEvent("order-1", "Order", "CustomerNotified", map[string]interface{}{"channel": "email"}),
	}

	// Workflow events
	workflowEvents := []domain.Event{
		domain.NewEvent("workflow-1", "Workflow", "Started", map[string]interface{}{"trigger": "manual"}),
		domain.NewEvent("workflow-1", "Workflow", "StepCompleted", map[string]interface{}{"step": "validation"}),
		domain.NewEvent("workflow-1", "Workflow", "StepFailed", map[string]interface{}{"step": "payment", "error": "insufficient_funds"}),
		domain.NewEvent("workflow-1", "Workflow", "Retried", map[string]interface{}{"attempt": 2}),
		domain.NewEvent("workflow-1", "Workflow", "Completed", map[string]interface{}{"duration_ms": 5000}),
	}

	fmt.Printf("Created %d standard events\n", len(events))
	fmt.Printf("Created %d business events\n", len(businessEvents))
	fmt.Printf("Created %d workflow events\n", len(workflowEvents))

	// All events implement the same Event interface
	allEvents := append(events, businessEvents...)
	allEvents = append(allEvents, workflowEvents...)

	fmt.Println("\nAll event types:")
	for _, event := range allEvents {
		fmt.Printf("- %s\n", event.EventType())
	}
}
