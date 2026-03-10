package infrastructure_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/segmentio/ksuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

var (
	dynamoOnce      sync.Once
	dynamoContainer testcontainers.Container
	dynamoEndpoint  string
	dynamoSetupErr  error
)

func startDynamoContainer(t *testing.T) (string, error) {
	t.Helper()

	dynamoOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				dynamoSetupErr = fmt.Errorf("Docker not available: %v", r)
			}
		}()

		ctx := context.Background()
		req := testcontainers.ContainerRequest{
			Image:        "amazon/dynamodb-local:latest",
			ExposedPorts: []string{"8000/tcp"},
			Cmd:          []string{"-jar", "DynamoDBLocal.jar", "-inMemory", "-sharedDb"},
			WaitingFor:   wait.ForListeningPort("8000/tcp").WithStartupTimeout(30 * time.Second),
		}

		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		if err != nil {
			dynamoSetupErr = fmt.Errorf("failed to start DynamoDB container: %w", err)
			return
		}
		dynamoContainer = container

		host, err := container.Host(ctx)
		if err != nil {
			dynamoSetupErr = fmt.Errorf("failed to get container host: %w", err)
			_ = container.Terminate(ctx)
			return
		}

		port, err := container.MappedPort(ctx, "8000")
		if err != nil {
			dynamoSetupErr = fmt.Errorf("failed to get mapped port: %w", err)
			_ = container.Terminate(ctx)
			return
		}

		dynamoEndpoint = fmt.Sprintf("http://%s:%s", host, port.Port())
	})

	return dynamoEndpoint, dynamoSetupErr
}

func newDynamoClient(t *testing.T, endpoint string) *dynamodb.Client {
	t.Helper()
	return dynamodb.New(dynamodb.Options{
		Region:       "us-east-1",
		BaseEndpoint: &endpoint,
		Credentials:  credentials.NewStaticCredentialsProvider("dummy", "dummy", "dummy"),
	})
}

func createDynamoTable(t *testing.T, client *dynamodb.Client, tableName string) {
	t.Helper()
	ctx := context.Background()

	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: &tableName,
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("aggregate_id"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sequence_no"), KeyType: types.KeyTypeRange},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("aggregate_id"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sequence_no"), AttributeType: types.ScalarAttributeTypeN},
			{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("event-id-index"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(5),
			WriteCapacityUnits: aws.Int64(5),
		},
	})
	if err != nil {
		t.Fatalf("failed to create DynamoDB table: %v", err)
	}

	// Wait for table to become active
	waiter := dynamodb.NewTableExistsWaiter(client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: &tableName}, 30*time.Second); err != nil {
		t.Fatalf("failed waiting for table to become active: %v", err)
	}
}

func setupDynamoStore(t *testing.T) domain.EventStore {
	t.Helper()

	endpoint, err := startDynamoContainer(t)
	if err != nil {
		t.Skipf("skipping DynamoDB test: %v (Docker may not be available)", err)
	}

	client := newDynamoClient(t, endpoint)
	tableName := "events-" + ksuid.New().String()
	createDynamoTable(t, client, tableName)

	t.Cleanup(func() {
		if _, err := client.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{TableName: &tableName}); err != nil {
			t.Logf("warning: failed to delete test table %s: %v", tableName, err)
		}
	})

	return infrastructure.NewDynamoEventStore(client, tableName)
}

func setupDynamoStoreWithEvents(t *testing.T) domain.EventStore {
	t.Helper()
	store := setupDynamoStore(t)
	ctx := context.Background()

	event := createTestEvent("agg-3", "event-1", "test.created", 1)
	if err := store.Append(ctx, "agg-3", -1, event); err != nil {
		t.Fatalf("failed to setup store: %v", err)
	}
	return store
}

func setupDynamoStoreWithMultipleEvents(t *testing.T) domain.EventStore {
	t.Helper()
	store := setupDynamoStore(t)
	ctx := context.Background()

	events := []domain.EventEnvelope[any]{
		createTestEvent("agg-4", "event-1", "test.created", 1),
		createTestEvent("agg-4", "event-2", "test.updated", 2),
		createTestEvent("agg-4", "event-3", "test.updated", 3),
	}

	if err := store.Append(ctx, "agg-4", -1, events...); err != nil {
		t.Fatalf("failed to setup store: %v", err)
	}
	return store
}

func TestDynamoStore_Integration(t *testing.T) {
	t.Parallel()

	t.Run("full workflow", func(t *testing.T) {
		t.Parallel()

		store := setupDynamoStore(t)
		defer store.Close()

		ctx := context.Background()
		aggregateID := "test-aggregate"

		events := []domain.EventEnvelope[any]{
			createTestEvent(aggregateID, "event-1", "test.created", 1),
			createTestEvent(aggregateID, "event-2", "test.updated", 2),
		}

		if err := store.Append(ctx, aggregateID, -1, events...); err != nil {
			t.Fatalf("failed to append events: %v", err)
		}

		retrieved, err := store.GetEvents(ctx, aggregateID)
		if err != nil {
			t.Fatalf("failed to get events: %v", err)
		}
		if len(retrieved) != 2 {
			t.Fatalf("expected 2 events, got %d", len(retrieved))
		}
		if retrieved[0].SequenceNo != 1 {
			t.Errorf("expected first event version 1, got %d", retrieved[0].SequenceNo)
		}
		if retrieved[1].SequenceNo != 2 {
			t.Errorf("expected second event version 2, got %d", retrieved[1].SequenceNo)
		}

		version, err := store.GetCurrentVersion(ctx, aggregateID)
		if err != nil {
			t.Fatalf("failed to get current version: %v", err)
		}
		if version != 2 {
			t.Errorf("expected current version 2, got %d", version)
		}

		newEvent := createTestEvent(aggregateID, "event-3", "test.updated", 3)
		if err := store.Append(ctx, aggregateID, 2, newEvent); err != nil {
			t.Fatalf("failed to append with version check: %v", err)
		}

		allEvents, err := store.GetEvents(ctx, aggregateID)
		if err != nil {
			t.Fatalf("failed to get all events: %v", err)
		}
		if len(allEvents) != 3 {
			t.Fatalf("expected 3 events, got %d", len(allEvents))
		}

		fromVersion2, err := store.GetEventsFromVersion(ctx, aggregateID, 2)
		if err != nil {
			t.Fatalf("failed to get events from version: %v", err)
		}
		if len(fromVersion2) != 2 {
			t.Fatalf("expected 2 events from version 2, got %d", len(fromVersion2))
		}

		event, err := store.GetEventByID(ctx, "event-2")
		if err != nil {
			t.Fatalf("failed to get event by ID: %v", err)
		}
		if event.ID != "event-2" {
			t.Errorf("expected event ID event-2, got %s", event.ID)
		}
	})

	t.Run("concurrency conflict", func(t *testing.T) {
		t.Parallel()

		store := setupDynamoStoreWithEvents(t)
		defer store.Close()

		ctx := context.Background()

		event := createTestEvent("agg-3", "event-2", "test.updated", 2)
		err := store.Append(ctx, "agg-3", 999, event)
		if err == nil {
			t.Fatal("expected concurrency conflict error, got nil")
		}
		if !errors.Is(err, domain.ErrConcurrencyConflict) {
			t.Fatalf("expected ErrConcurrencyConflict, got %v", err)
		}
	})

	t.Run("multiple aggregates", func(t *testing.T) {
		t.Parallel()

		store := setupDynamoStore(t)
		defer store.Close()

		ctx := context.Background()

		if err := store.Append(ctx, "agg-1", -1, createTestEvent("agg-1", "event-1", "test.created", 1)); err != nil {
			t.Fatalf("failed to append events for agg-1: %v", err)
		}
		if err := store.Append(ctx, "agg-2", -1,
			createTestEvent("agg-2", "event-2", "test.created", 1),
			createTestEvent("agg-2", "event-3", "test.updated", 2),
		); err != nil {
			t.Fatalf("failed to append events for agg-2: %v", err)
		}

		agg1Events, err := store.GetEvents(ctx, "agg-1")
		if err != nil {
			t.Fatalf("failed to get events for agg-1: %v", err)
		}
		if len(agg1Events) != 1 {
			t.Errorf("expected 1 event for agg-1, got %d", len(agg1Events))
		}

		agg2Events, err := store.GetEvents(ctx, "agg-2")
		if err != nil {
			t.Fatalf("failed to get events for agg-2: %v", err)
		}
		if len(agg2Events) != 2 {
			t.Errorf("expected 2 events for agg-2, got %d", len(agg2Events))
		}
	})

	t.Run("get event by ID not found", func(t *testing.T) {
		t.Parallel()

		store := setupDynamoStore(t)
		defer store.Close()

		ctx := context.Background()
		_, err := store.GetEventByID(ctx, "nonexistent")
		if !errors.Is(err, domain.ErrEventNotFound) {
			t.Fatalf("expected ErrEventNotFound, got %v", err)
		}
	})

	t.Run("payload round-trip fidelity", func(t *testing.T) {
		t.Parallel()

		store := setupDynamoStore(t)
		defer store.Close()

		ctx := context.Background()
		payload := map[string]any{
			"name":   "John",
			"age":    float64(30),
			"active": true,
		}

		event := domain.EventEnvelope[any]{
			ID:          "payload-test",
			AggregateID: "agg-payload",
			EventType:   "test.created",
			Payload:     payload,
			Created:     time.Now(),
			SequenceNo:  1,
		}

		if err := store.Append(ctx, "agg-payload", -1, event); err != nil {
			t.Fatalf("failed to append: %v", err)
		}

		retrieved, err := store.GetEventByID(ctx, "payload-test")
		if err != nil {
			t.Fatalf("failed to get event: %v", err)
		}

		p, ok := retrieved.Payload.(map[string]any)
		if !ok {
			t.Fatalf("payload is not map[string]any, got %T", retrieved.Payload)
		}
		if p["name"] != "John" {
			t.Errorf("expected name John, got %v", p["name"])
		}
		// DynamoDB attributevalue unmarshals numbers as float64
		if p["age"] != float64(30) {
			t.Errorf("expected age 30, got %v", p["age"])
		}
		if p["active"] != true {
			t.Errorf("expected active true, got %v", p["active"])
		}
	})

	t.Run("created timestamp round-trip", func(t *testing.T) {
		t.Parallel()

		store := setupDynamoStore(t)
		defer store.Close()

		ctx := context.Background()
		now := time.Now().UTC().Truncate(time.Millisecond)

		event := domain.EventEnvelope[any]{
			ID:          "ts-test",
			AggregateID: "agg-ts",
			EventType:   "test.created",
			Payload:     map[string]any{"test": "data"},
			Created:     now,
			SequenceNo:  1,
		}

		if err := store.Append(ctx, "agg-ts", -1, event); err != nil {
			t.Fatalf("failed to append: %v", err)
		}

		retrieved, err := store.GetEventByID(ctx, "ts-test")
		if err != nil {
			t.Fatalf("failed to get event: %v", err)
		}

		if !retrieved.Created.Truncate(time.Millisecond).Equal(now) {
			t.Errorf("expected Created %v, got %v", now, retrieved.Created)
		}
	})

	t.Run("metadata round-trip", func(t *testing.T) {
		t.Parallel()

		store := setupDynamoStore(t)
		defer store.Close()

		ctx := context.Background()
		event := domain.EventEnvelope[any]{
			ID:          "meta-test",
			AggregateID: "agg-meta",
			EventType:   "test.created",
			Payload:     map[string]any{"test": "data"},
			Created:     time.Now(),
			SequenceNo:  1,
			Metadata: map[string]any{
				"correlation_id": "corr-123",
				"user_id":        "user-456",
			},
		}

		if err := store.Append(ctx, "agg-meta", -1, event); err != nil {
			t.Fatalf("failed to append: %v", err)
		}

		retrieved, err := store.GetEventByID(ctx, "meta-test")
		if err != nil {
			t.Fatalf("failed to get event: %v", err)
		}

		if retrieved.Metadata == nil {
			t.Fatal("expected non-nil metadata")
		}
		if retrieved.Metadata["correlation_id"] != "corr-123" {
			t.Errorf("expected correlation_id corr-123, got %v", retrieved.Metadata["correlation_id"])
		}
		if retrieved.Metadata["user_id"] != "user-456" {
			t.Errorf("expected user_id user-456, got %v", retrieved.Metadata["user_id"])
		}
	})

	t.Run("struct payload round-trip via ToAnyEnvelope", func(t *testing.T) {
		t.Parallel()

		type TestPayload struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}

		store := setupDynamoStore(t)
		defer store.Close()

		ctx := context.Background()
		original := domain.NewEventEnvelope(TestPayload{Name: "test", Value: 42}, "agg-struct", "test.created", 1)
		anyEnvelope := domain.ToAnyEnvelope(original)

		if err := store.Append(ctx, "agg-struct", -1, anyEnvelope); err != nil {
			t.Fatalf("failed to append: %v", err)
		}

		retrieved, err := store.GetEventByID(ctx, anyEnvelope.ID)
		if err != nil {
			t.Fatalf("failed to get event: %v", err)
		}

		p, ok := retrieved.Payload.(map[string]any)
		if !ok {
			t.Fatalf("payload is not map[string]any, got %T", retrieved.Payload)
		}
		if p["name"] != "test" {
			t.Errorf("expected name test, got %v", p["name"])
		}
		if p["value"] != float64(42) {
			t.Errorf("expected value 42, got %v", p["value"])
		}
	})
}

func TestDynamoStore_GetEventsRange(t *testing.T) {
	t.Parallel()

	t.Run("range retrieval", func(t *testing.T) {
		t.Parallel()

		store := setupDynamoStore(t)
		defer store.Close()

		ctx := context.Background()
		aggregateID := "range-test"

		events := []domain.EventEnvelope[any]{
			createTestEvent(aggregateID, "event-1", "test.created", 1),
			createTestEvent(aggregateID, "event-2", "test.updated", 2),
			createTestEvent(aggregateID, "event-3", "test.updated", 3),
			createTestEvent(aggregateID, "event-4", "test.updated", 4),
		}

		if err := store.Append(ctx, aggregateID, -1, events...); err != nil {
			t.Fatalf("failed to append events: %v", err)
		}

		rangeEvents, err := store.GetEventsRange(ctx, aggregateID, 2, 3)
		if err != nil {
			t.Fatalf("failed to get events range: %v", err)
		}
		if len(rangeEvents) != 2 {
			t.Fatalf("expected 2 events in range, got %d", len(rangeEvents))
		}
		if rangeEvents[0].SequenceNo != 2 {
			t.Errorf("expected first event version 2, got %d", rangeEvents[0].SequenceNo)
		}
		if rangeEvents[1].SequenceNo != 3 {
			t.Errorf("expected second event version 3, got %d", rangeEvents[1].SequenceNo)
		}

		// Test with default fromVersion
		allFromStart, err := store.GetEventsRange(ctx, aggregateID, -1, 2)
		if err != nil {
			t.Fatalf("failed to get events range: %v", err)
		}
		if len(allFromStart) != 2 {
			t.Fatalf("expected 2 events from start to version 2, got %d", len(allFromStart))
		}

		// Test with toVersion -1
		allFromVersion2, err := store.GetEventsRange(ctx, aggregateID, 2, -1)
		if err != nil {
			t.Fatalf("failed to get events range: %v", err)
		}
		if len(allFromVersion2) != 3 {
			t.Fatalf("expected 3 events from version 2 onwards, got %d", len(allFromVersion2))
		}
	})
}
