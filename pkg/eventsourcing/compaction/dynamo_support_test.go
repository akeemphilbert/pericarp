package compaction_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
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

// errNoDocker means DynamoDB Local could not be started, so the one scenario
// that names a DynamoDB store is skipped rather than failed — unless CI has
// declared Docker mandatory, matching the policy the store's own tests use.
var errNoDocker = errors.New("DynamoDB Local is unavailable")

var (
	dynamoOnce     sync.Once
	dynamoEndpoint string
	dynamoErr      error
)

// newDynamoStore starts (once per test binary) a DynamoDB Local container and
// returns a store over a fresh table in it. The compaction contract only asks
// this store to hold an event and be refused, but it has to be a real store:
// the point of the scenario is that a store which cannot retire events is
// turned away, and a fake would prove nothing about the real one.
func newDynamoStore(ctx context.Context) (domain.EventStore, func(), error) {
	dynamoOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				dynamoErr = fmt.Errorf("%w: %v", errNoDocker, r)
			}
		}()

		container, err := testcontainers.GenericContainer(context.Background(), testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "amazon/dynamodb-local:latest",
				ExposedPorts: []string{"8000/tcp"},
				Cmd:          []string{"-jar", "DynamoDBLocal.jar", "-inMemory", "-sharedDb"},
				WaitingFor:   wait.ForListeningPort("8000/tcp").WithStartupTimeout(30 * time.Second),
			},
			Started: true,
		})
		if err != nil {
			dynamoErr = fmt.Errorf("%w: %v", errNoDocker, err)
			return
		}

		host, err := container.Host(context.Background())
		if err != nil {
			dynamoErr = fmt.Errorf("%w: %v", errNoDocker, err)
			return
		}
		port, err := container.MappedPort(context.Background(), "8000")
		if err != nil {
			dynamoErr = fmt.Errorf("%w: %v", errNoDocker, err)
			return
		}
		dynamoEndpoint = fmt.Sprintf("http://%s:%s", host, port.Port())

		// The TCP port opens before the service answers API calls, so probe
		// until a real request succeeds.
		probe := dynamoClient(dynamoEndpoint)
		for attempt := 0; ; attempt++ {
			if _, err := probe.ListTables(context.Background(), &dynamodb.ListTablesInput{}); err == nil {
				return
			} else if attempt == 9 {
				dynamoErr = fmt.Errorf("%w: not ready after probing: %v", errNoDocker, err)
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	})

	if dynamoErr != nil {
		if os.Getenv("PERICARP_REQUIRE_DOCKER_TESTS") != "" {
			return nil, nil, fmt.Errorf("PERICARP_REQUIRE_DOCKER_TESTS is set: %w", dynamoErr)
		}
		return nil, nil, dynamoErr
	}

	client := dynamoClient(dynamoEndpoint)
	table := "compaction-events-" + ksuid.New().String()
	if err := createDynamoTable(ctx, client, table); err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		_, _ = client.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{TableName: &table})
	}
	return infrastructure.NewDynamoEventStore(client, table), cleanup, nil
}

func dynamoClient(endpoint string) *dynamodb.Client {
	return dynamodb.New(dynamodb.Options{
		Region:       "us-east-1",
		BaseEndpoint: &endpoint,
		Credentials:  credentials.NewStaticCredentialsProvider("dummy", "dummy", "dummy"),
	})
}

func createDynamoTable(ctx context.Context, client *dynamodb.Client, table string) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: &table,
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("aggregate_id"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sequence_no"), KeyType: types.KeyTypeRange},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("aggregate_id"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sequence_no"), AttributeType: types.ScalarAttributeTypeN},
			{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName:             aws.String("event-id-index"),
			KeySchema:             []types.KeySchemaElement{{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash}},
			Projection:            &types.Projection{ProjectionType: types.ProjectionTypeAll},
			ProvisionedThroughput: &types.ProvisionedThroughput{ReadCapacityUnits: aws.Int64(5), WriteCapacityUnits: aws.Int64(5)},
		}},
		ProvisionedThroughput: &types.ProvisionedThroughput{ReadCapacityUnits: aws.Int64(5), WriteCapacityUnits: aws.Int64(5)},
	})
	if err != nil {
		return fmt.Errorf("create DynamoDB table: %w", err)
	}

	if err := dynamodb.NewTableExistsWaiter(client).Wait(ctx, &dynamodb.DescribeTableInput{TableName: &table}, 30*time.Second); err != nil {
		return fmt.Errorf("wait for DynamoDB table: %w", err)
	}
	return nil
}
