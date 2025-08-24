package infrastructure

import (
	"time"
)

// PerformanceConfig contains configuration options for performance optimization
type PerformanceConfig struct {
	// EventStore configuration
	EventStore EventStoreConfig `mapstructure:"event_store"`

	// UnitOfWork configuration
	UnitOfWork UnitOfWorkConfig `mapstructure:"unit_of_work"`

	// Middleware configuration
	Middleware MiddlewareConfig `mapstructure:"middleware"`
}

// EventStoreConfig contains event store performance settings
type EventStoreConfig struct {
	// BatchSize for bulk operations (default: 100)
	BatchSize int `mapstructure:"batch_size"`

	// MaxEventHistory limits the number of events loaded per aggregate (default: 10000)
	MaxEventHistory int `mapstructure:"max_event_history"`

	// ConnectionPoolSize for database connections (default: 10)
	ConnectionPoolSize int `mapstructure:"connection_pool_size"`

	// QueryTimeout for database queries (default: 30s)
	QueryTimeout time.Duration `mapstructure:"query_timeout"`

	// EnableQueryOptimization enables query optimization hints (default: true)
	EnableQueryOptimization bool `mapstructure:"enable_query_optimization"`
}

// UnitOfWorkConfig contains unit of work performance settings
type UnitOfWorkConfig struct {
	// InitialEventCapacity for pre-allocating event slices (default: 10)
	InitialEventCapacity int `mapstructure:"initial_event_capacity"`

	// MaxEventsPerTransaction limits events per transaction (default: 1000)
	MaxEventsPerTransaction int `mapstructure:"max_events_per_transaction"`

	// EnableAsyncDispatch enables asynchronous event dispatching (default: false)
	EnableAsyncDispatch bool `mapstructure:"enable_async_dispatch"`
}

// MiddlewareConfig contains middleware performance settings
type MiddlewareConfig struct {
	// EnableMetrics enables metrics collection (default: true)
	EnableMetrics bool `mapstructure:"enable_metrics"`

	// MetricsBufferSize for metrics collection (default: 1000)
	MetricsBufferSize int `mapstructure:"metrics_buffer_size"`

	// EnableDetailedLogging enables detailed request logging (default: false in production)
	EnableDetailedLogging bool `mapstructure:"enable_detailed_logging"`

	// CacheSize for query result caching (default: 1000)
	CacheSize int `mapstructure:"cache_size"`

	// CacheTTL for cache expiration (default: 5m)
	CacheTTL time.Duration `mapstructure:"cache_ttl"`
}

// DefaultPerformanceConfig returns default performance configuration
func DefaultPerformanceConfig() PerformanceConfig {
	return PerformanceConfig{
		EventStore: EventStoreConfig{
			BatchSize:               100,
			MaxEventHistory:         10000,
			ConnectionPoolSize:      10,
			QueryTimeout:            30 * time.Second,
			EnableQueryOptimization: true,
		},
		UnitOfWork: UnitOfWorkConfig{
			InitialEventCapacity:    10,
			MaxEventsPerTransaction: 1000,
			EnableAsyncDispatch:     false,
		},
		Middleware: MiddlewareConfig{
			EnableMetrics:         true,
			MetricsBufferSize:     1000,
			EnableDetailedLogging: false,
			CacheSize:             1000,
			CacheTTL:              5 * time.Minute,
		},
	}
}

// ProductionPerformanceConfig returns optimized configuration for production
func ProductionPerformanceConfig() PerformanceConfig {
	config := DefaultPerformanceConfig()

	// Production optimizations
	config.EventStore.BatchSize = 200
	config.EventStore.ConnectionPoolSize = 20
	config.EventStore.QueryTimeout = 10 * time.Second

	config.UnitOfWork.InitialEventCapacity = 20
	config.UnitOfWork.EnableAsyncDispatch = true

	config.Middleware.EnableDetailedLogging = false
	config.Middleware.MetricsBufferSize = 5000
	config.Middleware.CacheSize = 10000
	config.Middleware.CacheTTL = 10 * time.Minute

	return config
}

// DevelopmentPerformanceConfig returns configuration optimized for development
func DevelopmentPerformanceConfig() PerformanceConfig {
	config := DefaultPerformanceConfig()

	// Development optimizations (favor debugging over performance)
	config.EventStore.BatchSize = 50
	config.EventStore.ConnectionPoolSize = 5
	config.EventStore.QueryTimeout = 60 * time.Second

	config.UnitOfWork.InitialEventCapacity = 5
	config.UnitOfWork.EnableAsyncDispatch = false

	config.Middleware.EnableDetailedLogging = true
	config.Middleware.MetricsBufferSize = 100
	config.Middleware.CacheSize = 100
	config.Middleware.CacheTTL = 1 * time.Minute

	return config
}

// TestPerformanceConfig returns configuration optimized for testing
func TestPerformanceConfig() PerformanceConfig {
	config := DefaultPerformanceConfig()

	// Test optimizations (favor speed and determinism)
	config.EventStore.BatchSize = 10
	config.EventStore.ConnectionPoolSize = 2
	config.EventStore.QueryTimeout = 5 * time.Second

	config.UnitOfWork.InitialEventCapacity = 2
	config.UnitOfWork.EnableAsyncDispatch = false // Synchronous for deterministic tests

	config.Middleware.EnableDetailedLogging = false
	config.Middleware.EnableMetrics = false // Disable metrics in tests
	config.Middleware.CacheSize = 10
	config.Middleware.CacheTTL = 10 * time.Second

	return config
}
