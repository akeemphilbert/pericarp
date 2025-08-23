package infrastructure

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver string // "sqlite" or "postgres"
	DSN    string // Data Source Name
}

// NewDatabase creates a new GORM database connection based on the configuration
func NewDatabase(config DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch config.Driver {
	case "sqlite":
		dialector = sqlite.Open(config.DSN)
	case "postgres":
		dialector = postgres.Open(config.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", config.Driver)
	}

	// Configure GORM with appropriate settings
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

// DefaultSQLiteConfig returns a default SQLite configuration for development
func DefaultSQLiteConfig() DatabaseConfig {
	return DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:events.db?cache=shared&mode=rwc",
	}
}

// DefaultPostgreSQLConfig returns a default PostgreSQL configuration template
func DefaultPostgreSQLConfig(host, user, password, dbname string, port int) DatabaseConfig {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable",
		host, user, password, dbname, port)
	
	return DatabaseConfig{
		Driver: "postgres",
		DSN:    dsn,
	}
}