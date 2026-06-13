package subscriptions

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormCheckpointModel is the GORM model for subscriber checkpoints. The table
// is owned and auto-migrated by pericarp.
type GormCheckpointModel struct {
	Subscriber string    `gorm:"primaryKey;column:subscriber"`
	Position   int64     `gorm:"column:position;not null"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

// TableName returns the table name for the checkpoint model.
func (GormCheckpointModel) TableName() string {
	return "subscriber_checkpoints"
}

// GormCheckpointStore is a database-backed CheckpointStore. Each batch runs
// inside one database transaction: handlers that write to the same database
// through the batch transaction (TxFromContext) commit atomically with the
// checkpoint advance — exactly-once processing for same-database handlers.
//
// On Postgres the checkpoint row is locked with FOR UPDATE SKIP LOCKED, so N
// processes running the same subscriber coordinate as active/passive
// replicas: one wins each cycle, the others skip. SQLite relies on its
// serialized writers (single process).
type GormCheckpointStore struct {
	db       *gorm.DB
	postgres bool
}

var _ CheckpointStore = (*GormCheckpointStore)(nil)

// NewGormCheckpointStore creates a checkpoint store and auto-migrates the
// subscriber_checkpoints table. Like the event feed itself, it supports
// Postgres and SQLite: replica coordination relies on row locking (Postgres)
// or serialized writers (SQLite), and other engines would silently
// double-process events.
func NewGormCheckpointStore(db *gorm.DB) (*GormCheckpointStore, error) {
	dialect := db.Name()
	if dialect != "postgres" && dialect != "sqlite" {
		return nil, fmt.Errorf("checkpoint store supports postgres and sqlite, got dialect %q", dialect)
	}
	if err := db.AutoMigrate(&GormCheckpointModel{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate subscriber_checkpoints table: %w", err)
	}
	return &GormCheckpointStore{db: db, postgres: dialect == "postgres"}, nil
}

// Acquire begins a batch transaction holding the subscriber's checkpoint row.
// On Postgres, acquired is false when another process holds the row.
func (g *GormCheckpointStore) Acquire(ctx context.Context, subscriber string) (Batch, bool, error) {
	// Ensure the row exists before locking it. Runs outside the batch
	// transaction so a no-op conflict never interacts with row locks.
	if err := g.ensure(ctx, subscriber, 0); err != nil {
		return nil, false, err
	}

	// The transaction deliberately outlives ctx: its lifecycle is owned by
	// Batch.Commit/Rollback, and a batch in flight when the subscriber's run
	// context is cancelled must drain — database/sql would otherwise
	// auto-rollback the transaction the moment ctx is cancelled, silently
	// discarding handler writes and the checkpoint advance.
	tx := g.db.WithContext(context.WithoutCancel(ctx)).Begin()
	if tx.Error != nil {
		return nil, false, fmt.Errorf("failed to begin batch transaction: %w", tx.Error)
	}

	query := tx.Model(&GormCheckpointModel{}).Where("subscriber = ?", subscriber)
	if g.postgres {
		query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
	}
	var rows []GormCheckpointModel
	if err := query.Find(&rows).Error; err != nil {
		_ = tx.Rollback()
		return nil, false, fmt.Errorf("failed to lock checkpoint row: %w", err)
	}
	if len(rows) == 0 {
		if err := tx.Rollback().Error; err != nil {
			return nil, false, fmt.Errorf("failed to release batch transaction: %w", err)
		}
		if g.postgres {
			// The row exists (ensured above) but is locked by another
			// process — skip this cycle.
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("checkpoint row for subscriber %q disappeared", subscriber)
	}

	return &gormBatch{tx: tx, subscriber: subscriber, position: rows[0].Position}, true, nil
}

// Position returns the subscriber's committed checkpoint (0 if unknown).
func (g *GormCheckpointStore) Position(ctx context.Context, subscriber string) (int64, error) {
	var rows []GormCheckpointModel
	if err := g.db.WithContext(ctx).Where("subscriber = ?", subscriber).Find(&rows).Error; err != nil {
		return 0, fmt.Errorf("failed to read checkpoint: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].Position, nil
}

// Reset sets the subscriber's checkpoint, creating the row if needed. On
// Postgres it blocks until any in-flight batch for the subscriber finishes
// (the batch transaction holds the row). On SQLite a concurrent in-flight
// batch is not blocked; instead its Commit detects that the checkpoint moved
// and aborts, so the reset always wins and processing resumes from the reset
// position.
func (g *GormCheckpointStore) Reset(ctx context.Context, subscriber string, position int64) error {
	return g.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "subscriber"}},
		DoUpdates: clause.Assignments(map[string]any{"position": position, "updated_at": time.Now()}),
	}).Create(&GormCheckpointModel{Subscriber: subscriber, Position: position, UpdatedAt: time.Now()}).Error
}

// ensure creates the checkpoint row at the given position if it doesn't exist.
func (g *GormCheckpointStore) ensure(ctx context.Context, subscriber string, position int64) error {
	err := g.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "subscriber"}},
		DoNothing: true,
	}).Create(&GormCheckpointModel{Subscriber: subscriber, Position: position, UpdatedAt: time.Now()}).Error
	if err != nil {
		return fmt.Errorf("failed to ensure checkpoint row: %w", err)
	}
	return nil
}

type gormBatch struct {
	tx         *gorm.DB
	subscriber string
	position   int64
	done       bool
}

func (b *gormBatch) Position() int64 { return b.position }

// HandlerContext attaches the batch transaction so handlers can join it via
// TxFromContext.
func (b *gormBatch) HandlerContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, txContextKey{}, b.tx)
}

// Commit advances the checkpoint and commits the batch transaction, making
// the checkpoint advance and any handler writes through the transaction
// atomic.
func (b *gormBatch) Commit(ctx context.Context, position int64) error {
	if b.done {
		return fmt.Errorf("batch for subscriber %q already finished", b.subscriber)
	}
	b.done = true

	// The position guard makes the advance conditional on the checkpoint not
	// having moved since acquisition. On Postgres the row lock makes that a
	// given; on SQLite (no row lock) it detects a concurrent Reset, in which
	// case the whole batch aborts rather than silently clobbering the reset.
	res := b.tx.Model(&GormCheckpointModel{}).
		Where("subscriber = ? AND position = ?", b.subscriber, b.position).
		Updates(map[string]any{"position": position, "updated_at": time.Now()})
	if res.Error != nil {
		_ = b.tx.Rollback()
		return fmt.Errorf("failed to advance checkpoint: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		_ = b.tx.Rollback()
		return fmt.Errorf("checkpoint for subscriber %q moved during the batch (concurrent reset?); batch abandoned", b.subscriber)
	}
	if err := b.tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}
	return nil
}

// Rollback abandons the batch; the checkpoint stays where it was.
func (b *gormBatch) Rollback() error {
	if b.done {
		return nil
	}
	b.done = true
	return b.tx.Rollback().Error
}

// Savepoint marks a rollback point inside the batch transaction.
func (b *gormBatch) Savepoint(ctx context.Context, name string) error {
	return b.tx.SavePoint(name).Error
}

// RollbackToSavepoint discards writes made through the batch transaction
// after the named savepoint.
func (b *gormBatch) RollbackToSavepoint(ctx context.Context, name string) error {
	return b.tx.RollbackTo(name).Error
}

type txContextKey struct{}

// TxFromContext returns the batch transaction attached to a handler's context
// by a GormCheckpointStore batch, or nil when the handler is not running
// inside a database-backed batch. Handlers that write projections to the same
// database should write through this transaction: those writes then commit
// atomically with the checkpoint advance (exactly-once), instead of relying
// on at-least-once redelivery.
func TxFromContext(ctx context.Context) *gorm.DB {
	tx, _ := ctx.Value(txContextKey{}).(*gorm.DB)
	return tx
}
