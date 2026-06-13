package infrastructure

import (
	"fmt"

	"gorm.io/gorm"
)

// eventPositionsAdvisoryLockID serializes concurrent position migrations
// across replicas on Postgres via pg_advisory_xact_lock. The value is an
// arbitrary constant unique to this migration.
const eventPositionsAdvisoryLockID int64 = 0x7065726963617270 // "pericarp"

// migrateEventPositions upgrades the events table for the global ordered
// feed. It backfills the position column for rows that predate it and, on
// Postgres, wires up the machinery that keeps ReadAfter commit-safe under
// concurrent writers:
//
//   - events_position_seq assigns positions as the column's default
//   - xact_id (xid8, default pg_current_xact_id()) records the inserting
//     transaction so readers can withhold rows until every transaction that
//     could hold a smaller position has finished
//
// Backfill orders rows by id — KSUIDs sort by creation time, the best
// available approximation of commit order for pre-existing data. The whole
// migration is idempotent and safe to run on every store construction.
//
// Postgres 13+ is required (xid8, pg_current_xact_id, pg_current_snapshot).
func migrateEventPositions(db *gorm.DB) error {
	isPostgres := db.Name() == "postgres"

	return db.Transaction(func(tx *gorm.DB) error {
		if isPostgres {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", eventPositionsAdvisoryLockID).Error; err != nil {
				return fmt.Errorf("failed to acquire position migration lock: %w", err)
			}
			if err := tx.Exec("CREATE SEQUENCE IF NOT EXISTS events_position_seq OWNED BY events.position").Error; err != nil {
				return fmt.Errorf("failed to create position sequence: %w", err)
			}
		}

		// Backfill rows that predate the position column. The offset keeps
		// backfilled positions above any already-assigned ones.
		var offset int64
		if err := tx.Model(&GormEventModel{}).
			Select("COALESCE(MAX(position), 0)").
			Scan(&offset).Error; err != nil {
			return fmt.Errorf("failed to read max position: %w", err)
		}
		if err := tx.Exec(
			`UPDATE events SET position = sub.rn + ? `+
				`FROM (SELECT id, ROW_NUMBER() OVER (ORDER BY id) AS rn FROM events WHERE position IS NULL) AS sub `+
				`WHERE events.id = sub.id`,
			offset,
		).Error; err != nil {
			return fmt.Errorf("failed to backfill event positions: %w", err)
		}

		if !isPostgres {
			return nil
		}

		// Point the sequence past the backfilled maximum — but only if it has
		// never been used. Rewinding a live sequence would hand out duplicate
		// positions to in-flight writers.
		var isCalled bool
		if err := tx.Raw("SELECT is_called FROM events_position_seq").Scan(&isCalled).Error; err != nil {
			return fmt.Errorf("failed to inspect position sequence: %w", err)
		}
		if !isCalled {
			if err := tx.Exec("SELECT setval('events_position_seq', COALESCE((SELECT MAX(position) FROM events), 0) + 1, false)").Error; err != nil {
				return fmt.Errorf("failed to initialize position sequence: %w", err)
			}
		}

		if err := tx.Exec("ALTER TABLE events ALTER COLUMN position SET DEFAULT nextval('events_position_seq')").Error; err != nil {
			return fmt.Errorf("failed to set position default: %w", err)
		}
		if err := tx.Exec("ALTER TABLE events ALTER COLUMN position SET NOT NULL").Error; err != nil {
			return fmt.Errorf("failed to make position non-null: %w", err)
		}
		if err := tx.Exec("ALTER TABLE events ADD COLUMN IF NOT EXISTS xact_id xid8 NOT NULL DEFAULT pg_current_xact_id()").Error; err != nil {
			return fmt.Errorf("failed to add xact_id column: %w", err)
		}

		return nil
	})
}
