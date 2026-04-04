package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

var _ domain.EventStore = (*BigQueryEventStore)(nil)

// BigQueryEventStore is a BigQuery-based implementation of EventStore.
//
// Concurrency model: the version check in Append uses a read-then-write approach.
// The current version is read first; if it matches expectedVersion the events are
// inserted. This is a known limitation: two concurrent callers targeting the same
// aggregate can both pass the version check and insert events with duplicate
// sequence numbers. This violates strict optimistic concurrency guarantees.
// Production deployments should either enforce uniqueness on (aggregate_id, sequence_no)
// at the table level, or accept that serializable isolation is not provided by this
// implementation. For low-contention workloads this risk is typically acceptable.
type BigQueryEventStore struct {
	client    *bigquery.Client
	projectID string
	datasetID string
	tableID   string
}

// NewBigQueryEventStore creates a new BigQuery-based event store.
// The table must already exist — BigQuery tables are provisioned via IaC.
func NewBigQueryEventStore(client *bigquery.Client, projectID, datasetID, tableID string) *BigQueryEventStore {
	if client == nil {
		panic("bigquery client must not be nil")
	}
	if projectID == "" {
		panic("project ID must not be empty")
	}
	if datasetID == "" {
		panic("dataset ID must not be empty")
	}
	if tableID == "" {
		panic("table ID must not be empty")
	}
	return &BigQueryEventStore{
		client:    client,
		projectID: projectID,
		datasetID: datasetID,
		tableID:   tableID,
	}
}

func (s *BigQueryEventStore) fullTableID() string {
	return fmt.Sprintf("`%s.%s.%s`", s.projectID, s.datasetID, s.tableID)
}

// Append appends events to the store for the given aggregate.
// If expectedVersion is not -1, optimistic concurrency control is enforced.
func (s *BigQueryEventStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
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
		if event.EventType == "" {
			return fmt.Errorf("%w: event type is required", domain.ErrInvalidEvent)
		}
	}

	if expectedVersion == -1 {
		return s.appendWithoutVersionCheck(ctx, events)
	}

	return s.appendWithVersionCheck(ctx, aggregateID, expectedVersion, events)
}

func (s *BigQueryEventStore) appendWithoutVersionCheck(ctx context.Context, events []domain.EventEnvelope[any]) error {
	querySQL, params, err := s.buildInsertQuery(events)
	if err != nil {
		return err
	}

	q := s.client.Query(querySQL)
	q.Parameters = params

	job, err := q.Run(ctx)
	if err != nil {
		return fmt.Errorf("failed to run insert query: %w", err)
	}

	status, err := job.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for insert job: %w", err)
	}
	if err := status.Err(); err != nil {
		return fmt.Errorf("insert job failed: %w", err)
	}

	return nil
}

func (s *BigQueryEventStore) appendWithVersionCheck(ctx context.Context, aggregateID string, expectedVersion int, events []domain.EventEnvelope[any]) error {
	currentVersion, err := s.GetCurrentVersion(ctx, aggregateID)
	if err != nil {
		return fmt.Errorf("failed to get current version for conflict check: %w", err)
	}

	if currentVersion != expectedVersion {
		return fmt.Errorf("%w: expected version %d", domain.ErrConcurrencyConflict, expectedVersion)
	}

	return s.appendWithoutVersionCheck(ctx, events)
}

func (s *BigQueryEventStore) buildInsertQuery(events []domain.EventEnvelope[any]) (string, []bigquery.QueryParameter, error) {
	var valuePlaceholders []string
	var params []bigquery.QueryParameter

	for i, event := range events {
		payloadJSON, metadataJSON, err := marshalPayloadAndMetadata(event)
		if err != nil {
			return "", nil, fmt.Errorf("%w: %v", domain.ErrInvalidEvent, err)
		}

		idParam := fmt.Sprintf("id_%d", i)
		aggParam := fmt.Sprintf("agg_%d", i)
		typeParam := fmt.Sprintf("type_%d", i)
		seqParam := fmt.Sprintf("seq_%d", i)
		txParam := fmt.Sprintf("tx_%d", i)
		payParam := fmt.Sprintf("pay_%d", i)
		metaParam := fmt.Sprintf("meta_%d", i)
		tsParam := fmt.Sprintf("ts_%d", i)

		valuePlaceholders = append(valuePlaceholders, fmt.Sprintf("(@%s, @%s, @%s, @%s, @%s, @%s, @%s, @%s)",
			idParam, aggParam, typeParam, seqParam, txParam, payParam, metaParam, tsParam))

		txValue := bigquery.NullString{StringVal: event.TransactionID, Valid: event.TransactionID != ""}
		params = append(params,
			bigquery.QueryParameter{Name: idParam, Value: event.ID},
			bigquery.QueryParameter{Name: aggParam, Value: event.AggregateID},
			bigquery.QueryParameter{Name: typeParam, Value: event.EventType},
			bigquery.QueryParameter{Name: seqParam, Value: event.SequenceNo},
			bigquery.QueryParameter{Name: txParam, Value: txValue},
			bigquery.QueryParameter{Name: payParam, Value: payloadJSON},
			bigquery.QueryParameter{Name: metaParam, Value: metadataJSON},
			bigquery.QueryParameter{Name: tsParam, Value: event.Created.UTC()},
		)
	}

	querySQL := fmt.Sprintf("INSERT INTO %s (id, aggregate_id, event_type, sequence_no, transaction_id, payload, metadata, created_at) VALUES %s",
		s.fullTableID(), strings.Join(valuePlaceholders, ", "))

	return querySQL, params, nil
}

// GetEvents retrieves all events for the given aggregate ID.
func (s *BigQueryEventStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	query := fmt.Sprintf("SELECT id, aggregate_id, event_type, sequence_no, transaction_id, payload, metadata, created_at FROM %s WHERE aggregate_id = @agg ORDER BY sequence_no ASC",
		s.fullTableID())

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "agg", Value: aggregateID},
	}

	return s.queryEnvelopes(ctx, q)
}

// GetEventsFromVersion retrieves events starting from the specified version.
func (s *BigQueryEventStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	query := fmt.Sprintf("SELECT id, aggregate_id, event_type, sequence_no, transaction_id, payload, metadata, created_at FROM %s WHERE aggregate_id = @agg AND sequence_no >= @from_ver ORDER BY sequence_no ASC",
		s.fullTableID())

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "agg", Value: aggregateID},
		{Name: "from_ver", Value: fromVersion},
	}

	return s.queryEnvelopes(ctx, q)
}

// GetEventsRange retrieves events within a version range.
// If fromVersion is -1, it defaults to 1. If toVersion is -1, all events from fromVersion onwards are returned.
func (s *BigQueryEventStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	if fromVersion == -1 {
		fromVersion = 1
	}

	if toVersion == -1 {
		return s.GetEventsFromVersion(ctx, aggregateID, fromVersion)
	}

	query := fmt.Sprintf("SELECT id, aggregate_id, event_type, sequence_no, transaction_id, payload, metadata, created_at FROM %s WHERE aggregate_id = @agg AND sequence_no >= @from_ver AND sequence_no <= @to_ver ORDER BY sequence_no ASC",
		s.fullTableID())

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "agg", Value: aggregateID},
		{Name: "from_ver", Value: fromVersion},
		{Name: "to_ver", Value: toVersion},
	}

	return s.queryEnvelopes(ctx, q)
}

// GetEventByID retrieves a specific event by its ID.
// Note: This performs a full table scan since id is not in the clustering key.
func (s *BigQueryEventStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	query := fmt.Sprintf("SELECT id, aggregate_id, event_type, sequence_no, transaction_id, payload, metadata, created_at FROM %s WHERE id = @event_id LIMIT 1",
		s.fullTableID())

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "event_id", Value: eventID},
	}

	it, err := q.Read(ctx)
	if err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("failed to query event by ID: %w", err)
	}

	var row bigqueryEventRow
	err = it.Next(&row)
	if err == iterator.Done {
		return domain.EventEnvelope[any]{}, domain.ErrEventNotFound
	}
	if err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("failed to read event row: %w", err)
	}

	return bigqueryRowToEnvelope(row)
}

// GetCurrentVersion returns the current version for the aggregate.
// Returns 0 if the aggregate doesn't exist.
func (s *BigQueryEventStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	query := fmt.Sprintf("SELECT COALESCE(MAX(sequence_no), 0) AS max_seq FROM %s WHERE aggregate_id = @agg",
		s.fullTableID())

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "agg", Value: aggregateID},
	}

	it, err := q.Read(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to query current version: %w", err)
	}

	var result struct {
		MaxSeq int64 `bigquery:"max_seq"`
	}
	err = it.Next(&result)
	if err == iterator.Done {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to read current version: %w", err)
	}

	return int(result.MaxSeq), nil
}

// Close closes the BigQuery event store (no-op since client is managed externally).
func (s *BigQueryEventStore) Close() error {
	return nil
}

func (s *BigQueryEventStore) queryEnvelopes(ctx context.Context, q *bigquery.Query) ([]domain.EventEnvelope[any], error) {
	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	var envelopes []domain.EventEnvelope[any]
	for {
		var row bigqueryEventRow
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read event row: %w", err)
		}

		env, err := bigqueryRowToEnvelope(row)
		if err != nil {
			return nil, fmt.Errorf("event row conversion failed (aggregate=%s, seq=%d): %w", row.AggregateID, row.SequenceNo, err)
		}
		envelopes = append(envelopes, env)
	}

	if envelopes == nil {
		return []domain.EventEnvelope[any]{}, nil
	}
	return envelopes, nil
}

// bigqueryEventRow is the persistence-layer DTO for BigQuery rows.
// Fields must stay in sync with the BigQuery table schema.
// TransactionID uses bigquery.NullString so that existing rows with NULL
// transaction_id decode without error.
type bigqueryEventRow struct {
	ID            string              `bigquery:"id"`
	AggregateID   string              `bigquery:"aggregate_id"`
	EventType     string              `bigquery:"event_type"`
	SequenceNo    int64               `bigquery:"sequence_no"`
	TransactionID bigquery.NullString `bigquery:"transaction_id"`
	Payload       string              `bigquery:"payload"`
	Metadata      string              `bigquery:"metadata"`
	CreatedAt     time.Time           `bigquery:"created_at"`
}

func marshalPayloadAndMetadata(env domain.EventEnvelope[any]) (string, string, error) {
	payload, err := toAnyMap(env.Payload)
	if err != nil {
		return "", "", fmt.Errorf("failed to convert payload: %w", err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	metadata := env.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return string(payloadJSON), string(metadataJSON), nil
}

func bigqueryRowToEnvelope(row bigqueryEventRow) (domain.EventEnvelope[any], error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(row.Payload), &payload); err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("%w: failed to unmarshal payload for event %s: %v", domain.ErrInvalidEvent, row.ID, err)
	}

	var metadata map[string]any
	if row.Metadata != "" {
		if err := json.Unmarshal([]byte(row.Metadata), &metadata); err != nil {
			return domain.EventEnvelope[any]{}, fmt.Errorf("%w: failed to unmarshal metadata for event %s: %v", domain.ErrInvalidEvent, row.ID, err)
		}
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}

	txID := ""
	if row.TransactionID.Valid {
		txID = row.TransactionID.StringVal
	}

	return domain.EventEnvelope[any]{
		ID:            row.ID,
		AggregateID:   row.AggregateID,
		EventType:     row.EventType,
		Payload:       map[string]any(payload),
		Created:       row.CreatedAt,
		SequenceNo:    int(row.SequenceNo),
		TransactionID: txID,
		Metadata:      metadata,
	}, nil
}
