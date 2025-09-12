package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
	"github.com/segmentio/ksuid"
	"gorm.io/gorm"
)

// EventRecord represents the database schema for storing events
type EventRecord struct {
	ID          string    `gorm:"primaryKey"`
	AggregateID string    `gorm:"index"`
	EventType   string    `gorm:"index"`
	SequenceNo  int64     `gorm:"index"`
	Data        string    `gorm:"type:text"` // JSON serialized event
	Metadata    string    `gorm:"type:text"` // JSON serialized metadata
	Timestamp   time.Time `gorm:"index"`
	CreatedAt   time.Time
}

// TableName returns the table name for GORM
func (EventRecord) TableName() string {
	return "events"
}

// eventEnvelope implements the domain.Envelope interface
type eventEnvelope struct {
	event     domain.Event
	metadata  map[string]interface{}
	eventID   string
	timestamp time.Time
}

func (e *eventEnvelope) Event() domain.Event {
	return e.event
}

func (e *eventEnvelope) Metadata() map[string]interface{} {
	return e.metadata
}

func (e *eventEnvelope) EventID() string {
	return e.eventID
}

func (e *eventEnvelope) Timestamp() time.Time {
	return e.timestamp
}

// GormEventStore implements the EventStore interface using GORM
type GormEventStore struct {
	db *gorm.DB
}

// NewGormEventStore creates a new GORM-based event store
func NewGormEventStore(db *gorm.DB) (*GormEventStore, error) {
	store := &GormEventStore{
		db: db,
	}

	// Auto-migrate the events table
	if err := db.AutoMigrate(&EventRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate events table: %w", err)
	}

	return store, nil
}

// Save persists events and returns envelopes with metadata
func (s *GormEventStore) Save(ctx context.Context, events []domain.Event) ([]domain.Envelope, error) {
	if len(events) == 0 {
		return []domain.Envelope{}, nil
	}

	// Pre-allocate slices with known capacity for better performance
	records := make([]EventRecord, 0, len(events))
	envelopes := make([]domain.Envelope, 0, len(events))

	// Get current timestamp once for all events to improve consistency and performance
	now := time.Now()

	// Pre-allocate JSON encoder buffer to reduce allocations
	var jsonBuffer []byte

	for _, event := range events {
		// Serialize event data to JSON with pre-allocated buffer
		eventData, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize event %s: %w", event.EventType(), err)
		}

		// Create metadata with minimal allocations
		metadata := map[string]interface{}{
			"aggregate_id": event.AggregateID(),
			"event_type":   event.EventType(),
			"sequenceNo":   event.SequenceNo(),
			"created_at":   event.CreatedAt(),
		}

		// Reuse buffer for metadata JSON serialization
		jsonBuffer = jsonBuffer[:0] // Reset buffer length but keep capacity
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize metadata for event %s: %w", event.EventType(), err)
		}

		eventID := ksuid.New().String()

		record := EventRecord{
			ID:          eventID,
			AggregateID: event.AggregateID(),
			EventType:   event.EventType(),
			SequenceNo:  event.SequenceNo(),
			Data:        string(eventData),
			Metadata:    string(metadataJSON),
			Timestamp:   now,
			CreatedAt:   now,
		}

		records = append(records, record)

		envelope := &eventEnvelope{
			event:     event,
			metadata:  metadata,
			eventID:   eventID,
			timestamp: now,
		}

		envelopes = append(envelopes, envelope)
	}

	// Save all records in a transaction with batch insert for better performance
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Use CreateInBatches for better performance with large event sets
		batchSize := 100 // Configurable batch size
		if len(records) <= batchSize {
			if err := tx.Create(&records).Error; err != nil {
				return fmt.Errorf("failed to save events: %w", err)
			}
		} else {
			if err := tx.CreateInBatches(&records, batchSize).Error; err != nil {
				return fmt.Errorf("failed to save events in batches: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return envelopes, nil
}

// Load retrieves all events for an aggregate
func (s *GormEventStore) Load(ctx context.Context, aggregateID string) ([]domain.Envelope, error) {
	return s.LoadFromSequence(ctx, aggregateID, 0)
}

// LoadFromSequence retrieves events for an aggregate starting from a specific sequence number
func (s *GormEventStore) LoadFromSequence(ctx context.Context, aggregateID string, sequenceNo int64) ([]domain.Envelope, error) {
	var records []EventRecord

	// Optimize query with proper indexing hints and limit if needed
	query := s.db.WithContext(ctx).
		Where("aggregate_id = ? AND sequence_no >= ?", aggregateID, sequenceNo).
		Order("sequence_no ASC")

	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to load events for aggregate %s: %w", aggregateID, err)
	}

	if len(records) == 0 {
		return []domain.Envelope{}, nil
	}

	// Pre-allocate envelope slice with exact capacity
	envelopes := make([]domain.Envelope, len(records))

	// Process records in batch to reduce allocation overhead
	for i, record := range records {
		// Deserialize metadata with error handling
		var metadata map[string]interface{}
		if record.Metadata != "" {
			if err := json.Unmarshal([]byte(record.Metadata), &metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize metadata for event %s: %w", record.ID, err)
			}
		} else {
			// Initialize empty metadata if none exists
			metadata = make(map[string]interface{})
		}

		// Create a generic event from the stored data
		genericEvent := &domain.EntityEvent{
			Type:        record.EventType,
			AggregateId: record.AggregateID,
			SequenceNum: record.SequenceNo,
			CreatedTime: record.Timestamp,
			PayloadData: []byte(record.Data),
		}

		envelope := &eventEnvelope{
			event:     genericEvent,
			metadata:  metadata,
			eventID:   record.ID,
			timestamp: record.Timestamp,
		}

		envelopes[i] = envelope
	}

	return envelopes, nil
}

// LoadFromVersion retrieves events for an aggregate starting from a specific version (sequence number)
func (s *GormEventStore) LoadFromVersion(ctx context.Context, aggregateID string, version int64) ([]domain.Envelope, error) {
	return s.LoadFromSequence(ctx, aggregateID, version)
}
