package infrastructure_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	"github.com/segmentio/ksuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

const (
	bigtableTestProject  = "test-project"
	bigtableTestInstance = "test-instance"
)

var (
	bigtableOnce      sync.Once
	bigtableContainer testcontainers.Container
	bigtableEndpoint  string
	bigtableSetupErr  error
)

func startBigtableContainer(t *testing.T) (string, error) {
	t.Helper()

	bigtableOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				bigtableSetupErr = fmt.Errorf("Bigtable container setup panicked (Docker may not be available): %v", r)
			}
		}()

		ctx := context.Background()
		req := testcontainers.ContainerRequest{
			Image:        "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators",
			ExposedPorts: []string{"8086/tcp"},
			Cmd:          []string{"gcloud", "beta", "emulators", "bigtable", "start", "--host-port=0.0.0.0:8086"},
			WaitingFor:   wait.ForListeningPort("8086/tcp").WithStartupTimeout(90 * time.Second),
		}

		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		if err != nil {
			bigtableSetupErr = fmt.Errorf("failed to start Bigtable emulator: %w", err)
			return
		}
		bigtableContainer = container

		host, err := container.Host(ctx)
		if err != nil {
			bigtableSetupErr = fmt.Errorf("failed to get container host: %w", err)
			_ = container.Terminate(ctx)
			return
		}
		port, err := container.MappedPort(ctx, "8086")
		if err != nil {
			bigtableSetupErr = fmt.Errorf("failed to get mapped port: %w", err)
			_ = container.Terminate(ctx)
			return
		}
		bigtableEndpoint = fmt.Sprintf("%s:%s", host, port.Port())
	})

	return bigtableEndpoint, bigtableSetupErr
}

func newBigtableClient(t *testing.T, endpoint string) *bigtable.Client {
	t.Helper()
	ctx := context.Background()
	client, err := bigtable.NewClient(ctx, bigtableTestProject, bigtableTestInstance,
		option.WithEndpoint(endpoint),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		t.Fatalf("bigtable.NewClient: %v", err)
	}
	return client
}

func newBigtableAdminClient(t *testing.T, endpoint string) *bigtable.AdminClient {
	t.Helper()
	ctx := context.Background()
	admin, err := bigtable.NewAdminClient(ctx, bigtableTestProject, bigtableTestInstance,
		option.WithEndpoint(endpoint),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		t.Fatalf("bigtable.NewAdminClient: %v", err)
	}
	return admin
}

func createBigtableTable(t *testing.T, admin *bigtable.AdminClient, tableName string) {
	t.Helper()
	ctx := context.Background()
	if err := admin.CreateTable(ctx, tableName); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	if err := admin.CreateColumnFamily(ctx, tableName, infrastructure.BigtableColumnFamily); err != nil {
		t.Fatalf("CreateColumnFamily: %v", err)
	}
}

func setupBigtableStore(t *testing.T) domain.EventStore {
	t.Helper()

	endpoint, err := startBigtableContainer(t)
	if err != nil {
		t.Skipf("skipping Bigtable test: %v", err)
	}

	admin := newBigtableAdminClient(t, endpoint)
	tableName := "events_" + ksuid.New().String()
	createBigtableTable(t, admin, tableName)

	client := newBigtableClient(t, endpoint)

	t.Cleanup(func() {
		if err := admin.DeleteTable(context.Background(), tableName); err != nil {
			t.Logf("warning: failed to delete Bigtable test table %s: %v", tableName, err)
		}
		_ = admin.Close()
		// BigtableEventStore.Close is intentionally a no-op — the client is
		// caller-owned, so tests close it here to avoid leaking grpc
		// connections/goroutines across the parallel suite.
		_ = client.Close()
	})

	return infrastructure.NewBigtableEventStore(client, tableName)
}

func setupBigtableStoreWithEvents(t *testing.T) domain.EventStore {
	t.Helper()
	store := setupBigtableStore(t)
	ctx := context.Background()

	event := createTestEvent("agg-3", "event-1", "test.created", 1)
	if err := store.Append(ctx, "agg-3", -1, event); err != nil {
		t.Fatalf("failed to setup store: %v", err)
	}
	return store
}

func setupBigtableStoreWithMultipleEvents(t *testing.T) domain.EventStore {
	t.Helper()
	store := setupBigtableStore(t)
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

func TestBigtableStore_FullWorkflow(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	agg := "agg-workflow"

	events := []domain.EventEnvelope[any]{
		createTestEvent(agg, "evt-1", "test.created", 1),
		createTestEvent(agg, "evt-2", "test.updated", 2),
	}
	if err := store.Append(ctx, agg, -1, events...); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := store.GetEvents(ctx, agg)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(got) != 2 || got[0].SequenceNo != 1 || got[1].SequenceNo != 2 {
		t.Fatalf("unexpected events: %+v", got)
	}

	version, err := store.GetCurrentVersion(ctx, agg)
	if err != nil {
		t.Fatalf("GetCurrentVersion: %v", err)
	}
	if version != 2 {
		t.Fatalf("GetCurrentVersion = %d, want 2", version)
	}

	third := createTestEvent(agg, "evt-3", "test.updated", 3)
	if err := store.Append(ctx, agg, 2, third); err != nil {
		t.Fatalf("versioned append: %v", err)
	}

	fromVer2, err := store.GetEventsFromVersion(ctx, agg, 2)
	if err != nil {
		t.Fatalf("GetEventsFromVersion: %v", err)
	}
	if len(fromVer2) != 2 {
		t.Fatalf("GetEventsFromVersion len = %d, want 2", len(fromVer2))
	}

	byID, err := store.GetEventByID(ctx, "evt-2")
	if err != nil {
		t.Fatalf("GetEventByID: %v", err)
	}
	if byID.ID != "evt-2" || byID.SequenceNo != 2 {
		t.Fatalf("GetEventByID wrong: %+v", byID)
	}
}

func TestBigtableStore_GetEventByID_NotFound(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.GetEventByID(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound, got %v", err)
	}
}

func TestBigtableStore_ConcurrencyConflict(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	if err := store.Append(ctx, "agg-conflict", -1, createTestEvent("agg-conflict", "evt-1", "test.created", 1)); err != nil {
		t.Fatalf("seed append: %v", err)
	}

	// Current version is 1; claiming 999 must conflict.
	err := store.Append(ctx, "agg-conflict", 999, createTestEvent("agg-conflict", "evt-2", "test.updated", 2))
	if !errors.Is(err, domain.ErrConcurrencyConflict) {
		t.Fatalf("expected ErrConcurrencyConflict, got %v", err)
	}
}

func TestBigtableStore_GetEventsRange(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	agg := "agg-range"

	events := make([]domain.EventEnvelope[any], 5)
	for i := range events {
		events[i] = createTestEvent(agg, fmt.Sprintf("evt-%d", i+1), "test.created", i+1)
	}
	if err := store.Append(ctx, agg, -1, events...); err != nil {
		t.Fatalf("append: %v", err)
	}

	mid, err := store.GetEventsRange(ctx, agg, 2, 4)
	if err != nil {
		t.Fatalf("GetEventsRange: %v", err)
	}
	if len(mid) != 3 || mid[0].SequenceNo != 2 || mid[2].SequenceNo != 4 {
		t.Fatalf("GetEventsRange returned %+v", mid)
	}

	all, err := store.GetEventsRange(ctx, agg, -1, -1)
	if err != nil {
		t.Fatalf("GetEventsRange unbounded: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("GetEventsRange unbounded len = %d, want 5", len(all))
	}
}

func TestBigtableStore_TransactionIDLookup(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	txID := "tx-" + ksuid.New().String()
	// Three events in the same transaction across two aggregates.
	ev1 := createTestEventWithTxID("agg-tx-a", "tx-evt-1", "test.created", 1, txID)
	ev2 := createTestEventWithTxID("agg-tx-a", "tx-evt-2", "test.updated", 2, txID)
	ev3 := createTestEventWithTxID("agg-tx-b", "tx-evt-3", "test.created", 1, txID)

	if err := store.Append(ctx, "agg-tx-a", -1, ev1, ev2); err != nil {
		t.Fatalf("append a: %v", err)
	}
	if err := store.Append(ctx, "agg-tx-b", -1, ev3); err != nil {
		t.Fatalf("append b: %v", err)
	}

	// An unrelated event (no tx) to make sure the index scan doesn't sweep it up.
	other := createTestEvent("agg-tx-a", "noise-1", "test.updated", 3)
	if err := store.Append(ctx, "agg-tx-a", -1, other); err != nil {
		t.Fatalf("append noise: %v", err)
	}

	got, err := store.GetEventsByTransactionID(ctx, txID)
	if err != nil {
		t.Fatalf("GetEventsByTransactionID: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("GetEventsByTransactionID len = %d, want 3", len(got))
	}
	// Ordered by (agg, seq): agg-tx-a/1, agg-tx-a/2, agg-tx-b/1.
	if got[0].AggregateID != "agg-tx-a" || got[0].SequenceNo != 1 {
		t.Fatalf("got[0] = %s/%d", got[0].AggregateID, got[0].SequenceNo)
	}
	if got[1].AggregateID != "agg-tx-a" || got[1].SequenceNo != 2 {
		t.Fatalf("got[1] = %s/%d", got[1].AggregateID, got[1].SequenceNo)
	}
	if got[2].AggregateID != "agg-tx-b" || got[2].SequenceNo != 1 {
		t.Fatalf("got[2] = %s/%d", got[2].AggregateID, got[2].SequenceNo)
	}
}

func TestBigtableStore_EmptyTransactionIDRejected(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.GetEventsByTransactionID(context.Background(), "")
	if !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("expected ErrInvalidEvent for empty tx ID, got %v", err)
	}
}

func TestBigtableStore_PayloadMetadataTimestampRoundTrip(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	env := domain.EventEnvelope[any]{
		ID:          "rt-evt-1",
		AggregateID: "agg-rt",
		EventType:   "test.payload",
		Payload: map[string]any{
			"name":  "Ada",
			"count": float64(42),
			"tags":  []any{"a", "b"},
		},
		Metadata:   map[string]any{"correlation_id": "corr-123"},
		Created:    now,
		SequenceNo: 1,
	}

	if err := store.Append(ctx, "agg-rt", -1, env); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := store.GetEventByID(ctx, "rt-evt-1")
	if err != nil {
		t.Fatalf("GetEventByID: %v", err)
	}
	p, ok := got.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type %T", got.Payload)
	}
	if p["name"] != "Ada" || p["count"] != float64(42) {
		t.Fatalf("payload = %+v", p)
	}
	if got.Metadata["correlation_id"] != "corr-123" {
		t.Fatalf("metadata = %+v", got.Metadata)
	}
	if !got.Created.Equal(now) {
		t.Fatalf("Created %v != %v", got.Created, now)
	}
}

func TestBigtableStore_RejectsInvalidAggregateID(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	cases := []string{"user#admin", "has\x00null"}
	for _, agg := range cases {
		ev := createTestEvent(agg, "evt-reject", "test.created", 1)
		err := store.Append(ctx, agg, -1, ev)
		if !errors.Is(err, domain.ErrInvalidEvent) {
			t.Fatalf("aggregate %q: expected ErrInvalidEvent, got %v", agg, err)
		}
	}
}

func TestBigtableStore_RejectsInvalidTransactionID(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	ev := createTestEventWithTxID("agg-reject-tx", "evt-reject-tx", "test.created", 1, "tx#bad")
	if err := store.Append(ctx, "agg-reject-tx", -1, ev); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("Append with invalid tx ID: got %v, want ErrInvalidEvent", err)
	}

	if _, err := store.GetEventsByTransactionID(ctx, "tx#bad"); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("GetEventsByTransactionID with '#' tx ID: got %v, want ErrInvalidEvent", err)
	}
}

func TestBigtableStore_TransactionIDRoundTrip(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	txID := "tx-" + ksuid.New().String()
	ev := createTestEventWithTxID("agg-tx-rt", "tx-rt-1", "test.created", 1, txID)
	if err := store.Append(ctx, "agg-tx-rt", -1, ev); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := store.GetEventByID(ctx, "tx-rt-1")
	if err != nil {
		t.Fatalf("GetEventByID: %v", err)
	}
	if got.TransactionID != txID {
		t.Fatalf("TransactionID = %q, want %q", got.TransactionID, txID)
	}

	viaRange, err := store.GetEvents(ctx, "agg-tx-rt")
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(viaRange) != 1 || viaRange[0].TransactionID != txID {
		t.Fatalf("GetEvents TransactionID = %+v", viaRange)
	}
}

func TestBigtableStore_ToAnyEnvelopeStructPayload(t *testing.T) {
	t.Parallel()

	type TestPayload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	original := domain.NewEventEnvelope(TestPayload{Name: "ada", Value: 42}, "agg-struct", "test.created", 1)
	anyEnv := domain.ToAnyEnvelope(original)
	if err := store.Append(ctx, "agg-struct", -1, anyEnv); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := store.GetEventByID(ctx, anyEnv.ID)
	if err != nil {
		t.Fatalf("GetEventByID: %v", err)
	}
	p, ok := got.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type %T", got.Payload)
	}
	if p["name"] != "ada" || p["value"] != float64(42) {
		t.Fatalf("payload round-trip lost fields: %+v", p)
	}
}

func TestBigtableStore_CloseIsNoOp(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	// First Close is the one test cleanup drives; we're asserting the store's
	// Close contract itself — it must not close the underlying client, so a
	// subsequent GetEvents call should still work.
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := store.GetEvents(context.Background(), "agg-after-close"); err != nil {
		t.Fatalf("GetEvents after Close should still work (client caller-owned): %v", err)
	}
}

func TestBigtableStore_MultipleAggregatesIsolated(t *testing.T) {
	t.Parallel()

	store := setupBigtableStore(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	if err := store.Append(ctx, "agg-iso-1", -1, createTestEvent("agg-iso-1", "i-1", "test.created", 1)); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := store.Append(ctx, "agg-iso-2", -1,
		createTestEvent("agg-iso-2", "i-2", "test.created", 1),
		createTestEvent("agg-iso-2", "i-3", "test.updated", 2),
	); err != nil {
		t.Fatalf("append 2: %v", err)
	}

	a, err := store.GetEvents(ctx, "agg-iso-1")
	if err != nil || len(a) != 1 {
		t.Fatalf("agg-iso-1: %d events, err=%v", len(a), err)
	}
	b, err := store.GetEvents(ctx, "agg-iso-2")
	if err != nil || len(b) != 2 {
		t.Fatalf("agg-iso-2: %d events, err=%v", len(b), err)
	}

	v, err := store.GetCurrentVersion(ctx, "agg-iso-never")
	if err != nil {
		t.Fatalf("GetCurrentVersion unknown agg: %v", err)
	}
	if v != 0 {
		t.Fatalf("GetCurrentVersion unknown agg = %d, want 0", v)
	}
}
