package examples

import (
	"errors"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/segmentio/ksuid"
)

// OrderExample demonstrates the updated aggregate pattern using Entity and StandardEvent
type OrderExample struct {
	domain.Entity // Embed the standard Entity struct
	customerID    string
	items         []OrderItemExample
	status        OrderStatusExample
	totalAmount   MoneyExample
}

type OrderItemExample struct {
	ProductID string       `json:"product_id"`
	Quantity  int          `json:"quantity"`
	Price     MoneyExample `json:"price"`
}

type OrderStatusExample string

const (
	OrderStatusPending   OrderStatusExample = "pending"
	OrderStatusConfirmed OrderStatusExample = "confirmed"
	OrderStatusCancelled OrderStatusExample = "cancelled"
)

type MoneyExample struct {
	Amount   int64  `json:"amount"` // Amount in cents
	Currency string `json:"currency"`
}

// NewOrderExample creates a new order using the standard Entity and StandardEvent
func NewOrderExample(customerID string, items []OrderItemExample) (*OrderExample, error) {
	if customerID == "" {
		return nil, errors.New("customer ID is required")
	}

	if len(items) == 0 {
		return nil, errors.New("order must have at least one item")
	}

	// Validate items and calculate total
	var totalAmount int64
	for _, item := range items {
		if item.Quantity <= 0 {
			return nil, errors.New("item quantity must be positive")
		}
		if item.Price.Amount <= 0 {
			return nil, errors.New("item price must be positive")
		}
		totalAmount += item.Price.Amount * int64(item.Quantity)
	}

	orderID := ksuid.New().String()
	order := &OrderExample{
		Entity:     domain.NewEntity(orderID), // Use standard Entity
		customerID: customerID,
		items:      items,
		status:     OrderStatusPending,
		totalAmount: MoneyExample{
			Amount:   totalAmount,
			Currency: items[0].Price.Currency, // Assume same currency
		},
	}

	// Generate domain event using StandardEvent
	event := domain.NewEvent(orderID, "Order", "Created", map[string]interface{}{
		"customer_id":  customerID,
		"items":        items,
		"total_amount": order.totalAmount,
		"status":       string(OrderStatusPending),
		"created_at":   time.Now(),
	})

	order.AddEvent(event) // Use Entity's AddEvent method
	return order, nil
}

// ConfirmOrder confirms a pending order
func (o *OrderExample) ConfirmOrder() error {
	if o.status != OrderStatusPending {
		return errors.New("only pending orders can be confirmed")
	}

	o.status = OrderStatusConfirmed

	// Generate StandardEvent
	event := domain.NewEvent(o.ID(), "Order", "Confirmed", map[string]interface{}{
		"confirmed_at": time.Now(),
		"status":       string(OrderStatusConfirmed),
	})

	o.AddEvent(event) // Use Entity's AddEvent method
	return nil
}

// CancelOrder cancels an order if possible
func (o *OrderExample) CancelOrder(reason string) error {
	if o.status == OrderStatusCancelled {
		return errors.New("order is already cancelled")
	}

	oldStatus := o.status
	o.status = OrderStatusCancelled

	event := domain.NewEvent(o.ID(), "Order", "Cancelled", map[string]interface{}{
		"reason":       reason,
		"old_status":   string(oldStatus),
		"new_status":   string(OrderStatusCancelled),
		"cancelled_at": time.Now(),
	})

	o.AddEvent(event)
	return nil
}

// Getter methods
func (o *OrderExample) CustomerID() string         { return o.customerID }
func (o *OrderExample) Items() []OrderItemExample  { return o.items }
func (o *OrderExample) Status() OrderStatusExample { return o.status }
func (o *OrderExample) TotalAmount() MoneyExample  { return o.totalAmount }

// Business query methods
func (o *OrderExample) CanBeModified() bool {
	return o.status == OrderStatusPending
}

func (o *OrderExample) CanBeCancelled() bool {
	return o.status != OrderStatusCancelled
}

// LoadFromHistory reconstructs the aggregate from events
func (o *OrderExample) LoadFromHistory(events []domain.Event) {
	for _, event := range events {
		o.applyEvent(event)
	}
	// Call the Entity's LoadFromHistory to handle version and sequence
	o.Entity.LoadFromHistory(events)
}

// applyEvent applies a single event to the aggregate
func (o *OrderExample) applyEvent(event domain.Event) {
	// Cast to StandardEvent to access data
	standardEvent, ok := event.(*domain.StandardEvent)
	if !ok {
		return // Skip non-standard events
	}

	// Check if this is an Order event
	if standardEvent.EntityType() != "Order" {
		return
	}

	data := standardEvent.Data()

	switch standardEvent.ActionType() {
	case "Created":
		o.customerID = data["customer_id"].(string)
		o.status = OrderStatusExample(data["status"].(string))
		// Note: In a real implementation, you'd properly deserialize items and totalAmount

	case "Confirmed":
		o.status = OrderStatusConfirmed

	case "Cancelled":
		o.status = OrderStatusCancelled
	}
}
