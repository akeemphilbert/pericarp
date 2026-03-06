package infrastructure

import (
	"context"

	"gorm.io/gorm"
)

// GormEventRepository provides GORM-based persistence for event models.
type GormEventRepository struct {
	db *gorm.DB
}

// NewGormEventRepository creates a new GormEventRepository.
// Panics if db is nil.
func NewGormEventRepository(db *gorm.DB) *GormEventRepository {
	if db == nil {
		panic("gorm_repository: db must not be nil")
	}
	return &GormEventRepository{db: db}
}

// SaveEvents persists a batch of event models in an explicit transaction.
func (r *GormEventRepository) SaveEvents(ctx context.Context, events []GormEventModel) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(&events).Error
	})
}

// GetEventsByAggregateID retrieves all events for a given aggregate, ordered by sequence number.
func (r *GormEventRepository) GetEventsByAggregateID(ctx context.Context, aggregateID string) ([]GormEventModel, error) {
	var events []GormEventModel
	err := r.db.WithContext(ctx).
		Where("aggregate_id = ?", aggregateID).
		Order("sequence_no ASC").
		Find(&events).Error
	return events, err
}

// GetEventsByAggregateIDRange retrieves events for an aggregate within a sequence number range.
func (r *GormEventRepository) GetEventsByAggregateIDRange(ctx context.Context, aggregateID string, fromSeq, toSeq int) ([]GormEventModel, error) {
	var events []GormEventModel
	query := r.db.WithContext(ctx).Where("aggregate_id = ? AND sequence_no >= ?", aggregateID, fromSeq)
	if toSeq >= 0 {
		query = query.Where("sequence_no <= ?", toSeq)
	}
	err := query.Order("sequence_no ASC").Find(&events).Error
	return events, err
}

// GetEventByID retrieves a single event by its ID.
func (r *GormEventRepository) GetEventByID(ctx context.Context, eventID string) (*GormEventModel, error) {
	var event GormEventModel
	err := r.db.WithContext(ctx).Where("id = ?", eventID).First(&event).Error
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// GetCurrentVersion returns the highest sequence number for a given aggregate.
// Returns 0 if no events exist.
func (r *GormEventRepository) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	var maxSeq *int
	err := r.db.WithContext(ctx).
		Model(&GormEventModel{}).
		Where("aggregate_id = ?", aggregateID).
		Select("MAX(sequence_no)").
		Scan(&maxSeq).Error
	if err != nil {
		return 0, err
	}
	if maxSeq == nil {
		return 0, nil
	}
	return *maxSeq, nil
}
