// Package main sketches the recommended way to combine Postgres and DynamoDB:
// Postgres is the authoritative event store (it provides the global ordered
// feed and crash-safe subscriptions), and DynamoDB holds a read model built
// by a background subscriber. Crash recovery happens from Postgres — after a
// restart the subscriber resumes from its checkpoint and re-applies events to
// DynamoDB.
//
// The DynamoDB projection is at-least-once, NOT exactly-once: a DynamoDB write
// cannot join the subscriber's Postgres batch transaction, so a crash mid-batch
// redelivers events. The handler is therefore made idempotent with a
// conditional write keyed on the global feed position — replays are no-ops and
// application stays monotonic.
//
// Run with live infrastructure:
//
//	POSTGRES_DSN=postgres://... DYNAMO_PROJECTION_TABLE=accounts \
//	  AWS_REGION=us-east-1 go run ./examples/projection/
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/akeemphilbert/pericarp/pkg/ddd"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/application"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	esInfra "github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/subscriptions"
)

// Account is a minimal event-sourced aggregate. Its events are the source of
// truth in Postgres; the DynamoDB row is a derived projection.
type Account struct {
	*ddd.BaseEntity
}

// NewAccount creates an account and records its first event.
func NewAccount(id, name string) (*Account, error) {
	account := &Account{BaseEntity: ddd.NewBaseEntity(id)}
	if err := account.RecordEvent(map[string]any{"name": name}, "account.created"); err != nil {
		return nil, err
	}
	return account, nil
}

// Rename records a rename event.
func (a *Account) Rename(name string) error {
	return a.RecordEvent(map[string]any{"name": name}, "account.renamed")
}

// createAccount persists an account through the unit of work. This is the only
// write path: one atomic Postgres commit. No DynamoDB write happens here — the
// projection is built asynchronously by the subscriber, so the write side
// never has to coordinate two databases.
func createAccount(ctx context.Context, eventStore domain.EventStore, id, name string) error {
	account, err := NewAccount(id, name)
	if err != nil {
		return err
	}
	// nil dispatcher: projections are handled by the background subscriber,
	// not by synchronous in-commit dispatch.
	uow := application.NewSimpleUnitOfWork(eventStore, nil)
	if err := uow.Track(account); err != nil {
		return err
	}
	return uow.Commit(ctx)
}

// newProjectionHandler returns the subscriber handler that projects events into
// a DynamoDB table keyed by aggregate id.
//
// Idempotency is the whole point. We deliberately do NOT use
// subscriptions.TxFromContext here: the batch's transaction is a Postgres
// transaction, and a DynamoDB write cannot enlist in it. So redelivery after a
// crash is expected, and the conditional write below makes each event safe to
// apply more than once:
//
//   - applied_position records the global feed position last applied to the row.
//   - The UpdateItem only fires when the incoming position is greater, so a
//     replay of an already-applied event fails the condition and is a no-op,
//     and application is monotonic even if a batch is partially redelivered.
func newProjectionHandler(ddb *dynamodb.Client, table string) subscriptions.Handler {
	return func(ctx context.Context, event domain.EventEnvelope[any]) error {
		// Payloads round-trip through JSONB, so they come back as map[string]any.
		payload, _ := event.Payload.(map[string]any)
		name, _ := payload["name"].(string)

		_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName: aws.String(table),
			Key: map[string]ddbtypes.AttributeValue{
				"pk": &ddbtypes.AttributeValueMemberS{Value: event.AggregateID},
			},
			ConditionExpression: aws.String(
				"attribute_not_exists(applied_position) OR applied_position < :pos"),
			UpdateExpression: aws.String(
				"SET #name = :name, applied_position = :pos, last_event_type = :etype"),
			ExpressionAttributeNames: map[string]string{
				"#name": "name", // "name" is a DynamoDB reserved word
			},
			ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
				":name":  &ddbtypes.AttributeValueMemberS{Value: name},
				":pos":   &ddbtypes.AttributeValueMemberN{Value: strconv.FormatInt(event.Position, 10)},
				":etype": &ddbtypes.AttributeValueMemberS{Value: event.EventType},
			},
		})
		if err != nil {
			// The condition failing means this event (or a newer one) was
			// already applied — a redelivery. That is success, not an error.
			var alreadyApplied *ddbtypes.ConditionalCheckFailedException
			if errors.As(err, &alreadyApplied) {
				return nil
			}
			// Any other error fails the event. The subscriber retries it with
			// backoff and, after exhaustion, parks it (WithParkingLot below)
			// so the events behind it keep flowing.
			return fmt.Errorf("project %s@%d into dynamo: %w",
				event.AggregateID, event.Position, err)
		}
		return nil
	}
}

// runProjection wires Postgres (source of truth) to a DynamoDB projection and
// runs the subscriber until ctx is cancelled.
func runProjection(ctx context.Context, pgDSN, table string, ddb *dynamodb.Client) error {
	db, err := gorm.Open(postgres.Open(pgDSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}

	// Source of truth + global ordered feed. Auto-migrates the events table
	// (and on Postgres the position sequence + xact_id visibility guard).
	eventStore, err := esInfra.NewGormEventStore(db)
	if err != nil {
		return fmt.Errorf("event store: %w", err)
	}

	// Checkpoint + parking tables live in the same Postgres database. The
	// checkpoint advance commits in the batch transaction; the DynamoDB write
	// does not (that is why the handler is idempotent).
	checkpoints, err := subscriptions.NewGormCheckpointStore(db)
	if err != nil {
		return fmt.Errorf("checkpoint store: %w", err)
	}
	parking, err := subscriptions.NewGormParkingLot(db)
	if err != nil {
		return fmt.Errorf("parking lot: %w", err)
	}

	// Wake on commit via Postgres LISTEN/NOTIFY; polling stays the floor, so a
	// missed notification costs at most one poll interval, never correctness.
	listener, err := subscriptions.NewPostgresListener(pgDSN)
	if err != nil {
		return fmt.Errorf("listener: %w", err)
	}
	go func() {
		if err := listener.Run(ctx); err != nil {
			log.Printf("listener stopped: %v", err)
		}
	}()

	sub, err := subscriptions.NewSubscriber(
		"account-dynamo-projection", // checkpoint name; replicas share it
		eventStore,
		checkpoints,
		newProjectionHandler(ddb, table),
		subscriptions.WithParkingLot(parking),
		subscriptions.WithWakeSignal(listener.Subscribe()),
		subscriptions.WithBatchSize(200),
	)
	if err != nil {
		return fmt.Errorf("subscriber: %w", err)
	}

	// Run resumes from the checkpoint — the same code path for a fresh start
	// and for recovery after a crash. It returns nil on ctx cancellation.
	return sub.Run(ctx)
}

func main() {
	pgDSN := os.Getenv("POSTGRES_DSN")
	table := os.Getenv("DYNAMO_PROJECTION_TABLE")
	if pgDSN == "" || table == "" {
		fmt.Println("set POSTGRES_DSN and DYNAMO_PROJECTION_TABLE; this example needs live Postgres + DynamoDB")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	ddb := dynamodb.NewFromConfig(awsCfg)

	// Write some events (the source of truth). In a real service this happens
	// on the request path; the subscriber below projects them asynchronously.
	store, err := gorm.Open(postgres.Open(pgDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	eventStore, err := esInfra.NewGormEventStore(store)
	if err != nil {
		log.Fatalf("event store: %v", err)
	}
	if err := createAccount(ctx, eventStore, "acct-1", "Acme Inc"); err != nil {
		log.Fatalf("create account: %v", err)
	}

	log.Println("running projection; ctrl-c to stop")
	if err := runProjection(ctx, pgDSN, table, ddb); err != nil {
		log.Fatalf("projection: %v", err)
	}
}
