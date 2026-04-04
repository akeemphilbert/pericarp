package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"gorm.io/gorm"
)

var _ domain.EventStore = (*GormEventStore)(nil)

// GormEventStore is a GORM-based implementation of EventStore.
type GormEventStore struct {
	repo *GormEventRepository
	db   *gorm.DB
}

// NewGormEventStore creates a new GORM-based event store and auto-migrates the events table.
func NewGormEventStore(db *gorm.DB) (*GormEventStore, error) {
	if err := db.AutoMigrate(&GormEventModel{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate events table: %w", err)
	}
	return &GormEventStore{
		repo: NewGormEventRepository(db),
		db:   db,
	}, nil
}

// Append appends events to the store for the given aggregate.
// If expectedVersion is not -1, optimistic concurrency control is enforced within a transaction.
func (s *GormEventStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		if event.AggregateID != aggregateID {
			return fmt.Errorf("%w: aggregate ID mismatch", domain.ErrInvalidEvent)
		}
		if event.ID == "" {
			return fmt.Errorf("%w: event ID is required", domain.ErrInvalidEvent)
		}
	}

	models := make([]GormEventModel, len(events))
	for i, event := range events {
		m, err := envelopeToModel(event)
		if err != nil {
			return fmt.Errorf("%w: %v", domain.ErrInvalidEvent, err)
		}
		models[i] = m
	}

	if expectedVersion == -1 {
		return s.repo.SaveEvents(ctx, models)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxSeq *int
		if err := tx.Model(&GormEventModel{}).
			Where("aggregate_id = ?", aggregateID).
			Select("MAX(sequence_no)").
			Scan(&maxSeq).Error; err != nil {
			return fmt.Errorf("failed to check current version: %w", err)
		}

		currentVersion := 0
		if maxSeq != nil {
			currentVersion = *maxSeq
		}

		if currentVersion != expectedVersion {
			return fmt.Errorf("%w: expected version %d, got %d",
				domain.ErrConcurrencyConflict, expectedVersion, currentVersion)
		}

		return tx.Create(&models).Error
	})
}

// GetEvents retrieves all events for the given aggregate ID.
func (s *GormEventStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	models, err := s.repo.GetEventsByAggregateID(ctx, aggregateID)
	if err != nil {
		return nil, err
	}
	return modelsToEnvelopes(models), nil
}

// GetEventsFromVersion retrieves events starting from the specified version.
func (s *GormEventStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	models, err := s.repo.GetEventsByAggregateIDRange(ctx, aggregateID, fromVersion, -1)
	if err != nil {
		return nil, err
	}
	return modelsToEnvelopes(models), nil
}

// GetEventsRange retrieves events within a version range.
// If fromVersion is -1, it defaults to 1. If toVersion is -1, all events from fromVersion onwards are returned.
func (s *GormEventStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	if fromVersion == -1 {
		fromVersion = 1
	}
	models, err := s.repo.GetEventsByAggregateIDRange(ctx, aggregateID, fromVersion, toVersion)
	if err != nil {
		return nil, err
	}
	return modelsToEnvelopes(models), nil
}

// GetEventByID retrieves a specific event by its ID.
func (s *GormEventStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	model, err := s.repo.GetEventByID(ctx, eventID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.EventEnvelope[any]{}, domain.ErrEventNotFound
		}
		return domain.EventEnvelope[any]{}, err
	}
	return modelToEnvelope(*model), nil
}

// GetCurrentVersion returns the current version for the aggregate.
func (s *GormEventStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	return s.repo.GetCurrentVersion(ctx, aggregateID)
}

// Close closes the GORM event store (no-op since GORM connection is managed externally).
func (s *GormEventStore) Close() error {
	return nil
}

func envelopeToModel(env domain.EventEnvelope[any]) (GormEventModel, error) {
	payload, err := toJSONB(env.Payload)
	if err != nil {
		return GormEventModel{}, fmt.Errorf("failed to convert payload: %w", err)
	}

	metadata, err := toJSONB(env.Metadata)
	if err != nil {
		return GormEventModel{}, fmt.Errorf("failed to convert metadata: %w", err)
	}

	return GormEventModel{
		ID:            env.ID,
		AggregateID:   env.AggregateID,
		EventType:     env.EventType,
		SequenceNo:    env.SequenceNo,
		TransactionID: env.TransactionID,
		Payload:       payload,
		Metadata:      metadata,
		CreatedAt:     env.Created,
	}, nil
}

func toJSONB(v any) (JSONB, error) {
	switch p := v.(type) {
	case map[string]any:
		return JSONB(p), nil
	case nil:
		return nil, nil
	default:
		data, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal %T: %w", p, err)
		}
		var m JSONB
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %T to map: %w", p, err)
		}
		return m, nil
	}
}

func modelToEnvelope(m GormEventModel) domain.EventEnvelope[any] {
	var payload any
	if m.Payload != nil {
		payload = map[string]any(m.Payload)
	}

	metadata := make(map[string]any)
	if m.Metadata != nil {
		metadata = map[string]any(m.Metadata)
	}

	return domain.EventEnvelope[any]{
		ID:            m.ID,
		AggregateID:   m.AggregateID,
		EventType:     m.EventType,
		Payload:       payload,
		Created:       m.CreatedAt,
		SequenceNo:    m.SequenceNo,
		TransactionID: m.TransactionID,
		Metadata:      metadata,
	}
}

func modelsToEnvelopes(models []GormEventModel) []domain.EventEnvelope[any] {
	envelopes := make([]domain.EventEnvelope[any], len(models))
	for i, m := range models {
		envelopes[i] = modelToEnvelope(m)
	}
	return envelopes
}
