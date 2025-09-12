//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/akeemphilbert/pericarp/examples"
	pkgdomain "github.com/akeemphilbert/pericarp/pkg/domain"
	pkginfra "github.com/akeemphilbert/pericarp/pkg/infrastructure"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestEndToEndFlow tests complete event sourcing flows using examples
func TestEndToEndFlow(t *testing.T) {
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
			testEndToEndWithDatabase(t, tt.dbDriver, tt.dsn)
		})
	}
}

func testEndToEndWithDatabase(t *testing.T, driver, dsn string) {
	// Setup system
	system, cleanup := setupSystem(t, driver, dsn)
	defer cleanup()

	t.Run("UserLifecycle", func(t *testing.T) {
		testUserLifecycle(t, system)
	})

	t.Run("EventSourcingConsistency", func(t *testing.T) {
		testEventSourcingConsistency(t, system)
	})

	t.Run("ConcurrentOperations", func(t *testing.T) {
		testConcurrentOperations(t, system)
	})
}

// TestSystem encapsulates all system components
type TestSystem struct {
	DB              *gorm.DB
	Logger          pkgdomain.Logger
	EventStore      pkgdomain.EventStore
	EventDispatcher pkgdomain.EventDispatcher
}

func setupSystem(t *testing.T, driver, dsn string) (*TestSystem, func()) {
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

	// Setup logger
	logger := pkgdomain.NewLogger("test")

	// Setup event store and dispatcher
	eventStore := pkginfra.NewEventStore(db, logger)
	eventDispatcher := pkginfra.NewEventDispatcher(logger)

	// Register event handlers
	eventDispatcher.RegisterHandler("user", "created", func(ctx context.Context, event pkgdomain.Event) error {
		logger.Info("User created event handled", "event_id", event.ID())
		return nil
	})

	eventDispatcher.RegisterHandler("user", "email_changed", func(ctx context.Context, event pkgdomain.Event) error {
		logger.Info("User email changed event handled", "event_id", event.ID())
		return nil
	})

	eventDispatcher.RegisterHandler("user", "activated", func(ctx context.Context, event pkgdomain.Event) error {
		logger.Info("User activated event handled", "event_id", event.ID())
		return nil
	})

	eventDispatcher.RegisterHandler("user", "deactivated", func(ctx context.Context, event pkgdomain.Event) error {
		logger.Info("User deactivated event handled", "event_id", event.ID())
		return nil
	})

	system := &TestSystem{
		DB:              db,
		Logger:          logger,
		EventStore:      eventStore,
		EventDispatcher: eventDispatcher,
	}

	cleanup := func() {
		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.Close()
		}
	}

	return system, cleanup
}

func testUserLifecycle(t *testing.T, system *TestSystem) {
	ctx := context.Background()
	userID := uuid.New().String()
	email := "test@example.com"
	name := "Test User"

	// Create user
	user, err := examples.NewUser(userID, email, name)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Verify initial state
	if user.ID() != userID {
		t.Errorf("expected user ID %s, got %s", userID, user.ID())
	}
	if user.Email() != email {
		t.Errorf("expected email %s, got %s", email, user.Email())
	}
	if user.Name() != name {
		t.Errorf("expected name %s, got %s", name, user.Name())
	}
	if !user.IsActive() {
		t.Error("expected user to be active")
	}

	// Save initial events
	events := user.GetEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	for _, event := range events {
		if err := system.EventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save event: %v", err)
		}
		if err := system.EventDispatcher.Dispatch(ctx, event); err != nil {
			t.Errorf("failed to dispatch event: %v", err)
		}
	}

	// Change email
	newEmail := "newemail@example.com"
	if err := user.ChangeEmail(newEmail); err != nil {
		t.Fatalf("failed to change email: %v", err)
	}

	if user.Email() != newEmail {
		t.Errorf("expected email %s, got %s", newEmail, user.Email())
	}

	// Save email change events
	emailEvents := user.GetEvents()
	if len(emailEvents) != 1 {
		t.Errorf("expected 1 new event, got %d", len(emailEvents))
	}

	for _, event := range emailEvents {
		if err := system.EventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save email change event: %v", err)
		}
		if err := system.EventDispatcher.Dispatch(ctx, event); err != nil {
			t.Errorf("failed to dispatch email change event: %v", err)
		}
	}

	// Deactivate user
	if err := user.Deactivate(); err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	if user.IsActive() {
		t.Error("expected user to be inactive")
	}

	// Save deactivation events
	deactivateEvents := user.GetEvents()
	if len(deactivateEvents) != 1 {
		t.Errorf("expected 1 new event, got %d", len(deactivateEvents))
	}

	for _, event := range deactivateEvents {
		if err := system.EventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save deactivation event: %v", err)
		}
		if err := system.EventDispatcher.Dispatch(ctx, event); err != nil {
			t.Errorf("failed to dispatch deactivation event: %v", err)
		}
	}

	// Activate user again
	if err := user.Activate(); err != nil {
		t.Fatalf("failed to activate user: %v", err)
	}

	if !user.IsActive() {
		t.Error("expected user to be active")
	}

	// Save activation events
	activateEvents := user.GetEvents()
	if len(activateEvents) != 1 {
		t.Errorf("expected 1 new event, got %d", len(activateEvents))
	}

	for _, event := range activateEvents {
		if err := system.EventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save activation event: %v", err)
		}
		if err := system.EventDispatcher.Dispatch(ctx, event); err != nil {
			t.Errorf("failed to dispatch activation event: %v", err)
		}
	}
}

func testEventSourcingConsistency(t *testing.T, system *TestSystem) {
	ctx := context.Background()
	userID := uuid.New().String()
	email := "eventsourcing@example.com"
	name := "Event Sourcing User"

	// Create user and perform operations
	user, err := examples.NewUser(userID, email, name)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Save all events
	allEvents := user.GetEvents()
	for _, event := range allEvents {
		if err := system.EventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save event: %v", err)
		}
	}

	// Change email
	if err := user.ChangeEmail("newemail@example.com"); err != nil {
		t.Fatalf("failed to change email: %v", err)
	}

	emailEvents := user.GetEvents()
	for _, event := range emailEvents {
		if err := system.EventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save email change event: %v", err)
		}
	}

	// Deactivate
	if err := user.Deactivate(); err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	deactivateEvents := user.GetEvents()
	for _, event := range deactivateEvents {
		if err := system.EventStore.Save(ctx, event); err != nil {
			t.Fatalf("failed to save deactivation event: %v", err)
		}
	}

	// Load user from event store
	loadedEvents, err := system.EventStore.GetEvents(ctx, userID, 0)
	if err != nil {
		t.Fatalf("failed to load events: %v", err)
	}

	if len(loadedEvents) != 3 {
		t.Errorf("expected 3 events, got %d", len(loadedEvents))
	}

	// Reconstruct user from events
	reconstructedUser := &examples.User{}
	reconstructedUser.LoadFromHistory(loadedEvents)

	// Verify reconstructed user matches original
	if reconstructedUser.ID() != user.ID() {
		t.Errorf("expected ID %s, got %s", user.ID(), reconstructedUser.ID())
	}
	if reconstructedUser.Email() != user.Email() {
		t.Errorf("expected email %s, got %s", user.Email(), reconstructedUser.Email())
	}
	if reconstructedUser.Name() != user.Name() {
		t.Errorf("expected name %s, got %s", user.Name(), reconstructedUser.Name())
	}
	if reconstructedUser.IsActive() != user.IsActive() {
		t.Errorf("expected active %t, got %t", user.IsActive(), reconstructedUser.IsActive())
	}
}

func testConcurrentOperations(t *testing.T, system *TestSystem) {
	ctx := context.Background()
	numUsers := 10
	users := make([]*examples.User, numUsers)

	// Create users concurrently
	done := make(chan error, numUsers)
	for i := 0; i < numUsers; i++ {
		go func(i int) {
			userID := uuid.New().String()
			email := fmt.Sprintf("user%d@example.com", i)
			name := fmt.Sprintf("User %d", i)

			user, err := examples.NewUser(userID, email, name)
			if err != nil {
				done <- err
				return
			}

			users[i] = user

			// Save events
			events := user.GetEvents()
			for _, event := range events {
				if err := system.EventStore.Save(ctx, event); err != nil {
					done <- err
					return
				}
			}

			done <- nil
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numUsers; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent operation failed: %v", err)
		}
	}

	// Verify all users were created
	for i, user := range users {
		if user == nil {
			t.Errorf("user %d was not created", i)
			continue
		}

		// Load user from event store to verify persistence
		events, err := system.EventStore.GetEvents(ctx, user.ID(), 0)
		if err != nil {
			t.Errorf("failed to load events for user %d: %v", i, err)
			continue
		}

		if len(events) == 0 {
			t.Errorf("no events found for user %d", i)
			continue
		}

		// Reconstruct and verify
		reconstructed := &examples.User{}
		reconstructed.LoadFromHistory(events)

		if reconstructed.ID() != user.ID() {
			t.Errorf("user %d: expected ID %s, got %s", i, user.ID(), reconstructed.ID())
		}
	}
}
