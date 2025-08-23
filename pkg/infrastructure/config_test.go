package infrastructure

import (
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear any existing environment variables
	clearEnvVars()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Test default values
	if config.Database.Driver != "sqlite" {
		t.Errorf("Expected default database driver 'sqlite', got '%s'", config.Database.Driver)
	}

	if config.Database.DSN != "file:events.db?cache=shared&mode=rwc" {
		t.Errorf("Expected default database DSN, got '%s'", config.Database.DSN)
	}

	if config.Events.Publisher != "channel" {
		t.Errorf("Expected default events publisher 'channel', got '%s'", config.Events.Publisher)
	}

	if config.Logging.Level != "info" {
		t.Errorf("Expected default logging level 'info', got '%s'", config.Logging.Level)
	}

	if config.Logging.Format != "text" {
		t.Errorf("Expected default logging format 'text', got '%s'", config.Logging.Format)
	}
}

func TestLoadConfig_EnvironmentVariables(t *testing.T) {
	// Clear any existing environment variables
	clearEnvVars()

	// Set environment variables
	os.Setenv("PERICARP_DATABASE_DRIVER", "postgres")
	os.Setenv("PERICARP_DATABASE_DSN", "host=localhost user=test password=test dbname=test port=5432 sslmode=disable")
	os.Setenv("PERICARP_EVENTS_PUBLISHER", "pubsub")
	os.Setenv("PERICARP_LOGGING_LEVEL", "debug")
	os.Setenv("PERICARP_LOGGING_FORMAT", "json")

	defer clearEnvVars()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Test environment variable values
	if config.Database.Driver != "postgres" {
		t.Errorf("Expected database driver 'postgres', got '%s'", config.Database.Driver)
	}

	expectedDSN := "host=localhost user=test password=test dbname=test port=5432 sslmode=disable"
	if config.Database.DSN != expectedDSN {
		t.Errorf("Expected database DSN '%s', got '%s'", expectedDSN, config.Database.DSN)
	}

	if config.Events.Publisher != "pubsub" {
		t.Errorf("Expected events publisher 'pubsub', got '%s'", config.Events.Publisher)
	}

	if config.Logging.Level != "debug" {
		t.Errorf("Expected logging level 'debug', got '%s'", config.Logging.Level)
	}

	if config.Logging.Format != "json" {
		t.Errorf("Expected logging format 'json', got '%s'", config.Logging.Format)
	}
}

func TestValidateConfig_InvalidDriver(t *testing.T) {
	config := &Config{
		Database: DatabaseConfig{
			Driver: "invalid",
			DSN:    "some-dsn",
		},
		Events: EventsConfig{
			Publisher: "channel",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}

	err := validateConfig(config)
	if err == nil {
		t.Error("Expected validation error for invalid database driver")
	}
}

func TestValidateConfig_EmptyDSN(t *testing.T) {
	config := &Config{
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "",
		},
		Events: EventsConfig{
			Publisher: "channel",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}

	err := validateConfig(config)
	if err == nil {
		t.Error("Expected validation error for empty DSN")
	}
}

func TestValidateConfig_InvalidPublisher(t *testing.T) {
	config := &Config{
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "file:test.db",
		},
		Events: EventsConfig{
			Publisher: "invalid",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}

	err := validateConfig(config)
	if err == nil {
		t.Error("Expected validation error for invalid events publisher")
	}
}

func TestValidateConfig_InvalidLoggingLevel(t *testing.T) {
	config := &Config{
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "file:test.db",
		},
		Events: EventsConfig{
			Publisher: "channel",
		},
		Logging: LoggingConfig{
			Level:  "invalid",
			Format: "text",
		},
	}

	err := validateConfig(config)
	if err == nil {
		t.Error("Expected validation error for invalid logging level")
	}
}

func TestValidateConfig_InvalidLoggingFormat(t *testing.T) {
	config := &Config{
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "file:test.db",
		},
		Events: EventsConfig{
			Publisher: "channel",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "invalid",
		},
	}

	err := validateConfig(config)
	if err == nil {
		t.Error("Expected validation error for invalid logging format")
	}
}

func TestGetSQLiteDSN(t *testing.T) {
	dsn := GetSQLiteDSN("test.db")
	expected := "file:test.db?cache=shared&mode=rwc"
	if dsn != expected {
		t.Errorf("Expected SQLite DSN '%s', got '%s'", expected, dsn)
	}
}

func TestGetPostgresDSN(t *testing.T) {
	dsn := GetPostgresDSN("localhost", "user", "pass", "dbname", 5432, "disable")
	expected := "host=localhost user=user password=pass dbname=dbname port=5432 sslmode=disable"
	if dsn != expected {
		t.Errorf("Expected PostgreSQL DSN '%s', got '%s'", expected, dsn)
	}
}

func TestGetPostgresDSN_DefaultSSLMode(t *testing.T) {
	dsn := GetPostgresDSN("localhost", "user", "pass", "dbname", 5432, "")
	expected := "host=localhost user=user password=pass dbname=dbname port=5432 sslmode=disable"
	if dsn != expected {
		t.Errorf("Expected PostgreSQL DSN with default sslmode '%s', got '%s'", expected, dsn)
	}
}

// clearEnvVars clears all PERICARP environment variables
func clearEnvVars() {
	envVars := []string{
		"PERICARP_DATABASE_DRIVER",
		"PERICARP_DATABASE_DSN",
		"PERICARP_EVENTS_PUBLISHER",
		"PERICARP_LOGGING_LEVEL",
		"PERICARP_LOGGING_FORMAT",
	}

	for _, envVar := range envVars {
		os.Unsetenv(envVar)
	}
}