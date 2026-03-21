package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

var _ domain.EventStore = (*DynamoEventStore)(nil)

const (
	dynamoEventIDIndex     = "event-id-index"
	dynamoMaxTransactItems = 100
	dynamoTimeFormat       = time.RFC3339Nano
)

// DynamoEventStore is a DynamoDB-based implementation of EventStore.
type DynamoEventStore struct {
	client    *dynamodb.Client
	tableName string
}

// NewDynamoEventStore creates a new DynamoDB-based event store.
// The table must already exist — DynamoDB tables are provisioned via IaC.
func NewDynamoEventStore(client *dynamodb.Client, tableName string) *DynamoEventStore {
	if client == nil {
		panic("dynamodb client must not be nil")
	}
	if tableName == "" {
		panic("table name must not be empty")
	}
	return &DynamoEventStore{
		client:    client,
		tableName: tableName,
	}
}

// Append appends events to the store for the given aggregate.
// If expectedVersion is not -1, optimistic concurrency control is enforced.
func (s *DynamoEventStore) Append(ctx context.Context, aggregateID string, expectedVersion int, events ...domain.EventEnvelope[any]) error {
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

	if expectedVersion == -1 {
		if len(events) > dynamoMaxTransactItems {
			return fmt.Errorf("%w: batch size %d exceeds DynamoDB limit of %d",
				domain.ErrInvalidEvent, len(events), dynamoMaxTransactItems)
		}
		return s.appendWithoutVersionCheck(ctx, events)
	}

	// Reserve 1 slot for the ConditionCheck item used in version verification
	maxEvents := dynamoMaxTransactItems - 1
	if len(events) > maxEvents {
		return fmt.Errorf("%w: batch size %d exceeds DynamoDB TransactWriteItems limit of %d (with version check)",
			domain.ErrInvalidEvent, len(events), maxEvents)
	}

	return s.appendWithVersionCheck(ctx, aggregateID, expectedVersion, events)
}

func (s *DynamoEventStore) appendWithoutVersionCheck(ctx context.Context, events []domain.EventEnvelope[any]) error {
	// Use BatchWriteItem for batches of up to 25 items
	const batchLimit = 25
	for i := 0; i < len(events); i += batchLimit {
		end := min(i+batchLimit, len(events))
		batch := events[i:end]

		writeRequests := make([]types.WriteRequest, len(batch))
		for j, event := range batch {
			item, err := envelopeToDynamoItem(event)
			if err != nil {
				return fmt.Errorf("%w: %v", domain.ErrInvalidEvent, err)
			}
			writeRequests[j] = types.WriteRequest{
				PutRequest: &types.PutRequest{Item: item},
			}
		}

		result, err := s.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				s.tableName: writeRequests,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to batch write events: %w", err)
		}
		if len(result.UnprocessedItems) > 0 {
			return fmt.Errorf("failed to write all events: %d items were not processed by DynamoDB",
				len(result.UnprocessedItems[s.tableName]))
		}
	}
	return nil
}

func (s *DynamoEventStore) appendWithVersionCheck(ctx context.Context, aggregateID string, expectedVersion int, events []domain.EventEnvelope[any]) error {
	// Build Put items for each event with attribute_not_exists to prevent duplicate (aggregate_id, sequence_no)
	transactItems := make([]types.TransactWriteItem, 0, len(events)+1)

	// Add an atomic ConditionCheck to verify the expected version exists.
	// This eliminates the TOCTOU race of a separate read-then-write.
	// For expectedVersion > 0, verify that an item at that sequence_no exists for this aggregate.
	if expectedVersion > 0 {
		transactItems = append(transactItems, types.TransactWriteItem{
			ConditionCheck: &types.ConditionCheck{
				TableName: &s.tableName,
				Key: map[string]types.AttributeValue{
					"aggregate_id": &types.AttributeValueMemberS{Value: aggregateID},
					"sequence_no":  &types.AttributeValueMemberN{Value: strconv.Itoa(expectedVersion)},
				},
				ConditionExpression: aws.String("attribute_exists(aggregate_id)"),
			},
		})
	}

	for _, event := range events {
		item, err := envelopeToDynamoItem(event)
		if err != nil {
			return fmt.Errorf("%w: %v", domain.ErrInvalidEvent, err)
		}
		transactItems = append(transactItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName:           &s.tableName,
				Item:                item,
				ConditionExpression: aws.String("attribute_not_exists(aggregate_id)"),
			},
		})
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		var txCancelled *types.TransactionCanceledException
		if errors.As(err, &txCancelled) {
			for _, reason := range txCancelled.CancellationReasons {
				if reason.Code != nil && *reason.Code == "ConditionalCheckFailed" {
					return fmt.Errorf("%w: expected version %d",
						domain.ErrConcurrencyConflict, expectedVersion)
				}
			}
			// Not a concurrency conflict — surface the actual transaction cancellation
			return fmt.Errorf("transaction cancelled for aggregate %s: %w", aggregateID, err)
		}
		return fmt.Errorf("failed to transact write events: %w", err)
	}

	return nil
}

// GetEvents retrieves all events for the given aggregate ID.
func (s *DynamoEventStore) GetEvents(ctx context.Context, aggregateID string) ([]domain.EventEnvelope[any], error) {
	input := &dynamodb.QueryInput{
		TableName:              &s.tableName,
		KeyConditionExpression: aws.String("aggregate_id = :aid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":aid": &types.AttributeValueMemberS{Value: aggregateID},
		},
		ScanIndexForward: aws.Bool(true),
	}

	return s.queryEnvelopes(ctx, input)
}

// GetEventsFromVersion retrieves events starting from the specified version.
func (s *DynamoEventStore) GetEventsFromVersion(ctx context.Context, aggregateID string, fromVersion int) ([]domain.EventEnvelope[any], error) {
	input := &dynamodb.QueryInput{
		TableName:              &s.tableName,
		KeyConditionExpression: aws.String("aggregate_id = :aid AND sequence_no >= :from"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":aid":  &types.AttributeValueMemberS{Value: aggregateID},
			":from": &types.AttributeValueMemberN{Value: strconv.Itoa(fromVersion)},
		},
		ScanIndexForward: aws.Bool(true),
	}

	return s.queryEnvelopes(ctx, input)
}

// GetEventsRange retrieves events within a version range.
// If fromVersion is -1, it defaults to 1. If toVersion is -1, all events from fromVersion onwards are returned.
func (s *DynamoEventStore) GetEventsRange(ctx context.Context, aggregateID string, fromVersion, toVersion int) ([]domain.EventEnvelope[any], error) {
	if fromVersion == -1 {
		fromVersion = 1
	}

	if toVersion == -1 {
		return s.GetEventsFromVersion(ctx, aggregateID, fromVersion)
	}

	input := &dynamodb.QueryInput{
		TableName:              &s.tableName,
		KeyConditionExpression: aws.String("aggregate_id = :aid AND sequence_no BETWEEN :from AND :to"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":aid":  &types.AttributeValueMemberS{Value: aggregateID},
			":from": &types.AttributeValueMemberN{Value: strconv.Itoa(fromVersion)},
			":to":   &types.AttributeValueMemberN{Value: strconv.Itoa(toVersion)},
		},
		ScanIndexForward: aws.Bool(true),
	}

	return s.queryEnvelopes(ctx, input)
}

// GetEventByID retrieves a specific event by its ID using the GSI.
func (s *DynamoEventStore) GetEventByID(ctx context.Context, eventID string) (domain.EventEnvelope[any], error) {
	indexName := dynamoEventIDIndex
	input := &dynamodb.QueryInput{
		TableName:              &s.tableName,
		IndexName:              &indexName,
		KeyConditionExpression: aws.String("id = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: eventID},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("failed to query event by ID: %w", err)
	}

	if len(result.Items) == 0 {
		return domain.EventEnvelope[any]{}, domain.ErrEventNotFound
	}

	return dynamoItemToEnvelope(result.Items[0])
}

// GetCurrentVersion returns the current version for the aggregate.
// Returns 0 if the aggregate doesn't exist.
func (s *DynamoEventStore) GetCurrentVersion(ctx context.Context, aggregateID string) (int, error) {
	input := &dynamodb.QueryInput{
		TableName:              &s.tableName,
		KeyConditionExpression: aws.String("aggregate_id = :aid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":aid": &types.AttributeValueMemberS{Value: aggregateID},
		},
		ScanIndexForward:     aws.Bool(false),
		Limit:                aws.Int32(1),
		ProjectionExpression: aws.String("sequence_no"),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to query current version: %w", err)
	}

	if len(result.Items) == 0 {
		return 0, nil
	}

	var seqNo int
	if err := attributevalue.Unmarshal(result.Items[0]["sequence_no"], &seqNo); err != nil {
		return 0, fmt.Errorf("failed to unmarshal sequence_no: %w", err)
	}

	return seqNo, nil
}

// Close closes the DynamoDB event store (no-op since AWS SDK client is managed externally).
func (s *DynamoEventStore) Close() error {
	return nil
}

func (s *DynamoEventStore) queryEnvelopes(ctx context.Context, input *dynamodb.QueryInput) ([]domain.EventEnvelope[any], error) {
	var envelopes []domain.EventEnvelope[any]

	paginator := dynamodb.NewQueryPaginator(s.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query events: %w", err)
		}
		for _, item := range page.Items {
			env, err := dynamoItemToEnvelope(item)
			if err != nil {
				return nil, err
			}
			envelopes = append(envelopes, env)
		}
	}

	if envelopes == nil {
		return []domain.EventEnvelope[any]{}, nil
	}
	return envelopes, nil
}

func envelopeToDynamoItem(env domain.EventEnvelope[any]) (map[string]types.AttributeValue, error) {
	payload, err := toAnyMap(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to convert payload: %w", err)
	}

	metadata := env.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}

	payloadAV, err := attributevalue.MarshalMap(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	metadataAV, err := attributevalue.MarshalMap(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	item := map[string]types.AttributeValue{
		"id":           &types.AttributeValueMemberS{Value: env.ID},
		"aggregate_id": &types.AttributeValueMemberS{Value: env.AggregateID},
		"event_type":   &types.AttributeValueMemberS{Value: env.EventType},
		"sequence_no":  &types.AttributeValueMemberN{Value: strconv.Itoa(env.SequenceNo)},
		"payload":      &types.AttributeValueMemberM{Value: payloadAV},
		"metadata":     &types.AttributeValueMemberM{Value: metadataAV},
		"created_at":   &types.AttributeValueMemberS{Value: env.Created.UTC().Format(dynamoTimeFormat)},
	}

	return item, nil
}

func dynamoItemToEnvelope(item map[string]types.AttributeValue) (domain.EventEnvelope[any], error) {
	var id, aggregateID, eventType, createdStr string
	var sequenceNo int

	if err := attributevalue.Unmarshal(item["id"], &id); err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("failed to unmarshal id: %w", err)
	}
	if err := attributevalue.Unmarshal(item["aggregate_id"], &aggregateID); err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("failed to unmarshal aggregate_id: %w", err)
	}
	if err := attributevalue.Unmarshal(item["event_type"], &eventType); err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("failed to unmarshal event_type: %w", err)
	}
	if err := attributevalue.Unmarshal(item["sequence_no"], &sequenceNo); err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("failed to unmarshal sequence_no: %w", err)
	}
	if err := attributevalue.Unmarshal(item["created_at"], &createdStr); err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("failed to unmarshal created_at: %w", err)
	}

	created, err := time.Parse(dynamoTimeFormat, createdStr)
	if err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("failed to parse created_at: %w", err)
	}

	var payload map[string]any
	payloadAV, ok := item["payload"]
	if !ok {
		return domain.EventEnvelope[any]{}, fmt.Errorf("missing payload attribute for event %s", id)
	}
	payloadM, ok := payloadAV.(*types.AttributeValueMemberM)
	if !ok {
		return domain.EventEnvelope[any]{}, fmt.Errorf("unexpected payload attribute type %T for event %s", payloadAV, id)
	}
	if err := attributevalue.UnmarshalMap(payloadM.Value, &payload); err != nil {
		return domain.EventEnvelope[any]{}, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	var metadata map[string]any
	if metaAV, ok := item["metadata"]; ok {
		if m, ok := metaAV.(*types.AttributeValueMemberM); ok {
			if err := attributevalue.UnmarshalMap(m.Value, &metadata); err != nil {
				return domain.EventEnvelope[any]{}, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}

	return domain.EventEnvelope[any]{
		ID:          id,
		AggregateID: aggregateID,
		EventType:   eventType,
		Payload:     map[string]any(payload),
		Created:     created,
		SequenceNo:  sequenceNo,
		Metadata:    metadata,
	}, nil
}

