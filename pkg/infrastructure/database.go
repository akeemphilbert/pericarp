package infrastructure

import (
	"fmt"
	"time"

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

// Database wraps GORM DB with additional functionality
type Database struct {
	*gorm.DB
	config DatabaseConfig
}

// NewDatabaseWrapper creates a new Database wrapper
func NewDatabaseWrapper(config DatabaseConfig) (*Database, error) {
	db, err := NewDatabase(config)
	if err != nil {
		return nil, err
	}

	return &Database{
		DB:     db,
		config: config,
	}, nil
}

// Migrate runs database migrations for all required tables
func (d *Database) Migrate() error {
	// Migrate EventRecord table
	if err := d.AutoMigrate(&EventRecord{}); err != nil {
		return fmt.Errorf("failed to migrate events table: %w", err)
	}

	// Migrate UserReadModelGORM table (we need to import it)
	userReadModel := &struct {
		ID        string    `gorm:"primaryKey;type:varchar(36)"`
		Email     string    `gorm:"uniqueIndex;type:varchar(255);not null"`
		Name      string    `gorm:"type:varchar(255);not null"`
		IsActive  bool      `gorm:"not null;default:true"`
		CreatedAt time.Time `gorm:"not null"`
		UpdatedAt time.Time `gorm:"not null"`
	}{}

	// Set table name for migration
	if err := d.Table("user_read_models").AutoMigrate(userReadModel); err != nil {
		return fmt.Errorf("failed to migrate user_read_models table: %w", err)
	}

	return nil
}

// GetConfig returns the database configuration
func (d *Database) GetConfig() DatabaseConfig {
	return d.config
}

// HealthCheck performs a basic health check on the database connection
func (d *Database) HealthCheck() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}
