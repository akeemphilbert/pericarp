//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/examples"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	pkginfra "github.com/akeemphilbert/pericarp/pkg/infrastructure"
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
	// Setup database
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
		t.Fatalf("failed to connect to database: %v", err)
	}

	// Setup infrastructure
	infraDB := &pkginfra.Database{DB: db}
	if err := infraDB.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Setup event store
	logger := pkgdomain.NewLogger("test")
	eventStore := pkginfra.NewEventStore(db, logger)

	// Run tests
	t.Run("BasicSaveAndLoad", func(t *testing.T) {
		testBasicSaveAndLoad(t, eventStore)
	})

	t.Run("MultipleEvents", func(t *testing.T) {
		testMultipleEvents(t, eventStore)
	})

	t.Run("ConcurrentOperations", func(t *testing.T) {
		testConcurrentOperations(t, eventStore)
	})

	t.Run("EventOrdering", func(t *testing.T) {
		testEventOrdering(t, eventStore)
	})

	t.Run("PerformanceTest", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping performance test in short mode")
		}
		testEventStorePerformance(t, eventStore)
	})

	// Cleanup
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.Close()
	}
}

func testBasicSaveAndLoad(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	userID := uuid.New().String()

	// Create user and get events
	user, err := examples.NewUser(userID, "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	events := user.GetEvents()
	if len(events) == 0 {
		t.Fatal("no events generated")
	}

	// Save events
	for _, event := range events {
		if err := eventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save event: %v", err)
		}
	}

	// Load events
	loadedEvents, err := eventStore.GetEvents(ctx, userID, 0)
	if err != nil {
		t.Fatalf("failed to load events: %v", err)
	}

	if len(loadedEvents) != len(events) {
		t.Errorf("expected %d events, got %d", len(events), len(loadedEvents))
	}

	// Verify event content
	for i, loadedEvent := range loadedEvents {
		if loadedEvent.AggregateID() != events[i].AggregateID() {
			t.Errorf("event %d: expected aggregate ID %s, got %s", i, events[i].AggregateID(), loadedEvent.AggregateID())
		}
		if loadedEvent.Type() != events[i].Type() {
			t.Errorf("event %d: expected type %s, got %s", i, events[i].Type(), loadedEvent.Type())
		}
	}
}

func testMultipleEvents(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	userID := uuid.New().String()

	// Create user
	user, err := examples.NewUser(userID, "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Save initial events
	initialEvents := user.GetEvents()
	for _, event := range initialEvents {
		if err := eventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save initial event: %v", err)
		}
	}

	// Change email
	if err := user.ChangeEmail("newemail@example.com"); err != nil {
		t.Fatalf("failed to change email: %v", err)
	}

	// Save email change events
	emailEvents := user.GetEvents()
	for _, event := range emailEvents {
		if err := eventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save email change event: %v", err)
		}
	}

	// Deactivate user
	if err := user.Deactivate(); err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	// Save deactivation events
	deactivateEvents := user.GetEvents()
	for _, event := range deactivateEvents {
		if err := eventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save deactivation event: %v", err)
		}
	}

	// Load all events
	allEvents, err := eventStore.GetEvents(ctx, userID, 0)
	if err != nil {
		t.Fatalf("failed to load all events: %v", err)
	}

	expectedEventCount := len(initialEvents) + len(emailEvents) + len(deactivateEvents)
	if len(allEvents) != expectedEventCount {
		t.Errorf("expected %d total events, got %d", expectedEventCount, len(allEvents))
	}

	// Verify event ordering
	expectedTypes := []string{"created", "email_changed", "deactivated"}
	for i, event := range allEvents {
		if i < len(expectedTypes) {
			if event.Type() != expectedTypes[i] {
				t.Errorf("event %d: expected type %s, got %s", i, expectedTypes[i], event.Type())
			}
		}
	}
}

func testConcurrentOperations(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	numUsers := 10
	var wg sync.WaitGroup

	// Create users concurrently
	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			userID := uuid.New().String()
			user, err := examples.NewUser(userID, fmt.Sprintf("user%d@example.com", i), fmt.Sprintf("User %d", i))
			if err != nil {
				t.Errorf("failed to create user %d: %v", i, err)
				return
			}

			events := user.GetEvents()
			for _, event := range events {
				if err := eventStore.Save(ctx, event); err != nil {
					t.Errorf("failed to save event for user %d: %v", i, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all users were saved
	for i := 0; i < numUsers; i++ {
		// We can't easily verify specific users without knowing their IDs
		// This test mainly verifies that concurrent operations don't cause errors
	}
}

func testEventOrdering(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	userID := uuid.New().String()

	// Create user and perform multiple operations
	user, err := examples.NewUser(userID, "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Save initial events
	initialEvents := user.GetEvents()
	for _, event := range initialEvents {
		if err := eventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save initial event: %v", err)
		}
	}

	// Change email
	if err := user.ChangeEmail("email1@example.com"); err != nil {
		t.Fatalf("failed to change email: %v", err)
	}
	emailEvents1 := user.GetEvents()
	for _, event := range emailEvents1 {
		if err := eventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save email change event: %v", err)
		}
	}

	// Change email again
	if err := user.ChangeEmail("email2@example.com"); err != nil {
		t.Fatalf("failed to change email again: %v", err)
	}
	emailEvents2 := user.GetEvents()
	for _, event := range emailEvents2 {
		if err := eventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save second email change event: %v", err)
		}
	}

	// Load all events
	allEvents, err := eventStore.GetEvents(ctx, userID, 0)
	if err != nil {
		t.Fatalf("failed to load all events: %v", err)
	}

	// Verify events are in correct order
	expectedTypes := []string{"created", "email_changed", "email_changed"}
	if len(allEvents) != len(expectedTypes) {
		t.Errorf("expected %d events, got %d", len(expectedTypes), len(allEvents))
	}

	for i, event := range allEvents {
		if i < len(expectedTypes) {
			if event.Type() != expectedTypes[i] {
				t.Errorf("event %d: expected type %s, got %s", i, expectedTypes[i], event.Type())
			}
		}
	}
}

func testEventStorePerformance(t *testing.T) {
	ctx := context.Background()
	numEvents := 1000
	start := time.Now()

	// Create and save many events
	for i := 0; i < numEvents; i++ {
		userID := uuid.New().String()
		user, err := examples.NewUser(userID, fmt.Sprintf("user%d@example.com", i), fmt.Sprintf("User %d", i))
		if err != nil {
			t.Fatalf("failed to create user %d: %v", i, err)
		}

		events := user.GetEvents()
		for _, event := range events {
			if err := eventStore.Save(ctx, event); err != nil {
				t.Fatalf("failed to save event %d: %v", i, err)
			}
		}
	}

	duration := time.Since(start)
	eventsPerSecond := float64(numEvents) / duration.Seconds()

	t.Logf("Saved %d events in %v (%.2f events/sec)", numEvents, duration, eventsPerSecond)

	// Performance assertion (adjust as needed)
	if eventsPerSecond < 10 {
		t.Errorf("performance too low: %.2f events/sec (expected at least 10)", eventsPerSecond)
	}
}
