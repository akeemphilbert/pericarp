package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

// StoreSpec identifies an event store to open. It is populated from CLI flags
// or an HTTP request body (hence the JSON tags).
type StoreSpec struct {
	Backend  string `json:"backend"`  // sqlite | postgres | dynamo
	DSN      string `json:"dsn"`      // gorm DSN / sqlite file path (sqlite, postgres)
	Table    string `json:"table"`    // table name (dynamo)
	Endpoint string `json:"endpoint"` // custom endpoint (dynamo, optional; e.g. dynamodb-local)
	Region   string `json:"region"`   // AWS region (dynamo, optional)
}

// addStoreFlags registers the store-selection flags on fs, defaulting to the
// conventional environment variables.
func addStoreFlags(fs *flag.FlagSet, spec *StoreSpec) {
	fs.StringVar(&spec.Backend, "backend", os.Getenv("PERICARP_BACKEND"), "backend: sqlite|postgres|dynamo")
	fs.StringVar(&spec.DSN, "dsn", os.Getenv("PERICARP_DSN"), "DSN / sqlite file path (sqlite, postgres)")
	fs.StringVar(&spec.Table, "table", os.Getenv("PERICARP_TABLE"), "table name (dynamo)")
	fs.StringVar(&spec.Endpoint, "endpoint", os.Getenv("PERICARP_DYNAMO_ENDPOINT"), "custom endpoint (dynamo, optional)")
	fs.StringVar(&spec.Region, "region", os.Getenv("AWS_REGION"), "AWS region (dynamo, optional)")
}

// validate checks that the spec has the fields its backend requires.
func (s StoreSpec) validate() error {
	switch strings.ToLower(s.Backend) {
	case "":
		return fmt.Errorf("backend is required (sqlite|postgres|dynamo)")
	case "sqlite", "postgres":
		if s.DSN == "" {
			return fmt.Errorf("%s backend requires a dsn", strings.ToLower(s.Backend))
		}
	case "dynamo":
		if s.Table == "" {
			return fmt.Errorf("dynamo backend requires a table")
		}
	default:
		return fmt.Errorf("unknown backend %q (want sqlite, postgres, or dynamo)", s.Backend)
	}
	return nil
}

// openStore opens the event store described by spec and returns it with a
// closer that releases any underlying connections. The store implementations'
// own Close() is a no-op, so the closer owns the *sql.DB lifecycle.
func openStore(ctx context.Context, spec StoreSpec) (domain.EventStore, func() error, error) {
	if err := spec.validate(); err != nil {
		return nil, nil, err
	}
	switch strings.ToLower(spec.Backend) {
	case "sqlite":
		return openGorm(sqlite.Open(spec.DSN))
	case "postgres":
		return openGorm(postgres.Open(spec.DSN))
	case "dynamo":
		return openDynamo(ctx, spec)
	default:
		// unreachable: validate() already rejected unknown backends
		return nil, nil, fmt.Errorf("unknown backend %q", spec.Backend)
	}
}

// openGorm opens a GORM-backed store (SQLite or Postgres). NewGormEventStore
// self-provisions the events table and global-ordering machinery.
func openGorm(dialector gorm.Dialector) (domain.EventStore, func() error, error) {
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	store, err := infrastructure.NewGormEventStore(db)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize event store: %w", err)
	}
	closer := func() error {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return store, closer, nil
}

// openDynamo opens a DynamoDB-backed store. The table and its event-id GSI must
// already exist — NewDynamoEventStore provisions nothing.
func openDynamo(ctx context.Context, spec StoreSpec) (domain.EventStore, func() error, error) {
	var loadOpts []func(*awsconfig.LoadOptions) error
	if spec.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(spec.Region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("load AWS config: %w", err)
	}
	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if spec.Endpoint != "" {
			o.BaseEndpoint = aws.String(spec.Endpoint)
		}
	})
	store := infrastructure.NewDynamoEventStore(client, spec.Table)
	return store, func() error { return nil }, nil
}
