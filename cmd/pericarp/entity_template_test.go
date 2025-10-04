package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityTemplate_Generation(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	tests := []struct {
		name     string
		entity   Entity
		expected []string // Expected strings in the generated code
	}{
		{
			name: "simple entity with required fields",
			entity: Entity{
				Name: "User",
				Properties: []Property{
					{
						Name:     "id",
						Type:     "uuid",
						Required: true,
					},
					{
						Name:     "email",
						Type:     "string",
						Required: true,
					},
					{
						Name:     "name",
						Type:     "string",
						Required: false,
					},
				},
			},
			expected: []string{
				"type User struct",
				"Id ksuid.KSUID",
				"Email string",
				"Name string",
				"func NewUser(id ksuid.KSUID, email string) (*User, error)",
				"func (u *User) GetID() string",
				"func (u *User) Version() int",
				"func (u *User) UncommittedEvents() []domain.Event",
				"func (u *User) MarkEventsAsCommitted()",
				"func (u *User) LoadFromHistory(events []domain.Event)",
				"func (u *User) SetName(name string)",
				"func (u *User) recordEvent(event domain.Event)",
				"func (u *User) applyEvent(event domain.Event)",
			},
		},
		{
			name: "entity with validation tags",
			entity: Entity{
				Name: "Product",
				Properties: []Property{
					{
						Name:     "id",
						Type:     "uuid",
						Required: true,
					},
					{
						Name:       "name",
						Type:       "string",
						Required:   true,
						Validation: "min=1,max=100",
					},
					{
						Name:       "price",
						Type:       "float64",
						Required:   true,
						Validation: "min=0",
					},
					{
						Name:     "description",
						Type:     "string",
						Required: false,
					},
				},
			},
			expected: []string{
				"type Product struct",
				"Id ksuid.KSUID",
				"Name string `json:\"name\" validate:\"required,min=1,max=100\"`",
				"Price float64 `json:\"price\" validate:\"required,min=0\"`",
				"Description string `json:\"description,omitempty\"`",
				"func NewProduct(id ksuid.KSUID, name string, price float64) (*Product, error)",
				"func (p *Product) SetDescription(description string)",
			},
		},
		{
			name: "entity with methods",
			entity: Entity{
				Name: "Order",
				Properties: []Property{
					{
						Name:     "id",
						Type:     "uuid",
						Required: true,
					},
					{
						Name:     "status",
						Type:     "string",
						Required: true,
					},
				},
				Methods: []Method{
					{
						Name:        "Complete",
						Description: "marks the order as completed",
						Parameters: []Parameter{
							{Name: "completedAt", Type: "time.Time"},
						},
						ReturnType: "error",
					},
					{
						Name:        "Cancel",
						Description: "cancels the order",
						ReturnType:  "error",
					},
				},
			},
			expected: []string{
				"type Order struct",
				"func NewOrder(id ksuid.KSUID, status string) (*Order, error)",
				"func (o *Order) Complete(completedAt time.Time) error",
				"func (o *Order) Cancel() error",
				"// TODO: Implement Complete method",
				"// TODO: Implement Cancel method",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Execute("entity.go", tt.entity)
			require.NoError(t, err)
			assert.NotEmpty(t, result)

			// Check that all expected strings are present
			for _, expected := range tt.expected {
				assert.Contains(t, result, expected, "Expected string not found: %s", expected)
			}

			// Verify the generated code has proper Go syntax structure
			assert.Contains(t, result, "package domain")
			assert.Contains(t, result, "import (")
			assert.Contains(t, result, "github.com/segmentio/ksuid")
			assert.Contains(t, result, "github.com/akeemphilbert/pericarp/pkg/domain")

			// Verify aggregate root pattern implementation
			assert.Contains(t, result, "version           int")
			assert.Contains(t, result, "uncommittedEvents []domain.Event")
		})
	}
}

func TestEntityTemplate_AggregateRootPatterns(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "Customer",
		Properties: []Property{
			{
				Name:     "id",
				Type:     "uuid",
				Required: true,
			},
			{
				Name:     "email",
				Type:     "string",
				Required: true,
			},
		},
	}

	result, err := engine.Execute("entity.go", entity)
	require.NoError(t, err)

	// Test aggregate root patterns (Requirement 8.2, 8.5)
	aggregatePatterns := []string{
		// Aggregate root fields
		"version           int",
		"uncommittedEvents []domain.Event",

		// Required aggregate methods
		"func (c *Customer) GetID() string",
		"func (c *Customer) Version() int",
		"func (c *Customer) UncommittedEvents() []domain.Event",
		"func (c *Customer) MarkEventsAsCommitted()",
		"func (c *Customer) LoadFromHistory(events []domain.Event)",

		// Event handling
		"func (c *Customer) recordEvent(event domain.Event)",
		"func (c *Customer) applyEvent(event domain.Event)",

		// Event creation in constructor
		"event := NewCustomerCreatedEvent(id, email)",
		"customer.recordEvent(event)",
	}

	for _, pattern := range aggregatePatterns {
		assert.Contains(t, result, pattern, "Aggregate root pattern not found: %s", pattern)
	}
}

func TestEntityTemplate_ValidationTags(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "Account",
		Properties: []Property{
			{
				Name:     "id",
				Type:     "uuid",
				Required: true,
			},
			{
				Name:       "username",
				Type:       "string",
				Required:   true,
				Validation: "min=3,max=50",
			},
			{
				Name:       "email",
				Type:       "string",
				Required:   true,
				Validation: "email",
			},
			{
				Name:     "bio",
				Type:     "string",
				Required: false,
			},
		},
	}

	result, err := engine.Execute("entity.go", entity)
	require.NoError(t, err)

	// Test validation tags (Requirement 8.2)
	validationTests := []string{
		"Id ksuid.KSUID `json:\"id\"`",
		"Username string `json:\"username\" validate:\"required,min=3,max=50\"`",
		"Email string `json:\"email\" validate:\"required,email\"`",
		"Bio string `json:\"bio,omitempty\"`",
	}

	for _, validation := range validationTests {
		assert.Contains(t, result, validation, "Validation tag not found: %s", validation)
	}
}

func TestEntityTemplate_EventRecording(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "Invoice",
		Properties: []Property{
			{
				Name:     "id",
				Type:     "uuid",
				Required: true,
			},
			{
				Name:     "amount",
				Type:     "float64",
				Required: true,
			},
			{
				Name:     "status",
				Type:     "string",
				Required: false,
			},
		},
	}

	result, err := engine.Execute("entity.go", entity)
	require.NoError(t, err)

	// Test event recording patterns (Requirement 8.5)
	eventPatterns := []string{
		// Creation event
		"event := NewInvoiceCreatedEvent(id, amount)",
		"invoice.recordEvent(event)",

		// Update events for optional fields
		"func (i *Invoice) SetStatus(status string)",
		"event := NewInvoiceStatusUpdatedEvent(i.Id, status)",

		// Event application
		"case *InvoiceCreatedEvent:",
		"i.Id = e.AggregateID",
		"case *InvoiceStatusUpdatedEvent:",
		"i.Status = e.Status",
	}

	for _, pattern := range eventPatterns {
		assert.Contains(t, result, pattern, "Event recording pattern not found: %s", pattern)
	}
}

func TestEntityTemplate_MethodGeneration(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "Subscription",
		Properties: []Property{
			{
				Name:     "id",
				Type:     "uuid",
				Required: true,
			},
		},
		Methods: []Method{
			{
				Name:        "Activate",
				Description: "activates the subscription",
				Parameters: []Parameter{
					{Name: "activatedAt", Type: "time.Time"},
					{Name: "plan", Type: "string"},
				},
				ReturnType:     "error",
				Implementation: "// Custom activation logic\nreturn nil",
			},
			{
				Name:        "IsActive",
				Description: "checks if subscription is active",
				ReturnType:  "bool",
			},
		},
	}

	result, err := engine.Execute("entity.go", entity)
	require.NoError(t, err)

	// Test method generation (Requirement 3.6)
	methodPatterns := []string{
		"// Activate activates the subscription",
		"func (s *Subscription) Activate(activatedAt time.Time, plan string) error {",
		"// Custom activation logic",
		"return nil",

		"// IsActive checks if subscription is active",
		"func (s *Subscription) IsActive() bool {",
		"// TODO: Implement IsActive method",
		"panic(\"not implemented\")",
	}

	for _, pattern := range methodPatterns {
		assert.Contains(t, result, pattern, "Method generation pattern not found: %s", pattern)
	}
}

func TestEntityTemplate_ConstructorValidation(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "Document",
		Properties: []Property{
			{
				Name:     "id",
				Type:     "uuid",
				Required: true,
			},
			{
				Name:     "title",
				Type:     "string",
				Required: true,
			},
			{
				Name:     "content",
				Type:     "string",
				Required: true,
			},
			{
				Name:     "tags",
				Type:     "[]string",
				Required: false,
			},
		},
	}

	result, err := engine.Execute("entity.go", entity)
	require.NoError(t, err)

	// Test constructor validation
	constructorPatterns := []string{
		"func NewDocument(id ksuid.KSUID, title string, content string) (*Document, error)",
		"if id == ksuid.KSUID{} {",
		"return nil, errors.New(\"id cannot be empty\")",
		"if title == \"\" {",
		"return nil, errors.New(\"title cannot be empty\")",
		"if content == \"\" {",
		"return nil, errors.New(\"content cannot be empty\")",
	}

	for _, pattern := range constructorPatterns {
		assert.Contains(t, result, pattern, "Constructor validation pattern not found: %s", pattern)
	}
}

func TestEntityEventsTemplate_Generation(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "Task",
		Properties: []Property{
			{
				Name:     "id",
				Type:     "uuid",
				Required: true,
			},
			{
				Name:     "title",
				Type:     "string",
				Required: true,
			},
			{
				Name:     "completed",
				Type:     "bool",
				Required: false,
			},
		},
	}

	result, err := engine.Execute("entity_events.go", entity)
	require.NoError(t, err)

	// Test event generation (Requirement 8.5)
	eventPatterns := []string{
		"package domain",
		"github.com/akeemphilbert/pericarp/pkg/domain",

		// Created event
		"type TaskCreatedEvent struct",
		"*domain.EntityEvent",
		"func NewTaskCreatedEvent(aggregateID ksuid.KSUID, id ksuid.KSUID, title string) *TaskCreatedEvent",

		// Update events for optional fields
		"type TaskCompletedUpdatedEvent struct",
		"func NewTaskCompletedUpdatedEvent(aggregateID ksuid.KSUID, completed bool) *TaskCompletedUpdatedEvent",
	}

	for _, pattern := range eventPatterns {
		assert.Contains(t, result, pattern, "Event generation pattern not found: %s", pattern)
	}
}

func TestEntityTemplate_ComplexTypes(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "Profile",
		Properties: []Property{
			{
				Name:     "id",
				Type:     "uuid",
				Required: true,
			},
			{
				Name:     "createdAt",
				Type:     "time",
				Required: true,
			},
			{
				Name:     "metadata",
				Type:     "map[string]interface{}",
				Required: false,
			},
			{
				Name:     "tags",
				Type:     "[]string",
				Required: false,
			},
		},
	}

	result, err := engine.Execute("entity.go", entity)
	require.NoError(t, err)

	// Test complex type handling
	typePatterns := []string{
		"Id ksuid.KSUID",
		"Createdat time.Time",
		"Metadata map[string]interface{} `json:\"metadata,omitempty\"`",
		"Tags []string `json:\"tags,omitempty\"`",
		"func NewProfile(id ksuid.KSUID, createdAt time.Time) (*Profile, error)",
		"if createdAt == time.Time{} {",
		"Metadata: nil,",
		"Tags: nil,",
	}

	for _, pattern := range typePatterns {
		assert.Contains(t, result, pattern, "Complex type pattern not found: %s", pattern)
	}
}
