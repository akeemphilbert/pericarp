package examples

import (
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
		t.Errorf("Expected customer ID 'customer-123', got %s", order.CustomerID())
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

	standardEvent, ok := events[0].(*domain.StandardEvent)
	if !ok {
		t.Fatal("Expected StandardEvent")
	}

	if standardEvent.EventType() != "Order.Created" {
		t.Errorf("Expected event type 'Order.Created', got %s", standardEvent.EventType())
	}

	if standardEvent.AggregateID() != order.ID() {
		t.Error("Event aggregate ID should match order ID")
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

	// Check the confirmation event (StandardEvent)
	confirmEvent, ok := events[1].(*domain.StandardEvent)
	if !ok {
		t.Error("Expected StandardEvent")
	}

	if confirmEvent.EventType() != "Order.Confirmed" {
		t.Errorf("Expected 'Order.Confirmed', got %s", confirmEvent.EventType())
	}

	if confirmEvent.AggregateID() != order.ID() {
		t.Error("Event aggregate ID should match order ID")
	}

	// Check event data
	data := confirmEvent.Data()
	if data["status"] != "confirmed" {
		t.Errorf("Expected status 'confirmed' in event data, got %v", data["status"])
	}
}

func TestOrderExample_LoadFromHistory(t *testing.T) {
	// Arrange - create events that represent order history
	orderID := "order-123"
	events := []domain.Event{
		domain.NewEvent(orderID, "Order", "Created", map[string]interface{}{
			"customer_id": "customer-456",
			"status":      "pending",
		}),
		domain.NewEvent(orderID, "Order", "Confirmed", map[string]interface{}{
			"status": "confirmed",
		}),
	}

	// Act - reconstruct order from events
	order := &OrderExample{Entity: domain.NewEntity(orderID)}
	order.LoadFromHistory(events)

	// Assert
	if order.ID() != orderID {
		t.Errorf("Expected ID %s, got %s", orderID, order.ID())
	}

	if order.Status() != OrderStatusConfirmed {
		t.Errorf("Expected status confirmed, got %v", order.Status())
	}

	if order.Version() != 2 {
		t.Errorf("Expected version 2, got %d", order.Version())
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
	cancelEvent, ok := events[1].(*domain.StandardEvent)
	if !ok {
		t.Error("Expected StandardEvent")
	}

	if cancelEvent.EventType() != "Order.Cancelled" {
		t.Errorf("Expected 'Order.Cancelled', got %s", cancelEvent.EventType())
	}

	// Check event data
	data := cancelEvent.Data()
	if data["reason"] != "customer_requested" {
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
