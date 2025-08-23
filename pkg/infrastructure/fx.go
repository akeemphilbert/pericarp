package infrastructure

import (
	"context"

	"github.com/example/pericarp/pkg/application"
	"github.com/example/pericarp/pkg/domain"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

// InfrastructureModule provides all infrastructure layer dependencies
var InfrastructureModule = fx.Options(
	fx.Provide(
		LoadConfig,
		DatabaseProvider,
		EventStoreProvider,
		EventDispatcherProvider,
		UnitOfWorkProvider,
		LoggerProvider,
		MetricsProvider,
	),
	fx.Invoke(
		// Register lifecycle hooks
		registerDatabaseLifecycle,
		registerEventDispatcherLifecycle,
	),
)

// registerDatabaseLifecycle registers database connection lifecycle management
func registerDatabaseLifecycle(lc fx.Lifecycle, db *gorm.DB, logger domain.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting database connection")
			// Test database connection
			sqlDB, err := db.DB()
			if err != nil {
				logger.Error("Failed to get underlying database connection", "error", err)
				return err
			}
			
			if err := sqlDB.PingContext(ctx); err != nil {
				logger.Error("Failed to ping database", "error", err)
				return err
			}
			
			logger.Info("Database connection established successfully")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Closing database connection")
			// Close database connection
			sqlDB, err := db.DB()
			if err != nil {
				logger.Error("Failed to get underlying database connection for closing", "error", err)
				return err
			}
			
			if err := sqlDB.Close(); err != nil {
				logger.Error("Failed to close database connection", "error", err)
				return err
			}
			
			logger.Info("Database connection closed successfully")
			return nil
		},
	})
}

// registerEventDispatcherLifecycle registers event dispatcher lifecycle management
func registerEventDispatcherLifecycle(lc fx.Lifecycle, dispatcher domain.EventDispatcher, logger domain.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting event dispatcher")
			// Initialize event dispatcher if needed
			if initializer, ok := dispatcher.(interface{ Initialize(context.Context) error }); ok {
				if err := initializer.Initialize(ctx); err != nil {
					logger.Error("Failed to initialize event dispatcher", "error", err)
					return err
				}
			}
			logger.Info("Event dispatcher started successfully")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping event dispatcher")
			// Shutdown event dispatcher gracefully
			if closer, ok := dispatcher.(interface{ Close() error }); ok {
				if err := closer.Close(); err != nil {
					logger.Error("Failed to close event dispatcher", "error", err)
					return err
				}
			}
			logger.Info("Event dispatcher stopped successfully")
			return nil
		},
	})
}

// DatabaseProvider creates a database connection from config
func DatabaseProvider(config *Config) (*gorm.DB, error) {
	return NewDatabase(config.Database)
}

// EventStoreProvider creates an event store with database dependency
func EventStoreProvider(db *gorm.DB) (domain.EventStore, error) {
	return NewGormEventStore(db)
}

// EventDispatcherProvider creates an event dispatcher based on config
func EventDispatcherProvider(config *Config) (domain.EventDispatcher, error) {
	switch config.Events.Publisher {
	case "channel":
		return NewWatermillEventDispatcher(nil)
	case "pubsub":
		// For now, return watermill dispatcher as pubsub is not implemented yet
		return NewWatermillEventDispatcher(nil)
	default:
		return NewWatermillEventDispatcher(nil)
	}
}

// UnitOfWorkProvider creates a unit of work with dependencies
func UnitOfWorkProvider(eventStore domain.EventStore, dispatcher domain.EventDispatcher) domain.UnitOfWork {
	return NewUnitOfWork(eventStore, dispatcher)
}

// LoggerProvider creates a logger based on config
func LoggerProvider(config *Config) domain.Logger {
	return NewLogger(config.Logging.Level, config.Logging.Format)
}

// MetricsProvider creates a metrics collector
func MetricsProvider(logger domain.Logger) application.MetricsCollector {
	return NewSimpleMetricsCollector(logger)
}