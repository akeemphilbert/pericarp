package infrastructure

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// JSONB is a map type that implements driver.Valuer and sql.Scanner for JSON storage in databases.
type JSONB map[string]any

// Value returns the JSON encoding of JSONB for database storage.
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan reads a JSON-encoded value from the database into JSONB.
func (j *JSONB) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("unsupported type: %T", value)
	}

	return json.Unmarshal(bytes, j)
}

// GormEventModel is the GORM model for persisting events.
type GormEventModel struct {
	ID          string    `gorm:"primaryKey;column:id"`
	AggregateID string    `gorm:"column:aggregate_id;index;uniqueIndex:idx_aggregate_sequence"`
	EventType   string    `gorm:"column:event_type"`
	SequenceNo  int       `gorm:"column:sequence_no;uniqueIndex:idx_aggregate_sequence"`
	Payload     JSONB     `gorm:"column:payload;type:jsonb"`
	Metadata    JSONB     `gorm:"column:metadata;type:jsonb"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

// TableName returns the table name for the event model.
func (GormEventModel) TableName() string {
	return "events"
}
