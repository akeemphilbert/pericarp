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

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	internalapp "github.com/akeemphilbert/pericarp/internal/application"
	internaldomain "github.com/akeemphilbert/pericarp/internal/domain"
	internalinfra "github.com/akeemphilbert/pericarp/internal/infrastructure"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	pkginfra "github.com/akeemphilbert/pericarp/pkg/infrastructure"
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
	system, cleanup := setupSystem(t, driver, dsn)
	defer cleanup()

	t.Run("BulkUserCreation", func(t *testing.T) {
		testBulkUserCreation(t, system)
	})

	t.Run("ConcurrentUserOperations", func(t *testing.T) {
		testConcurrentUserOperations(t, system)
	})

	t.Run("LargeEventStreamPerformance", func(t *testing.T) {
		testLargeEventStreamPerformance(t, system)
	})

	t.Run("QueryPerformance", func(t *testing.T) {
		testQueryPerformance(t, system)
	})

	t.Run("ConcurrentReadWrite", func(t *testing.T) {
		testConcurrentReadWrite(t, system)
	})
}

func testBulkUserCreation(t *testing.T, system *TestSystem) {
	ctx := context.Background()
	numUsers := 1000

	t.Logf("Creating %d users...", numUsers)
	start := time.Now()

	for i := 0; i < numUsers; i++ {
		cmd := internalapp.CreateUserCommand{
			ID:    uuid.New(),
			Email: fmt.Sprintf("bulk%d@example.com", i),
			Name:  fmt.Sprintf("Bulk User %d", i),
		}

		err := system.CreateUserHandler.Handle(ctx, system.Logger, cmd)
		if err != nil {
			t.Fatalf("failed to create user %d: %v", i, err)
		}

		// Log progress every 100 users
		if (i+1)%100 == 0 {
			elapsed := time.Since(start)
			rate := float64(i+1) / elapsed.Seconds()
			t.Logf("Created %d users in %v (%.2f users/sec)", i+1, elapsed, rate)
		}
	}

	duration := time.Since(start)
	rate := float64(numUsers) / duration.Seconds()

	t.Logf("Created %d users in %v (%.2f users/sec)", numUsers, duration, rate)

	// Performance assertions
	maxDuration := 30 * time.Second
	minRate := 30.0 // users per second

	if duration > maxDuration {
		t.Errorf("bulk creation too slow: %v (max: %v)", duration, maxDuration)
	}

	if rate < minRate {
		t.Errorf("creation rate too low: %.2f users/sec (min: %.2f)", rate, minRate)
	}

	// Wait for projections
	time.Sleep(2 * time.Second)

	// Verify all users were created
	listQuery := internalapp.ListUsersQuery{
		Page:     1,
		PageSize: numUsers + 100, // Buffer for other tests
	}

	result, err := system.ListUsersHandler.Handle(ctx, system.Logger, listQuery)
	if err != nil {
		t.Fatalf("failed to list users: %v", err)
	}

	if result.TotalCount < numUsers {
		t.Errorf("expected at least %d users, got %d", numUsers, result.TotalCount)
	}
}

func testConcurrentUserOperations(t *testing.T, system *TestSystem) {
	ctx := context.Background()
	numGoroutines := 20
	operationsPerGoroutine := 50

	t.Logf("Running %d concurrent goroutines with %d operations each...", numGoroutines, operationsPerGoroutine)

	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines*operationsPerGoroutine)
	start := time.Now()

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < operationsPerGoroutine; i++ {
				userID := uuid.New()

				// Create user
				createCmd := internalapp.CreateUserCommand{
					ID:    userID,
					Email: fmt.Sprintf("concurrent%d_%d@example.com", goroutineID, i),
					Name:  fmt.Sprintf("Concurrent User %d_%d", goroutineID, i),
				}

				if err := system.CreateUserHandler.Handle(ctx, system.Logger, createCmd); err != nil {
					errChan <- fmt.Errorf("goroutine %d, operation %d, create: %w", goroutineID, i, err)
					continue
				}

				// Update email
				updateEmailCmd := internalapp.UpdateUserEmailCommand{
					ID:       userID,
					NewEmail: fmt.Sprintf("updated%d_%d@example.com", goroutineID, i),
				}

				if err := system.UpdateUserEmailHandler.Handle(ctx, system.Logger, updateEmailCmd); err != nil {
					errChan <- fmt.Errorf("goroutine %d, operation %d, update email: %w", goroutineID, i, err)
					continue
				}

				// Update name
				updateNameCmd := internalapp.UpdateUserNameCommand{
					ID:      userID,
					NewName: fmt.Sprintf("Updated User %d_%d", goroutineID, i),
				}

				if err := system.UpdateUserNameHandler.Handle(ctx, system.Logger, updateNameCmd); err != nil {
					errChan <- fmt.Errorf("goroutine %d, operation %d, update name: %w", goroutineID, i, err)
					continue
				}
			}
		}(g)
	}

	wg.Wait()
	close(errChan)

	duration := time.Since(start)
	totalOperations := numGoroutines * operationsPerGoroutine * 3 // 3 operations per user
	rate := float64(totalOperations) / duration.Seconds()

	t.Logf("Completed %d concurrent operations in %v (%.2f ops/sec)", totalOperations, duration, rate)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		t.Errorf("concurrent operations had %d errors, first error: %v", len(errors), errors[0])
		// Log first few errors for debugging
		for i, err := range errors {
			if i >= 5 {
				break
			}
			t.Logf("Error %d: %v", i+1, err)
		}
	}

	// Performance assertions
	maxDuration := 60 * time.Second
	minRate := 50.0 // operations per second

	if duration > maxDuration {
		t.Errorf("concurrent operations too slow: %v (max: %v)", duration, maxDuration)
	}

	if rate < minRate {
		t.Errorf("operation rate too low: %.2f ops/sec (min: %.2f)", rate, minRate)
	}
}

func testLargeEventStreamPerformance(t *testing.T, system *TestSystem) {
	ctx := context.Background()
	userID := uuid.New()

	// Create user first
	createCmd := internalapp.CreateUserCommand{
		ID:    userID,
		Email: "eventstream@example.com",
		Name:  "Event Stream User",
	}

	err := system.CreateUserHandler.Handle(ctx, system.Logger, createCmd)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Perform many updates to create large event stream
	numUpdates := 500
	t.Logf("Performing %d updates to create large event stream...", numUpdates)

	start := time.Now()
	for i := 0; i < numUpdates; i++ {
		if i%2 == 0 {
			// Update email
			updateCmd := internalapp.UpdateUserEmailCommand{
				ID:       userID,
				NewEmail: fmt.Sprintf("stream%d@example.com", i),
			}
			err = system.UpdateUserEmailHandler.Handle(ctx, system.Logger, updateCmd)
		} else {
			// Update name
			updateCmd := internalapp.UpdateUserNameCommand{
				ID:      userID,
				NewName: fmt.Sprintf("Stream User %d", i),
			}
			err = system.UpdateUserNameHandler.Handle(ctx, system.Logger, updateCmd)
		}

		if err != nil {
			t.Fatalf("failed to update user at iteration %d: %v", i, err)
		}

		// Log progress
		if (i+1)%100 == 0 {
			elapsed := time.Since(start)
			rate := float64(i+1) / elapsed.Seconds()
			t.Logf("Completed %d updates in %v (%.2f updates/sec)", i+1, elapsed, rate)
		}
	}

	updateDuration := time.Since(start)
	updateRate := float64(numUpdates) / updateDuration.Seconds()

	t.Logf("Completed %d updates in %v (%.2f updates/sec)", numUpdates, updateDuration, updateRate)

	// Test event loading performance
	t.Log("Loading large event stream...")
	loadStart := time.Now()
	envelopes, err := system.EventStore.Load(ctx, userID.String())
	loadDuration := time.Since(loadStart)

	if err != nil {
		t.Fatalf("failed to load events: %v", err)
	}

	expectedEvents := numUpdates + 1 // +1 for initial creation
	if len(envelopes) != expectedEvents {
		t.Errorf("expected %d events, got %d", expectedEvents, len(envelopes))
	}

	t.Logf("Loaded %d events in %v", len(envelopes), loadDuration)

	// Test reconstruction performance
	t.Log("Reconstructing aggregate from large event stream...")
	reconstructStart := time.Now()

	events := make([]pkgdomain.Event, len(envelopes))
	for i, envelope := range envelopes {
		events[i] = envelope.Event()
	}

	reconstructedUser := &internaldomain.User{}
	reconstructedUser.LoadFromHistory(events)

	reconstructDuration := time.Since(reconstructStart)

	t.Logf("Reconstructed aggregate in %v", reconstructDuration)

	// Performance assertions
	maxUpdateDuration := 30 * time.Second
	maxLoadDuration := 5 * time.Second
	maxReconstructDuration := 1 * time.Second
	minUpdateRate := 15.0 // updates per second

	if updateDuration > maxUpdateDuration {
		t.Errorf("updates too slow: %v (max: %v)", updateDuration, maxUpdateDuration)
	}

	if updateRate < minUpdateRate {
		t.Errorf("update rate too low: %.2f updates/sec (min: %.2f)", updateRate, minUpdateRate)
	}

	if loadDuration > maxLoadDuration {
		t.Errorf("event loading too slow: %v (max: %v)", loadDuration, maxLoadDuration)
	}

	if reconstructDuration > maxReconstructDuration {
		t.Errorf("reconstruction too slow: %v (max: %v)", reconstructDuration, maxReconstructDuration)
	}

	// Verify final state
	if reconstructedUser.Version() != expectedEvents {
		t.Errorf("expected version %d, got %d", expectedEvents, reconstructedUser.Version())
	}
}

func testQueryPerformance(t *testing.T, system *TestSystem) {
	ctx := context.Background()

	// Create users for query testing
	numUsers := 100
	userIDs := make([]uuid.UUID, numUsers)

	t.Logf("Creating %d users for query testing...", numUsers)
	for i := 0; i < numUsers; i++ {
		userIDs[i] = uuid.New()
		cmd := internalapp.CreateUserCommand{
			ID:    userIDs[i],
			Email: fmt.Sprintf("query%d@example.com", i),
			Name:  fmt.Sprintf("Query User %d", i),
		}

		err := system.CreateUserHandler.Handle(ctx, system.Logger, cmd)
		if err != nil {
			t.Fatalf("failed to create user %d: %v", i, err)
		}
	}

	// Wait for projections
	time.Sleep(1 * time.Second)

	// Test individual user queries
	t.Log("Testing individual user queries...")
	queryStart := time.Now()

	for i := 0; i < numUsers; i++ {
		query := internalapp.GetUserQuery{ID: userIDs[i]}
		_, err := system.GetUserHandler.Handle(ctx, system.Logger, query)
		if err != nil {
			t.Errorf("failed to query user %d: %v", i, err)
		}
	}

	queryDuration := time.Since(queryStart)
	queryRate := float64(numUsers) / queryDuration.Seconds()

	t.Logf("Completed %d individual queries in %v (%.2f queries/sec)", numUsers, queryDuration, queryRate)

	// Test email queries
	t.Log("Testing email queries...")
	emailQueryStart := time.Now()

	for i := 0; i < numUsers; i++ {
		query := internalapp.GetUserByEmailQuery{Email: fmt.Sprintf("query%d@example.com", i)}
		_, err := system.GetUserByEmailHandler.Handle(ctx, system.Logger, query)
		if err != nil {
			t.Errorf("failed to query user by email %d: %v", i, err)
		}
	}

	emailQueryDuration := time.Since(emailQueryStart)
	emailQueryRate := float64(numUsers) / emailQueryDuration.Seconds()

	t.Logf("Completed %d email queries in %v (%.2f queries/sec)", numUsers, emailQueryDuration, emailQueryRate)

	// Test list queries with different page sizes
	pageSizes := []int{10, 25, 50, 100}
	for _, pageSize := range pageSizes {
		t.Logf("Testing list query with page size %d...", pageSize)
		listStart := time.Now()

		query := internalapp.ListUsersQuery{
			Page:     1,
			PageSize: pageSize,
		}

		result, err := system.ListUsersHandler.Handle(ctx, system.Logger, query)
		if err != nil {
			t.Errorf("failed to list users with page size %d: %v", pageSize, err)
			continue
		}

		listDuration := time.Since(listStart)
		t.Logf("List query (page size %d) completed in %v, returned %d users",
			pageSize, listDuration, len(result.Users))

		// Performance assertion for list queries
		maxListDuration := 1 * time.Second
		if listDuration > maxListDuration {
			t.Errorf("list query too slow for page size %d: %v (max: %v)",
				pageSize, listDuration, maxListDuration)
		}
	}

	// Performance assertions
	maxQueryDuration := 5 * time.Second
	maxEmailQueryDuration := 5 * time.Second
	minQueryRate := 20.0 // queries per second

	if queryDuration > maxQueryDuration {
		t.Errorf("individual queries too slow: %v (max: %v)", queryDuration, maxQueryDuration)
	}

	if emailQueryDuration > maxEmailQueryDuration {
		t.Errorf("email queries too slow: %v (max: %v)", emailQueryDuration, maxEmailQueryDuration)
	}

	if queryRate < minQueryRate {
		t.Errorf("query rate too low: %.2f queries/sec (min: %.2f)", queryRate, minQueryRate)
	}

	if emailQueryRate < minQueryRate {
		t.Errorf("email query rate too low: %.2f queries/sec (min: %.2f)", emailQueryRate, minQueryRate)
	}
}

func testConcurrentReadWrite(t *testing.T, system *TestSystem) {
	ctx := context.Background()
	duration := 10 * time.Second

	t.Logf("Running concurrent read/write test for %v...", duration)

	var wg sync.WaitGroup
	var writeCount, readCount int64
	var writeErrors, readErrors int64

	// Start time
	start := time.Now()
	deadline := start.Add(duration)

	// Writer goroutines
	numWriters := 5
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()

			for time.Now().Before(deadline) {
				userID := uuid.New()
				cmd := internalapp.CreateUserCommand{
					ID:    userID,
					Email: fmt.Sprintf("rw%d_%d@example.com", writerID, time.Now().UnixNano()),
					Name:  fmt.Sprintf("RW User %d", writerID),
				}

				if err := system.CreateUserHandler.Handle(ctx, system.Logger, cmd); err != nil {
					atomic.AddInt64(&writeErrors, 1)
				} else {
					atomic.AddInt64(&writeCount, 1)
				}

				// Small delay to prevent overwhelming the system
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Reader goroutines
	numReaders := 10
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()

			for time.Now().Before(deadline) {
				// List users
				listQuery := internalapp.ListUsersQuery{
					Page:     1,
					PageSize: 10,
				}

				if _, err := system.ListUsersHandler.Handle(ctx, system.Logger, listQuery); err != nil {
					atomic.AddInt64(&readErrors, 1)
				} else {
					atomic.AddInt64(&readCount, 1)
				}

				// Small delay
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	actualDuration := time.Since(start)
	finalWriteCount := atomic.LoadInt64(&writeCount)
	finalReadCount := atomic.LoadInt64(&readCount)
	finalWriteErrors := atomic.LoadInt64(&writeErrors)
	finalReadErrors := atomic.LoadInt64(&readErrors)

	writeRate := float64(finalWriteCount) / actualDuration.Seconds()
	readRate := float64(finalReadCount) / actualDuration.Seconds()

	t.Logf("Concurrent read/write results:")
	t.Logf("  Duration: %v", actualDuration)
	t.Logf("  Writes: %d (%.2f/sec), Errors: %d", finalWriteCount, writeRate, finalWriteErrors)
	t.Logf("  Reads: %d (%.2f/sec), Errors: %d", finalReadCount, readRate, finalReadErrors)

	// Performance assertions
	minWriteRate := 5.0  // writes per second
	minReadRate := 20.0  // reads per second
	maxErrorRate := 0.05 // 5% error rate

	if writeRate < minWriteRate {
		t.Errorf("write rate too low: %.2f writes/sec (min: %.2f)", writeRate, minWriteRate)
	}

	if readRate < minReadRate {
		t.Errorf("read rate too low: %.2f reads/sec (min: %.2f)", readRate, minReadRate)
	}

	writeErrorRate := float64(finalWriteErrors) / float64(finalWriteCount+finalWriteErrors)
	if writeErrorRate > maxErrorRate {
		t.Errorf("write error rate too high: %.2f%% (max: %.2f%%)", writeErrorRate*100, maxErrorRate*100)
	}

	readErrorRate := float64(finalReadErrors) / float64(finalReadCount+finalReadErrors)
	if readErrorRate > maxErrorRate {
		t.Errorf("read error rate too high: %.2f%% (max: %.2f%%)", readErrorRate*100, maxErrorRate*100)
	}
}
