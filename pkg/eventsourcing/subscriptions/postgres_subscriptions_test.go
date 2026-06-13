package subscriptions_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/subscriptions"
)

var (
	subsPgOnce     sync.Once
	subsPgBaseDSN  string
	subsPgSetupErr error
)

// startSubscriptionsPostgres provisions one Postgres instance for this test
// binary (POSTGRES_TEST_DSN bypasses the container), mirroring the
// infrastructure package's setup.
func startSubscriptionsPostgres(t *testing.T) (string, error) {
	t.Helper()

	if dsn := os.Getenv("POSTGRES_TEST_DSN"); dsn != "" {
		return dsn, nil
	}

	subsPgOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				subsPgSetupErr = fmt.Errorf("Docker not available: %v", r)
			}
		}()

		ctx := context.Background()
		req := testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "pericarp",
				"POSTGRES_PASSWORD": "pericarp",
				"POSTGRES_DB":       "pericarp",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60 * time.Second),
		}
		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		if err != nil {
			subsPgSetupErr = fmt.Errorf("failed to start Postgres container: %w", err)
			return
		}
		host, err := container.Host(ctx)
		if err != nil {
			subsPgSetupErr = fmt.Errorf("failed to get container host: %w", err)
			_ = container.Terminate(ctx)
			return
		}
		port, err := container.MappedPort(ctx, "5432")
		if err != nil {
			subsPgSetupErr = fmt.Errorf("failed to get mapped port: %w", err)
			_ = container.Terminate(ctx)
			return
		}
		subsPgBaseDSN = fmt.Sprintf("postgres://pericarp:pericarp@%s:%s/pericarp?sslmode=disable", host, port.Port())

		for i := range 20 {
			db, err := sql.Open("pgx", subsPgBaseDSN)
			if err == nil {
				err = db.Ping()
				_ = db.Close()
			}
			if err == nil {
				return
			}
			if i == 19 {
				subsPgSetupErr = fmt.Errorf("Postgres not ready after probing: %w", err)
				_ = container.Terminate(ctx)
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	})

	return subsPgBaseDSN, subsPgSetupErr
}

// newPostgresFixture provisions schema-isolated event, checkpoint, parking,
// and projection tables on Postgres and returns the schema-scoped DSN for
// listeners. Tests using it deliberately avoid t.Parallel(): the feed's
// commit-visibility guard is cluster-wide, so concurrent tests holding write
// transactions would stall each other's feeds.
func newPostgresFixture(t *testing.T) (*gorm.DB, string, domain.EventStore, *subscriptions.GormCheckpointStore) {
	t.Helper()

	dsn, err := startSubscriptionsPostgres(t)
	if err != nil {
		if os.Getenv("PERICARP_REQUIRE_DOCKER_TESTS") != "" {
			t.Fatalf("PERICARP_REQUIRE_DOCKER_TESTS is set but the Postgres container failed to start: %v", err)
		}
		t.Skipf("skipping Postgres test: %v (Docker may not be available)", err)
	}

	schema := "s_" + strings.ToLower(ksuid.New().String())
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("failed to open admin connection: %v", err)
	}
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	scopedDSN := dsn + sep + "search_path=" + schema
	db, err := gorm.Open(postgres.Open(scopedDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open schema-scoped connection: %v", err)
	}

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Logf("warning: failed to drop schema %s: %v", schema, err)
		}
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	store, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	checkpoints, err := subscriptions.NewGormCheckpointStore(db)
	if err != nil {
		t.Fatalf("failed to create checkpoint store: %v", err)
	}
	if err := db.AutoMigrate(&projectionRow{}); err != nil {
		t.Fatalf("failed to migrate projection table: %v", err)
	}
	return db, scopedDSN, store, checkpoints
}

// TestPostgresSubscriptions_TwoReplicasProcessExactlyOnce is the epic's
// replica-coordination criterion: two subscriber processes with the same name
// against one database process each event exactly once between them, via
// FOR UPDATE SKIP LOCKED on the checkpoint row — active/passive failover with
// no leader election.
func TestPostgresSubscriptions_TwoReplicasProcessExactlyOnce(t *testing.T) {
	db, _, store, checkpoints := newPostgresFixture(t)
	const total = 20
	appendNumberedEvents(t, store, 1, total)

	mk := func() *subscriptions.Subscriber {
		sub, err := subscriptions.NewSubscriber("replicated", store, checkpoints,
			projectingHandler(t, ""),
			subscriptions.WithPollInterval(subscriptionTestPollInterval),
			subscriptions.WithBatchSize(3))
		if err != nil {
			t.Fatalf("failed to create subscriber: %v", err)
		}
		return sub
	}
	stopA := runSubscriber(t, mk())
	stopB := runSubscriber(t, mk())
	waitForCheckpoint(t, checkpoints, "replicated", total)
	stopA()
	stopB()

	got := projectedEventIDs(t, db)
	if len(got) != total {
		t.Fatalf("expected %d projection rows (each event exactly once across replicas), got %d: %v", total, len(got), got)
	}
	seen := make(map[string]bool, total)
	for _, id := range got {
		if seen[id] {
			t.Fatalf("event %s processed more than once across replicas: %v", id, got)
		}
		seen[id] = true
	}
}

// TestPostgresSubscriptions_NotifyWakesListeningSubscriber proves the
// LISTEN/NOTIFY path end to end: the poll interval is far beyond the test
// deadline, so only the event store's NOTIFY-on-commit reaching the listener
// can get the events processed in time.
func TestPostgresSubscriptions_NotifyWakesListeningSubscriber(t *testing.T) {
	db, scopedDSN, store, checkpoints := newPostgresFixture(t)

	listener, err := subscriptions.NewPostgresListener(scopedDSN,
		subscriptions.WithListenerReconnectDelay(100*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	listenerCtx, cancelListener := context.WithCancel(context.Background())
	listenerDone := make(chan error, 1)
	go func() { listenerDone <- listener.Run(listenerCtx) }()
	defer func() {
		cancelListener()
		select {
		case <-listenerDone:
		case <-time.After(10 * time.Second):
			t.Error("listener did not stop within 10s")
		}
	}()

	// Wait until the LISTEN is established: a manual NOTIFY must reach the
	// wake channel (the buffered token is consumed here, before the
	// subscriber takes over the channel).
	waitFor(t, 10*time.Second, func() bool {
		if err := db.Exec("SELECT pg_notify(?, '')", infrastructure.PostgresNotifyChannel).Error; err != nil {
			return false
		}
		select {
		case <-listener.Wake():
			return true
		case <-time.After(100 * time.Millisecond):
			return false
		}
	}, "listener to establish LISTEN")

	sub, err := subscriptions.NewSubscriber("listening", store, checkpoints,
		projectingHandler(t, ""),
		subscriptions.WithPollInterval(time.Hour),
		subscriptions.WithWakeSignal(listener.Wake()))
	if err != nil {
		t.Fatalf("failed to create subscriber: %v", err)
	}
	stop := runSubscriber(t, sub)
	defer stop()

	// Committing through the event store fires NOTIFY; the subscriber must
	// process the events long before its one-hour poll.
	appendNumberedEvents(t, store, 1, 3)
	waitForCheckpoint(t, checkpoints, "listening", 3)

	got := projectedEventIDs(t, db)
	if len(got) != 3 {
		t.Fatalf("expected 3 events processed via LISTEN/NOTIFY, got %v", got)
	}
}
