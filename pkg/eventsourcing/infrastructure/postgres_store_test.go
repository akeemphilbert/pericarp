package infrastructure_test

import (
	"context"
	"database/sql"
	"errors"
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
)

var (
	pgOnce      sync.Once
	pgContainer testcontainers.Container
	pgBaseDSN   string
	pgSetupErr  error
)

// startPostgresContainer provisions a shared Postgres instance for the test
// binary. POSTGRES_TEST_DSN (a postgres:// URL) bypasses the container.
func startPostgresContainer(t *testing.T) (string, error) {
	t.Helper()

	if dsn := os.Getenv("POSTGRES_TEST_DSN"); dsn != "" {
		return dsn, nil
	}

	pgOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				pgSetupErr = fmt.Errorf("Docker not available: %v", r)
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
			pgSetupErr = fmt.Errorf("failed to start Postgres container: %w", err)
			return
		}
		pgContainer = container

		host, err := container.Host(ctx)
		if err != nil {
			pgSetupErr = fmt.Errorf("failed to get container host: %w", err)
			_ = container.Terminate(ctx)
			return
		}
		port, err := container.MappedPort(ctx, "5432")
		if err != nil {
			pgSetupErr = fmt.Errorf("failed to get mapped port: %w", err)
			_ = container.Terminate(ctx)
			return
		}

		pgBaseDSN = fmt.Sprintf("postgres://pericarp:pericarp@%s:%s/pericarp?sslmode=disable", host, port.Port())

		// Probe readiness — the log line can appear before connections are accepted.
		for i := range 20 {
			db, err := sql.Open("pgx", pgBaseDSN)
			if err == nil {
				err = db.Ping()
				_ = db.Close()
			}
			if err == nil {
				return
			}
			if i == 19 {
				pgSetupErr = fmt.Errorf("Postgres not ready after probing: %w", err)
				_ = container.Terminate(ctx)
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	})

	return pgBaseDSN, pgSetupErr
}

// setupPostgresDB returns a GORM connection scoped to a fresh schema so tests
// don't share an events table. Skips when no Postgres is available (unless
// PERICARP_REQUIRE_DOCKER_TESTS is set).
//
// The Postgres tests deliberately do NOT call t.Parallel(): the ReadAfter
// commit-visibility guard (xact_id < pg_snapshot_xmin) is cluster-wide, so a
// test that holds a transaction open stalls every other test's feed even
// across schemas — exactly the behavior the guard exists to provide.
func setupPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn, err := startPostgresContainer(t)
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
	db, err := gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), &gorm.Config{
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

	return db
}

func TestPostgresStore_ReadAfter_RoundTrip(t *testing.T) {
	db := setupPostgresDB(t)
	ctx := context.Background()

	store, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to create gorm event store: %v", err)
	}
	defer func() { _ = store.Close() }()

	want := appendFeedFixture(t, store)

	events, err := store.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, want)

	var lastPos int64
	for _, event := range events {
		if event.Position <= lastPos {
			t.Errorf("event %s: position %d not strictly increasing (previous %d)",
				event.ID, event.Position, lastPos)
		}
		lastPos = event.Position
	}

	// Resumable pagination.
	head, err := store.ReadAfter(ctx, 0, 2)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, head, want[:2])
	rest, err := store.ReadAfter(ctx, head[len(head)-1].Position, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, rest, want[2:])
}

// TestPostgresStore_ReadAfter_CommitVisibilityGuard pins the epic's core
// concurrency requirement: a transaction that holds an earlier position but
// commits later must never be skipped. While the earlier-position transaction
// is still in flight, ReadAfter must withhold the later, already-committed
// event; once it commits, both appear in position order.
func TestPostgresStore_ReadAfter_CommitVisibilityGuard(t *testing.T) {
	db := setupPostgresDB(t)
	ctx := context.Background()

	store, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to create gorm event store: %v", err)
	}
	defer func() { _ = store.Close() }()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}

	// Seed one committed event: the guard must withhold in-flight writes
	// without hiding already-committed ones (liveness, not just safety).
	if err := store.Append(ctx, "agg-seed", -1, createTestEvent("agg-seed", "ev-0", "test.created", 1)); err != nil {
		t.Fatalf("failed to append seed event: %v", err)
	}

	// Writer A: insert an event (claiming the earliest position via the
	// sequence default) but do not commit yet.
	connA, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatalf("failed to get connection: %v", err)
	}
	defer func() { _ = connA.Close() }()

	if _, err := connA.ExecContext(ctx, "BEGIN"); err != nil {
		t.Fatalf("failed to begin writer A: %v", err)
	}
	const insertSQL = `INSERT INTO events (id, aggregate_id, event_type, sequence_no, transaction_id, payload, metadata, created_at)
		VALUES ($1, $2, 'test.created', 1, '', '{}', '{}', now())`
	if _, err := connA.ExecContext(ctx, insertSQL, "ev-early", "agg-early"); err != nil {
		t.Fatalf("failed to insert writer A event: %v", err)
	}

	// Writer B: append and commit through the store, taking a later position.
	if err := store.Append(ctx, "agg-late", -1, createTestEvent("agg-late", "ev-late", "test.created", 1)); err != nil {
		t.Fatalf("failed to append writer B event: %v", err)
	}

	// The earlier-position transaction is still open: the committed later
	// event must be withheld (a feed reader advancing past it would lose
	// ev-early forever), while the previously committed seed event must
	// remain visible.
	events, err := store.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-0"})

	if _, err := connA.ExecContext(ctx, "COMMIT"); err != nil {
		t.Fatalf("failed to commit writer A: %v", err)
	}

	events, err = store.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-0", "ev-early", "ev-late"})
	for i := 1; i < len(events); i++ {
		if events[i-1].Position >= events[i].Position {
			t.Errorf("expected strictly increasing positions, got %d then %d",
				events[i-1].Position, events[i].Position)
		}
	}
}

// TestPostgresStore_MigrationRerun_SequenceContinues pins the migration's
// idempotency: constructing the store again on a live database (the normal
// every-startup path) must not rewind the position sequence or fail on the
// already-applied DDL.
func TestPostgresStore_MigrationRerun_SequenceContinues(t *testing.T) {
	db := setupPostgresDB(t)
	ctx := context.Background()

	store1, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to create first store: %v", err)
	}
	if err := store1.Append(ctx, "agg-a", -1, createTestEvent("agg-a", "ev-1", "test.created", 1)); err != nil {
		t.Fatalf("failed to append via first store: %v", err)
	}

	store2, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to re-create store on live database: %v", err)
	}
	defer func() { _ = store2.Close() }()
	if err := store2.Append(ctx, "agg-b", -1, createTestEvent("agg-b", "ev-2", "test.created", 1)); err != nil {
		t.Fatalf("failed to append via second store: %v", err)
	}

	events, err := store2.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-1", "ev-2"})
	if events[0].Position >= events[1].Position {
		t.Errorf("expected positions to continue across constructions, got %d then %d",
			events[0].Position, events[1].Position)
	}
}

// TestPostgresStore_ExistingMethods_RoundTrip covers the dialect whose insert
// path changed (Omit + sequence default): optimistic concurrency conflicts
// and per-aggregate reads must behave exactly as on the other stores.
func TestPostgresStore_ExistingMethods_RoundTrip(t *testing.T) {
	db := setupPostgresDB(t)
	ctx := context.Background()

	store, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to create gorm event store: %v", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Append(ctx, "agg-a", 0, createTestEvent("agg-a", "ev-1", "test.created", 1)); err != nil {
		t.Fatalf("failed to append: %v", err)
	}

	err = store.Append(ctx, "agg-a", 0, createTestEvent("agg-a", "ev-dup", "test.updated", 2))
	if !errors.Is(err, domain.ErrConcurrencyConflict) {
		t.Fatalf("expected ErrConcurrencyConflict on stale version, got %v", err)
	}

	if err := store.Append(ctx, "agg-a", 1, createTestEvent("agg-a", "ev-2", "test.updated", 2)); err != nil {
		t.Fatalf("failed to append with correct version: %v", err)
	}

	events, err := store.GetEvents(ctx, "agg-a")
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-1", "ev-2"})

	version, err := store.GetCurrentVersion(ctx, "agg-a")
	if err != nil {
		t.Fatalf("GetCurrentVersion failed: %v", err)
	}
	if version != 2 {
		t.Errorf("expected version 2, got %d", version)
	}
}

func TestPostgresStore_ConcurrentAppend_AllEventsReadable(t *testing.T) {
	db := setupPostgresDB(t)
	ctx := context.Background()

	store, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to create gorm event store: %v", err)
	}
	defer func() { _ = store.Close() }()

	const writers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := range writers {
		wg.Go(func() {
			aggregateID := fmt.Sprintf("agg-%d", i)
			eventID := fmt.Sprintf("ev-%d", i)
			errCh <- store.Append(ctx, aggregateID, -1, createTestEvent(aggregateID, eventID, "test.created", 1))
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent append failed: %v", err)
		}
	}

	events, err := store.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	if len(events) != writers {
		t.Fatalf("expected %d events, got %d", writers, len(events))
	}
	seen := make(map[int64]bool)
	var lastPos int64
	for _, event := range events {
		if seen[event.Position] {
			t.Errorf("duplicate position %d", event.Position)
		}
		seen[event.Position] = true
		if event.Position <= lastPos {
			t.Errorf("positions not strictly increasing: %d after %d", event.Position, lastPos)
		}
		lastPos = event.Position
	}
}

func TestPostgresStore_BackfillsLegacyRows(t *testing.T) {
	db := setupPostgresDB(t)
	ctx := context.Background()

	// Recreate the pre-position schema and rows by hand.
	legacyDDL := `CREATE TABLE events (
		id TEXT PRIMARY KEY,
		aggregate_id TEXT,
		event_type TEXT,
		sequence_no BIGINT,
		transaction_id TEXT,
		payload JSONB,
		metadata JSONB,
		created_at TIMESTAMPTZ
	)`
	if err := db.Exec(legacyDDL).Error; err != nil {
		t.Fatalf("failed to create legacy table: %v", err)
	}
	insert := `INSERT INTO events (id, aggregate_id, event_type, sequence_no, transaction_id, payload, metadata, created_at)
		VALUES ($1, $2, 'test.created', $3, '', '{}', '{}', now())`
	for _, row := range []struct {
		id, agg string
		seq     int
	}{
		{"ev-2", "agg-a", 2},
		{"ev-1", "agg-a", 1},
		{"ev-3", "agg-b", 1},
	} {
		if err := db.Exec(insert, row.id, row.agg, row.seq).Error; err != nil {
			t.Fatalf("failed to insert legacy row %s: %v", row.id, err)
		}
	}

	store, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("failed to create gorm event store: %v", err)
	}
	defer func() { _ = store.Close() }()

	events, err := store.ReadAfter(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-1", "ev-2", "ev-3"})
	for i, event := range events {
		if event.Position != int64(i+1) {
			t.Errorf("event %s: expected backfilled position %d, got %d", event.ID, i+1, event.Position)
		}
	}

	// The sequence must continue past the backfilled range.
	if err := store.Append(ctx, "agg-c", -1, createTestEvent("agg-c", "ev-4", "test.created", 1)); err != nil {
		t.Fatalf("failed to append after backfill: %v", err)
	}
	events, err = store.ReadAfter(ctx, 3, 0)
	if err != nil {
		t.Fatalf("ReadAfter failed: %v", err)
	}
	assertEventIDs(t, events, []string{"ev-4"})
	if events[0].Position != 4 {
		t.Errorf("expected new event at position 4, got %d", events[0].Position)
	}
}
