package projection

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// PolymorphicRepository provides CRUD and list operations for resources stored
// in a shared projection table with a discriminator column.
type PolymorphicRepository[T any] struct {
	db        *gorm.DB
	registry  *Registry
	converter ResourceConverter[T]
	table     string
}

// NewPolymorphicRepository creates a repository for the given converter's resource type.
// The table name is resolved from the registry (inherited from abstract parent if applicable).
func NewPolymorphicRepository[T any](db *gorm.DB, registry *Registry, converter ResourceConverter[T]) (*PolymorphicRepository[T], error) {
	typeName := converter.ResourceType()
	if !registry.IsRegistered(typeName) {
		return nil, fmt.Errorf("%w: %s", ErrTypeNotRegistered, typeName)
	}

	table, err := registry.GetTable(typeName)
	if err != nil {
		return nil, err
	}

	return &PolymorphicRepository[T]{
		db:        db,
		registry:  registry,
		converter: converter,
		table:     table,
	}, nil
}

func (r *PolymorphicRepository[T]) Save(ctx context.Context, entity T) error {
	model, err := r.converter.ToModel(entity)
	if err != nil {
		return fmt.Errorf("projection: convert to model: %w", err)
	}
	model.ResourceType = r.converter.ResourceType()
	return r.db.WithContext(ctx).Table(r.table).Save(model).Error
}

func (r *PolymorphicRepository[T]) FindByID(ctx context.Context, id string) (T, error) {
	var zero T
	var model ResourceModel
	err := r.db.WithContext(ctx).Table(r.table).Where("id = ?", id).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return zero, nil
		}
		return zero, err
	}
	return r.converter.FromModel(&model)
}

// FindAll returns resources matching this repository's concrete type with
// cursor-based pagination.
func (r *PolymorphicRepository[T]) FindAll(ctx context.Context, cursor string, limit int) (*PaginatedResponse[T], error) {
	return r.findWithTypes(ctx, []string{r.converter.ResourceType()}, cursor, limit)
}

// FindAllByParentType returns resources for all concrete subtypes of the given
// parent type. This is how you list all "FinancialProducts" and get back Loans,
// DepositAccounts, etc.
func (r *PolymorphicRepository[T]) FindAllByParentType(ctx context.Context, parentType string, cursor string, limit int) (*PaginatedResponse[T], error) {
	types, err := r.registry.GetConcreteTypes(parentType)
	if err != nil {
		return nil, err
	}
	if len(types) == 0 {
		return &PaginatedResponse[T]{
			Data:  make([]T, 0),
			Limit: normalizeLimit(limit),
		}, nil
	}
	return r.findWithTypes(ctx, types, cursor, limit)
}

func (r *PolymorphicRepository[T]) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Table(r.table).Where("id = ?", id).Delete(&ResourceModel{}).Error
}

func (r *PolymorphicRepository[T]) findWithTypes(ctx context.Context, types []string, cursor string, limit int) (*PaginatedResponse[T], error) {
	limit = normalizeLimit(limit)

	query := r.db.WithContext(ctx).Table(r.table).Where("resource_type IN ?", types).Order("id ASC")
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}

	var records []ResourceModel
	if err := query.Limit(limit + 1).Find(&records).Error; err != nil {
		return nil, err
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}

	result := &PaginatedResponse[T]{
		Data:    make([]T, 0, len(records)),
		Limit:   limit,
		HasMore: hasMore,
	}

	for i := range records {
		entity, err := r.converter.FromModel(&records[i])
		if err != nil {
			return nil, fmt.Errorf("projection: convert from model: %w", err)
		}
		result.Data = append(result.Data, entity)
	}

	if len(records) > 0 {
		result.Cursor = records[len(records)-1].ID
	}

	return result, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	return limit
}
