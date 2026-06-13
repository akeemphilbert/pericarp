package infrastructure

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// GormEventRepository provides GORM-based persistence for event models.
type GormEventRepository struct {
	db       *gorm.DB
	postgres bool
}

// NewGormEventRepository creates a new GormEventRepository.
// Panics if db is nil.
func NewGormEventRepository(db *gorm.DB) *GormEventRepository {
	if db == nil {
		panic("gorm_repository: db must not be nil")
	}
	return &GormEventRepository{db: db, postgres: db.Name() == "postgres"}
}

// SaveEvents persists a batch of event models in an explicit transaction.
func (r *GormEventRepository) SaveEvents(ctx context.Context, events []GormEventModel) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return r.insertEventsTx(tx, events)
	})
}

// insertEventsTx inserts event models inside an existing transaction,
// assigning each a global position. On Postgres the position column is
// omitted from the insert so the events_position_seq default assigns it
// (safe under concurrent writers). On single-writer engines like SQLite the
// position is computed as MAX(position)+1 inside the write transaction.
func (r *GormEventRepository) insertEventsTx(tx *gorm.DB, events []GormEventModel) error {
	if r.postgres {
		return tx.Omit("Position").Create(&events).Error
	}

	var maxPos int64
	if err := tx.Model(&GormEventModel{}).
		Select("COALESCE(MAX(position), 0)").
		Scan(&maxPos).Error; err != nil {
		return fmt.Errorf("failed to read max position: %w", err)
	}
	for i := range events {
		maxPos++
		events[i].Position = maxPos
	}
	return tx.Create(&events).Error
}

// GetEventsAfterPosition retrieves committed events with position greater than
// afterPosition, ordered by position. limit <= 0 means no limit. On Postgres,
// rows whose inserting transaction could still be in flight are withheld:
// positions become visible at commit, not in position order, so a row is only
// safe to hand out once every transaction that could hold a smaller position
// has finished (xact_id < pg_snapshot_xmin(pg_current_snapshot())).
func (r *GormEventRepository) GetEventsAfterPosition(ctx context.Context, afterPosition int64, limit int) ([]GormEventModel, error) {
	query := r.db.WithContext(ctx).Where("position > ?", afterPosition)
	if r.postgres {
		query = query.Where("xact_id < pg_snapshot_xmin(pg_current_snapshot())")
	}
	query = query.Order("position ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var events []GormEventModel
	err := query.Find(&events).Error
	return events, err
}

// GetHeadPosition returns the highest committed position (0 when empty),
// applying the same visibility guard as GetEventsAfterPosition on Postgres.
func (r *GormEventRepository) GetHeadPosition(ctx context.Context) (int64, error) {
	query := r.db.WithContext(ctx).Model(&GormEventModel{})
	if r.postgres {
		query = query.Where("xact_id < pg_snapshot_xmin(pg_current_snapshot())")
	}
	var head int64
	err := query.Select("COALESCE(MAX(position), 0)").Scan(&head).Error
	return head, err
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

// GetEventsByTransactionID retrieves all events with a given transaction ID, ordered by aggregate and sequence.
func (r *GormEventRepository) GetEventsByTransactionID(ctx context.Context, transactionID string) ([]GormEventModel, error) {
	var events []GormEventModel
	err := r.db.WithContext(ctx).
		Where("transaction_id = ?", transactionID).
		Order("aggregate_id ASC, sequence_no ASC").
		Find(&events).Error
	return events, err
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
