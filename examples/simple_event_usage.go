package examples

import (
	"fmt"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

// Simple entities for demonstration
type SimpleUser struct {
	*domain.BasicEntity
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type SimpleOrder struct {
	*domain.BasicEntity
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
	Tracking  string `json:"tracking"`
}

type SimpleProduct struct {
	*domain.BasicEntity
	Reason    string `json:"reason"`
	DeletedBy string `json:"deleted_by"`
}

type SimplePayment struct {
	*domain.BasicEntity
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	Gateway       string  `json:"gateway"`
	TransactionID string  `json:"transaction_id"`
}

type SimpleAuditLog struct {
	*domain.BasicEntity
	UserID    string `json:"user_id"`
	Resource  string `json:"resource"`
	IPAddress string `json:"ip_address"`
}

type SimpleItem struct {
	*domain.BasicEntity
	Name string `json:"name"`
}

type SimpleWorkflow struct {
	*domain.BasicEntity
	Trigger    string `json:"trigger,omitempty"`
	Step       string `json:"step,omitempty"`
	Error      string `json:"error,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
	DurationMs int    `json:"duration_ms,omitempty"`
}

// SimpleEventUsageExample demonstrates the clean, simple API for creating events
// using the NewEntityEvent factory function.
func SimpleEventUsageExample() {
	// User creation event
	user := &SimpleUser{
		BasicEntity: domain.NewEntity("user-123"),
		Email:       "john@example.com",
		Name:        "John Doe",
		Role:        "admin",
	}
	userCreated := domain.NewEntityEvent(nil, nil, "user", "created", "user-123", user)
	fmt.Printf("Created event: %s\n", userCreated.EventType())

	// Order status update event
	order := &SimpleOrder{
		BasicEntity: domain.NewEntity("order-456"),
		OldStatus:   "pending",
		NewStatus:   "shipped",
		Tracking:    "TRACK123",
	}
	orderUpdated := domain.NewEntityEvent(nil, nil, "order", "status_changed", "order-456", order)
	fmt.Printf("Order event: %s\n", orderUpdated.EventType())

	// Product deletion event
	product := &SimpleProduct{
		BasicEntity: domain.NewEntity("product-789"),
		Reason:      "discontinued",
		DeletedBy:   "admin",
	}
	productDeleted := domain.NewEntityEvent(nil, nil, "product", "deleted", "product-789", product)
	fmt.Printf("Product event: %s\n", productDeleted.EventType())

	// Custom business event
	payment := &SimplePayment{
		BasicEntity:   domain.NewEntity("payment-999"),
		Amount:        99.99,
		Currency:      "USD",
		Gateway:       "stripe",
		TransactionID: "txn_abc123",
	}
	paymentProcessed := domain.NewEntityEvent(nil, nil, "payment", "processing_completed", "payment-999", payment)
	fmt.Printf("Payment event: %s\n", paymentProcessed.EventType())

	// Event with metadata
	audit := &SimpleAuditLog{
		BasicEntity: domain.NewEntity("audit-001"),
		UserID:      "user-123",
		Resource:    "/api/users",
		IPAddress:   "192.168.1.1",
	}
	auditEvent := domain.NewEntityEvent(nil, nil, "audit_log", "access_granted", "audit-001", audit)
	auditEvent.SetMetadata("correlation_id", "req-abc123")
	auditEvent.SetMetadata("source", "api_gateway")
	fmt.Printf("Audit event: %s with metadata: %v\n", auditEvent.EventType(), auditEvent.GetMetadata("correlation_id"))
}

// DemoFlexibleEventTypes shows how the single factory supports any event type
func DemoFlexibleEventTypes() {
	// Standard CRUD operations
	item1 := &SimpleItem{BasicEntity: domain.NewEntity("item-1"), Name: "Widget"}
	item2 := &SimpleItem{BasicEntity: domain.NewEntity("item-1"), Name: "Super Widget"}
	item3 := &SimpleItem{BasicEntity: domain.NewEntity("item-1"), Name: "Widget"}

	events := []domain.Event{
		domain.NewEntityEvent(nil, nil, "item", "created", "item-1", item1),
		domain.NewEntityEvent(nil, nil, "item", "updated", "item-1", item2),
		domain.NewEntityEvent(nil, nil, "item", "deleted", "item-1", item3),
	}

	// Business-specific events
	payment1 := &SimplePayment{BasicEntity: domain.NewEntity("order-1"), Amount: 100.0}
	workflow1 := &SimpleWorkflow{BasicEntity: domain.NewEntity("order-1"), Trigger: "manual"}
	workflow2 := &SimpleWorkflow{BasicEntity: domain.NewEntity("order-1"), Step: "validation"}
	workflow3 := &SimpleWorkflow{BasicEntity: domain.NewEntity("order-1"), Step: "payment", Error: "insufficient_funds"}
	workflow4 := &SimpleWorkflow{BasicEntity: domain.NewEntity("order-1"), Attempt: 2}
	workflow5 := &SimpleWorkflow{BasicEntity: domain.NewEntity("order-1"), DurationMs: 5000}

	businessEvents := []domain.Event{
		domain.NewEntityEvent(nil, nil, "order", "payment_received", "order-1", payment1),
		domain.NewEntityEvent(nil, nil, "order", "inventory_reserved", "order-1", workflow1),
		domain.NewEntityEvent(nil, nil, "order", "shipping_label_generated", "order-1", workflow2),
		domain.NewEntityEvent(nil, nil, "order", "customer_notified", "order-1", workflow3),
	}

	// Workflow events
	workflowEvents := []domain.Event{
		domain.NewEntityEvent(nil, nil, "workflow", "started", "workflow-1", workflow1),
		domain.NewEntityEvent(nil, nil, "workflow", "step_completed", "workflow-1", workflow2),
		domain.NewEntityEvent(nil, nil, "workflow", "step_failed", "workflow-1", workflow3),
		domain.NewEntityEvent(nil, nil, "workflow", "retried", "workflow-1", workflow4),
		domain.NewEntityEvent(nil, nil, "workflow", "completed", "workflow-1", workflow5),
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
