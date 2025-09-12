package examples

import (
	"fmt"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// SimpleEventUsageExample demonstrates the clean, simple API for creating events
// using the NewEntityEvent factory function.
func SimpleEventUsageExample() {
	// User creation event
	userCreated := domain.NewEntityEvent("user", "created", "user-123", "", "", map[string]interface{}{
		"email": "john@example.com",
		"name":  "John Doe",
		"role":  "admin",
	})
	fmt.Printf("Created event: %s\n", userCreated.EventType())

	// Order status update event
	orderUpdated := domain.NewEntityEvent("order", "status_changed", "order-456", "", "", map[string]interface{}{
		"old_status": "pending",
		"new_status": "shipped",
		"tracking":   "TRACK123",
	})
	fmt.Printf("Order event: %s\n", orderUpdated.EventType())

	// Product deletion event
	productDeleted := domain.NewEntityEvent("product", "deleted", "product-789", "", "", map[string]interface{}{
		"reason":     "discontinued",
		"deleted_by": "admin",
	})
	fmt.Printf("Product event: %s\n", productDeleted.EventType())

	// Custom business event
	paymentProcessed := domain.NewEntityEvent("payment", "processing_completed", "payment-999", "", "", map[string]interface{}{
		"amount":         99.99,
		"currency":       "USD",
		"gateway":        "stripe",
		"transaction_id": "txn_abc123",
	})
	fmt.Printf("Payment event: %s\n", paymentProcessed.EventType())

	// Event with metadata
	auditEvent := domain.NewEntityEvent("audit_log", "access_granted", "audit-001", "", "", map[string]interface{}{
		"user_id":    "user-123",
		"resource":   "/api/users",
		"ip_address": "192.168.1.1",
	})
	auditEvent.SetMetadata("correlation_id", "req-abc123")
	auditEvent.SetMetadata("source", "api_gateway")
	fmt.Printf("Audit event: %s with metadata: %v\n", auditEvent.EventType(), auditEvent.GetMetadata("correlation_id"))
}

// DemoFlexibleEventTypes shows how the single factory supports any event type
func DemoFlexibleEventTypes() {
	// Standard CRUD operations
	events := []domain.Event{
		domain.NewEntityEvent("item", "created", "item-1", "", "", map[string]interface{}{"name": "Widget"}),
		domain.NewEntityEvent("item", "updated", "item-1", "", "", map[string]interface{}{"name": "Super Widget"}),
		domain.NewEntityEvent("item", "deleted", "item-1", "", "", map[string]interface{}{"reason": "obsolete"}),
	}

	// Business-specific events
	businessEvents := []domain.Event{
		domain.NewEntityEvent("order", "payment_received", "order-1", "", "", map[string]interface{}{"amount": 100.0}),
		domain.NewEntityEvent("order", "inventory_reserved", "order-1", "", "", map[string]interface{}{"items": []string{"item-1", "item-2"}}),
		domain.NewEntityEvent("order", "shipping_label_generated", "order-1", "", "", map[string]interface{}{"tracking": "TRACK123"}),
		domain.NewEntityEvent("order", "customer_notified", "order-1", "", "", map[string]interface{}{"channel": "email"}),
	}

	// Workflow events
	workflowEvents := []domain.Event{
		domain.NewEntityEvent("workflow", "started", "workflow-1", "", "", map[string]interface{}{"trigger": "manual"}),
		domain.NewEntityEvent("workflow", "step_completed", "workflow-1", "", "", map[string]interface{}{"step": "validation"}),
		domain.NewEntityEvent("workflow", "step_failed", "workflow-1", "", "", map[string]interface{}{"step": "payment", "error": "insufficient_funds"}),
		domain.NewEntityEvent("workflow", "retried", "workflow-1", "", "", map[string]interface{}{"attempt": 2}),
		domain.NewEntityEvent("workflow", "completed", "workflow-1", "", "", map[string]interface{}{"duration_ms": 5000}),
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
