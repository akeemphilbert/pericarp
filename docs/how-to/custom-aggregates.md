# How to Implement Custom Aggregates

This guide shows you how to create custom domain aggregates using the standard Entity struct and StandardEvent API.

## Problem

You need to create a domain aggregate that:
- Encapsulates business rules and invariants
- Generates domain events when state changes
- Supports event sourcing for persistence
- Maintains consistency boundaries

## Solution

Follow these steps to implement a custom aggregate using the standard Entity struct:

### Step 1: Define the Aggregate Structure

```go
package domain

import (
    "errors"
    "time"
    "github.com/segmentio/ksuid"
    "github.com/akeemphilbert/pericarp/pkg/domain"
)

// Order represents an e-commerce order aggregate
type Order struct {
    *domain.Entity  // Embed the standard Entity struct
    customerID      string
    items           []OrderItem
    status          OrderStatus
    totalAmount     Money
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
        return nil, errors.New("customer GetID is required")
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
    order := &Order{
        Entity:      new(domain.Entity).WithID(orderID), // Use standard Entity
        customerID:  customerID,
        items:       items,
        status:      OrderStatusPending,
        totalAmount: Money{
            Amount:   totalAmount,
            Currency: items[0].Price.Currency, // Assume same currency
        },
    }
    
    // Generate domain event using EntityEvent
    order.AddEvent(domain.NewEntityEvent("Order", "created", orderID, "", "", order))
    
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
    
    // Generate EntityEvent
    o.AddEvent(domain.NewEntityEvent("Order", "confirmed", o.ID(), "", "", map[string]interface{}{
        "confirmed_at": time.Now(),
        "status":       string(OrderStatusConfirmed),
    }))
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
            oldQuantity := o.items[i].Quantity
            o.items[i].Quantity += item.Quantity
            o.recalculateTotal()
            
            o.AddEvent(domain.NewEntityEvent("Order", "item_updated", o.ID(), "", "", map[string]interface{}{
                "product_id":    item.ProductID,
                "old_quantity":  oldQuantity,
                "new_quantity":  o.items[i].Quantity,
                "updated_at":    time.Now(),
            }))
            return nil
        }
    }
    
    // Add new item
    o.items = append(o.items, item)
    o.recalculateTotal()
    
    o.AddEvent(domain.NewEntityEvent("Order", "item_added", o.ID(), "", "", map[string]interface{}{
        "product_id": item.ProductID,
        "quantity":   item.Quantity,
        "price":      item.Price,
        "added_at":   time.Now(),
    }))
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
    
    oldStatus := o.status
    o.status = OrderStatusCancelled
    
    o.AddEvent(domain.NewEntityEvent("Order", "cancelled", o.ID(), "", "", map[string]interface{}{
        "reason":       reason,
        "old_status":   string(oldStatus),
        "new_status":   string(OrderStatusCancelled),
        "cancelled_at": time.Now(),
    }))
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
```

### Step 4: Implement Event Sourcing Support

Since we're using the standard Entity struct, we only need to implement the event application logic:

```go
// LoadFromHistory reconstructs the aggregate from events
func (o *Order) LoadFromHistory(events []domain.Event) {
    for _, event := range events {
        o.applyEvent(event)
    }
    // Call the Entity's LoadFromHistory to handle version and sequence
    o.Entity.LoadFromHistory(events)
}

// applyEvent applies a single event to the aggregate
func (o *Order) applyEvent(event domain.Event) {
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
        o.status = OrderStatus(data["status"].(string))
        // Note: In a real implementation, you'd need to properly deserialize items and totalAmount
        
    case "Confirmed":
        o.status = OrderStatusConfirmed
        
    case "ItemAdded":
        // In a real implementation, deserialize the item from data
        productID := data["product_id"].(string)
        quantity := int(data["quantity"].(float64))
        // Add item logic here
        
    case "ItemUpdated":
        productID := data["product_id"].(string)
        newQuantity := int(data["new_quantity"].(float64))
        for i, item := range o.items {
            if item.ProductID == productID {
                o.items[i].Quantity = newQuantity
                break
            }
        }
        o.recalculateTotal()
        
    case "Cancelled":
        o.status = OrderStatusCancelled
    }
}
```

### Step 5: No Need for Custom Event Types!

With the StandardEvent approach, you don't need to define custom event types. All events are created using `domain.NewEvent()` with flexible data structures. This eliminates boilerplate code and makes your aggregates more maintainable.

The events are automatically structured as:
- **Event Type**: `EntityType.ActionType` (e.g., "Order.Created", "Order.Confirmed")
- **Data**: Flexible map containing all relevant information
- **Metadata**: Additional context like correlation IDs, user information, etc.

Example events generated by our Order aggregate:
```json
{
  "event_type": "Order.Created",
  "aggregate_id": "order-123",
  "entity_type": "Order",
  "event_action": "Created",
  "data": {
    "customer_id": "customer-456",
    "items": [...],
    "total_amount": {...},
    "status": "pending",
    "created_at": "2023-12-01T10:00:00Z"
  },
  "version": 1,
  "occurred_at": "2023-12-01T10:00:00Z"
}
```

### Step 5: Add Getter Methods

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

// Note: GetID(), GetSequenceNo(), UncommittedEvents(), MarkEventsAsCommitted() 
// are inherited from the embedded Entity struct
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

// Check the generated events
events := order.UncommittedEvents()
fmt.Printf("Generated %d events:\n", len(events))
for _, event := range events {
    fmt.Printf("- %s\n", event.EventType())
}
// Output:
// Generated 3 events:
// - Order.Created
// - Order.ItemAdded  
// - Order.Confirmed

// Save the order (this will persist the events)
err = orderRepository.Save(ctx, order)
if err != nil {
    return err
}
```

## Best Practices

### 1. Use the Standard Entity Struct
- Embed `domain.Entity` to get event management for free
- Don't reimplement ID, version, or event handling
- Focus on your business logic, not infrastructure

### 2. Use StandardEvent for Flexibility
- Use `domain.NewEvent()` for all events - no custom event types needed
- Include all relevant data in the event data map
- Use clear, descriptive action names ("Created", "Confirmed", "Cancelled")

### 3. Encapsulate Business Logic
- Keep all business rules inside the aggregate
- Don't expose internal state directly
- Use methods to modify state, not direct field access

### 4. Generate Meaningful Events
- Events should capture business intent, not just data changes
- Include relevant context in event data
- Use past tense for event action names (Created, not Create)

### 5. Maintain Invariants
- Validate all state changes
- Ensure the aggregate is always in a valid state
- Use private methods to enforce consistency

### 6. Handle Event Sourcing Properly
- Implement `LoadFromHistory` to reconstruct state
- Apply events in the same order they were generated
- Don't include business logic in event application
- Call `Entity.LoadFromHistory()` to handle version/sequence

### 7. Keep Aggregates Focused
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
        t.Error("Event aggregate GetID should match order GetID")
    }
    
    // Check event data
    data := confirmEvent.Data().(map[string]interface{})
    if data["status"] != "confirmed" {
        t.Errorf("Expected status 'confirmed' in event data, got %v", data["status"])
    }
}

func TestOrder_LoadFromHistory(t *testing.T) {
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
    order := &Order{Entity: domain.NewEntity(orderID)}
    order.LoadFromHistory(events)
    
    // Assert
    if order.ID() != orderID {
        t.Errorf("Expected GetID %s, got %s", orderID, order.ID())
    }
    
    if order.Status() != OrderStatusConfirmed {
        t.Errorf("Expected status confirmed, got %v", order.Status())
    }
    
    if order.SequenceNo() != 2 {
        t.Errorf("Expected sequence number 2, got %d", order.SequenceNo())
    }
    
    // Should have no uncommitted events after loading from history
    if len(order.UncommittedEvents()) != 0 {
        t.Errorf("Expected no uncommitted events, got %d", len(order.UncommittedEvents()))
    }
}
```

## Key Benefits of This Approach

### 1. Less Boilerplate
- No need to implement aggregate root interface methods
- No custom event types to define and maintain
- Entity struct handles all event management automatically

### 2. Consistent Event Format
- All events follow the same `EntityType.ActionType` pattern
- Flexible data structure accommodates any business data
- Built-in JSON serialization and metadata support

### 3. Easy Testing
- StandardEvent makes it easy to verify event data
- Entity provides consistent behavior across all aggregates
- Clear separation between business logic and infrastructure

### 4. Maintainable Code
- Focus on business logic, not infrastructure concerns
- Single factory function for all events
- Embedded Entity provides proven, tested functionality

## Alternatives

### 1. Custom Event Types (Not Recommended)
You could define specific event types for each business event, but this creates unnecessary boilerplate and maintenance overhead.

### 2. Manual Event Management (Not Recommended)
You could implement your own event handling instead of using Entity, but you'd lose the benefits of the tested, optimized implementation.

### 3. Anemic Domain Model
If you don't need complex business logic, you might use simpler data structures, but this loses the benefits of encapsulation and domain modeling.

## Related Guides

- [Event Handlers](event-handlers.md) - Process the events generated by your aggregates
- [Testing Strategies](testing-strategies.md) - Comprehensive testing approaches
- [Performance Optimization](performance.md) - Optimize aggregate performance
- [Error Handling Patterns](error-handling.md) - Handle errors in domain logic

## Common Pitfalls

1. **Not using the standard Entity** - Don't reimplement event management yourself
2. **Creating custom event types** - Use StandardEvent instead of defining specific event classes
3. **Making aggregates too large** - Keep them focused on a single consistency boundary
4. **Exposing internal state** - Use methods, not public fields
5. **Forgetting to generate events** - Every state change should generate an event
6. **Complex event application** - Keep `applyEvent` simple and free of business logic
7. **Not validating invariants** - Always validate state changes
8. **Forgetting to call Entity.LoadFromHistory()** - This handles version and sequence management