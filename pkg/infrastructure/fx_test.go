package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/domain"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestInfrastructureModule(t *testing.T) {
	app := fxtest.New(t,
		InfrastructureModule,
		fx.Invoke(func(
			config *Config,
			logger domain.Logger,
			eventStore domain.EventStore,
			eventDispatcher domain.EventDispatcher,
			unitOfWork domain.UnitOfWork,
		) {
			// Test that all dependencies are properly injected
			if config == nil {
				t.Error("Config should not be nil")
			}
			if logger == nil {
				t.Error("Logger should not be nil")
			}
			if eventStore == nil {
				t.Error("EventStore should not be nil")
			}
			if eventDispatcher == nil {
				t.Error("EventDispatcher should not be nil")
			}
			if unitOfWork == nil {
				t.Error("UnitOfWork should not be nil")
			}

			// Test logger functionality
			logger.Info("Test log message", "key", "value")
			logger.Debug("Debug message")
			logger.Warn("Warning message")
			logger.Error("Error message")
		}),
	)

	defer app.RequireStart().RequireStop()
}

func TestDatabaseProvider(t *testing.T) {
	config := &Config{
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    ":memory:",
		},
	}

	db, err := DatabaseProvider(config)
	if err != nil {
		t.Fatalf("DatabaseProvider failed: %v", err)
	}

	if db == nil {
		t.Error("Database should not be nil")
	}

	// Test database connection
	sqlDB, err := db.DB.DB()
	if err != nil {
		t.Fatalf("Failed to get SQL DB: %v", err)
	}

	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Database ping failed: %v", err)
	}
}

func TestEventStoreProvider(t *testing.T) {
	config := &Config{
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    ":memory:",
		},
	}

	db, err := DatabaseProvider(config)
	if err != nil {
		t.Fatalf("DatabaseProvider failed: %v", err)
	}

	eventStore, err := EventStoreProvider(db)
	if err != nil {
		t.Fatalf("EventStoreProvider failed: %v", err)
	}

	if eventStore == nil {
		t.Error("EventStore should not be nil")
	}

	// Test basic event store functionality
	ctx := context.Background()
	events := []domain.Event{} // Empty slice for now
	envelopes, err := eventStore.Save(ctx, events)
	if err != nil {
		t.Fatalf("EventStore.Save failed: %v", err)
	}

	if len(envelopes) != 0 {
		t.Errorf("Expected 0 envelopes, got %d", len(envelopes))
	}
}

func TestEventDispatcherProvider(t *testing.T) {
	config := &Config{
		Events: EventsConfig{
			Publisher: "channel",
		},
	}

	logger := LoggerProvider(config)
	dispatcher, err := EventDispatcherProvider(logger)
	if err != nil {
		t.Fatalf("EventDispatcherProvider failed: %v", err)
	}

	if dispatcher == nil {
		t.Error("EventDispatcher should not be nil")
	}

	// Test basic dispatcher functionality
	ctx := context.Background()
	envelopes := []domain.Envelope{} // Empty slice for now
	err = dispatcher.Dispatch(ctx, envelopes)
	if err != nil {
		t.Fatalf("EventDispatcher.Dispatch failed: %v", err)
	}
}

func TestLoggerProvider(t *testing.T) {
	config := &Config{
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}

	logger := LoggerProvider(config)
	if logger == nil {
		t.Error("Logger should not be nil")
	}

	// Test logger methods don't panic
	logger.Debug("Debug message")
	logger.Info("Info message")
	logger.Warn("Warning message")
	logger.Error("Error message")
	logger.Debugf("Debug formatted: %s", "test")
	logger.Infof("Info formatted: %d", 42)
}

func TestUnitOfWorkProvider(t *testing.T) {
	config := &Config{
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    ":memory:",
		},
		Events: EventsConfig{
			Publisher: "channel",
		},
	}

	db, err := DatabaseProvider(config)
	if err != nil {
		t.Fatalf("DatabaseProvider failed: %v", err)
	}

	eventStore, err := EventStoreProvider(db)
	if err != nil {
		t.Fatalf("EventStoreProvider failed: %v", err)
	}

	logger := LoggerProvider(config)
	dispatcher, err := EventDispatcherProvider(logger)
	if err != nil {
		t.Fatalf("EventDispatcherProvider failed: %v", err)
	}

	unitOfWork := UnitOfWorkProvider(eventStore, dispatcher)
	if unitOfWork == nil {
		t.Error("UnitOfWork should not be nil")
	}

	// Test basic unit of work functionality
	ctx := context.Background()
	events := []domain.Event{} // Empty slice for now
	unitOfWork.RegisterEvents(events)

	envelopes, err := unitOfWork.Commit(ctx)
	if err != nil {
		t.Fatalf("UnitOfWork.Commit failed: %v", err)
	}

	if len(envelopes) != 0 {
		t.Errorf("Expected 0 envelopes, got %d", len(envelopes))
	}
}

func TestLifecycleHooks(t *testing.T) {
	app := fxtest.New(t,
		InfrastructureModule,
		fx.StartTimeout(5*time.Second),
		fx.StopTimeout(5*time.Second),
	)

	// Test that the app can start and stop without errors
	defer app.RequireStart().RequireStop()
}
