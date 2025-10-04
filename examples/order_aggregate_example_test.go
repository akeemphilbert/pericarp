package examples

import (
	"encoding/json"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

func TestNewOrderExample(t *testing.T) {
	items := []OrderItemExample{
		{ProductID: "prod-1", Quantity: 2, Price: MoneyExample{Amount: 1000, Currency: "USD"}},
		{ProductID: "prod-2", Quantity: 1, Price: MoneyExample{Amount: 2500, Currency: "USD"}},
	}

	order, err := NewOrderExample("customer-123", items)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if order.CustomerID() != "customer-123" {
		t.Errorf("Expected customer GetID 'customer-123', got %s", order.CustomerID())
	}

	if order.Status() != OrderStatusPending {
		t.Errorf("Expected status pending, got %v", order.Status())
	}

	if order.TotalAmount().Amount != 4500 {
		t.Errorf("Expected total amount 4500, got %d", order.TotalAmount().Amount)
	}

	// Check event generation
	events := order.UncommittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	entityEvent, ok := events[0].(*domain.EntityEvent)
	if !ok {
		t.Fatal("Expected EntityEvent")
	}

	if entityEvent.EventType() != "order.created" {
		t.Errorf("Expected event type 'order.created', got %s", entityEvent.EventType())
	}

	if entityEvent.AggregateID() != order.GetID() {
		t.Error("Event aggregate GetID should match order GetID")
	}
}

func TestOrderExample_ConfirmOrder(t *testing.T) {
	// Arrange
	items := []OrderItemExample{
		{ProductID: "prod-1", Quantity: 1, Price: MoneyExample{Amount: 1000, Currency: "USD"}},
	}
	order, _ := NewOrderExample("customer-123", items)

	// Act
	err := order.ConfirmOrder()

	// Assert
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if order.Status() != OrderStatusConfirmed {
		t.Errorf("Expected status to be confirmed, got %v", order.Status())
	}

	events := order.UncommittedEvents()
	if len(events) != 2 { // Order.Created + Order.Confirmed
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	// Check the confirmation event (EntityEvent)
	confirmEvent, ok := events[1].(*domain.EntityEvent)
	if !ok {
		t.Error("Expected EntityEvent")
	}

	if confirmEvent.EventType() != "order.confirmed" {
		t.Errorf("Expected 'order.confirmed', got %s", confirmEvent.EventType())
	}

	if confirmEvent.AggregateID() != order.GetID() {
		t.Error("Event aggregate GetID should match order GetID")
	}

	// Check event data
	var data map[string]interface{}
	if err := json.Unmarshal(confirmEvent.Payload(), &data); err != nil {
		t.Fatalf("Failed to parse payload: %v", err)
	}
	if status, ok := data["status"].(string); !ok || status != "confirmed" {
		t.Errorf("Expected status 'confirmed' in event data, got %v", data["status"])
	}
}

func TestOrderExample_LoadFromHistory(t *testing.T) {
	// Arrange - create events that represent order history
	orderID := "order-123"
	events := []domain.Event{
		domain.NewEntityEvent(nil, nil, "order", "created", orderID, map[string]interface{}{
			"customer_id": "customer-456",
			"status":      "pending",
		}),
		domain.NewEntityEvent(nil, nil, "order", "confirmed", orderID, map[string]interface{}{
			"status": "confirmed",
		}),
	}

	// Act - reconstruct order from events
	order := &OrderExample{BasicEntity: *domain.NewEntity(orderID)}
	order.LoadFromHistory(events)

	// Assert
	if order.GetID() != orderID {
		t.Errorf("Expected GetID %s, got %s", orderID, order.GetID())
	}

	if order.Status() != OrderStatusConfirmed {
		t.Errorf("Expected status confirmed, got %v", order.Status())
	}

	if order.GetSequenceNo() != 2 {
		t.Errorf("Expected sequence number 2, got %d", order.GetSequenceNo())
	}

	// Should have no uncommitted events after loading from history
	if len(order.UncommittedEvents()) != 0 {
		t.Errorf("Expected no uncommitted events, got %d", len(order.UncommittedEvents()))
	}
}

func TestOrderExample_CancelOrder(t *testing.T) {
	// Arrange
	items := []OrderItemExample{
		{ProductID: "prod-1", Quantity: 1, Price: MoneyExample{Amount: 1000, Currency: "USD"}},
	}
	order, _ := NewOrderExample("customer-123", items)

	// Act
	err := order.CancelOrder("customer_requested")

	// Assert
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if order.Status() != OrderStatusCancelled {
		t.Errorf("Expected status to be cancelled, got %v", order.Status())
	}

	events := order.UncommittedEvents()
	if len(events) != 2 { // Order.Created + Order.Cancelled
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	// Check the cancellation event
	cancelEvent, ok := events[1].(*domain.EntityEvent)
	if !ok {
		t.Error("Expected EntityEvent")
	}

	if cancelEvent.EventType() != "order.cancelled" {
		t.Errorf("Expected 'order.cancelled', got %s", cancelEvent.EventType())
	}

	// Check event data
	var data map[string]interface{}
	if err := json.Unmarshal(cancelEvent.Payload(), &data); err != nil {
		t.Fatalf("Failed to parse payload: %v", err)
	}
	if reason, ok := data["reason"].(string); !ok || reason != "customer_requested" {
		t.Errorf("Expected reason 'customer_requested' in event data, got %v", data["reason"])
	}
}

func TestOrderExample_BusinessRules(t *testing.T) {
	items := []OrderItemExample{
		{ProductID: "prod-1", Quantity: 1, Price: MoneyExample{Amount: 1000, Currency: "USD"}},
	}
	order, _ := NewOrderExample("customer-123", items)

	// Test business query methods
	if !order.CanBeModified() {
		t.Error("Pending order should be modifiable")
	}

	if !order.CanBeCancelled() {
		t.Error("Pending order should be cancellable")
	}

	// Confirm order
	order.ConfirmOrder()

	if order.CanBeModified() {
		t.Error("Confirmed order should not be modifiable")
	}

	if !order.CanBeCancelled() {
		t.Error("Confirmed order should still be cancellable")
	}

	// Cancel order
	order.CancelOrder("test")

	if order.CanBeCancelled() {
		t.Error("Cancelled order should not be cancellable")
	}
}
