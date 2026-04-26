package projection

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

// ResourceModel is the generic GORM model for polymorphic projection storage.
// Concrete subtypes of an abstract resource share a single table, distinguished
// by the ResourceType discriminator column. Type-specific fields are stored in
// the JSONB Data column.
type ResourceModel struct {
	ID           string              `gorm:"primaryKey;column:id"`
	ResourceType string              `gorm:"column:resource_type;not null;index"`
	Data         infrastructure.JSONB `gorm:"column:data;type:jsonb"`
	SequenceNo   int                 `gorm:"column:sequence_no"`
	CreatedAt    time.Time           `gorm:"column:created_at"`
	UpdatedAt    time.Time           `gorm:"column:updated_at"`
}

// ResourceConverter converts between a domain entity and the generic ResourceModel.
type ResourceConverter[T any] interface {
	ToModel(entity T) (*ResourceModel, error)
	FromModel(model *ResourceModel) (T, error)
	ResourceType() string
}

// PaginatedResponse represents a paginated response with cursor-based pagination.
type PaginatedResponse[T any] struct {
	Data    []T
	Cursor  string
	Limit   int
	HasMore bool
}
