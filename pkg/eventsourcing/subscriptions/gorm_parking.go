package subscriptions

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// GormParkedEventModel is the GORM model for parked events. The table is
// owned and auto-migrated by pericarp.
type GormParkedEventModel struct {
	Subscriber string    `gorm:"primaryKey;column:subscriber"`
	EventID    string    `gorm:"primaryKey;column:event_id"`
	EventType  string    `gorm:"column:event_type"`
	Position   int64     `gorm:"column:position"`
	Error      string    `gorm:"column:error"`
	Attempts   int       `gorm:"column:attempts"`
	ParkedAt   time.Time `gorm:"column:parked_at"`
}

// TableName returns the table name for the parked event model.
func (GormParkedEventModel) TableName() string {
	return "parked_events"
}

// GormParkingLot is a database-backed ParkingLot. When parking happens inside
// a subscriber batch, the row is written through the batch transaction so the
// parking and the checkpoint advance commit atomically — a crash between the
// two cannot lose a poison event or park it twice.
type GormParkingLot struct {
	db *gorm.DB
}

var _ ParkingLot = (*GormParkingLot)(nil)

// NewGormParkingLot creates a parking lot and auto-migrates the parked_events
// table.
func NewGormParkingLot(db *gorm.DB) (*GormParkingLot, error) {
	if err := db.AutoMigrate(&GormParkedEventModel{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate parked_events table: %w", err)
	}
	return &GormParkingLot{db: db}, nil
}

// Park records a poison event, writing through the batch transaction when ctx
// carries one. Re-parking an already-parked event updates its error, attempt
// count, and timestamp (the event may be reprocessed after a checkpoint
// reset).
func (g *GormParkingLot) Park(ctx context.Context, parked ParkedEvent) error {
	db := g.db
	if tx := TxFromContext(ctx); tx != nil {
		db = tx
	}
	model := GormParkedEventModel(parked)
	err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "subscriber"}, {Name: "event_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"error":     parked.Error,
			"attempts":  parked.Attempts,
			"parked_at": parked.ParkedAt,
		}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("failed to park event %s: %w", parked.EventID, err)
	}
	return nil
}

// List returns the subscriber's parked events ordered by position.
func (g *GormParkingLot) List(ctx context.Context, subscriber string) ([]ParkedEvent, error) {
	var models []GormParkedEventModel
	err := g.db.WithContext(ctx).
		Where("subscriber = ?", subscriber).
		Order("position ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list parked events: %w", err)
	}
	result := make([]ParkedEvent, len(models))
	for i, m := range models {
		result[i] = ParkedEvent(m)
	}
	return result, nil
}

// Replay re-runs the handler and clears the parked row in one transaction.
// The handler receives the transaction via TxFromContext, so a failed replay
// rolls back both the handler's writes and the row deletion.
func (g *GormParkingLot) Replay(ctx context.Context, subscriber, eventID string, event domain.EventEnvelope[any], handler Handler) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []GormParkedEventModel
		if err := tx.Where("subscriber = ? AND event_id = ?", subscriber, eventID).Find(&rows).Error; err != nil {
			return fmt.Errorf("failed to look up parked event: %w", err)
		}
		if len(rows) == 0 {
			return fmt.Errorf("%w: event %s for subscriber %q", ErrEventNotParked, eventID, subscriber)
		}

		handlerCtx := context.WithValue(ctx, txContextKey{}, tx)
		if err := handler(handlerCtx, event); err != nil {
			return fmt.Errorf("handler failed during replay of event %s: %w", eventID, err)
		}

		if err := tx.Delete(&GormParkedEventModel{}, "subscriber = ? AND event_id = ?", subscriber, eventID).Error; err != nil {
			return fmt.Errorf("failed to clear parked event %s: %w", eventID, err)
		}
		return nil
	})
}
