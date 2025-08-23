package infrastructure

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	Database DatabaseConfig `mapstructure:"database"`
	Events   EventsConfig   `mapstructure:"events"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

// EventsConfig holds event system configuration
type EventsConfig struct {
	Publisher string `mapstructure:"publisher"` // channel, pubsub
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`  // debug, info, warn, error, fatal
	Format string `mapstructure:"format"` // json, text
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("./config")

	// Environment variable support
	viper.AutomaticEnv()
	viper.SetEnvPrefix("PERICARP")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set defaults
	setDefaults()

	// Read config file (optional)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found is OK, we'll use defaults and env vars
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	// Database defaults
	viper.SetDefault("database.driver", "sqlite")
	viper.SetDefault("database.dsn", "file:events.db?cache=shared&mode=rwc")

	// Events defaults
	viper.SetDefault("events.publisher", "channel")

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "text")
}

// validateConfig validates the configuration values
func validateConfig(config *Config) error {
	// Validate database driver
	switch config.Database.Driver {
	case "sqlite", "postgres":
		// Valid drivers
	default:
		return fmt.Errorf("unsupported database driver: %s (supported: sqlite, postgres)", config.Database.Driver)
	}

	// Validate DSN is not empty
	if config.Database.DSN == "" {
		return fmt.Errorf("database DSN cannot be empty")
	}

	// Validate events publisher
	switch config.Events.Publisher {
	case "channel", "pubsub":
		// Valid publishers
	default:
		return fmt.Errorf("unsupported events publisher: %s (supported: channel, pubsub)", config.Events.Publisher)
	}

	// Validate logging level
	switch config.Logging.Level {
	case "debug", "info", "warn", "error", "fatal":
		// Valid levels
	default:
		return fmt.Errorf("unsupported logging level: %s (supported: debug, info, warn, error, fatal)", config.Logging.Level)
	}

	// Validate logging format
	switch config.Logging.Format {
	case "json", "text":
		// Valid formats
	default:
		return fmt.Errorf("unsupported logging format: %s (supported: json, text)", config.Logging.Format)
	}

	return nil
}

// GetSQLiteDSN returns a SQLite DSN for the given database file
func GetSQLiteDSN(dbFile string) string {
	return fmt.Sprintf("file:%s?cache=shared&mode=rwc", dbFile)
}

// GetPostgresDSN returns a PostgreSQL DSN with the given parameters
func GetPostgresDSN(host, user, password, dbname string, port int, sslmode string) string {
	if sslmode == "" {
		sslmode = "disable"
	}
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s",
		host, user, password, dbname, port, sslmode)
}