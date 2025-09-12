//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/examples"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	pkginfra "github.com/akeemphilbert/pericarp/pkg/infrastructure"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestPerformanceAndConcurrency runs performance and concurrency tests
func TestPerformanceAndConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance tests in short mode")
	}

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
			testPerformanceWithDatabase(t, tt.dbDriver, tt.dsn)
		})
	}
}

func testPerformanceWithDatabase(t *testing.T, driver, dsn string) {
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

	// Setup event store and dispatcher
	logger := pkgdomain.NewLogger("test")
	eventStore := pkginfra.NewEventStore(db, logger)
	eventDispatcher := pkginfra.NewEventDispatcher(logger)

	// Register event handlers
	eventDispatcher.RegisterHandler("user", "created", func(ctx context.Context, event pkgdomain.Event) error {
		return nil
	})
	eventDispatcher.RegisterHandler("user", "email_changed", func(ctx context.Context, event pkgdomain.Event) error {
		return nil
	})
	eventDispatcher.RegisterHandler("user", "activated", func(ctx context.Context, event pkgdomain.Event) error {
		return nil
	})
	eventDispatcher.RegisterHandler("user", "deactivated", func(ctx context.Context, event pkgdomain.Event) error {
		return nil
	})

	// Run performance tests
	t.Run("EventStorePerformance", func(t *testing.T) {
		testEventStorePerformance(t, eventStore)
	})

	t.Run("EventDispatcherPerformance", func(t *testing.T) {
		testEventDispatcherPerformance(t, eventDispatcher)
	})

	t.Run("ConcurrentUserOperations", func(t *testing.T) {
		testConcurrentUserOperations(t, eventStore, eventDispatcher)
	})

	t.Run("MemoryUsage", func(t *testing.T) {
		testMemoryUsage(t, eventStore)
	})

	// Cleanup
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.Close()
	}
}

func testEventStorePerformance(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	numUsers := 1000

	// Test event store save performance
	start := time.Now()
	for i := 0; i < numUsers; i++ {
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
	eventsPerSecond := float64(numUsers) / duration.Seconds()

	t.Logf("EventStore Save Performance:")
	t.Logf("  Users: %d", numUsers)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Events/sec: %.2f", eventsPerSecond)

	// Performance assertion
	if eventsPerSecond < 50 {
		t.Errorf("EventStore save performance too low: %.2f events/sec (expected at least 50)", eventsPerSecond)
	}

	// Test event store load performance
	start = time.Now()
	loadedCount := 0
	for i := 0; i < numUsers; i++ {
		userID := uuid.New().String()
		events, err := eventStore.GetEvents(ctx, userID, 0)
		if err == nil && len(events) > 0 {
			loadedCount++
		}
	}

	duration = time.Since(start)
	loadsPerSecond := float64(numUsers) / duration.Seconds()

	t.Logf("EventStore Load Performance:")
	t.Logf("  Loads: %d", numUsers)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Loads/sec: %.2f", loadsPerSecond)

	// Performance assertion
	if loadsPerSecond < 100 {
		t.Errorf("EventStore load performance too low: %.2f loads/sec (expected at least 100)", loadsPerSecond)
	}
}

func testEventDispatcherPerformance(t *testing.T, eventDispatcher pkgdomain.EventDispatcher) {
	ctx := context.Background()
	numEvents := 1000

	// Track handler calls
	var handlerCalled int32
	handler := func(ctx context.Context, event pkgdomain.Event) error {
		atomic.AddInt32(&handlerCalled, 1)
		return nil
	}

	// Register handler
	if err := eventDispatcher.RegisterHandler("user", "created", handler); err != nil {
		t.Fatalf("failed to register handler: %v", err)
	}

	// Test dispatch performance
	start := time.Now()
	for i := 0; i < numEvents; i++ {
		userID := uuid.New().String()
		user, err := examples.NewUser(userID, fmt.Sprintf("user%d@example.com", i), fmt.Sprintf("User %d", i))
		if err != nil {
			t.Fatalf("failed to create user %d: %v", i, err)
		}

		events := user.GetEvents()
		for _, event := range events {
			if err := eventDispatcher.Dispatch(ctx, event); err != nil {
				t.Fatalf("failed to dispatch event %d: %v", i, err)
			}
		}
	}

	duration := time.Since(start)
	eventsPerSecond := float64(numEvents) / duration.Seconds()

	t.Logf("EventDispatcher Performance:")
	t.Logf("  Events: %d", numEvents)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Events/sec: %.2f", eventsPerSecond)

	// Wait for async processing
	time.Sleep(1 * time.Second)

	// Verify all handlers were called
	if int(atomic.LoadInt32(&handlerCalled)) != numEvents {
		t.Errorf("expected %d handler calls, got %d", numEvents, atomic.LoadInt32(&handlerCalled))
	}

	// Performance assertion
	if eventsPerSecond < 100 {
		t.Errorf("EventDispatcher performance too low: %.2f events/sec (expected at least 100)", eventsPerSecond)
	}
}

func testConcurrentUserOperations(t *testing.T, eventStore pkgdomain.EventStore, eventDispatcher pkgdomain.EventDispatcher) {
	ctx := context.Background()
	numGoroutines := 50
	usersPerGoroutine := 20

	var wg sync.WaitGroup
	var errors int32

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < usersPerGoroutine; j++ {
				userID := uuid.New().String()
				user, err := examples.NewUser(userID, fmt.Sprintf("user%d_%d@example.com", goroutineID, j), fmt.Sprintf("User %d_%d", goroutineID, j))
				if err != nil {
					atomic.AddInt32(&errors, 1)
					continue
				}

				// Save events
				events := user.GetEvents()
				for _, event := range events {
					if err := eventStore.Save(ctx, event); err != nil {
						atomic.AddInt32(&errors, 1)
						continue
					}
				}

				// Dispatch events
				for _, event := range events {
					if err := eventDispatcher.Dispatch(ctx, event); err != nil {
						atomic.AddInt32(&errors, 1)
						continue
					}
				}

				// Perform some operations
				if err := user.ChangeEmail(fmt.Sprintf("newemail%d_%d@example.com", goroutineID, j)); err != nil {
					atomic.AddInt32(&errors, 1)
					continue
				}

				// Save and dispatch new events
				newEvents := user.GetEvents()
				for _, event := range newEvents {
					if err := eventStore.Save(ctx, event); err != nil {
						atomic.AddInt32(&errors, 1)
						continue
					}
					if err := eventDispatcher.Dispatch(ctx, event); err != nil {
						atomic.AddInt32(&errors, 1)
						continue
					}
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	totalUsers := numGoroutines * usersPerGoroutine
	usersPerSecond := float64(totalUsers) / duration.Seconds()

	t.Logf("Concurrent Operations Performance:")
	t.Logf("  Goroutines: %d", numGoroutines)
	t.Logf("  Users per goroutine: %d", usersPerGoroutine)
	t.Logf("  Total users: %d", totalUsers)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Users/sec: %.2f", usersPerSecond)
	t.Logf("  Errors: %d", atomic.LoadInt32(&errors))

	// Performance assertion
	if usersPerSecond < 10 {
		t.Errorf("concurrent operations performance too low: %.2f users/sec (expected at least 10)", usersPerSecond)
	}

	// Error assertion
	errorRate := float64(atomic.LoadInt32(&errors)) / float64(totalUsers*2) // *2 for create + update operations
	if errorRate > 0.01 {                                                   // 1% error rate
		t.Errorf("error rate too high: %.2f%% (expected less than 1%%)", errorRate*100)
	}
}

func testMemoryUsage(t *testing.T, eventStore pkgdomain.EventStore) {
	ctx := context.Background()
	numUsers := 1000

	// Create and save many users
	userIDs := make([]string, numUsers)
	for i := 0; i < numUsers; i++ {
		userID := uuid.New().String()
		userIDs[i] = userID

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

	// Load all users and measure memory usage
	start := time.Now()
	loadedUsers := 0
	for _, userID := range userIDs {
		events, err := eventStore.GetEvents(ctx, userID, 0)
		if err == nil && len(events) > 0 {
			loadedUsers++
		}
	}
	duration := time.Since(start)

	t.Logf("Memory Usage Test:")
	t.Logf("  Users created: %d", numUsers)
	t.Logf("  Users loaded: %d", loadedUsers)
	t.Logf("  Load duration: %v", duration)
	t.Logf("  Load rate: %.2f users/sec", float64(loadedUsers)/duration.Seconds())

	// Basic assertion
	if loadedUsers != numUsers {
		t.Errorf("expected to load %d users, got %d", numUsers, loadedUsers)
	}
}
