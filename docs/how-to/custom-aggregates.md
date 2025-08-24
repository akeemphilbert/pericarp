# How to Implement Custom Aggregates

This guide shows you how to create custom domain aggregates that encapsulate business logic and generate domain events.

## Problem

You need to create a domain aggregate that:
- Encapsulates business rules and invariants
- Generates domain events when state changes
- Supports event sourcing for persistence
- Maintains consistency boundaries

## Solution

Follow these steps to implement a custom aggregate:

### Step 1: Define the Aggregate Structure

```go
package domain

import (
    "errors"
    "time"
    "github.com/google/uuid"
    "github.com/your-org/pericarp/pkg/domain"
)

// Order represents an e-commerce order aggregate
type Order struct {
    id          string
    customerID  string
    items       []OrderItem
    status      OrderStatus
    totalAmount Money
    version     int
    events      []domain.Event
}

type OrderItem struct {
    ProductID string
    Quantity  int
    Price     Money
}

type OrderStatus string

const (
    OrderStatusPending   OrderStatus = "pending"
    OrderStatusConfirmed OrderStatus = "confirmed"
    OrderStatusShipped   OrderStatus = "shipped"
    OrderStatusDelivered OrderStatus = "delivered"
    OrderStatusCancelled OrderStatus = "cancelled"
)

type Money struct {
    Amount   int64  // Amount in cents
    Currency string
}
```

### Step 2: Implement the Constructor

```go
// NewOrder creates a new order with validation
func NewOrder(customerID string, items []OrderItem) (*Order, error) {
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
    order := &Order{
        id:         orderID,
        customerID: customerID,
        items:      items,
        status:     OrderStatusPending,
        totalAmount: Money{
            Amount:   totalAmount,
            Currency: items[0].Price.Currency, // Assume same currency
        },
        version: 1,
    }
    
    // Generate domain event
    event := OrderCreatedEvent{
        OrderID:     orderID,
        CustomerID:  customerID,
        Items:       items,
        TotalAmount: order.totalAmount,
        CreatedAt:   time.Now(),
    }
    
    order.addEvent(event)
    return order, nil
}
```

### Step 3: Implement Business Methods

```go
// ConfirmOrder confirms a pending order
func (o *Order) ConfirmOrder() error {
    if o.status != OrderStatusPending {
        return errors.New("only pending orders can be confirmed")
    }
    
    o.status = OrderStatusConfirmed
    o.version++
    
    event := OrderConfirmedEvent{
        OrderID:     o.id,
        ConfirmedAt: time.Now(),
    }
    
    o.addEvent(event)
    return nil
}

// AddItem adds an item to a pending order
func (o *Order) AddItem(item OrderItem) error {
    if o.status != OrderStatusPending {
        return errors.New("cannot add items to non-pending order")
    }
    
    if item.Quantity <= 0 {
        return errors.New("item quantity must be positive")
    }
    
    // Check if item already exists
    for i, existingItem := range o.items {
        if existingItem.ProductID == item.ProductID {
            o.items[i].Quantity += item.Quantity
            o.recalculateTotal()
            o.version++
            
            event := OrderItemUpdatedEvent{
                OrderID:   o.id,
                ProductID: item.ProductID,
                Quantity:  o.items[i].Quantity,
                UpdatedAt: time.Now(),
            }
            
            o.addEvent(event)
            return nil
        }
    }
    
    // Add new item
    o.items = append(o.items, item)
    o.recalculateTotal()
    o.version++
    
    event := OrderItemAddedEvent{
        OrderID:   o.id,
        Item:      item,
        AddedAt:   time.Now(),
    }
    
    o.addEvent(event)
    return nil
}

// CancelOrder cancels an order if possible
func (o *Order) CancelOrder(reason string) error {
    if o.status == OrderStatusDelivered {
        return errors.New("cannot cancel delivered order")
    }
    
    if o.status == OrderStatusCancelled {
        return errors.New("order is already cancelled")
    }
    
    o.status = OrderStatusCancelled
    o.version++
    
    event := OrderCancelledEvent{
        OrderID:     o.id,
        Reason:      reason,
        CancelledAt: time.Now(),
    }
    
    o.addEvent(event)
    return nil
}

// Private helper methods
func (o *Order) recalculateTotal() {
    var total int64
    for _, item := range o.items {
        total += item.Price.Amount * int64(item.Quantity)
    }
    o.totalAmount.Amount = total
}

func (o *Order) addEvent(event domain.Event) {
    o.events = append(o.events, event)
}
```

### Step 4: Implement Aggregate Root Interface

```go
// ID returns the aggregate ID
func (o *Order) ID() string {
    return o.id
}

// Version returns the aggregate version
func (o *Order) Version() int {
    return o.version
}

// UncommittedEvents returns events that haven't been persisted
func (o *Order) UncommittedEvents() []domain.Event {
    return o.events
}

// MarkEventsAsCommitted clears the uncommitted events
func (o *Order) MarkEventsAsCommitted() {
    o.events = nil
}

// LoadFromHistory reconstructs the aggregate from events
func (o *Order) LoadFromHistory(events []domain.Event) {
    for _, event := range events {
        o.applyEvent(event)
        o.version++
    }
    // Clear events after loading
    o.events = nil
}

// applyEvent applies a single event to the aggregate
func (o *Order) applyEvent(event domain.Event) {
    switch e := event.(type) {
    case OrderCreatedEvent:
        o.id = e.OrderID
        o.customerID = e.CustomerID
        o.items = e.Items
        o.totalAmount = e.TotalAmount
        o.status = OrderStatusPending
        
    case OrderConfirmedEvent:
        o.status = OrderStatusConfirmed
        
    case OrderItemAddedEvent:
        o.items = append(o.items, e.Item)
        o.recalculateTotal()
        
    case OrderItemUpdatedEvent:
        for i, item := range o.items {
            if item.ProductID == e.ProductID {
                o.items[i].Quantity = e.Quantity
                break
            }
        }
        o.recalculateTotal()
        
    case OrderCancelledEvent:
        o.status = OrderStatusCancelled
    }
}
```

### Step 5: Define Domain Events

```go
// OrderCreatedEvent represents order creation
type OrderCreatedEvent struct {
    OrderID     string      `json:"order_id"`
    CustomerID  string      `json:"customer_id"`
    Items       []OrderItem `json:"items"`
    TotalAmount Money       `json:"total_amount"`
    CreatedAt   time.Time   `json:"created_at"`
}

func (e OrderCreatedEvent) EventType() string { return "OrderCreated" }
func (e OrderCreatedEvent) AggregateID() string { return e.OrderID }
func (e OrderCreatedEvent) Version() int { return 1 }
func (e OrderCreatedEvent) OccurredAt() time.Time { return e.CreatedAt }

// OrderConfirmedEvent represents order confirmation
type OrderConfirmedEvent struct {
    OrderID     string    `json:"order_id"`
    ConfirmedAt time.Time `json:"confirmed_at"`
}

func (e OrderConfirmedEvent) EventType() string { return "OrderConfirmed" }
func (e OrderConfirmedEvent) AggregateID() string { return e.OrderID }
func (e OrderConfirmedEvent) Version() int { return 1 }
func (e OrderConfirmedEvent) OccurredAt() time.Time { return e.ConfirmedAt }

// OrderItemAddedEvent represents item addition
type OrderItemAddedEvent struct {
    OrderID string      `json:"order_id"`
    Item    OrderItem   `json:"item"`
    AddedAt time.Time   `json:"added_at"`
}

func (e OrderItemAddedEvent) EventType() string { return "OrderItemAdded" }
func (e OrderItemAddedEvent) AggregateID() string { return e.OrderID }
func (e OrderItemAddedEvent) Version() int { return 1 }
func (e OrderItemAddedEvent) OccurredAt() time.Time { return e.AddedAt }

// OrderItemUpdatedEvent represents item quantity update
type OrderItemUpdatedEvent struct {
    OrderID   string    `json:"order_id"`
    ProductID string    `json:"product_id"`
    Quantity  int       `json:"quantity"`
    UpdatedAt time.Time `json:"updated_at"`
}

func (e OrderItemUpdatedEvent) EventType() string { return "OrderItemUpdated" }
func (e OrderItemUpdatedEvent) AggregateID() string { return e.OrderID }
func (e OrderItemUpdatedEvent) Version() int { return 1 }
func (e OrderItemUpdatedEvent) OccurredAt() time.Time { return e.UpdatedAt }

// OrderCancelledEvent represents order cancellation
type OrderCancelledEvent struct {
    OrderID     string    `json:"order_id"`
    Reason      string    `json:"reason"`
    CancelledAt time.Time `json:"cancelled_at"`
}

func (e OrderCancelledEvent) EventType() string { return "OrderCancelled" }
func (e OrderCancelledEvent) AggregateID() string { return e.OrderID }
func (e OrderCancelledEvent) Version() int { return 1 }
func (e OrderCancelledEvent) OccurredAt() time.Time { return e.CancelledAt }
```

### Step 6: Add Getter Methods

```go
// Getter methods for accessing aggregate state
func (o *Order) CustomerID() string { return o.customerID }
func (o *Order) Items() []OrderItem { return o.items }
func (o *Order) Status() OrderStatus { return o.status }
func (o *Order) TotalAmount() Money { return o.totalAmount }

// Business query methods
func (o *Order) CanBeModified() bool {
    return o.status == OrderStatusPending
}

func (o *Order) CanBeCancelled() bool {
    return o.status != OrderStatusDelivered && o.status != OrderStatusCancelled
}

func (o *Order) ItemCount() int {
    count := 0
    for _, item := range o.items {
        count += item.Quantity
    }
    return count
}
```

## Usage Example

```go
// Create a new order
items := []OrderItem{
    {ProductID: "prod-1", Quantity: 2, Price: Money{Amount: 1000, Currency: "USD"}},
    {ProductID: "prod-2", Quantity: 1, Price: Money{Amount: 2500, Currency: "USD"}},
}

order, err := NewOrder("customer-123", items)
if err != nil {
    return err
}

// Modify the order
err = order.AddItem(OrderItem{
    ProductID: "prod-3", 
    Quantity: 1, 
    Price: Money{Amount: 500, Currency: "USD"},
})
if err != nil {
    return err
}

// Confirm the order
err = order.ConfirmOrder()
if err != nil {
    return err
}

// Save the order (this will persist the events)
err = orderRepository.Save(ctx, order)
if err != nil {
    return err
}
```

## Best Practices

### 1. Encapsulate Business Logic
- Keep all business rules inside the aggregate
- Don't expose internal state directly
- Use methods to modify state, not direct field access

### 2. Generate Meaningful Events
- Events should capture business intent, not just data changes
- Include relevant context in event data
- Use past tense for event names (OrderCreated, not CreateOrder)

### 3. Maintain Invariants
- Validate all state changes
- Ensure the aggregate is always in a valid state
- Use private methods to enforce consistency

### 4. Handle Event Sourcing Properly
- Implement `LoadFromHistory` to reconstruct state
- Apply events in the same order they were generated
- Don't include business logic in event application

### 5. Keep Aggregates Focused
- One aggregate should manage one consistency boundary
- Don't make aggregates too large or complex
- Use domain services for cross-aggregate logic

## Testing Your Aggregate

```go
func TestOrder_ConfirmOrder(t *testing.T) {
    // Arrange
    items := []OrderItem{
        {ProductID: "prod-1", Quantity: 1, Price: Money{Amount: 1000, Currency: "USD"}},
    }
    order, _ := NewOrder("customer-123", items)
    
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
    if len(events) != 2 { // OrderCreated + OrderConfirmed
        t.Errorf("Expected 2 events, got %d", len(events))
    }
    
    // Check the confirmation event
    confirmEvent, ok := events[1].(OrderConfirmedEvent)
    if !ok {
        t.Error("Expected OrderConfirmedEvent")
    }
    
    if confirmEvent.OrderID != order.ID() {
        t.Error("Event order ID should match aggregate ID")
    }
}
```

## Alternatives

### 1. Anemic Domain Model
If you don't need complex business logic, you might use a simpler data structure with external services handling the logic. However, this loses the benefits of encapsulation.

### 2. Active Record Pattern
You could combine data and persistence logic in the same class, but this violates separation of concerns and makes testing harder.

### 3. Transaction Script
For simple operations, you might use procedural code. This works for simple cases but doesn't scale well with complexity.

## Related Guides

- [Event Handlers](event-handlers.md) - Process the events generated by your aggregates
- [Testing Strategies](testing-strategies.md) - Comprehensive testing approaches
- [Performance Optimization](performance.md) - Optimize aggregate performance
- [Error Handling Patterns](error-handling.md) - Handle errors in domain logic

## Common Pitfalls

1. **Making aggregates too large** - Keep them focused on a single consistency boundary
2. **Exposing internal state** - Use methods, not public fields
3. **Forgetting to generate events** - Every state change should generate an event
4. **Complex event application** - Keep `applyEvent` simple and free of business logic
5. **Not validating invariants** - Always validate state changes