# Domain-Driven Design in Pericarp

Domain-Driven Design (DDD) is the foundational philosophy behind Pericarp's architecture. This document explains how DDD principles are implemented and why they matter for building maintainable software.

## What is Domain-Driven Design?

Domain-Driven Design is an approach to software development that focuses on:

1. **Understanding the business domain** - The core problem your software solves
2. **Creating a shared language** - Terms that both developers and domain experts understand
3. **Modeling the domain** - Representing business concepts in code
4. **Organizing code around the domain** - Structure that reflects business reality

## DDD in Pericarp

Pericarp implements DDD through several key patterns and principles:

### Strategic Design

#### Bounded Contexts
While Pericarp doesn't enforce bounded contexts directly, it provides the tools to implement them:

```go
// Each bounded context can have its own module
package usercontext

// Domain model specific to user management
type User struct {
    // User-specific logic
}

package ordercontext

// Domain model specific to order management  
type Order struct {
    // Order-specific logic
}
```

#### Ubiquitous Language
Pericarp encourages using domain terminology in code:

```go
// Good - uses domain language
type Order struct {
    id     OrderID
    status OrderStatus
}

func (o *Order) ConfirmOrder() error {
    // Business logic using domain terms
}

// Avoid - technical language that doesn't reflect the domain
type OrderRecord struct {
    pk     int
    status_code int
}

func (o *OrderRecord) UpdateStatus(code int) error {
    // Generic technical operation
}
```

### Tactical Design

#### Aggregates
Pericarp's aggregate pattern ensures consistency and encapsulation:

```go
type Order struct {
    // Private fields - encapsulation
    id          string
    customerID  string
    items       []OrderItem
    status      OrderStatus
    version     int
    events      []domain.Event
}

// Public methods that enforce business rules
func (o *Order) AddItem(item OrderItem) error {
    // Business rule: can only add items to pending orders
    if o.status != OrderStatusPending {
        return errors.New("cannot add items to confirmed order")
    }
    
    // Business logic
    o.items = append(o.items, item)
    o.recalculateTotal()
    
    // Generate domain event
    event := OrderItemAddedEvent{...}
    o.addEvent(event)
    
    return nil
}
```

**Key Aggregate Principles in Pericarp:**

1. **Consistency Boundary** - Each aggregate maintains its own consistency
2. **Identity** - Each aggregate has a unique identifier
3. **Encapsulation** - Internal state is private, modified through methods
4. **Event Generation** - State changes generate domain events

#### Entities vs Value Objects

**Entities** have identity and lifecycle:
```go
type User struct {
    id       UserID    // Identity
    email    Email     // Can change over time
    name     string    // Can change over time
    version  int       // Tracks changes
}

func (u *User) ID() UserID { return u.id }
func (u *User) ChangeEmail(newEmail Email) error {
    // Business logic for email change
}
```

**Value Objects** are immutable and defined by their attributes:
```go
type Email struct {
    value string
}

func NewEmail(email string) (Email, error) {
    if !isValidEmail(email) {
        return Email{}, errors.New("invalid email format")
    }
    return Email{value: email}, nil
}

func (e Email) String() string { return e.value }

// Value objects are immutable - no setter methods
```

#### Domain Services
For logic that doesn't belong to a single aggregate:

```go
type PricingService struct {
    discountRepo DiscountRepository
    taxRepo      TaxRepository
}

func (s *PricingService) CalculateOrderTotal(order *Order, customerType CustomerType) (Money, error) {
    // Cross-aggregate business logic
    baseTotal := order.BaseTotal()
    
    discount, err := s.discountRepo.GetDiscountFor(customerType)
    if err != nil {
        return Money{}, err
    }
    
    tax, err := s.taxRepo.GetTaxRate(order.ShippingAddress())
    if err != nil {
        return Money{}, err
    }
    
    return baseTotal.ApplyDiscount(discount).ApplyTax(tax), nil
}
```

#### Repositories
Abstract data access with domain-focused interfaces:

```go
// Domain-focused repository interface
type OrderRepository interface {
    Save(ctx context.Context, order *Order) error
    Load(ctx context.Context, id OrderID) (*Order, error)
    FindByCustomer(ctx context.Context, customerID CustomerID) ([]*Order, error)
}

// Implementation is in infrastructure layer
type OrderEventSourcingRepository struct {
    eventStore domain.EventStore
    uow        domain.UnitOfWork
}
```

## Benefits of DDD in Pericarp

### 1. Business Alignment
Code structure reflects business structure:

```go
// Business concept: "An order can be confirmed if it's pending"
func (o *Order) ConfirmOrder() error {
    if o.status != OrderStatusPending {
        return errors.New("only pending orders can be confirmed")
    }
    // ... rest of logic
}
```

### 2. Maintainability
Changes to business rules are localized:

```go
// All order confirmation logic is in one place
func (o *Order) ConfirmOrder() error {
    // Easy to modify business rules here
    if !o.hasValidItems() {
        return errors.New("cannot confirm order without valid items")
    }
    
    if !o.hasValidShippingAddress() {
        return errors.New("cannot confirm order without shipping address")
    }
    
    // ... confirmation logic
}
```

### 3. Testability
Domain logic can be tested in isolation:

```go
func TestOrder_ConfirmOrder_WithInvalidItems_ShouldFail(t *testing.T) {
    // Arrange - pure domain objects, no infrastructure
    order := NewOrder(customerID, invalidItems)
    
    // Act - pure business logic
    err := order.ConfirmOrder()
    
    // Assert - business rule verification
    if err == nil {
        t.Error("Expected error when confirming order with invalid items")
    }
}
```

### 4. Evolution
Domain model can evolve independently:

```go
// V1: Simple order confirmation
func (o *Order) ConfirmOrder() error {
    o.status = OrderStatusConfirmed
    return nil
}

// V2: Add business rules
func (o *Order) ConfirmOrder() error {
    if o.totalAmount.IsZero() {
        return errors.New("cannot confirm empty order")
    }
    o.status = OrderStatusConfirmed
    return nil
}

// V3: Add more complex logic
func (o *Order) ConfirmOrder() error {
    if err := o.validateForConfirmation(); err != nil {
        return err
    }
    
    if err := o.reserveInventory(); err != nil {
        return err
    }
    
    o.status = OrderStatusConfirmed
    o.addEvent(OrderConfirmedEvent{...})
    return nil
}
```

## DDD Anti-Patterns to Avoid

### 1. Anemic Domain Model
```go
// Bad - no business logic in domain objects
type Order struct {
    ID     string
    Status string
    Items  []OrderItem
}

// Business logic in services instead of domain objects
type OrderService struct{}

func (s *OrderService) ConfirmOrder(order *Order) error {
    if order.Status != "pending" {
        return errors.New("invalid status")
    }
    order.Status = "confirmed"
    return nil
}
```

```go
// Good - business logic in domain objects
type Order struct {
    id     string
    status OrderStatus
    items  []OrderItem
}

func (o *Order) ConfirmOrder() error {
    if o.status != OrderStatusPending {
        return errors.New("only pending orders can be confirmed")
    }
    o.status = OrderStatusConfirmed
    return nil
}
```

### 2. Exposing Internal State
```go
// Bad - exposes internal state
type Order struct {
    ID     string
    Status OrderStatus
    Items  []OrderItem  // Public field
}

// Anyone can modify items directly
order.Items = append(order.Items, invalidItem)
```

```go
// Good - encapsulates state
type Order struct {
    id     string
    status OrderStatus
    items  []OrderItem  // Private field
}

func (o *Order) AddItem(item OrderItem) error {
    // Validation and business rules
    if !item.IsValid() {
        return errors.New("invalid item")
    }
    o.items = append(o.items, item)
    return nil
}

func (o *Order) Items() []OrderItem {
    // Return copy to prevent external modification
    return append([]OrderItem{}, o.items...)
}
```

### 3. Technology Leakage
```go
// Bad - domain depends on infrastructure
type Order struct {
    ID string `gorm:"primaryKey"`  // Database concern in domain
}

func (o *Order) Save() error {
    // Database logic in domain
    return db.Save(o)
}
```

```go
// Good - domain is technology-agnostic
type Order struct {
    id string  // Pure domain concept
}

// Repository handles persistence
type OrderRepository interface {
    Save(ctx context.Context, order *Order) error
}
```

## DDD and Event Sourcing

Pericarp combines DDD with Event Sourcing naturally:

### Events as Domain Concepts
```go
// Events represent business facts
type OrderConfirmedEvent struct {
    OrderID     string
    ConfirmedAt time.Time
    ConfirmedBy UserID
}

// Not technical events
type OrderStatusChangedEvent struct {
    OrderID   string
    OldStatus string
    NewStatus string
}
```

### Aggregate Reconstruction
```go
func (o *Order) LoadFromHistory(events []domain.Event) {
    for _, event := range events {
        switch e := event.(type) {
        case OrderCreatedEvent:
            // Apply business meaning of event
            o.id = e.OrderID
            o.customerID = e.CustomerID
            o.status = OrderStatusPending
            
        case OrderConfirmedEvent:
            // Apply business meaning of event
            o.status = OrderStatusConfirmed
            o.confirmedAt = e.ConfirmedAt
        }
    }
}
```

## Practical Guidelines

### 1. Start with the Domain
- Begin by understanding the business problem
- Identify key concepts and their relationships
- Model behavior, not just data

### 2. Use Domain Language
- Name classes, methods, and variables using business terms
- Avoid technical jargon in domain code
- Create a glossary of domain terms

### 3. Keep Domain Pure
- No infrastructure dependencies in domain layer
- No framework-specific annotations
- Focus on business logic only

### 4. Test Domain Logic
- Write tests that verify business rules
- Use domain language in test names
- Test edge cases and invariants

### 5. Evolve Gradually
- Start simple and add complexity as needed
- Refactor when you learn more about the domain
- Don't over-engineer early

## Conclusion

DDD in Pericarp provides:

- **Clear separation** between business logic and technical concerns
- **Maintainable code** that reflects business reality
- **Testable components** that can evolve independently
- **Shared understanding** between developers and domain experts

The key is to focus on the business domain first, then use Pericarp's patterns to implement that understanding in code. The technology serves the domain, not the other way around.