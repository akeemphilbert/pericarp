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
func EventStoreProvider(db *Database) (domain.EventStore, error) {
	return NewGormEventStore(db.DB)
}

// EventDispatcherProvider creates a Watermill-based event dispatcher
func EventDispatcherProvider(logger domain.Logger) (domain.EventDispatcher, error) {
	watermillLogger := &WatermillLoggerAdapter{Logger: logger}
	return NewWatermillEventDispatcher(watermillLogger)
}

// UnitOfWorkProvider creates a unit of work implementation
func UnitOfWorkProvider(eventStore domain.EventStore, dispatcher domain.EventDispatcher) domain.UnitOfWork {
	return NewUnitOfWork(eventStore, dispatcher)
}

// LoggerProvider creates a logger implementation
func LoggerProvider(config *Config) domain.Logger {
	return NewLogger(config.Logging.Level, config.Logging.Format)
}
