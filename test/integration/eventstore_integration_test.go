//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	internaldomain "github.com/example/pericarp/internal/domain"
	pkgdomain "github.com/example/pericarp/pkg/domain"
	pkginfra "github.com/example/pericarp/pkg/infrastructure"
)

// TestEventStoreIntegration tests the EventStore with real database connections
func TestEventStoreIntegration(t *testing.T) {
	tests := []struct {
		name     string
		dbDriver string
		dsn      string
	}{
		{
			name:     "SQLite",
			dbDriver: "sqlite",
			dsn:      ":memory:",
		},
	}

	// Add PostgreSQL test if available
	if postgresURL := os.Getenv("POSTGRES_TEST_DSN"); postgresURL != "" {
		tests = append(tests, struct {
			name     string
			dbDriver string
			dsn      string
		}{
			name:     "PostgreSQL",
			dbDriver: "postgres",
			dsn:      postgresURL,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testEventStoreWithDatabase(t, tt.dbDriver, tt.dsn)
		})
	}
}

func testEventStoreWithDatabase(t *testing.T, driver, dsn string) {
	// Setup database connection
	var db *gorm.DB
	var err error

	switch driver {
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	case "postgres":
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	default:
		t.Fatalf("unsupported database driver: %s", driver)
	}

	if err != nil {
		t.Fatalf("failed to connect to %s database: %v", driver, err)
	}

	// Auto-migrate tables
	if err := db.AutoMigrate(&pkginfra.EventRecord{}); err != nil {
		t.Fatalf("failed to migrate event store tables: %v", err)
	}

	// Create event store
	eventStore := pkginfra.NewEventStore(db)

	t.Run("SaveAndLoadEvents", func(t *testing.T) {
		testSaveAndLoadEvents(t, eventStore)
	})

	t.Run("LoadFromVersion", func(t *testing.T) {
		testLoadFromVersion(t, eventStore)
	})

	t.Run("ConcurrentEventSaving", func(t *testing.T) {
		testConcurrentEventSaving(t, eventStore)
	})

	t.Run("LargeEventStream", func(t *testing.T) {
		testLargeEventStream(t, eventStore)
	})

	t.Run("EventOrdering", func(t *testing.T) {
		testEventOrdering(t, eventStore)
	})

	// Cleanup
	if driver == "sqlite" && dsn != ":memory:" {
		os.Remove(dsn)
	}
}

func testSaveAndLoadEvents(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	aggregateID := uuid.New().String()

	// Create test events
	events := []pkgdomain.Event{
		internaldomain.NewUserCreatedEvent(
			uuid.MustParse(aggregateID),
			"test@example.com",
			"Test User",
			aggregateID,
			1,
		),
		internaldomain.NewUserEmailUpdatedEvent(
			uuid.MustParse(aggregateID),
			"test@example.com",
			"updated@example.com",
			aggregateID,
			2,
		),
	}

	// Save events
	envelopes, err := eventStore.Save(ctx, events)
	if err != nil {
		t.Fatalf("failed to save events: %v", err)
	}

	if len(envelopes) != len(events) {
		t.Fatalf("expected %d envelopes, got %d", len(events), len(envelopes))
	}

	// Verify envelope properties
	for i, envelope := range envelopes {
		if envelope.Event().AggregateID() != aggregateID {
			t.Errorf("envelope %d: expected aggregate ID %s, got %s", i, aggregateID, envelope.Event().AggregateID())
		}

		if envelope.Event().Version() != events[i].Version() {
			t.Errorf("envelope %d: expected version %d, got %d", i, events[i].Version(), envelope.Event().Version())
		}

		if envelope.EventID() == "" {
			t.Errorf("envelope %d: event ID should not be empty", i)
		}

		if envelope.Timestamp().IsZero() {
			t.Errorf("envelope %d: timestamp should not be zero", i)
		}
	}

	// Load events
	loadedEnvelopes, err := eventStore.Load(ctx, aggregateID)
	if err != nil {
		t.Fatalf("failed to load events: %v", err)
	}

	if len(loadedEnvelopes) != len(events) {
		t.Fatalf("expected %d loaded events, got %d", len(events), len(loadedEnvelopes))
	}

	// Verify loaded events
	for i, envelope := range loadedEnvelopes {
		event := envelope.Event()
		originalEvent := events[i]

		if event.EventType() != originalEvent.EventType() {
			t.Errorf("event %d: expected type %s, got %s", i, originalEvent.EventType(), event.EventType())
		}

		if event.AggregateID() != originalEvent.AggregateID() {
			t.Errorf("event %d: expected aggregate ID %s, got %s", i, originalEvent.AggregateID(), event.AggregateID())
		}

		if event.Version() != originalEvent.Version() {
			t.Errorf("event %d: expected version %d, got %d", i, originalEvent.Version(), event.Version())
		}
	}
}

func testLoadFromVersion(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	aggregateID := uuid.New().String()

	// Create multiple events
	events := []pkgdomain.Event{
		internaldomain.NewUserCreatedEvent(
			uuid.MustParse(aggregateID),
			"test@example.com",
			"Test User",
			aggregateID,
			1,
		),
		internaldomain.NewUserEmailUpdatedEvent(
			uuid.MustParse(aggregateID),
			"test@example.com",
			"updated@example.com",
			aggregateID,
			2,
		),
		internaldomain.NewUserNameUpdatedEvent(
			uuid.MustParse(aggregateID),
			"Test User",
			"Updated User",
			aggregateID,
			3,
		),
	}

	// Save all events
	_, err := eventStore.Save(ctx, events)
	if err != nil {
		t.Fatalf("failed to save events: %v", err)
	}

	// Load from version 2
	envelopes, err := eventStore.LoadFromVersion(ctx, aggregateID, 2)
	if err != nil {
		t.Fatalf("failed to load events from version: %v", err)
	}

	// Should get events with version >= 2
	expectedCount := 2 // versions 2 and 3
	if len(envelopes) != expectedCount {
		t.Fatalf("expected %d events from version 2, got %d", expectedCount, len(envelopes))
	}

	// Verify versions
	for i, envelope := range envelopes {
		expectedVersion := i + 2 // versions 2, 3
		if envelope.Event().Version() != expectedVersion {
			t.Errorf("event %d: expected version %d, got %d", i, expectedVersion, envelope.Event().Version())
		}
	}
}

func testConcurrentEventSaving(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	numGoroutines := 10
	eventsPerGoroutine := 5

	var wg sync.WaitGroup
	var mu sync.Mutex
	allEnvelopes := make([]pkgdomain.Envelope, 0)
	errors := make([]error, 0)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			aggregateID := uuid.New().String()
			events := make([]pkgdomain.Event, eventsPerGoroutine)

			for j := 0; j < eventsPerGoroutine; j++ {
				events[j] = internaldomain.NewUserCreatedEvent(
					uuid.MustParse(aggregateID),
					fmt.Sprintf("user%d_%d@example.com", goroutineID, j),
					fmt.Sprintf("User %d_%d", goroutineID, j),
					aggregateID,
					j+1,
				)
			}

			envelopes, err := eventStore.Save(ctx, events)
			
			mu.Lock()
			if err != nil {
				errors = append(errors, err)
			} else {
				allEnvelopes = append(allEnvelopes, envelopes...)
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Check for errors
	if len(errors) > 0 {
		t.Fatalf("concurrent save failed with %d errors, first error: %v", len(errors), errors[0])
	}

	// Verify all events were saved
	expectedTotal := numGoroutines * eventsPerGoroutine
	if len(allEnvelopes) != expectedTotal {
		t.Fatalf("expected %d total envelopes, got %d", expectedTotal, len(allEnvelopes))
	}

	// Verify all envelopes have unique event IDs
	eventIDs := make(map[string]bool)
	for _, envelope := range allEnvelopes {
		eventID := envelope.EventID()
		if eventIDs[eventID] {
			t.Errorf("duplicate event ID found: %s", eventID)
		}
		eventIDs[eventID] = true
	}
}

func testLargeEventStream(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	aggregateID := uuid.New().String()
	numEvents := 1000

	// Create large number of events
	events := make([]pkgdomain.Event, numEvents)
	for i := 0; i < numEvents; i++ {
		events[i] = internaldomain.NewUserEmailUpdatedEvent(
			uuid.MustParse(aggregateID),
			fmt.Sprintf("old%d@example.com", i),
			fmt.Sprintf("new%d@example.com", i),
			aggregateID,
			i+1,
		)
	}

	// Measure save performance
	start := time.Now()
	envelopes, err := eventStore.Save(ctx, events)
	saveTime := time.Since(start)

	if err != nil {
		t.Fatalf("failed to save large event stream: %v", err)
	}

	if len(envelopes) != numEvents {
		t.Fatalf("expected %d envelopes, got %d", numEvents, len(envelopes))
	}

	t.Logf("Saved %d events in %v", numEvents, saveTime)

	// Measure load performance
	start = time.Now()
	loadedEnvelopes, err := eventStore.Load(ctx, aggregateID)
	loadTime := time.Since(start)

	if err != nil {
		t.Fatalf("failed to load large event stream: %v", err)
	}

	if len(loadedEnvelopes) != numEvents {
		t.Fatalf("expected %d loaded events, got %d", numEvents, len(loadedEnvelopes))
	}

	t.Logf("Loaded %d events in %v", numEvents, loadTime)

	// Performance assertions (adjust thresholds as needed)
	maxSaveTime := 10 * time.Second
	maxLoadTime := 5 * time.Second

	if saveTime > maxSaveTime {
		t.Errorf("save operation too slow: %v (max: %v)", saveTime, maxSaveTime)
	}

	if loadTime > maxLoadTime {
		t.Errorf("load operation too slow: %v (max: %v)", loadTime, maxLoadTime)
	}
}

func testEventOrdering(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	aggregateID := uuid.New().String()

	// Create events with specific timestamps
	baseTime := time.Now()
	events := []pkgdomain.Event{
		internaldomain.NewUserCreatedEvent(
			uuid.MustParse(aggregateID),
			"test@example.com",
			"Test User",
			aggregateID,
			1,
		),
		internaldomain.NewUserEmailUpdatedEvent(
			uuid.MustParse(aggregateID),
			"test@example.com",
			"updated@example.com",
			aggregateID,
			2,
		),
		internaldomain.NewUserNameUpdatedEvent(
			uuid.MustParse(aggregateID),
			"Test User",
			"Updated User",
			aggregateID,
			3,
		),
	}

	// Save events
	_, err := eventStore.Save(ctx, events)
	if err != nil {
		t.Fatalf("failed to save events: %v", err)
	}

	// Load events
	envelopes, err := eventStore.Load(ctx, aggregateID)
	if err != nil {
		t.Fatalf("failed to load events: %v", err)
	}

	// Verify ordering by version
	for i := 1; i < len(envelopes); i++ {
		prevVersion := envelopes[i-1].Event().Version()
		currVersion := envelopes[i].Event().Version()

		if currVersion <= prevVersion {
			t.Errorf("events not ordered by version: event %d has version %d, event %d has version %d",
				i-1, prevVersion, i, currVersion)
		}
	}

	// Verify ordering by timestamp
	for i := 1; i < len(envelopes); i++ {
		prevTime := envelopes[i-1].Timestamp()
		currTime := envelopes[i].Timestamp()

		if currTime.Before(prevTime) {
			t.Errorf("events not ordered by timestamp: event %d at %v, event %d at %v",
				i-1, prevTime, i, currTime)
		}
	}

	// Verify all timestamps are after base time
	for i, envelope := range envelopes {
		if envelope.Timestamp().Before(baseTime) {
			t.Errorf("event %d timestamp %v is before base time %v", i, envelope.Timestamp(), baseTime)
		}
	}
}

// TestEventStoreErrorHandling tests error scenarios
func TestEventStoreErrorHandling(t *testing.T) {
	// Test with invalid database connection
	t.Run("InvalidConnection", func(t *testing.T) {
		// Create event store with nil database (should panic or handle gracefully)
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic with nil database")
			}
		}()
		
		pkginfra.NewEventStore(nil)
	})

	// Test with closed database connection
	t.Run("ClosedConnection", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			t.Fatalf("failed to create database: %v", err)
		}

		// Auto-migrate
		if err := db.AutoMigrate(&pkginfra.EventRecord{}); err != nil {
			t.Fatalf("failed to migrate: %v", err)
		}

		eventStore := pkginfra.NewEventStore(db)

		// Close the database connection
		sqlDB, _ := db.DB()
		sqlDB.Close()

		// Try to save events (should fail)
		ctx := context.Background()
		events := []pkgdomain.Event{
			internaldomain.NewUserCreatedEvent(
				uuid.New(),
				"test@example.com",
				"Test User",
				uuid.New().String(),
				1,
			),
		}

		_, err = eventStore.Save(ctx, events)
		if err == nil {
			t.Error("expected error when saving to closed database")
		}
	})
}

// TestEventStorePerformance runs performance benchmarks
func TestEventStorePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance tests in short mode")
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	if err := db.AutoMigrate(&pkginfra.EventRecord{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	eventStore := pkginfra.NewEventStore(db)
	ctx := context.Background()

	// Benchmark single event save
	t.Run("SingleEventSave", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			aggregateID := uuid.New().String()
			events := []pkgdomain.Event{
				internaldomain.NewUserCreatedEvent(
					uuid.MustParse(aggregateID),
					fmt.Sprintf("user%d@example.com", i),
					fmt.Sprintf("User %d", i),
					aggregateID,
					1,
				),
			}

			start := time.Now()
			_, err := eventStore.Save(ctx, events)
			duration := time.Since(start)

			if err != nil {
				t.Fatalf("save failed: %v", err)
			}

			if duration > 100*time.Millisecond {
				t.Errorf("single event save too slow: %v", duration)
			}
		}
	})

	// Benchmark batch event save
	t.Run("BatchEventSave", func(t *testing.T) {
		batchSize := 100
		aggregateID := uuid.New().String()
		events := make([]pkgdomain.Event, batchSize)

		for i := 0; i < batchSize; i++ {
			events[i] = internaldomain.NewUserEmailUpdatedEvent(
				uuid.MustParse(aggregateID),
				fmt.Sprintf("old%d@example.com", i),
				fmt.Sprintf("new%d@example.com", i),
				aggregateID,
				i+1,
			)
		}

		start := time.Now()
		_, err := eventStore.Save(ctx, events)
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("batch save failed: %v", err)
		}

		t.Logf("Batch save of %d events took %v", batchSize, duration)

		// Should be faster than individual saves
		maxExpectedTime := 1 * time.Second
		if duration > maxExpectedTime {
			t.Errorf("batch save too slow: %v (max: %v)", duration, maxExpectedTime)
		}
	})
}