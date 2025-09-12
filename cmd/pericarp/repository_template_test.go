package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryInterfaceTemplate_Generation(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	tests := []struct {
		name     string
		entity   Entity
		expected []string
	}{
		{
			name: "basic repository interface",
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
				"package domain",
				"type UserRepository interface",
				"Save(ctx context.Context, user *User) error",
				"Load(ctx context.Context, id ksuid.KSUID) (*User, error)",
				"Delete(ctx context.Context, id ksuid.KSUID) error",
				"Exists(ctx context.Context, id ksuid.KSUID) (bool, error)",
				"FindByEmail(ctx context.Context, email string) ([]*User, error)",
				"FindByName(ctx context.Context, name string) ([]*User, error)",
				"FindAll(ctx context.Context, limit, offset int) ([]*User, error)",
				"Count(ctx context.Context) (int64, error)",
			},
		},
		{
			name: "repository with no string fields",
			entity: Entity{
				Name: "Order",
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
						Name:     "quantity",
						Type:     "int",
						Required: false,
					},
				},
			},
			expected: []string{
				"package domain",
				"type OrderRepository interface",
				"Save(ctx context.Context, order *Order) error",
				"Load(ctx context.Context, id ksuid.KSUID) (*Order, error)",
				"Delete(ctx context.Context, id ksuid.KSUID) error",
				"Exists(ctx context.Context, id ksuid.KSUID) (bool, error)",
				"FindAll(ctx context.Context, limit, offset int) ([]*Order, error)",
				"Count(ctx context.Context) (int64, error)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Execute("repository_interface.go", tt.entity)
			require.NoError(t, err)
			assert.NotEmpty(t, result)

			// Check that all expected strings are present
			for _, expected := range tt.expected {
				assert.Contains(t, result, expected, "Expected string not found: %s", expected)
			}

			// Verify proper Go syntax structure
			assert.Contains(t, result, "import (")
			assert.Contains(t, result, "context")
			assert.Contains(t, result, "github.com/segmentio/ksuid")
		})
	}
}

func TestRepositoryImplementationTemplate_Generation(t *testing.T) {
	logger := NewTestLogger()
	engine, err := NewTemplateEngine(logger)
	require.NoError(t, err)

	entity := Entity{
		Name: "Product",
		Properties: []Property{
			{
				Name:     "id",
				Type:     "uuid",
				Required: true,
			},
			{
				Name:     "name",
				Type:     "string",
				Required: true,
			},
			{
				Name:     "price",
				Type:     "float64",
				Required: true,
			},
		},
	}

	// Add ProjectName to the entity for template context
	templateData := struct {
		Entity
		ProjectName string
	}{
		Entity:      entity,
		ProjectName: "test-project",
	}

	result, err := engine.Execute("repository_implementation.go", templateData)
	require.NoError(t, err)

	// Test implementation patterns (Requirement 8.3, 3.7)
	implementationPatterns := []string{
		"package infrastructure",
		"type ProductEventSourcingRepository struct",
		"eventStore domain.EventStore",
		"logger     domain.Logger",
		"func NewProductEventSourcingRepository(eventStore domain.EventStore, logger domain.Logger) *ProductEventSourcingRepository",

		// Save method
		"func (r *ProductEventSourcingRepository) Save(ctx context.Context, product *appDomain.Product) error",
		"if product == nil {",
		"return fmt.Errorf(\"product cannot be nil\")",
		"events := product.UncommittedEvents()",
		"r.eventStore.Save(ctx, events)",
		"product.MarkEventsAsCommitted()",

		// Load method
		"func (r *ProductEventSourcingRepository) Load(ctx context.Context, id ksuid.KSUID) (*appDomain.Product, error)",
		"envelopes, err := r.eventStore.Load(ctx, aggregateID)",
		"product := &appDomain.Product{}",
		"product.LoadFromHistory(events)",

		// Other methods
		"func (r *ProductEventSourcingRepository) Delete(ctx context.Context, id ksuid.KSUID) error",
		"func (r *ProductEventSourcingRepository) Exists(ctx context.Context, id ksuid.KSUID) (bool, error)",
		"func (r *ProductEventSourcingRepository) FindByName(ctx context.Context, name string) ([]*appDomain.Product, error)",
		"func (r *ProductEventSourcingRepository) FindAll(ctx context.Context, limit, offset int) ([]*appDomain.Product, error)",
		"func (r *ProductEventSourcingRepository) Count(ctx context.Context) (int64, error)",
	}

	for _, pattern := range implementationPatterns {
		assert.Contains(t, result, pattern, "Implementation pattern not found: %s", pattern)
	}
}

func TestRepositoryTemplate_ErrorHandling(t *testing.T) {
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
				Name:     "username",
				Type:     "string",
				Required: true,
			},
		},
	}

	templateData := struct {
		Entity
		ProjectName string
	}{
		Entity:      entity,
		ProjectName: "banking-app",
	}

	result, err := engine.Execute("repository_implementation.go", templateData)
	require.NoError(t, err)

	// Test error handling patterns (Requirement 8.3)
	errorPatterns := []string{
		// Nil check
		"if account == nil {",
		"return fmt.Errorf(\"account cannot be nil\")",

		// Event store errors
		"if _, err := r.eventStore.Save(ctx, events); err != nil {",
		"return fmt.Errorf(\"failed to save events for account %s: %w\", account.ID(), err)",

		// Load errors
		"if err != nil {",
		"return nil, fmt.Errorf(\"failed to load events for account %s: %w\", aggregateID, err)",

		// Not found errors
		"if len(envelopes) == 0 {",
		"return nil, fmt.Errorf(\"account not found: %s\", aggregateID)",

		// Existence check errors
		"return false, fmt.Errorf(\"failed to check existence of account %s: %w\", aggregateID, err)",
	}

	for _, pattern := range errorPatterns {
		assert.Contains(t, result, pattern, "Error handling pattern not found: %s", pattern)
	}
}

func TestRepositoryTemplate_ContextUsage(t *testing.T) {
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
		},
	}

	// Test interface template
	interfaceResult, err := engine.Execute("repository_interface.go", entity)
	require.NoError(t, err)

	// Test implementation template
	templateData := struct {
		Entity
		ProjectName string
	}{
		Entity:      entity,
		ProjectName: "doc-system",
	}

	implResult, err := engine.Execute("repository_implementation.go", templateData)
	require.NoError(t, err)

	// Test context usage patterns (Requirement 8.3)
	contextPatterns := []string{
		// Interface methods with context
		"Save(ctx context.Context, document *Document) error",
		"Load(ctx context.Context, id ksuid.KSUID) (*Document, error)",
		"Delete(ctx context.Context, id ksuid.KSUID) error",
		"Exists(ctx context.Context, id ksuid.KSUID) (bool, error)",
		"FindAll(ctx context.Context, limit, offset int) ([]*Document, error)",
		"Count(ctx context.Context) (int64, error)",
	}

	for _, pattern := range contextPatterns {
		assert.Contains(t, interfaceResult, pattern, "Context pattern not found in interface: %s", pattern)
	}

	// Implementation context usage
	implContextPatterns := []string{
		"r.eventStore.Save(ctx, events)",
		"r.eventStore.Load(ctx, aggregateID)",
	}

	for _, pattern := range implContextPatterns {
		assert.Contains(t, implResult, pattern, "Context pattern not found in implementation: %s", pattern)
	}
}

func TestRepositoryTemplate_CRUDOperations(t *testing.T) {
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
				Name:     "status",
				Type:     "string",
				Required: false,
			},
		},
	}

	// Test interface
	interfaceResult, err := engine.Execute("repository_interface.go", entity)
	require.NoError(t, err)

	// Test CRUD operations in interface (Requirement 3.7)
	crudPatterns := []string{
		// Create/Update (Save)
		"Save(ctx context.Context, task *Task) error",

		// Read operations
		"Load(ctx context.Context, id ksuid.KSUID) (*Task, error)",
		"FindByTitle(ctx context.Context, title string) ([]*Task, error)",
		"FindByStatus(ctx context.Context, status string) ([]*Task, error)",
		"FindAll(ctx context.Context, limit, offset int) ([]*Task, error)",
		"Count(ctx context.Context) (int64, error)",

		// Delete
		"Delete(ctx context.Context, id ksuid.KSUID) error",

		// Utility
		"Exists(ctx context.Context, id ksuid.KSUID) (bool, error)",
	}

	for _, pattern := range crudPatterns {
		assert.Contains(t, interfaceResult, pattern, "CRUD pattern not found: %s", pattern)
	}
}

func TestRepositoryTemplate_QueryMethods(t *testing.T) {
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
			{
				Name:     "firstName",
				Type:     "string",
				Required: true,
			},
			{
				Name:     "lastName",
				Type:     "string",
				Required: false,
			},
			{
				Name:     "age",
				Type:     "int",
				Required: false,
			},
		},
	}

	result, err := engine.Execute("repository_interface.go", entity)
	require.NoError(t, err)

	// Test query method generation (Requirement 3.7)
	queryPatterns := []string{
		// Only string fields should have FindBy methods
		"FindByEmail(ctx context.Context, email string) ([]*Customer, error)",
		"FindByFirstname(ctx context.Context, firstName string) ([]*Customer, error)",
		"FindByLastname(ctx context.Context, lastName string) ([]*Customer, error)",

		// Non-string fields should not have FindBy methods
		// (we test by ensuring they're not present)
	}

	for _, pattern := range queryPatterns {
		assert.Contains(t, result, pattern, "Query pattern not found: %s", pattern)
	}

	// Ensure non-string fields don't have FindBy methods
	nonStringPatterns := []string{
		"FindByAge(ctx context.Context, age int)",
	}

	for _, pattern := range nonStringPatterns {
		assert.NotContains(t, result, pattern, "Non-string field should not have FindBy method: %s", pattern)
	}
}

func TestRepositoryTemplate_EventSourcingPatterns(t *testing.T) {
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
				Name:     "number",
				Type:     "string",
				Required: true,
			},
		},
	}

	templateData := struct {
		Entity
		ProjectName string
	}{
		Entity:      entity,
		ProjectName: "billing-system",
	}

	result, err := engine.Execute("repository_implementation.go", templateData)
	require.NoError(t, err)

	// Test event sourcing patterns (Requirement 8.3)
	eventSourcingPatterns := []string{
		// Event store dependency
		"eventStore domain.EventStore",

		// Save with event sourcing
		"events := invoice.UncommittedEvents()",
		"if len(events) == 0 {",
		"r.logger.Debug(\"No uncommitted events to save for invoice %s\", invoice.ID())",
		"r.eventStore.Save(ctx, events)",
		"invoice.MarkEventsAsCommitted()",

		// Load with event replay
		"envelopes, err := r.eventStore.Load(ctx, aggregateID)",
		"invoice := &appDomain.Invoice{}",
		"invoice.LoadFromHistory(events)",

		// Logging
		"r.logger.Debug(\"Saved %d events for invoice %s\", len(events), invoice.ID())",
		"r.logger.Debug(\"Loaded invoice %s from %d events\", aggregateID, len(events))",
	}

	for _, pattern := range eventSourcingPatterns {
		assert.Contains(t, result, pattern, "Event sourcing pattern not found: %s", pattern)
	}
}

func TestRepositoryTemplate_ReadModelComments(t *testing.T) {
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
			{
				Name:     "planName",
				Type:     "string",
				Required: true,
			},
		},
	}

	templateData := struct {
		Entity
		ProjectName string
	}{
		Entity:      entity,
		ProjectName: "subscription-service",
	}

	result, err := engine.Execute("repository_implementation.go", templateData)
	require.NoError(t, err)

	// Test read model implementation comments (Requirement 8.3)
	readModelPatterns := []string{
		// Comments indicating need for read models
		"// Note: This is a simplified implementation. In a real event-sourced system,",
		"// you would typically use read models/projections for queries",
		"// This would typically query a read model/projection",
		"// For now, return an error indicating this needs a proper read model implementation",
		"FindByPlanname requires read model implementation",
		"FindAll requires read model implementation",
		"Count requires read model implementation",
	}

	for _, pattern := range readModelPatterns {
		assert.Contains(t, result, pattern, "Read model comment not found: %s", pattern)
	}
}
