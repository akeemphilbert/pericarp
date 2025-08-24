package infrastructure

import (
	"github.com/example/pericarp/pkg/domain"
	"go.uber.org/fx"
)

// InfrastructureModule provides all infrastructure layer dependencies
var InfrastructureModule = fx.Options(
	fx.Provide(
		ConfigProvider,
		DatabaseProvider,
		EventStoreProvider,
		EventDispatcherProvider,
		UnitOfWorkProvider,
		LoggerProvider,
	),
)

// ConfigProvider loads the application configuration
func ConfigProvider() (*Config, error) {
	return LoadConfig()
}

// DatabaseProvider creates a database wrapper
func DatabaseProvider(config *Config) (*Database, error) {
	return NewDatabaseWrapper(config.Database)
}

// EventStoreProvider creates a GORM-based event store
func EventStoreProvider(db *Database) domain.EventStore {
	store, err := NewGormEventStore(db.DB)
	if err != nil {
		panic(err) // In production, handle this more gracefully
	}
	return store
}

// EventDispatcherProvider creates a Watermill-based event dispatcher
func EventDispatcherProvider(logger domain.Logger) domain.EventDispatcher {
	watermillLogger := &WatermillLoggerAdapter{logger: logger}
	dispatcher, err := NewWatermillEventDispatcher(watermillLogger)
	if err != nil {
		panic(err) // In production, handle this more gracefully
	}
	return dispatcher
}

// UnitOfWorkProvider creates a unit of work implementation
func UnitOfWorkProvider(eventStore domain.EventStore, dispatcher domain.EventDispatcher) domain.UnitOfWork {
	return NewUnitOfWork(eventStore, dispatcher)
}

// LoggerProvider creates a logger implementation
func LoggerProvider() domain.Logger {
	return NewLogger("info", "text") // Default to info level and text format
}
