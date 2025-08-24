package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestConfig holds configuration for BDD tests
type TestConfig struct {
	DatabaseURL string
	LogLevel    string
}

// NewTestConfig creates a new test configuration
func NewTestConfig() *TestConfig {
	return &TestConfig{
		DatabaseURL: ":memory:", // Use in-memory SQLite for tests
		LogLevel:    "error",    // Reduce log noise during tests
	}
}

// GetTestDatabase creates a test database connection
func (c *TestConfig) GetTestDatabase() (*gorm.DB, error) {
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Silent logging for tests
	}

	db, err := gorm.Open(sqlite.Open(c.DatabaseURL), config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to test database: %w", err)
	}

	return db, nil
}

// GetTestDatabaseFile creates a temporary file-based SQLite database for tests
func (c *TestConfig) GetTestDatabaseFile() (*gorm.DB, string, error) {
	// Create temporary file
	tempDir := os.TempDir()
	dbFile := filepath.Join(tempDir, fmt.Sprintf("test_%d.db", os.Getpid()))

	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	db, err := gorm.Open(sqlite.Open(dbFile), config)
	if err != nil {
		return nil, "", fmt.Errorf("failed to connect to test database file: %w", err)
	}

	return db, dbFile, nil
}

// CleanupTestDatabase removes the test database file
func (c *TestConfig) CleanupTestDatabase(dbFile string) error {
	if dbFile != "" && dbFile != ":memory:" {
		return os.Remove(dbFile)
	}
	return nil
}

// TestEnvironment provides utilities for setting up test environment
type TestEnvironment struct {
	config *TestConfig
}

// NewTestEnvironment creates a new test environment
func NewTestEnvironment() *TestEnvironment {
	return &TestEnvironment{
		config: NewTestConfig(),
	}
}

// Setup prepares the test environment
func (e *TestEnvironment) Setup() error {
	// Set environment variables for testing
	os.Setenv("PERICARP_LOG_LEVEL", e.config.LogLevel)
	os.Setenv("PERICARP_DATABASE_DRIVER", "sqlite")
	os.Setenv("PERICARP_DATABASE_DSN", e.config.DatabaseURL)

	return nil
}

// Teardown cleans up the test environment
func (e *TestEnvironment) Teardown() error {
	// Clean up environment variables
	os.Unsetenv("PERICARP_LOG_LEVEL")
	os.Unsetenv("PERICARP_DATABASE_DRIVER")
	os.Unsetenv("PERICARP_DATABASE_DSN")

	return nil
}

// GetConfig returns the test configuration
func (e *TestEnvironment) GetConfig() *TestConfig {
	return e.config
}
