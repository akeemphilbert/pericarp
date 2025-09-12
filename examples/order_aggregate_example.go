package examples

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/google/uuid"
)

// OrderExample demonstrates the updated aggregate pattern using Entity and EntityEvent
type OrderExample struct {
	domain.BasicEntity // Embed the standard Entity struct
	customerID         string
	items              []OrderItemExample
	status             OrderStatusExample
	totalAmount        MoneyExample
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

// NewOrderExample creates a new order using the standard Entity and EntityEvent
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

	orderID := uuid.New().String()
	order := &OrderExample{
		BasicEntity: domain.NewEntity(orderID), // Use standard Entity
		customerID:  customerID,
		items:       items,
		status:      OrderStatusPending,
		totalAmount: MoneyExample{
			Amount:   totalAmount,
			Currency: items[0].Price.Currency, // Assume same currency
		},
	}

	// Generate domain event using EntityEvent
	eventData := struct {
		CustomerID  string             `json:"customer_id"`
		Items       []OrderItemExample `json:"items"`
		TotalAmount MoneyExample       `json:"total_amount"`
		Status      string             `json:"status"`
		CreatedAt   time.Time          `json:"created_at"`
	}{
		CustomerID:  customerID,
		Items:       items,
		TotalAmount: order.totalAmount,
		Status:      string(OrderStatusPending),
		CreatedAt:   time.Now(),
	}
	event := domain.NewEntityEvent("order", "created", orderID, "", "", eventData)

	order.AddEvent(event) // Use Entity's AddEvent method
	return order, nil
}

// ConfirmOrder confirms a pending order
func (o *OrderExample) ConfirmOrder() error {
	if o.status != OrderStatusPending {
		return errors.New("only pending orders can be confirmed")
	}

	o.status = OrderStatusConfirmed

	// Generate EntityEvent
	eventData := struct {
		ConfirmedAt time.Time `json:"confirmed_at"`
		Status      string    `json:"status"`
	}{
		ConfirmedAt: time.Now(),
		Status:      string(OrderStatusConfirmed),
	}
	event := domain.NewEntityEvent("order", "confirmed", o.ID(), "", "", eventData)

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

	eventData := struct {
		Reason      string    `json:"reason"`
		OldStatus   string    `json:"old_status"`
		NewStatus   string    `json:"new_status"`
		CancelledAt time.Time `json:"cancelled_at"`
	}{
		Reason:      reason,
		OldStatus:   string(oldStatus),
		NewStatus:   string(OrderStatusCancelled),
		CancelledAt: time.Now(),
	}
	event := domain.NewEntityEvent("order", "cancelled", o.ID(), "", "", eventData)

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
	// Call the Entity's LoadFromHistory to handle sequence number
	o.BasicEntity.LoadFromHistory(events)
}

// applyEvent applies a single event to the aggregate
func (o *OrderExample) applyEvent(event domain.Event) {
	// Cast to EntityEvent to access data
	entityEvent, ok := event.(*domain.EntityEvent)
	if !ok {
		return // Skip non-entity events
	}

	// Check if this is an order event
	if entityEvent.EntityType != "order" {
		return
	}

	// Parse the payload to access event data
	var data map[string]interface{}
	if err := json.Unmarshal(entityEvent.Payload(), &data); err != nil {
		return
	}

	switch entityEvent.Type {
	case "created":
		if customerID, ok := data["customer_id"].(string); ok {
			o.customerID = customerID
		}
		if status, ok := data["status"].(string); ok {
			o.status = OrderStatusExample(status)
		}
		// Note: In a real implementation, you'd properly deserialize items and totalAmount

	case "confirmed":
		o.status = OrderStatusConfirmed

	case "cancelled":
		o.status = OrderStatusCancelled
	}
}
